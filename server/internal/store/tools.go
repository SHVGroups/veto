package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
)

// ToolStats is a tool profile with its recent activity.
type ToolStats struct {
	domain.Tool
	Calls7d       int        `json:"calls_7d"`
	Held7d        int        `json:"held_7d"`
	Denied7d      int        `json:"denied_7d"`
	SaferTaken7d  int        `json:"safer_taken_7d"`
	LastCalledAt  *time.Time `json:"last_called_at"`
	AvgConfidence *float64   `json:"avg_confidence_7d"`
}

const toolColumns = `t.workspace_id, t.name, t.description, t.risk_class, t.reversible, t.pack, t.safer,
	t.auto_confidence, t.risk_ceiling, t.block_confidence, t.double_check, t.fail_mode, t.max_per_hour,
	t.step_up, t.registered, t.created_at, t.updated_at`

func scanTool(row interface{ Scan(...any) error }, t *domain.Tool, extra ...any) error {
	var auto, ceiling, block *float32
	var safer []byte
	err := row.Scan(append([]any{&t.WorkspaceID, &t.Name, &t.Description, &t.RiskClass, &t.Reversible, &t.Pack, &safer,
		&auto, &ceiling, &block, &t.DoubleCheck, &t.FailMode, &t.MaxPerHour, &t.StepUp, &t.Registered,
		&t.CreatedAt, &t.UpdatedAt}, extra...)...)
	if err != nil {
		return err
	}
	t.Safer = json.RawMessage(safer)
	t.AutoConfidence = widen(auto)
	t.RiskCeiling = widen(ceiling)
	t.BlockConfidence = widen(block)
	return nil
}

func widen(f *float32) *float64 {
	if f == nil {
		return nil
	}
	v := float64(*f)
	// real columns hold about seven significant digits; round so 0.85 reads
	// back as 0.85 rather than 0.8500000238.
	v = float64(int64(v*1e6+0.5)) / 1e6
	return &v
}

// GetTool loads a tool profile.
func (q *Queries) GetTool(ctx context.Context, workspaceID, name string) (domain.Tool, error) {
	var t domain.Tool
	err := scanTool(q.db.QueryRow(ctx, `SELECT `+toolColumns+` FROM tools t WHERE t.workspace_id = $1 AND t.name = $2`, workspaceID, name), &t)
	return t, notFound(err)
}

// EnsureTool returns the tool's profile, first recording it as unregistered
// if Veto has never seen it. Unregistered tools show up in the console for
// someone to classify. created reports whether this call added it.
func (q *Queries) EnsureTool(ctx context.Context, workspaceID, name string) (t domain.Tool, created bool, err error) {
	def := domain.UnregisteredTool(workspaceID, name)
	tag, err := q.db.Exec(ctx,
		`INSERT INTO tools (workspace_id, name, risk_class, pack, safer, fail_mode, registered)
		 VALUES ($1, $2, $3, $4, $5, $6, false) ON CONFLICT DO NOTHING`,
		workspaceID, name, def.RiskClass, def.Pack, []byte(def.Safer), def.FailMode)
	if err != nil {
		return t, false, err
	}
	t, err = q.GetTool(ctx, workspaceID, name)
	return t, tag.RowsAffected() == 1, err
}

// UpsertTool creates or replaces a tool profile and marks it registered.
func (q *Queries) UpsertTool(ctx context.Context, t domain.Tool) error {
	safer := []byte(t.Safer)
	if len(safer) == 0 {
		safer = []byte("[]")
	}
	_, err := q.db.Exec(ctx, `
		INSERT INTO tools (workspace_id, name, description, risk_class, reversible, pack, safer,
		                   auto_confidence, risk_ceiling, block_confidence, double_check, fail_mode,
		                   max_per_hour, step_up, registered)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, true)
		ON CONFLICT (workspace_id, name) DO UPDATE SET
		    description = excluded.description, risk_class = excluded.risk_class,
		    reversible = excluded.reversible, pack = excluded.pack, safer = excluded.safer,
		    auto_confidence = excluded.auto_confidence, risk_ceiling = excluded.risk_ceiling,
		    block_confidence = excluded.block_confidence, double_check = excluded.double_check,
		    fail_mode = excluded.fail_mode, max_per_hour = excluded.max_per_hour,
		    step_up = excluded.step_up, registered = true, updated_at = now()`,
		t.WorkspaceID, t.Name, t.Description, t.RiskClass, t.Reversible, t.Pack, safer,
		t.AutoConfidence, t.RiskCeiling, t.BlockConfidence, t.DoubleCheck, t.FailMode,
		t.MaxPerHour, t.StepUp)
	return err
}

// DeleteTool removes a tool profile. Its next call registers it again as
// unclassified.
func (q *Queries) DeleteTool(ctx context.Context, workspaceID, name string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM tools WHERE workspace_id = $1 AND name = $2`, workspaceID, name)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ListTools returns tool profiles with seven-day activity, unclassified
// tools first.
func (q *Queries) ListTools(ctx context.Context, workspaceID string) ([]ToolStats, error) {
	rows, err := q.db.Query(ctx, `
		SELECT `+toolColumns+`,
		       coalesce(s.total, 0), coalesce(s.held, 0), coalesce(s.denied, 0), coalesce(s.safer, 0),
		       s.last_called, s.avg_conf
		FROM tools t
		LEFT JOIN LATERAL (
		    SELECT count(*) AS total,
		           count(*) FILTER (WHERE a.hold_reasons <> '[]'::jsonb) AS held,
		           count(*) FILTER (WHERE a.status IN ('denied', 'expired')) AS denied,
		           count(*) FILTER (WHERE d.option = 'proceed_safer' AND d.outcome = 'approved') AS safer,
		           max(a.received_at) AS last_called,
		           avg(d.model_confidence) AS avg_conf
		    FROM actions a LEFT JOIN decisions d ON d.action_id = a.id
		    WHERE a.workspace_id = t.workspace_id AND a.tool = t.name
		      AND a.received_at > now() - interval '7 days'
		) s ON true
		WHERE t.workspace_id = $1
		ORDER BY t.registered, lower(t.name)`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ToolStats
	for rows.Next() {
		var s ToolStats
		if err := scanTool(rows, &s.Tool, &s.Calls7d, &s.Held7d, &s.Denied7d, &s.SaferTaken7d, &s.LastCalledAt, &s.AvgConfidence); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
