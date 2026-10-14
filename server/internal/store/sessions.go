package store

import (
	"context"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
)

// Session is a signed-in console session joined with its user and role.
type Session struct {
	TokenHash   []byte
	UserID      string
	WorkspaceID string
	Role        domain.Role
	UserEmail   string
	UserName    string
	CreatedAt   time.Time
	LastSeenAt  time.Time
	ExpiresAt   time.Time
}

// CreateSession stores a new session.
func (q *Queries) CreateSession(ctx context.Context, tokenHash []byte, userID, workspaceID string, expires time.Time, ip, userAgent string) error {
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	_, err := q.db.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, workspace_id, expires_at, ip, user_agent)
		 VALUES ($1, $2, $3, $4, $5, $6)`, tokenHash, userID, workspaceID, expires, ip, userAgent)
	return err
}

// GetSession returns a live session. Expired sessions, disabled users and
// removed memberships all read as not found.
func (q *Queries) GetSession(ctx context.Context, tokenHash []byte) (Session, error) {
	var s Session
	err := q.db.QueryRow(ctx,
		`SELECT s.token_hash, s.user_id, s.workspace_id, m.role, u.email, u.name, s.created_at, s.last_seen_at, s.expires_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id AND u.disabled_at IS NULL
		 JOIN memberships m ON m.user_id = s.user_id AND m.workspace_id = s.workspace_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash,
	).Scan(&s.TokenHash, &s.UserID, &s.WorkspaceID, &s.Role, &s.UserEmail, &s.UserName, &s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt)
	return s, notFound(err)
}

// ExtendSession slides a session's expiry forward.
func (q *Queries) ExtendSession(ctx context.Context, tokenHash []byte, expires time.Time) error {
	_, err := q.db.Exec(ctx,
		`UPDATE sessions SET last_seen_at = now(), expires_at = $2 WHERE token_hash = $1`, tokenHash, expires)
	return err
}

// DeleteSession signs a session out.
func (q *Queries) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := q.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

// DeleteUserSessions signs a user out everywhere, optionally keeping one
// session.
func (q *Queries) DeleteUserSessions(ctx context.Context, userID string, keep []byte) error {
	_, err := q.db.Exec(ctx,
		`DELETE FROM sessions WHERE user_id = $1 AND ($2::bytea IS NULL OR token_hash <> $2)`, userID, keep)
	return err
}

// DeleteExpiredSessions removes sessions past their expiry.
func (q *Queries) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := q.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	return tag.RowsAffected(), err
}
