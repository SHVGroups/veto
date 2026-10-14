package store

import (
	"context"
	"encoding/json"
	"time"
)

// PolicyVersion is one saved revision of a workspace's Cedar policies.
type PolicyVersion struct {
	Version     int        `json:"version"`
	Source      string     `json:"source"`
	Note        string     `json:"note"`
	CreatedBy   *string    `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at"`
	Active      bool       `json:"active"`
}

// CreatePolicyVersion saves a new revision and returns its version number.
func (q *Queries) CreatePolicyVersion(ctx context.Context, workspaceID, source, note string, createdBy *string) (int, error) {
	var v int
	err := q.db.QueryRow(ctx, `
		INSERT INTO policy_versions (workspace_id, version, source, note, created_by)
		SELECT $1, coalesce(max(version), 0) + 1, $2, $3, $4 FROM policy_versions WHERE workspace_id = $1
		RETURNING version`, workspaceID, source, note, createdBy).Scan(&v)
	if isUniqueViolation(err) {
		return 0, ErrConflict
	}
	return v, err
}

// ActivatePolicyVersion makes a revision the one the gate evaluates.
func (q *Queries) ActivatePolicyVersion(ctx context.Context, workspaceID string, version int) error {
	tag, err := q.db.Exec(ctx,
		`UPDATE policy_versions SET activated_at = now() WHERE workspace_id = $1 AND version = $2`, workspaceID, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = q.db.Exec(ctx, `UPDATE workspaces SET active_policy_version = $2 WHERE id = $1`, workspaceID, version)
	return err
}

const policyColumns = `p.version, p.source, p.note, p.created_by, p.created_at, p.activated_at,
	(w.active_policy_version IS NOT DISTINCT FROM p.version)`

func scanPolicy(row interface{ Scan(...any) error }, p *PolicyVersion) error {
	return row.Scan(&p.Version, &p.Source, &p.Note, &p.CreatedBy, &p.CreatedAt, &p.ActivatedAt, &p.Active)
}

// GetPolicyVersion loads one revision.
func (q *Queries) GetPolicyVersion(ctx context.Context, workspaceID string, version int) (PolicyVersion, error) {
	var p PolicyVersion
	err := scanPolicy(q.db.QueryRow(ctx,
		`SELECT `+policyColumns+` FROM policy_versions p JOIN workspaces w ON w.id = p.workspace_id
		 WHERE p.workspace_id = $1 AND p.version = $2`, workspaceID, version), &p)
	return p, notFound(err)
}

// GetActivePolicy loads the revision the gate evaluates.
func (q *Queries) GetActivePolicy(ctx context.Context, workspaceID string) (PolicyVersion, error) {
	var p PolicyVersion
	err := scanPolicy(q.db.QueryRow(ctx,
		`SELECT `+policyColumns+` FROM policy_versions p JOIN workspaces w ON w.id = p.workspace_id
		 WHERE p.workspace_id = $1 AND p.version = w.active_policy_version`, workspaceID), &p)
	return p, notFound(err)
}

// ListPolicyVersions returns revisions, newest first, without their source.
func (q *Queries) ListPolicyVersions(ctx context.Context, workspaceID string) ([]PolicyVersion, error) {
	rows, err := q.db.Query(ctx,
		`SELECT `+policyColumns+` FROM policy_versions p JOIN workspaces w ON w.id = p.workspace_id
		 WHERE p.workspace_id = $1 ORDER BY p.version DESC LIMIT 200`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PolicyVersion
	for rows.Next() {
		var p PolicyVersion
		if err := scanPolicy(rows, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PackVersion is a workspace's saved revision of a question pack.
type PackVersion struct {
	Name       string          `json:"name"`
	Version    int             `json:"version"`
	Definition json.RawMessage `json:"definition"`
	CreatedBy  *string         `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
	Active     bool            `json:"active"`
}

// CreatePackVersion saves a pack revision and makes it the active override.
func (q *Queries) CreatePackVersion(ctx context.Context, workspaceID, name string, definition []byte, createdBy *string) (int, error) {
	if _, err := q.db.Exec(ctx,
		`UPDATE question_packs SET active = false WHERE workspace_id = $1 AND name = $2 AND active`, workspaceID, name); err != nil {
		return 0, err
	}
	var v int
	err := q.db.QueryRow(ctx, `
		INSERT INTO question_packs (workspace_id, name, version, definition, created_by, active)
		SELECT $1, $2, coalesce(max(version), 0) + 1, $3, $4, true FROM question_packs WHERE workspace_id = $1 AND name = $2
		RETURNING version`, workspaceID, name, definition, createdBy).Scan(&v)
	return v, err
}

// DeactivatePack reverts a pack to the built-in definition.
func (q *Queries) DeactivatePack(ctx context.Context, workspaceID, name string) error {
	_, err := q.db.Exec(ctx,
		`UPDATE question_packs SET active = false WHERE workspace_id = $1 AND name = $2 AND active`, workspaceID, name)
	return err
}

// ActivePackOverrides returns the active override of every pack that has one.
func (q *Queries) ActivePackOverrides(ctx context.Context, workspaceID string) ([]PackVersion, error) {
	return q.listPacks(ctx, `WHERE workspace_id = $1 AND active`, workspaceID)
}

// ListPackVersions returns every saved revision of one pack.
func (q *Queries) ListPackVersions(ctx context.Context, workspaceID, name string) ([]PackVersion, error) {
	return q.listPacks(ctx, `WHERE workspace_id = $1 AND name = $2`, workspaceID, name)
}

func (q *Queries) listPacks(ctx context.Context, where string, args ...any) ([]PackVersion, error) {
	rows, err := q.db.Query(ctx,
		`SELECT name, version, definition, created_by, created_at, active FROM question_packs `+where+
			` ORDER BY name, version DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PackVersion
	for rows.Next() {
		var p PackVersion
		var def []byte
		if err := rows.Scan(&p.Name, &p.Version, &def, &p.CreatedBy, &p.CreatedAt, &p.Active); err != nil {
			return nil, err
		}
		p.Definition = def
		out = append(out, p)
	}
	return out, rows.Err()
}
