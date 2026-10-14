package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
)

// Audit is an after-the-fact human review of an automatic decision.
type Audit struct {
	ID           string               `json:"id"`
	ActionID     string               `json:"action_id"`
	WorkspaceID  string               `json:"-"`
	Reason       string               `json:"reason"`
	Verdict      *domain.AuditVerdict `json:"verdict"`
	ReviewerID   *string              `json:"reviewer_id"`
	ReviewerName *string              `json:"reviewer_name"`
	Note         string               `json:"note"`
	CreatedAt    time.Time            `json:"created_at"`
	ReviewedAt   *time.Time           `json:"reviewed_at"`
}

// AuditItem is an audit with enough of its action to review it in a list.
type AuditItem struct {
	Audit
	Tool       string          `json:"tool"`
	AgentName  string          `json:"agent_name"`
	Request    string          `json:"request"`
	Option     string          `json:"option"`
	DecidedBy  string          `json:"decided_by"`
	Confidence *float64        `json:"confidence"`
	Risk       *float64        `json:"risk"`
	FinalTool  string          `json:"final_tool"`
	FinalArgs  json.RawMessage `json:"final_args"`
	ReceivedAt time.Time       `json:"received_at"`
	Outcome    *string         `json:"outcome"`
}

// InsertAudit queues an action for audit. A second request for the same
// action is ignored.
func (q *Queries) InsertAudit(ctx context.Context, a Audit) (bool, error) {
	tag, err := q.db.Exec(ctx, `
		INSERT INTO audits (id, action_id, workspace_id, reason) VALUES ($1, $2, $3, $4)
		ON CONFLICT (action_id) DO NOTHING`, a.ID, a.ActionID, a.WorkspaceID, a.Reason)
	return tag.RowsAffected() == 1, err
}

// RecordAuditVerdict stores a reviewer's verdict. It returns ErrNotFound if
// the audit does not exist.
func (q *Queries) RecordAuditVerdict(ctx context.Context, workspaceID, id string, verdict domain.AuditVerdict, reviewerID, note string) (Audit, error) {
	var a Audit
	err := q.db.QueryRow(ctx, `
		UPDATE audits SET verdict = $3, reviewer_id = $4, note = $5, reviewed_at = now()
		WHERE workspace_id = $1 AND id = $2
		RETURNING id, action_id, workspace_id, reason, verdict, reviewer_id, note, created_at, reviewed_at`,
		workspaceID, id, verdict, reviewerID, note,
	).Scan(&a.ID, &a.ActionID, &a.WorkspaceID, &a.Reason, &a.Verdict, &a.ReviewerID, &a.Note, &a.CreatedAt, &a.ReviewedAt)
	return a, notFound(err)
}

// GetAuditForAction returns the audit attached to an action, if any.
func (q *Queries) GetAuditForAction(ctx context.Context, workspaceID, actionID string) (Audit, error) {
	var a Audit
	err := q.db.QueryRow(ctx, `
		SELECT a.id, a.action_id, a.workspace_id, a.reason, a.verdict, a.reviewer_id, u.name, a.note, a.created_at, a.reviewed_at
		FROM audits a LEFT JOIN users u ON u.id = a.reviewer_id
		WHERE a.workspace_id = $1 AND a.action_id = $2`, workspaceID, actionID,
	).Scan(&a.ID, &a.ActionID, &a.WorkspaceID, &a.Reason, &a.Verdict, &a.ReviewerID, &a.ReviewerName, &a.Note, &a.CreatedAt, &a.ReviewedAt)
	return a, notFound(err)
}

// ListAudits returns open audits oldest first, or reviewed audits newest
// first.
func (q *Queries) ListAudits(ctx context.Context, workspaceID string, open bool, limit int) ([]AuditItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	cond, order := "au.verdict IS NULL", "au.created_at ASC"
	if !open {
		cond, order = "au.verdict IS NOT NULL", "au.reviewed_at DESC"
	}
	rows, err := q.db.Query(ctx, `
		SELECT au.id, au.action_id, au.workspace_id, au.reason, au.verdict, au.reviewer_id, u.name, au.note,
		       au.created_at, au.reviewed_at,
		       a.tool, ag.name, left(a.request, 400), d.option, d.decided_by, d.confidence, d.risk,
		       d.final_tool, d.final_args, a.received_at,
		       (SELECT o.status FROM outcomes o WHERE o.action_id = a.id ORDER BY o.created_at DESC LIMIT 1)
		FROM audits au
		JOIN actions a ON a.id = au.action_id
		JOIN agents ag ON ag.id = a.agent_id
		JOIN decisions d ON d.action_id = a.id
		LEFT JOIN users u ON u.id = au.reviewer_id
		WHERE au.workspace_id = $1 AND `+cond+`
		ORDER BY `+order+` LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditItem
	for rows.Next() {
		var it AuditItem
		var conf, risk *float32
		var finalArgs []byte
		if err := rows.Scan(&it.ID, &it.ActionID, &it.WorkspaceID, &it.Reason, &it.Verdict, &it.ReviewerID, &it.ReviewerName,
			&it.Note, &it.CreatedAt, &it.ReviewedAt, &it.Tool, &it.AgentName, &it.Request, &it.Option, &it.DecidedBy,
			&conf, &risk, &it.FinalTool, &finalArgs, &it.ReceivedAt, &it.Outcome); err != nil {
			return nil, err
		}
		it.Confidence, it.Risk = widen(conf), widen(risk)
		it.FinalArgs = finalArgs
		out = append(out, it)
	}
	return out, rows.Err()
}

// SigningKey is a workspace's Ed25519 key for grants and checkpoints.
type SigningKey struct {
	ID            string     `json:"id"`
	WorkspaceID   string     `json:"-"`
	KeyID         string     `json:"key_id"`
	PublicKey     []byte     `json:"public_key"`
	PrivateKeyEnc []byte     `json:"-"`
	CreatedAt     time.Time  `json:"created_at"`
	RetiredAt     *time.Time `json:"retired_at"`
}

// InsertSigningKey stores a key. The private half arrives sealed.
func (q *Queries) InsertSigningKey(ctx context.Context, k SigningKey) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO signing_keys (id, workspace_id, key_id, public_key, private_key_enc) VALUES ($1, $2, $3, $4, $5)`,
		k.ID, k.WorkspaceID, k.KeyID, k.PublicKey, k.PrivateKeyEnc)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// RetireSigningKeys marks the active key retired so a new one can be added.
// Retired keys still verify grants and checkpoints they signed.
func (q *Queries) RetireSigningKeys(ctx context.Context, workspaceID string) error {
	_, err := q.db.Exec(ctx, `UPDATE signing_keys SET retired_at = now() WHERE workspace_id = $1 AND retired_at IS NULL`, workspaceID)
	return err
}

// ActiveSigningKey returns the key that signs new grants.
func (q *Queries) ActiveSigningKey(ctx context.Context, workspaceID string) (SigningKey, error) {
	return q.signingKey(ctx, `workspace_id = $1 AND retired_at IS NULL`, workspaceID)
}

// SigningKeyByKeyID returns a key, retired or not, by its key id.
func (q *Queries) SigningKeyByKeyID(ctx context.Context, workspaceID, keyID string) (SigningKey, error) {
	return q.signingKey(ctx, `workspace_id = $1 AND key_id = $2`, workspaceID, keyID)
}

func (q *Queries) signingKey(ctx context.Context, cond string, args ...any) (SigningKey, error) {
	var k SigningKey
	err := q.db.QueryRow(ctx, `
		SELECT id, workspace_id, key_id, public_key, private_key_enc, created_at, retired_at
		FROM signing_keys WHERE `+cond, args...,
	).Scan(&k.ID, &k.WorkspaceID, &k.KeyID, &k.PublicKey, &k.PrivateKeyEnc, &k.CreatedAt, &k.RetiredAt)
	return k, notFound(err)
}

// ListSigningKeys returns every key of a workspace, newest first.
func (q *Queries) ListSigningKeys(ctx context.Context, workspaceID string) ([]SigningKey, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, workspace_id, key_id, public_key, private_key_enc, created_at, retired_at
		FROM signing_keys WHERE workspace_id = $1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SigningKey
	for rows.Next() {
		var k SigningKey
		if err := rows.Scan(&k.ID, &k.WorkspaceID, &k.KeyID, &k.PublicKey, &k.PrivateKeyEnc, &k.CreatedAt, &k.RetiredAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// AllSealedSigningKeys returns every sealed private key, for master key
// rotation.
func (q *Queries) AllSealedSigningKeys(ctx context.Context) ([]SigningKey, error) {
	rows, err := q.db.Query(ctx, `SELECT id, workspace_id, key_id, public_key, private_key_enc, created_at, retired_at FROM signing_keys`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SigningKey
	for rows.Next() {
		var k SigningKey
		if err := rows.Scan(&k.ID, &k.WorkspaceID, &k.KeyID, &k.PublicKey, &k.PrivateKeyEnc, &k.CreatedAt, &k.RetiredAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// UpdateSealedSigningKey replaces a sealed private key after rotation.
func (q *Queries) UpdateSealedSigningKey(ctx context.Context, id string, sealed []byte) error {
	_, err := q.db.Exec(ctx, `UPDATE signing_keys SET private_key_enc = $2 WHERE id = $1`, id, sealed)
	return err
}
