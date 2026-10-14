package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/ledger"
)

// Workspace is a tenant: one organization's agents, tools, policies and
// history.
type Workspace struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Settings            domain.Settings `json:"settings"`
	ActivePolicyVersion *int            `json:"active_policy_version"`
	CreatedAt           time.Time       `json:"created_at"`
}

// CreateWorkspace inserts a workspace and initializes its ledger head.
func (q *Queries) CreateWorkspace(ctx context.Context, id, name string, settings domain.Settings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	if _, err := q.db.Exec(ctx,
		`INSERT INTO workspaces (id, name, settings) VALUES ($1, $2, $3)`, id, name, raw); err != nil {
		return err
	}
	_, err = q.db.Exec(ctx,
		`INSERT INTO ledger_heads (workspace_id, seq, hash) VALUES ($1, 0, $2)`, id, ledger.Genesis(id))
	return err
}

// GetWorkspace loads a workspace. Settings missing from storage take their
// default values, so new settings roll out without a data migration.
func (q *Queries) GetWorkspace(ctx context.Context, id string) (Workspace, error) {
	var w Workspace
	var raw []byte
	err := q.db.QueryRow(ctx,
		`SELECT id, name, settings, active_policy_version, created_at FROM workspaces WHERE id = $1`, id,
	).Scan(&w.ID, &w.Name, &raw, &w.ActivePolicyVersion, &w.CreatedAt)
	if err != nil {
		return w, notFound(err)
	}
	w.Settings = domain.DefaultSettings()
	if err := json.Unmarshal(raw, &w.Settings); err != nil {
		return w, err
	}
	return w, nil
}

// UpdateWorkspace changes the name and settings.
func (q *Queries) UpdateWorkspace(ctx context.Context, id, name string, settings domain.Settings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	tag, err := q.db.Exec(ctx, `UPDATE workspaces SET name = $2, settings = $3 WHERE id = $1`, id, name, raw)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ListWorkspaceIDs returns every workspace id. Background jobs iterate over
// it.
func (q *Queries) ListWorkspaceIDs(ctx context.Context) ([]string, error) {
	rows, err := q.db.Query(ctx, `SELECT id FROM workspaces ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CountUsers returns the number of users in the installation. Zero means
// first-run setup has not happened.
func (q *Queries) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := q.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}
