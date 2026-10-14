package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
)

// Action is one proposed tool call and everything Veto decided about it.
type Action struct {
	ID             string              `json:"id"`
	WorkspaceID    string              `json:"-"`
	AgentID        string              `json:"agent_id"`
	AgentName      string              `json:"agent_name"`
	Tool           string              `json:"tool"`
	Args           json.RawMessage     `json:"args"`
	ArgsHash       []byte              `json:"-"`
	Principal      domain.Principal    `json:"principal"`
	Request        string              `json:"request"`
	Untrusted      []domain.Untrusted  `json:"untrusted"`
	Context        json.RawMessage     `json:"context"`
	Source         domain.Source       `json:"source"`
	IdempotencyKey *string             `json:"idempotency_key"`
	Status         domain.ActionStatus `json:"status"`
	Options        []domain.OptionSpec `json:"options"`
	Ranking        []domain.Option     `json:"ranking"`
	HoldReasons    []domain.Reason     `json:"hold_reasons"`
	PolicyVersion  *int                `json:"policy_version"`
	ReceivedAt     time.Time           `json:"received_at"`
	DecidedAt      *time.Time          `json:"decided_at"`
	ReviewDeadline *time.Time          `json:"review_deadline"`
	RedactedAt     *time.Time          `json:"redacted_at"`
}

// Option returns the stored spec for an option.
func (a *Action) Option(o domain.Option) (domain.OptionSpec, bool) {
	for _, s := range a.Options {
		if s.Key == o {
			return s, s.Available
		}
	}
	return domain.OptionSpec{}, false
}

const actionColumns = `a.id, a.workspace_id, a.agent_id, ag.name, a.tool, a.args, a.args_hash,
	a.principal_id, a.principal_display, a.request, a.untrusted, a.context, a.source, a.idempotency_key,
	a.status, a.options, a.ranking, a.hold_reasons, a.policy_version, a.received_at, a.decided_at,
	a.review_deadline, a.redacted_at`

func scanAction(row interface{ Scan(...any) error }, a *Action) error {
	var args, untrusted, ctxRaw, options, ranking, reasons []byte
	err := row.Scan(&a.ID, &a.WorkspaceID, &a.AgentID, &a.AgentName, &a.Tool, &args, &a.ArgsHash,
		&a.Principal.ID, &a.Principal.Display, &a.Request, &untrusted, &ctxRaw, &a.Source, &a.IdempotencyKey,
		&a.Status, &options, &ranking, &reasons, &a.PolicyVersion, &a.ReceivedAt, &a.DecidedAt,
		&a.ReviewDeadline, &a.RedactedAt)
	if err != nil {
		return err
	}
	a.Args = args
	a.Context = ctxRaw
	for _, f := range []struct {
		raw []byte
		dst any
	}{{untrusted, &a.Untrusted}, {options, &a.Options}, {ranking, &a.Ranking}, {reasons, &a.HoldReasons}} {
		if err := json.Unmarshal(f.raw, f.dst); err != nil {
			return fmt.Errorf("store: decode action %s: %w", a.ID, err)
		}
	}
	return nil
}

// InsertAction stores a newly received action. It returns ErrConflict when
// the agent already sent an action with the same idempotency key.
func (q *Queries) InsertAction(ctx context.Context, a *Action) error {
	untrusted, err := json.Marshal(nonNil(a.Untrusted))
	if err != nil {
		return err
	}
	ctxRaw := a.Context
	if len(ctxRaw) == 0 {
		ctxRaw = json.RawMessage("{}")
	}
	err = q.db.QueryRow(ctx, `
		INSERT INTO actions (id, workspace_id, agent_id, tool, args, args_hash, principal_id, principal_display,
		                     request, untrusted, context, source, idempotency_key, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING received_at`,
		a.ID, a.WorkspaceID, a.AgentID, a.Tool, []byte(a.Args), a.ArgsHash, a.Principal.ID, a.Principal.Display,
		a.Request, untrusted, []byte(ctxRaw), a.Source, a.IdempotencyKey, a.Status,
	).Scan(&a.ReceivedAt)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// GetAction loads an action.
func (q *Queries) GetAction(ctx context.Context, workspaceID, id string) (Action, error) {
	var a Action
	err := scanAction(q.db.QueryRow(ctx,
		`SELECT `+actionColumns+` FROM actions a JOIN agents ag ON ag.id = a.agent_id
		 WHERE a.workspace_id = $1 AND a.id = $2`, workspaceID, id), &a)
	return a, notFound(err)
}

// GetActionForUpdate loads an action and locks its row until the
// transaction ends, so two reviewers cannot decide it at once.
func (q *Queries) GetActionForUpdate(ctx context.Context, workspaceID, id string) (Action, error) {
	var a Action
	err := scanAction(q.db.QueryRow(ctx,
		`SELECT `+actionColumns+` FROM actions a JOIN agents ag ON ag.id = a.agent_id
		 WHERE a.workspace_id = $1 AND a.id = $2 FOR UPDATE OF a`, workspaceID, id), &a)
	return a, notFound(err)
}

// GetActionByIdempotencyKey finds an earlier action with the same key.
func (q *Queries) GetActionByIdempotencyKey(ctx context.Context, workspaceID, agentID, key string) (Action, error) {
	var a Action
	err := scanAction(q.db.QueryRow(ctx,
		`SELECT `+actionColumns+` FROM actions a JOIN agents ag ON ag.id = a.agent_id
		 WHERE a.workspace_id = $1 AND a.agent_id = $2 AND a.idempotency_key = $3`, workspaceID, agentID, key), &a)
	return a, notFound(err)
}

// EvaluatedAction is what the pipeline records once it has judged an action.
type EvaluatedAction struct {
	Status         domain.ActionStatus
	Options        []domain.OptionSpec
	Ranking        []domain.Option
	HoldReasons    []domain.Reason
	PolicyVersion  *int
	ReviewDeadline *time.Time
}

// RecordEvaluation stores the options, ranking and routing of an action.
func (q *Queries) RecordEvaluation(ctx context.Context, workspaceID, id string, e EvaluatedAction) error {
	options, _ := json.Marshal(nonNil(e.Options))
	ranking, _ := json.Marshal(nonNil(e.Ranking))
	reasons, _ := json.Marshal(nonNil(e.HoldReasons))
	_, err := q.db.Exec(ctx, `
		UPDATE actions SET status = $3, options = $4, ranking = $5, hold_reasons = $6,
		                   policy_version = $7, review_deadline = $8
		WHERE workspace_id = $1 AND id = $2`,
		workspaceID, id, e.Status, options, ranking, reasons, e.PolicyVersion, e.ReviewDeadline)
	return err
}

// FinalizeAction moves an undecided action to a final status. It returns
// ErrConflict if the action was already decided.
func (q *Queries) FinalizeAction(ctx context.Context, workspaceID, id string, status domain.ActionStatus) (time.Time, error) {
	var decidedAt time.Time
	err := q.db.QueryRow(ctx, `
		UPDATE actions SET status = $3, decided_at = now()
		WHERE workspace_id = $1 AND id = $2 AND status IN ('evaluating', 'pending_review')
		RETURNING decided_at`, workspaceID, id, status).Scan(&decidedAt)
	if err != nil {
		if errors.Is(notFound(err), ErrNotFound) {
			return decidedAt, ErrConflict
		}
		return decidedAt, err
	}
	return decidedAt, nil
}

// CountRecentCalls counts an agent's calls to a tool in the last hour, for
// per-tool rate limits.
func (q *Queries) CountRecentCalls(ctx context.Context, workspaceID, agentID, tool string, window time.Duration) (int, error) {
	var n int
	err := q.db.QueryRow(ctx, `
		SELECT count(*) FROM actions
		WHERE workspace_id = $1 AND agent_id = $2 AND tool = $3 AND received_at > now() - make_interval(secs => $4)`,
		workspaceID, agentID, tool, window.Seconds()).Scan(&n)
	return n, err
}

// ActionSummary is one row of the actions list.
type ActionSummary struct {
	ID               string              `json:"id"`
	AgentID          string              `json:"agent_id"`
	AgentName        string              `json:"agent_name"`
	Tool             string              `json:"tool"`
	PrincipalDisplay string              `json:"principal_display"`
	Request          string              `json:"request"`
	Status           domain.ActionStatus `json:"status"`
	Source           domain.Source       `json:"source"`
	HoldReasons      []domain.Reason     `json:"hold_reasons"`
	Ranking          []domain.Option     `json:"ranking"`
	ReceivedAt       time.Time           `json:"received_at"`
	DecidedAt        *time.Time          `json:"decided_at"`
	ReviewDeadline   *time.Time          `json:"review_deadline"`
	Option           *string             `json:"option"`
	DecidedBy        *string             `json:"decided_by"`
	Confidence       *float64            `json:"confidence"`
	Risk             *float64            `json:"risk"`
	ReviewerName     *string             `json:"reviewer_name"`
	Outcome          *string             `json:"outcome"`
	DecisionMs       *float64            `json:"decision_ms"`
}

// ActionFilter narrows the actions list. Empty fields do not filter.
type ActionFilter struct {
	Statuses  []string
	AgentID   string
	Tool      string
	DecidedBy string
	Option    string
	Source    string
	Query     string
	HeldOnly  bool
	Since     *time.Time
	Until     *time.Time
	Cursor    string
	Limit     int
}

// ListActions returns actions newest first with keyset pagination. The
// returned cursor is empty on the last page.
func (q *Queries) ListActions(ctx context.Context, workspaceID string, f ActionFilter) ([]ActionSummary, string, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	w := whereBuilder{}
	w.add("a.workspace_id = %s", workspaceID)
	if len(f.Statuses) > 0 {
		w.add("a.status = ANY(%s)", f.Statuses)
	}
	if f.AgentID != "" {
		w.add("a.agent_id = %s", f.AgentID)
	}
	if f.Tool != "" {
		w.add("a.tool = %s", f.Tool)
	}
	if f.DecidedBy != "" {
		w.add("d.decided_by = %s", f.DecidedBy)
	}
	if f.Option != "" {
		w.add("d.option = %s", f.Option)
	}
	if f.Source != "" {
		w.add("a.source = %s", f.Source)
	}
	if f.HeldOnly {
		w.addRaw("a.hold_reasons <> '[]'::jsonb")
	}
	if f.Since != nil {
		w.add("a.received_at >= %s", *f.Since)
	}
	if f.Until != nil {
		w.add("a.received_at < %s", *f.Until)
	}
	if s := strings.TrimSpace(f.Query); s != "" {
		pattern := "%" + escapeLike(s) + "%"
		w.add("(a.id = %[1]s OR a.tool ILIKE %[2]s OR a.request ILIKE %[2]s OR a.principal_display ILIKE %[2]s OR ag.name ILIKE %[2]s)", s, pattern)
	}
	if f.Cursor != "" {
		t, id, err := decodeCursor(f.Cursor)
		if err != nil {
			return nil, "", err
		}
		w.add("(a.received_at, a.id) < (%s, %s)", t, id)
	}
	sql := `
		SELECT a.id, a.agent_id, ag.name, a.tool, a.principal_display, left(a.request, 240), a.status, a.source,
		       a.hold_reasons, a.ranking, a.received_at, a.decided_at, a.review_deadline,
		       d.option, d.decided_by, d.confidence, d.risk, u.name,
		       (SELECT o.status FROM outcomes o WHERE o.action_id = a.id ORDER BY o.created_at DESC LIMIT 1),
		       CASE WHEN a.decided_at IS NOT NULL AND d.decided_by <> 'human'
		            THEN extract(epoch FROM a.decided_at - a.received_at) * 1000 END
		FROM actions a
		JOIN agents ag ON ag.id = a.agent_id
		LEFT JOIN decisions d ON d.action_id = a.id
		LEFT JOIN users u ON u.id = d.reviewer_id
		WHERE ` + w.sql() + fmt.Sprintf(`
		ORDER BY a.received_at DESC, a.id DESC
		LIMIT %d`, f.Limit+1)
	rows, err := q.db.Query(ctx, sql, w.args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []ActionSummary
	for rows.Next() {
		var s ActionSummary
		var reasons, ranking []byte
		var conf, risk *float32
		if err := rows.Scan(&s.ID, &s.AgentID, &s.AgentName, &s.Tool, &s.PrincipalDisplay, &s.Request, &s.Status, &s.Source,
			&reasons, &ranking, &s.ReceivedAt, &s.DecidedAt, &s.ReviewDeadline,
			&s.Option, &s.DecidedBy, &conf, &risk, &s.ReviewerName, &s.Outcome, &s.DecisionMs); err != nil {
			return nil, "", err
		}
		s.Confidence, s.Risk = widen(conf), widen(risk)
		if err := json.Unmarshal(reasons, &s.HoldReasons); err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(ranking, &s.Ranking); err != nil {
			return nil, "", err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > f.Limit {
		out = out[:f.Limit]
		last := out[len(out)-1]
		next = encodeCursor(last.ReceivedAt, last.ID)
	}
	return out, next, nil
}

// PendingRef identifies a held action for background jobs.
type PendingRef struct {
	WorkspaceID string
	ActionID    string
	Deadline    time.Time
	ReceivedAt  time.Time
}

// ExpiredPending returns held actions whose review deadline has passed.
func (q *Queries) ExpiredPending(ctx context.Context, limit int) ([]PendingRef, error) {
	return q.pendingRefs(ctx, `review_deadline <= now()`, limit)
}

// PendingNeedingReminder returns held actions halfway to their deadline that
// have not had a reminder.
func (q *Queries) PendingNeedingReminder(ctx context.Context, limit int) ([]PendingRef, error) {
	return q.pendingRefs(ctx,
		`reminded_at IS NULL AND review_deadline > now() AND now() >= received_at + (review_deadline - received_at) / 2`, limit)
}

func (q *Queries) pendingRefs(ctx context.Context, cond string, limit int) ([]PendingRef, error) {
	rows, err := q.db.Query(ctx, `
		SELECT workspace_id, id, review_deadline, received_at FROM actions
		WHERE status = 'pending_review' AND `+cond+` ORDER BY review_deadline LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingRef
	for rows.Next() {
		var r PendingRef
		if err := rows.Scan(&r.WorkspaceID, &r.ActionID, &r.Deadline, &r.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkReminded records that a reminder went out for a held action.
func (q *Queries) MarkReminded(ctx context.Context, workspaceID, id string) error {
	_, err := q.db.Exec(ctx, `UPDATE actions SET reminded_at = now() WHERE workspace_id = $1 AND id = $2`, workspaceID, id)
	return err
}

// RedactBefore removes arguments, untrusted content, requests and raw model
// exchanges from decided actions received before the cutoff. Decisions,
// argument hashes and the ledger are kept, so the record of what happened
// survives without the data. It returns the number of actions redacted.
func (q *Queries) RedactBefore(ctx context.Context, workspaceID string, cutoff time.Time, batch int) (int64, error) {
	tag, err := q.db.Exec(ctx, `
		WITH target AS (
		    SELECT id FROM actions
		    WHERE workspace_id = $1 AND received_at < $2 AND redacted_at IS NULL AND status <> 'pending_review'
		    ORDER BY received_at LIMIT $3
		), ev AS (
		    UPDATE evaluations SET request = NULL, response = NULL WHERE action_id IN (SELECT id FROM target)
		), dec AS (
		    UPDATE decisions SET final_args = NULL WHERE action_id IN (SELECT id FROM target)
		)
		UPDATE actions SET args = '{"redacted": true}', untrusted = '[]', request = '', context = '{}',
		       options = coalesce((SELECT jsonb_agg(o - 'args') FROM jsonb_array_elements(options) o), '[]'),
		       redacted_at = now()
		WHERE id IN (SELECT id FROM target)`, workspaceID, cutoff, batch)
	return tag.RowsAffected(), err
}

// whereBuilder assembles a parameterized WHERE clause. Each fragment uses %s
// (or indexed %[n]s) where its arguments go.
type whereBuilder struct {
	parts []string
	args  []any
}

func (w *whereBuilder) add(fragment string, args ...any) {
	placeholders := make([]any, len(args))
	for i, a := range args {
		w.args = append(w.args, a)
		placeholders[i] = fmt.Sprintf("$%d", len(w.args))
	}
	w.parts = append(w.parts, fmt.Sprintf(fragment, placeholders...))
}

func (w *whereBuilder) addRaw(fragment string) { w.parts = append(w.parts, fragment) }

func (w *whereBuilder) sql() string {
	if len(w.parts) == 0 {
		return "true"
	}
	return strings.Join(w.parts, " AND ")
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func encodeCursor(t time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeCursor(c string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("store: invalid cursor")
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, "", fmt.Errorf("store: invalid cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("store: invalid cursor")
	}
	return t, id, nil
}

// nonNil turns a nil slice into an empty one so it encodes as [] not null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
