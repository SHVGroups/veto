package store

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/shvgroups/veto/server/internal/canonical"
	"github.com/shvgroups/veto/server/internal/ledger"
)

// AppendLedger adds an entry to the workspace's chain. It locks the chain
// head, so call it inside the transaction that makes the change it records:
// the entry and the change then commit or roll back together.
func (q *Queries) AppendLedger(ctx context.Context, workspaceID, kind, actionID, actor string, payload any) (ledger.Entry, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ledger.Entry{}, err
	}
	v, err := canonical.Decode(raw)
	if err != nil {
		return ledger.Entry{}, err
	}
	body, err := canonical.Marshal(v)
	if err != nil {
		return ledger.Entry{}, err
	}

	var seq int64
	var prev []byte
	if err := q.db.QueryRow(ctx,
		`SELECT seq, hash FROM ledger_heads WHERE workspace_id = $1 FOR UPDATE`, workspaceID).Scan(&seq, &prev); err != nil {
		return ledger.Entry{}, notFound(err)
	}
	e := ledger.Entry{
		WorkspaceID: workspaceID,
		Seq:         seq + 1,
		Kind:        kind,
		ActionID:    actionID,
		Actor:       actor,
		Payload:     body,
		PrevHash:    prev,
		CreatedAt:   ledger.Normalize(time.Now()),
	}
	if e.Hash, err = ledger.Hash(e); err != nil {
		return ledger.Entry{}, err
	}
	if _, err := q.db.Exec(ctx, `
		INSERT INTO ledger_entries (workspace_id, seq, kind, action_id, actor, payload, prev_hash, hash, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9)`,
		e.WorkspaceID, e.Seq, e.Kind, e.ActionID, e.Actor, []byte(e.Payload), e.PrevHash, e.Hash, e.CreatedAt); err != nil {
		return ledger.Entry{}, err
	}
	if _, err := q.db.Exec(ctx,
		`UPDATE ledger_heads SET seq = $2, hash = $3 WHERE workspace_id = $1`, workspaceID, e.Seq, e.Hash); err != nil {
		return ledger.Entry{}, err
	}
	return e, nil
}

const ledgerColumns = `workspace_id, seq, kind, coalesce(action_id, ''), actor, payload, prev_hash, hash, created_at`

func scanLedger(row interface{ Scan(...any) error }, e *ledger.Entry) error {
	var payload []byte
	if err := row.Scan(&e.WorkspaceID, &e.Seq, &e.Kind, &e.ActionID, &e.Actor, &payload, &e.PrevHash, &e.Hash, &e.CreatedAt); err != nil {
		return err
	}
	e.Payload = payload
	return nil
}

// LedgerFilter narrows a ledger listing.
type LedgerFilter struct {
	Kind      string
	ActionID  string
	BeforeSeq int64
	Limit     int
}

// ListLedger returns entries newest first.
func (q *Queries) ListLedger(ctx context.Context, workspaceID string, f LedgerFilter) ([]ledger.Entry, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	w := whereBuilder{}
	w.add("workspace_id = %s", workspaceID)
	if f.Kind != "" {
		w.add("kind LIKE %s", escapeLike(f.Kind)+"%")
	}
	if f.ActionID != "" {
		w.add("action_id = %s", f.ActionID)
	}
	if f.BeforeSeq > 0 {
		w.add("seq < %s", f.BeforeSeq)
	}
	rows, err := q.db.Query(ctx, `SELECT `+ledgerColumns+` FROM ledger_entries WHERE `+w.sql()+
		` ORDER BY seq DESC LIMIT `+strconv.Itoa(f.Limit), w.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ledger.Entry
	for rows.Next() {
		var e ledger.Entry
		if err := scanLedger(rows, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LedgerRange returns entries with from <= seq <= to in order.
func (q *Queries) LedgerRange(ctx context.Context, workspaceID string, from, to int64) ([]ledger.Entry, error) {
	rows, err := q.db.Query(ctx, `SELECT `+ledgerColumns+` FROM ledger_entries
		WHERE workspace_id = $1 AND seq BETWEEN $2 AND $3 ORDER BY seq`, workspaceID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ledger.Entry
	for rows.Next() {
		var e ledger.Entry
		if err := scanLedger(rows, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LedgerHead returns the sequence number and hash of the newest entry.
func (q *Queries) LedgerHead(ctx context.Context, workspaceID string) (int64, []byte, error) {
	var seq int64
	var hash []byte
	err := q.db.QueryRow(ctx, `SELECT seq, hash FROM ledger_heads WHERE workspace_id = $1`, workspaceID).Scan(&seq, &hash)
	return seq, hash, notFound(err)
}

// Checkpoint is a signed statement of the chain head at a point in time.
type Checkpoint struct {
	Seq       int64     `json:"seq"`
	Hash      []byte    `json:"-"`
	KeyID     string    `json:"key_id"`
	Signature []byte    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// InsertCheckpoint stores a signed checkpoint.
func (q *Queries) InsertCheckpoint(ctx context.Context, workspaceID string, c Checkpoint) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO ledger_checkpoints (workspace_id, seq, hash, key_id, signature) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING`, workspaceID, c.Seq, c.Hash, c.KeyID, c.Signature)
	return err
}

// ListCheckpoints returns recent checkpoints, newest first.
func (q *Queries) ListCheckpoints(ctx context.Context, workspaceID string, limit int) ([]Checkpoint, error) {
	rows, err := q.db.Query(ctx, `
		SELECT seq, hash, key_id, signature, created_at FROM ledger_checkpoints
		WHERE workspace_id = $1 ORDER BY seq DESC LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Checkpoint
	for rows.Next() {
		var c Checkpoint
		if err := rows.Scan(&c.Seq, &c.Hash, &c.KeyID, &c.Signature, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
