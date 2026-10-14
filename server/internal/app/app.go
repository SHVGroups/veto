// Package app implements Veto's operations: receiving and deciding actions,
// reviews, audits, outcomes, grants, and the administration of agents, tools,
// policies, packs, connections and members. It is the only layer that
// combines the store, the decision engine, the vault and the event bus, and
// every state change it makes is written to the ledger in the same
// transaction.
package app

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shvgroups/veto/server/internal/decide"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/events"
	"github.com/shvgroups/veto/server/internal/grant"
	"github.com/shvgroups/veto/server/internal/id"
	"github.com/shvgroups/veto/server/internal/ledger"
	"github.com/shvgroups/veto/server/internal/notify"
	"github.com/shvgroups/veto/server/internal/policy"
	"github.com/shvgroups/veto/server/internal/store"
	"github.com/shvgroups/veto/server/internal/vault"
)

// Version is the release of this build. The release pipeline overrides it
// with -ldflags.
var Version = "1.0.0"

// Deps are the collaborators an App needs.
type Deps struct {
	Store   *store.Store
	Bus     *events.Bus
	Keyring *vault.Keyring
	// Model is nil when no decision model is configured. Actions that need a
	// model then follow their tool's fail mode.
	Model     decide.Model
	ModelName string
	Log       *slog.Logger
	Metrics   *Metrics
	Slack     *notify.Slack
	// Connector executes connection calls to public addresses.
	Connector ConnectorExecutor
	// PrivateConnector, when set, executes calls for connections marked
	// allow_private. It is nil unless the operator allows private
	// destinations with VETO_CONNECTOR_ALLOW_PRIVATE.
	PrivateConnector ConnectorExecutor

	PublicURL       string
	DecisionTimeout time.Duration
	SessionTTL      time.Duration
	MaxWait         time.Duration
}

// App is the application service.
type App struct {
	Deps
	packs map[string]*decide.Pack
	now   func() time.Time

	policyMu sync.Mutex
	policies map[string]*policy.Set

	packMu    sync.Mutex
	overrides map[string]packOverrides

	keyMu sync.Mutex
	keys  map[string]signingKey
}

type packOverrides struct {
	packs  map[string]*decide.Pack
	loaded time.Time
}

type signingKey struct {
	id      string
	private ed25519.PrivateKey
	loaded  time.Time
}

// packCacheTTL bounds how long another replica's pack change takes to apply.
const packCacheTTL = 15 * time.Second

// New builds an App. It fails only if the built-in packs are invalid, which
// would be a build defect.
func New(d Deps) (*App, error) {
	packs, err := decide.Builtin()
	if err != nil {
		return nil, fmt.Errorf("app: built-in packs: %w", err)
	}
	if d.DecisionTimeout <= 0 {
		d.DecisionTimeout = 2500 * time.Millisecond
	}
	if d.SessionTTL <= 0 {
		d.SessionTTL = 14 * 24 * time.Hour
	}
	if d.MaxWait <= 0 {
		d.MaxWait = 60 * time.Second
	}
	if d.Metrics == nil {
		d.Metrics = NewMetrics(nil)
	}
	return &App{
		Deps:      d,
		packs:     packs,
		now:       time.Now,
		policies:  map[string]*policy.Set{},
		overrides: map[string]packOverrides{},
		keys:      map[string]signingKey{},
	}, nil
}

// ModelConfigured reports whether a decision model is available.
func (a *App) ModelConfigured() bool { return a.Model != nil }

// Code classifies an application error for the API layer.
type Code string

const (
	CodeInvalid      Code = "invalid_request"
	CodeUnauthorized Code = "unauthorized"
	CodeForbidden    Code = "forbidden"
	CodeNotFound     Code = "not_found"
	CodeConflict     Code = "conflict"
	CodeRateLimited  Code = "rate_limited"
	CodeUnavailable  Code = "unavailable"
	CodeUpstream     Code = "upstream_error"
)

// Error is an expected failure the caller can act on.
type Error struct {
	Code    Code
	Message string
	Details any
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

func newErr(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func invalidf(format string, args ...any) *Error { return newErr(CodeInvalid, format, args...) }

func notFoundf(format string, args ...any) *Error { return newErr(CodeNotFound, format, args...) }

func conflictf(format string, args ...any) *Error { return newErr(CodeConflict, format, args...) }

func forbiddenf(format string, args ...any) *Error { return newErr(CodeForbidden, format, args...) }

// mapStoreErr turns a store sentinel into an application error with context.
func mapStoreErr(err error, what string) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return notFoundf("%s not found", what)
	case errors.Is(err, store.ErrConflict):
		return conflictf("%s already exists", what)
	}
	return err
}

// ActorKind is the type of principal performing an operation.
type ActorKind string

const (
	ActorUser   ActorKind = "user"
	ActorAgent  ActorKind = "agent"
	ActorKey    ActorKind = "key"
	ActorSystem ActorKind = "system"
)

// Actor is who is performing an operation.
type Actor struct {
	Kind        ActorKind
	ID          string
	Name        string
	WorkspaceID string
	Role        domain.Role
	// AgentID is set for agent API keys.
	AgentID       string
	AgentDisabled bool
	// SessionHash is set for console sessions.
	SessionHash []byte
}

// String identifies the actor in the ledger, e.g. "user:usr_01J...".
func (a Actor) String() string {
	if a.Kind == ActorSystem {
		return "system"
	}
	return string(a.Kind) + ":" + a.ID
}

// Can reports whether the actor holds a permission. Agent keys hold none of
// the console permissions; their operations check AgentID instead.
func (a Actor) Can(p domain.Permission) bool {
	switch a.Kind {
	case ActorSystem:
		return true
	case ActorUser, ActorKey:
		return a.Role.Can(p)
	}
	return false
}

// System is the actor for background jobs.
func System(workspaceID string) Actor {
	return Actor{Kind: ActorSystem, WorkspaceID: workspaceID, Name: "Veto"}
}

func require(actor Actor, p domain.Permission) error {
	if !actor.Can(p) {
		return forbiddenf("your role does not allow this")
	}
	return nil
}

// record appends a ledger entry and publishes an event inside a
// transaction.
func record(ctx context.Context, q *store.Queries, ws, kind, actionID string, actor Actor, payload any) error {
	_, err := q.AppendLedger(ctx, ws, kind, actionID, actor.String(), payload)
	return err
}

func publish(ctx context.Context, q *store.Queries, e events.Event) error {
	return q.Notify(ctx, events.Channel, events.Payload(e))
}

// enqueueWebhook queues a webhook event for every subscribed endpoint.
func enqueueWebhook(ctx context.Context, q *store.Queries, ws, event string, data any) error {
	body, err := json.Marshal(map[string]any{
		"id":           id.New(id.Delivery),
		"event":        event,
		"workspace_id": ws,
		"created_at":   time.Now().UTC(),
		"data":         data,
	})
	if err != nil {
		return err
	}
	_, err = q.EnqueueDeliveries(ctx, ws, event, body, func() string { return id.New(id.Delivery) })
	return err
}

// reviewURL links to an action in the console, or returns "" when no public
// URL is configured.
func (a *App) reviewURL(actionID string) string {
	if a.PublicURL == "" {
		return ""
	}
	return a.PublicURL + "/actions/" + actionID
}

// policySet returns the compiled active policies of a workspace, compiling
// and caching them the first time a version is seen.
func (a *App) policySet(ctx context.Context, ws store.Workspace) (*policy.Set, error) {
	if ws.ActivePolicyVersion == nil {
		return nil, nil
	}
	a.policyMu.Lock()
	cached := a.policies[ws.ID]
	a.policyMu.Unlock()
	if cached != nil && cached.Version == *ws.ActivePolicyVersion {
		return cached, nil
	}
	pv, err := a.Store.GetPolicyVersion(ctx, ws.ID, *ws.ActivePolicyVersion)
	if err != nil {
		return nil, fmt.Errorf("app: load policy v%d: %w", *ws.ActivePolicyVersion, err)
	}
	set, err := policy.Compile(pv.Source, pv.Version)
	if err != nil {
		// An active version always compiled when it was activated; failing now
		// means the Cedar library changed under it. Refuse rather than guess.
		return nil, fmt.Errorf("app: active policy v%d no longer compiles: %w", pv.Version, err)
	}
	a.policyMu.Lock()
	a.policies[ws.ID] = set
	a.policyMu.Unlock()
	return set, nil
}

// pack returns the question pack for a tool: the workspace's active override
// if it has one, otherwise the built-in pack, otherwise "general".
func (a *App) pack(ctx context.Context, workspaceID, name string) (*decide.Pack, error) {
	a.packMu.Lock()
	ov, ok := a.overrides[workspaceID]
	a.packMu.Unlock()
	if !ok || a.now().Sub(ov.loaded) > packCacheTTL {
		rows, err := a.Store.ActivePackOverrides(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		ov = packOverrides{packs: map[string]*decide.Pack{}, loaded: a.now()}
		for _, r := range rows {
			p, err := decide.ParsePack(r.Definition)
			if err != nil {
				a.Log.Error("ignoring invalid stored pack", "workspace", workspaceID, "pack", r.Name, "version", r.Version, "error", err)
				continue
			}
			ov.packs[r.Name] = p
		}
		a.packMu.Lock()
		a.overrides[workspaceID] = ov
		a.packMu.Unlock()
	}
	if p, ok := ov.packs[name]; ok {
		return p, nil
	}
	if p, ok := a.packs[name]; ok {
		return p, nil
	}
	return a.packs["general"], nil
}

func (a *App) invalidatePacks(workspaceID string) {
	a.packMu.Lock()
	delete(a.overrides, workspaceID)
	a.packMu.Unlock()
}

// signer returns the workspace's active signing key, creating one if the
// workspace has none.
func (a *App) signer(ctx context.Context, workspaceID string) (signingKey, error) {
	a.keyMu.Lock()
	k, ok := a.keys[workspaceID]
	a.keyMu.Unlock()
	if ok && a.now().Sub(k.loaded) < 5*time.Minute {
		return k, nil
	}
	sk, err := a.Store.ActiveSigningKey(ctx, workspaceID)
	if errors.Is(err, store.ErrNotFound) {
		if err := a.createSigningKey(ctx, a.Store.Queries, workspaceID); err != nil && !errors.Is(err, store.ErrConflict) {
			return signingKey{}, err
		}
		sk, err = a.Store.ActiveSigningKey(ctx, workspaceID)
	}
	if err != nil {
		return signingKey{}, err
	}
	priv, err := a.Keyring.Open(sk.PrivateKeyEnc, "signing_key:"+sk.ID)
	if err != nil {
		return signingKey{}, fmt.Errorf("app: open signing key: %w", err)
	}
	k = signingKey{id: sk.KeyID, private: ed25519.PrivateKey(priv), loaded: a.now()}
	a.keyMu.Lock()
	a.keys[workspaceID] = k
	a.keyMu.Unlock()
	return k, nil
}

func (a *App) createSigningKey(ctx context.Context, q *store.Queries, workspaceID string) error {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	rowID := id.New(id.SigningKey)
	return q.InsertSigningKey(ctx, store.SigningKey{
		ID:            rowID,
		WorkspaceID:   workspaceID,
		KeyID:         grant.KeyID(pub),
		PublicKey:     pub,
		PrivateKeyEnc: a.Keyring.Seal(priv, "signing_key:"+rowID),
	})
}

// RotateSigningKey retires the active key and creates a new one. Grants and
// checkpoints signed by the old key keep verifying.
func (a *App) RotateSigningKey(ctx context.Context, actor Actor) error {
	if err := require(actor, domain.PermOwn); err != nil {
		return err
	}
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.RetireSigningKeys(ctx, actor.WorkspaceID); err != nil {
			return err
		}
		if err := a.createSigningKey(ctx, q, actor.WorkspaceID); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindSigningKeyRotated, "", actor, map[string]any{})
	})
	if err != nil {
		return err
	}
	a.keyMu.Lock()
	delete(a.keys, actor.WorkspaceID)
	a.keyMu.Unlock()
	return nil
}
