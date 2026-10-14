package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
)

// Evaluation records one stage of judging an action: the policy gate, a rate
// limit check, or a model call with its full request and response.
type Evaluation struct {
	ID          string          `json:"id"`
	ActionID    string          `json:"action_id"`
	WorkspaceID string          `json:"-"`
	Stage       string          `json:"stage"`
	Provider    string          `json:"provider"`
	Model       string          `json:"model"`
	Request     json.RawMessage `json:"request"`
	Response    json.RawMessage `json:"response"`
	Result      json.RawMessage `json:"result"`
	LatencyMs   int             `json:"latency_ms"`
	InputTokens int             `json:"input_tokens"`
	Error       string          `json:"error"`
	CreatedAt   time.Time       `json:"created_at"`
}

// InsertEvaluation stores an evaluation record.
func (q *Queries) InsertEvaluation(ctx context.Context, e Evaluation) error {
	result := e.Result
	if len(result) == 0 {
		result = json.RawMessage("{}")
	}
	_, err := q.db.Exec(ctx, `
		INSERT INTO evaluations (id, action_id, workspace_id, stage, provider, model, request, response, result,
		                         latency_ms, input_tokens, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		e.ID, e.ActionID, e.WorkspaceID, e.Stage, e.Provider, e.Model, nullJSON(e.Request), nullJSON(e.Response),
		[]byte(result), e.LatencyMs, e.InputTokens, e.Error)
	return err
}

// ListEvaluations returns an action's evaluations in the order they ran.
func (q *Queries) ListEvaluations(ctx context.Context, workspaceID, actionID string) ([]Evaluation, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, action_id, workspace_id, stage, provider, model, request, response, result, latency_ms,
		       input_tokens, error, created_at
		FROM evaluations WHERE workspace_id = $1 AND action_id = $2 ORDER BY created_at, id`, workspaceID, actionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Evaluation
	for rows.Next() {
		var e Evaluation
		var req, resp, res []byte
		if err := rows.Scan(&e.ID, &e.ActionID, &e.WorkspaceID, &e.Stage, &e.Provider, &e.Model, &req, &resp, &res,
			&e.LatencyMs, &e.InputTokens, &e.Error, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Request, e.Response, e.Result = req, resp, res
		out = append(out, e)
	}
	return out, rows.Err()
}

// Decision is the final resolution of an action.
type Decision struct {
	ActionID        string            `json:"action_id"`
	WorkspaceID     string            `json:"-"`
	Option          domain.Option     `json:"option"`
	Outcome         string            `json:"outcome"`
	DecidedBy       domain.DecidedBy  `json:"decided_by"`
	ReviewerID      *string           `json:"reviewer_id"`
	ReviewerName    *string           `json:"reviewer_name"`
	FinalTool       string            `json:"final_tool"`
	FinalArgs       json.RawMessage   `json:"final_args"`
	Confidence      *float64          `json:"confidence"`
	Risk            *float64          `json:"risk"`
	ModelChoice     *string           `json:"model_choice"`
	ModelConfidence *float64          `json:"model_confidence"`
	Reasons         []domain.Reason   `json:"reasons"`
	Thresholds      domain.Thresholds `json:"thresholds"`
	Note            string            `json:"note"`
	ReviewSeconds   *float64          `json:"review_seconds"`
	CreatedAt       time.Time         `json:"created_at"`
}

// InsertDecision stores a decision.
func (q *Queries) InsertDecision(ctx context.Context, d *Decision) error {
	reasons, _ := json.Marshal(nonNil(d.Reasons))
	thresholds, _ := json.Marshal(d.Thresholds)
	return q.db.QueryRow(ctx, `
		INSERT INTO decisions (action_id, workspace_id, option, outcome, decided_by, reviewer_id, final_tool, final_args,
		                       confidence, risk, model_choice, model_confidence, reasons, thresholds, note, review_seconds)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING created_at`,
		d.ActionID, d.WorkspaceID, d.Option, d.Outcome, d.DecidedBy, d.ReviewerID, d.FinalTool, nullJSON(d.FinalArgs),
		d.Confidence, d.Risk, d.ModelChoice, d.ModelConfidence, reasons, thresholds, d.Note, d.ReviewSeconds,
	).Scan(&d.CreatedAt)
}

// GetDecision loads an action's decision.
func (q *Queries) GetDecision(ctx context.Context, workspaceID, actionID string) (Decision, error) {
	var d Decision
	var finalArgs, reasons, thresholds []byte
	var conf, risk, modelConf, reviewSecs *float32
	err := q.db.QueryRow(ctx, `
		SELECT d.action_id, d.workspace_id, d.option, d.outcome, d.decided_by, d.reviewer_id, u.name, d.final_tool,
		       d.final_args, d.confidence, d.risk, d.model_choice, d.model_confidence, d.reasons, d.thresholds,
		       d.note, d.review_seconds, d.created_at
		FROM decisions d LEFT JOIN users u ON u.id = d.reviewer_id
		WHERE d.workspace_id = $1 AND d.action_id = $2`, workspaceID, actionID,
	).Scan(&d.ActionID, &d.WorkspaceID, &d.Option, &d.Outcome, &d.DecidedBy, &d.ReviewerID, &d.ReviewerName, &d.FinalTool,
		&finalArgs, &conf, &risk, &d.ModelChoice, &modelConf, &reasons, &thresholds, &d.Note, &reviewSecs, &d.CreatedAt)
	if err != nil {
		return d, notFound(err)
	}
	d.FinalArgs = finalArgs
	d.Confidence, d.Risk, d.ModelConfidence, d.ReviewSeconds = widen(conf), widen(risk), widen(modelConf), widen(reviewSecs)
	if err := json.Unmarshal(reasons, &d.Reasons); err != nil {
		return d, err
	}
	err = json.Unmarshal(thresholds, &d.Thresholds)
	return d, err
}

// Grant is a stored record of an issued permission token.
type Grant struct {
	ID          string     `json:"id"`
	ActionID    string     `json:"action_id"`
	WorkspaceID string     `json:"-"`
	Tool        string     `json:"tool"`
	ArgsHash    []byte     `json:"-"`
	KeyID       string     `json:"key_id"`
	ExpiresAt   time.Time  `json:"expires_at"`
	ConsumedAt  *time.Time `json:"consumed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// InsertGrant stores an issued grant.
func (q *Queries) InsertGrant(ctx context.Context, g Grant) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO grants (id, action_id, workspace_id, tool, args_hash, key_id, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, g.ID, g.ActionID, g.WorkspaceID, g.Tool, g.ArgsHash, g.KeyID, g.ExpiresAt)
	return err
}

// ConsumeGrant marks a grant used. An action runs at most once, so a grant
// cannot be consumed if any grant of the same action already was. It returns
// ErrConflict if the grant was already used, has expired, or its action
// already ran, and ErrNotFound if it does not exist.
func (q *Queries) ConsumeGrant(ctx context.Context, workspaceID, id string) error {
	tag, err := q.db.Exec(ctx, `
		UPDATE grants SET consumed_at = now()
		WHERE workspace_id = $1 AND id = $2 AND consumed_at IS NULL AND expires_at > now()
		  AND NOT EXISTS (SELECT 1 FROM grants g2 WHERE g2.action_id = grants.action_id AND g2.consumed_at IS NOT NULL)`, workspaceID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var exists bool
	if err := q.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM grants WHERE workspace_id = $1 AND id = $2)`, workspaceID, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return ErrConflict
}

// ListGrants returns the grants issued for an action.
func (q *Queries) ListGrants(ctx context.Context, workspaceID, actionID string) ([]Grant, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, action_id, workspace_id, tool, args_hash, key_id, expires_at, consumed_at, created_at
		FROM grants WHERE workspace_id = $1 AND action_id = $2 ORDER BY created_at`, workspaceID, actionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.ID, &g.ActionID, &g.WorkspaceID, &g.Tool, &g.ArgsHash, &g.KeyID, &g.ExpiresAt, &g.ConsumedAt, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Outcome is a report of what happened after an action ran.
type Outcome struct {
	ID          string               `json:"id"`
	ActionID    string               `json:"action_id"`
	WorkspaceID string               `json:"-"`
	Status      domain.OutcomeStatus `json:"status"`
	Detail      json.RawMessage      `json:"detail"`
	ReportedBy  string               `json:"reported_by"`
	CreatedAt   time.Time            `json:"created_at"`
}

// InsertOutcome stores an outcome report.
func (q *Queries) InsertOutcome(ctx context.Context, o *Outcome) error {
	detail := o.Detail
	if len(detail) == 0 {
		detail = json.RawMessage("{}")
	}
	return q.db.QueryRow(ctx, `
		INSERT INTO outcomes (id, action_id, workspace_id, status, detail, reported_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING created_at`,
		o.ID, o.ActionID, o.WorkspaceID, o.Status, []byte(detail), o.ReportedBy).Scan(&o.CreatedAt)
}

// ListOutcomes returns an action's outcome reports in order.
func (q *Queries) ListOutcomes(ctx context.Context, workspaceID, actionID string) ([]Outcome, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, action_id, workspace_id, status, detail, reported_by, created_at
		FROM outcomes WHERE workspace_id = $1 AND action_id = $2 ORDER BY created_at`, workspaceID, actionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Outcome
	for rows.Next() {
		var o Outcome
		var detail []byte
		if err := rows.Scan(&o.ID, &o.ActionID, &o.WorkspaceID, &o.Status, &detail, &o.ReportedBy, &o.CreatedAt); err != nil {
			return nil, err
		}
		o.Detail = detail
		out = append(out, o)
	}
	return out, rows.Err()
}

func nullJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}
