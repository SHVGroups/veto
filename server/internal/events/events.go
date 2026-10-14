// Package events fans out workspace events to every vetod replica using
// PostgreSQL LISTEN/NOTIFY. Events are published inside the transaction that
// causes them, so subscribers hear about a change only once it is committed.
//
// Subscribers are the console's live stream and API requests waiting for a
// held action to be decided. Events carry identifiers only; subscribers read
// current state from the database.
package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Channel is the Postgres notification channel.
const Channel = "veto_events"

// Event kinds.
const (
	ActionReceived  = "action.received"
	ActionHeld      = "action.held"
	ActionDecided   = "action.decided"
	OutcomeReported = "outcome.reported"
	AuditSampled    = "audit.sampled"
	AuditRecorded   = "audit.recorded"
	ToolRegistered  = "tool.registered"
	ConfigChanged   = "config.changed"
)

// Event is a notification that something changed in a workspace.
type Event struct {
	WorkspaceID string    `json:"ws"`
	Kind        string    `json:"kind"`
	ActionID    string    `json:"action_id,omitempty"`
	Status      string    `json:"status,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	At          time.Time `json:"at"`
}

// Payload encodes an event for pg_notify.
func Payload(e Event) string {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	b, _ := json.Marshal(e)
	return string(b)
}

// Bus delivers notifications to in-process subscribers.
type Bus struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	mu     sync.RWMutex
	subs   map[string]map[*subscription]struct{}
	nextID int
}

type subscription struct {
	ch     chan Event
	filter func(Event) bool
}

// NewBus returns a bus that listens on the given pool.
func NewBus(pool *pgxpool.Pool, log *slog.Logger) *Bus {
	return &Bus{pool: pool, log: log, subs: map[string]map[*subscription]struct{}{}}
}

// Subscribe returns a channel of events for a workspace that pass the
// filter, and a function that ends the subscription. A nil filter passes
// everything. Slow subscribers drop events rather than block the bus; the
// console treats events as hints to refetch, so a drop costs freshness, not
// correctness.
func (b *Bus) Subscribe(workspaceID string, filter func(Event) bool) (<-chan Event, func()) {
	s := &subscription{ch: make(chan Event, 64), filter: filter}
	b.mu.Lock()
	if b.subs[workspaceID] == nil {
		b.subs[workspaceID] = map[*subscription]struct{}{}
	}
	b.subs[workspaceID][s] = struct{}{}
	b.mu.Unlock()
	var once sync.Once
	return s.ch, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs[workspaceID], s)
			if len(b.subs[workspaceID]) == 0 {
				delete(b.subs, workspaceID)
			}
			b.mu.Unlock()
		})
	}
}

// Dispatch delivers an event to local subscribers. Run calls it for every
// notification; tests call it directly.
func (b *Bus) Dispatch(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs[e.WorkspaceID] {
		if s.filter != nil && !s.filter(e) {
			continue
		}
		select {
		case s.ch <- e:
		default:
		}
	}
}

// Run listens for notifications until the context ends, reconnecting with
// backoff if the connection drops.
func (b *Bus) Run(ctx context.Context) {
	backoff := 250 * time.Millisecond
	for ctx.Err() == nil {
		err := b.listen(ctx)
		if ctx.Err() != nil {
			return
		}
		b.log.Warn("event listener disconnected; reconnecting", "error", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

func (b *Bus) listen(ctx context.Context) error {
	conn, err := b.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	b.log.Debug("listening for events", "channel", Channel)
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			// The connection's state is unknown after an error; destroy it
			// rather than return it to the pool still listening.
			conn.Conn().Close(context.Background()) //nolint:errcheck
			return err
		}
		var e Event
		if err := json.Unmarshal([]byte(n.Payload), &e); err != nil {
			b.log.Warn("ignoring malformed event", "payload", n.Payload)
			continue
		}
		b.Dispatch(e)
	}
}
