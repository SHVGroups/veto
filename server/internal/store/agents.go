package store

import (
	"context"
	"time"
)

// Agent is one AI agent registered with a workspace. Every action is
// attributed to an agent through the API key it used.
type Agent struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"-"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Vendor      string     `json:"vendor"`
	CreatedAt   time.Time  `json:"created_at"`
	DisabledAt  *time.Time `json:"disabled_at"`
}

// AgentStats summarizes an agent's recent activity.
type AgentStats struct {
	Agent
	Actions7d  int        `json:"actions_7d"`
	Held7d     int        `json:"held_7d"`
	Denied7d   int        `json:"denied_7d"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	ActiveKeys int        `json:"active_keys"`
	PendingNow int        `json:"pending_now"`
	AutoRate7d *float64   `json:"auto_rate_7d"`
	TopTools7d []string   `json:"top_tools_7d"`
}

// APIKey is a credential used by an agent or by automation.
type APIKey struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"-"`
	AgentID     *string    `json:"agent_id"`
	AgentName   *string    `json:"agent_name"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	Prefix      string     `json:"prefix"`
	CreatedBy   *string    `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
}

const agentColumns = `a.id, a.workspace_id, a.name, a.description, a.vendor, a.created_at, a.disabled_at`

func scanAgent(row interface{ Scan(...any) error }, a *Agent, extra ...any) error {
	return row.Scan(append([]any{&a.ID, &a.WorkspaceID, &a.Name, &a.Description, &a.Vendor, &a.CreatedAt, &a.DisabledAt}, extra...)...)
}

// CreateAgent inserts an agent. It returns ErrConflict if the name is taken.
func (q *Queries) CreateAgent(ctx context.Context, a Agent) error {
	_, err := q.db.Exec(ctx,
		`INSERT INTO agents (id, workspace_id, name, description, vendor) VALUES ($1, $2, $3, $4, $5)`,
		a.ID, a.WorkspaceID, a.Name, a.Description, a.Vendor)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// GetAgent loads an agent.
func (q *Queries) GetAgent(ctx context.Context, workspaceID, id string) (Agent, error) {
	var a Agent
	err := scanAgent(q.db.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents a WHERE a.workspace_id = $1 AND a.id = $2`, workspaceID, id), &a)
	return a, notFound(err)
}

// UpdateAgent changes an agent's details and enabled state.
func (q *Queries) UpdateAgent(ctx context.Context, a Agent) error {
	tag, err := q.db.Exec(ctx,
		`UPDATE agents SET name = $3, description = $4, vendor = $5, disabled_at = $6
		 WHERE workspace_id = $1 AND id = $2`,
		a.WorkspaceID, a.ID, a.Name, a.Description, a.Vendor, a.DisabledAt)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ListAgents returns agents with their activity over the last seven days.
func (q *Queries) ListAgents(ctx context.Context, workspaceID string) ([]AgentStats, error) {
	rows, err := q.db.Query(ctx, `
		SELECT `+agentColumns+`,
		       coalesce(s.total, 0), coalesce(s.held, 0), coalesce(s.denied, 0), s.last_seen,
		       (SELECT count(*) FROM api_keys k WHERE k.agent_id = a.id AND k.revoked_at IS NULL),
		       (SELECT count(*) FROM actions p WHERE p.agent_id = a.id AND p.status = 'pending_review'),
		       s.auto_rate,
		       coalesce(t.tools, '{}')
		FROM agents a
		LEFT JOIN LATERAL (
		    SELECT count(*) AS total,
		           count(*) FILTER (WHERE x.hold_reasons <> '[]'::jsonb) AS held,
		           count(*) FILTER (WHERE x.status IN ('denied', 'expired')) AS denied,
		           max(x.received_at) AS last_seen,
		           avg(CASE WHEN d.decided_by IN ('model', 'policy') THEN 1.0 ELSE 0.0 END)
		               FILTER (WHERE d.action_id IS NOT NULL) AS auto_rate
		    FROM actions x LEFT JOIN decisions d ON d.action_id = x.id
		    WHERE x.agent_id = a.id AND x.received_at > now() - interval '7 days'
		) s ON true
		LEFT JOIN LATERAL (
		    SELECT array_agg(tool ORDER BY n DESC) AS tools FROM (
		        SELECT tool, count(*) AS n FROM actions
		        WHERE agent_id = a.id AND received_at > now() - interval '7 days'
		        GROUP BY tool ORDER BY n DESC LIMIT 3
		    ) top
		) t ON true
		WHERE a.workspace_id = $1
		ORDER BY a.disabled_at IS NOT NULL, lower(a.name)`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentStats
	for rows.Next() {
		var s AgentStats
		if err := scanAgent(rows, &s.Agent, &s.Actions7d, &s.Held7d, &s.Denied7d, &s.LastSeenAt,
			&s.ActiveKeys, &s.PendingNow, &s.AutoRate7d, &s.TopTools7d); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CreateAPIKey stores a new key by the hash of its secret.
func (q *Queries) CreateAPIKey(ctx context.Context, k APIKey, secretHash []byte) error {
	_, err := q.db.Exec(ctx,
		`INSERT INTO api_keys (id, workspace_id, agent_id, name, kind, prefix, secret_hash, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		k.ID, k.WorkspaceID, k.AgentID, k.Name, k.Kind, k.Prefix, secretHash, k.CreatedBy)
	return err
}

const apiKeyColumns = `k.id, k.workspace_id, k.agent_id, a.name, k.name, k.kind, k.prefix, k.created_by, k.created_at, k.last_used_at, k.revoked_at`

func scanAPIKey(row interface{ Scan(...any) error }, k *APIKey) error {
	return row.Scan(&k.ID, &k.WorkspaceID, &k.AgentID, &k.AgentName, &k.Name, &k.Kind, &k.Prefix,
		&k.CreatedBy, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt)
}

// AuthenticatedKey is a live API key resolved from its secret.
type AuthenticatedKey struct {
	APIKey
	AgentDisabled bool
}

// GetAPIKeyBySecret resolves an unrevoked key from the hash of its secret.
func (q *Queries) GetAPIKeyBySecret(ctx context.Context, secretHash []byte) (AuthenticatedKey, error) {
	var k AuthenticatedKey
	err := q.db.QueryRow(ctx,
		`SELECT `+apiKeyColumns+`, coalesce(a.disabled_at IS NOT NULL, false)
		 FROM api_keys k LEFT JOIN agents a ON a.id = k.agent_id
		 WHERE k.secret_hash = $1 AND k.revoked_at IS NULL`, secretHash,
	).Scan(&k.ID, &k.WorkspaceID, &k.AgentID, &k.AgentName, &k.Name, &k.Kind, &k.Prefix,
		&k.CreatedBy, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt, &k.AgentDisabled)
	return k, notFound(err)
}

// TouchAPIKey records use of a key at most once a minute, to keep writes
// off the hot path.
func (q *Queries) TouchAPIKey(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx,
		`UPDATE api_keys SET last_used_at = now()
		 WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, id)
	return err
}

// ListAPIKeys returns a workspace's keys, optionally for one agent.
func (q *Queries) ListAPIKeys(ctx context.Context, workspaceID, agentID string) ([]APIKey, error) {
	rows, err := q.db.Query(ctx,
		`SELECT `+apiKeyColumns+` FROM api_keys k LEFT JOIN agents a ON a.id = k.agent_id
		 WHERE k.workspace_id = $1 AND ($2 = '' OR k.agent_id = $2)
		 ORDER BY k.revoked_at IS NOT NULL, k.created_at DESC`, workspaceID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := scanAPIKey(rows, &k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// GetAPIKey loads one key.
func (q *Queries) GetAPIKey(ctx context.Context, workspaceID, id string) (APIKey, error) {
	var k APIKey
	err := scanAPIKey(q.db.QueryRow(ctx,
		`SELECT `+apiKeyColumns+` FROM api_keys k LEFT JOIN agents a ON a.id = k.agent_id
		 WHERE k.workspace_id = $1 AND k.id = $2`, workspaceID, id), &k)
	return k, notFound(err)
}

// RevokeAPIKey stops a key from working.
func (q *Queries) RevokeAPIKey(ctx context.Context, workspaceID, id string) error {
	tag, err := q.db.Exec(ctx,
		`UPDATE api_keys SET revoked_at = now() WHERE workspace_id = $1 AND id = $2 AND revoked_at IS NULL`, workspaceID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
