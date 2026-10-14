package store

import (
	"context"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
)

// User is a person who signs in to the console.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	PasswordHash string     `json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	DisabledAt   *time.Time `json:"disabled_at"`
}

// Member is a user's membership of a workspace.
type Member struct {
	User
	Role     domain.Role `json:"role"`
	JoinedAt time.Time   `json:"joined_at"`
}

// Invitation lets someone join a workspace by setting a password.
type Invitation struct {
	ID          string      `json:"id"`
	WorkspaceID string      `json:"-"`
	Email       string      `json:"email"`
	Role        domain.Role `json:"role"`
	CreatedBy   *string     `json:"created_by"`
	CreatedAt   time.Time   `json:"created_at"`
	ExpiresAt   time.Time   `json:"expires_at"`
	AcceptedAt  *time.Time  `json:"accepted_at"`
	RevokedAt   *time.Time  `json:"revoked_at"`
}

const userColumns = `u.id, u.email, u.name, u.password_hash, u.created_at, u.last_login_at, u.disabled_at`

func scanUser(row interface{ Scan(...any) error }, u *User, extra ...any) error {
	return row.Scan(append([]any{&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt, &u.LastLoginAt, &u.DisabledAt}, extra...)...)
}

// CreateUser inserts a user. It returns ErrConflict if the email is taken.
func (q *Queries) CreateUser(ctx context.Context, u User) error {
	_, err := q.db.Exec(ctx,
		`INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, $3, $4)`,
		u.ID, u.Email, u.Name, u.PasswordHash)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// GetUserByEmail looks up a user case-insensitively.
func (q *Queries) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := scanUser(q.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users u WHERE lower(u.email) = lower($1)`, email), &u)
	return u, notFound(err)
}

// GetUser looks up a user by id.
func (q *Queries) GetUser(ctx context.Context, id string) (User, error) {
	var u User
	err := scanUser(q.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users u WHERE u.id = $1`, id), &u)
	return u, notFound(err)
}

// UpdateUserPassword replaces a password hash.
func (q *Queries) UpdateUserPassword(ctx context.Context, id, hash string) error {
	_, err := q.db.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, id, hash)
	return err
}

// UpdateUserProfile changes a user's display name.
func (q *Queries) UpdateUserProfile(ctx context.Context, id, name string) error {
	_, err := q.db.Exec(ctx, `UPDATE users SET name = $2 WHERE id = $1`, id, name)
	return err
}

// TouchLogin records a successful sign-in.
func (q *Queries) TouchLogin(ctx context.Context, id string) error {
	_, err := q.db.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, id)
	return err
}

// AddMembership gives a user a role in a workspace.
func (q *Queries) AddMembership(ctx context.Context, workspaceID, userID string, role domain.Role) error {
	_, err := q.db.Exec(ctx,
		`INSERT INTO memberships (workspace_id, user_id, role) VALUES ($1, $2, $3)`, workspaceID, userID, role)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// UpdateMembershipRole changes a member's role.
func (q *Queries) UpdateMembershipRole(ctx context.Context, workspaceID, userID string, role domain.Role) error {
	tag, err := q.db.Exec(ctx,
		`UPDATE memberships SET role = $3 WHERE workspace_id = $1 AND user_id = $2`, workspaceID, userID, role)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// RemoveMembership removes a member from a workspace.
func (q *Queries) RemoveMembership(ctx context.Context, workspaceID, userID string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM memberships WHERE workspace_id = $1 AND user_id = $2`, workspaceID, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// GetMember returns one member of a workspace.
func (q *Queries) GetMember(ctx context.Context, workspaceID, userID string) (Member, error) {
	var m Member
	err := scanUser(q.db.QueryRow(ctx,
		`SELECT `+userColumns+`, m.role, m.created_at FROM memberships m JOIN users u ON u.id = m.user_id
		 WHERE m.workspace_id = $1 AND m.user_id = $2`, workspaceID, userID), &m.User, &m.Role, &m.JoinedAt)
	return m, notFound(err)
}

// ListMembers returns a workspace's members, owners first.
func (q *Queries) ListMembers(ctx context.Context, workspaceID string) ([]Member, error) {
	rows, err := q.db.Query(ctx,
		`SELECT `+userColumns+`, m.role, m.created_at FROM memberships m JOIN users u ON u.id = m.user_id
		 WHERE m.workspace_id = $1
		 ORDER BY array_position(ARRAY['owner','admin','reviewer','viewer'], m.role), lower(u.name)`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := scanUser(rows, &m.User, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountOwners returns the number of owners of a workspace. The API refuses
// to remove or demote the last one.
func (q *Queries) CountOwners(ctx context.Context, workspaceID string) (int, error) {
	var n int
	err := q.db.QueryRow(ctx,
		`SELECT count(*) FROM memberships WHERE workspace_id = $1 AND role = 'owner'`, workspaceID).Scan(&n)
	return n, err
}

// FirstMembership returns the workspace a user belongs to. Veto 1 gives each
// user one workspace; the schema allows several for later versions.
func (q *Queries) FirstMembership(ctx context.Context, userID string) (string, domain.Role, error) {
	var ws string
	var role domain.Role
	err := q.db.QueryRow(ctx,
		`SELECT workspace_id, role FROM memberships WHERE user_id = $1 ORDER BY created_at LIMIT 1`, userID,
	).Scan(&ws, &role)
	return ws, role, notFound(err)
}

// CreateInvitation stores a pending invitation.
func (q *Queries) CreateInvitation(ctx context.Context, inv Invitation, tokenHash []byte) error {
	_, err := q.db.Exec(ctx,
		`INSERT INTO invitations (id, workspace_id, email, role, token_hash, created_by, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		inv.ID, inv.WorkspaceID, inv.Email, inv.Role, tokenHash, inv.CreatedBy, inv.ExpiresAt)
	return err
}

const invitationColumns = `id, workspace_id, email, role, created_by, created_at, expires_at, accepted_at, revoked_at`

func scanInvitation(row interface{ Scan(...any) error }, inv *Invitation) error {
	return row.Scan(&inv.ID, &inv.WorkspaceID, &inv.Email, &inv.Role, &inv.CreatedBy, &inv.CreatedAt,
		&inv.ExpiresAt, &inv.AcceptedAt, &inv.RevokedAt)
}

// GetInvitationByToken finds an invitation by the hash of its token.
func (q *Queries) GetInvitationByToken(ctx context.Context, tokenHash []byte) (Invitation, error) {
	var inv Invitation
	err := scanInvitation(q.db.QueryRow(ctx,
		`SELECT `+invitationColumns+` FROM invitations WHERE token_hash = $1`, tokenHash), &inv)
	return inv, notFound(err)
}

// AcceptInvitation marks an invitation used. It returns ErrNotFound if the
// invitation was already used, revoked, or has expired.
func (q *Queries) AcceptInvitation(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx,
		`UPDATE invitations SET accepted_at = now()
		 WHERE id = $1 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// RevokeInvitation cancels a pending invitation.
func (q *Queries) RevokeInvitation(ctx context.Context, workspaceID, id string) error {
	tag, err := q.db.Exec(ctx,
		`UPDATE invitations SET revoked_at = now()
		 WHERE workspace_id = $1 AND id = $2 AND accepted_at IS NULL AND revoked_at IS NULL`, workspaceID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ListInvitations returns a workspace's invitations, newest first.
func (q *Queries) ListInvitations(ctx context.Context, workspaceID string) ([]Invitation, error) {
	rows, err := q.db.Query(ctx,
		`SELECT `+invitationColumns+` FROM invitations WHERE workspace_id = $1 ORDER BY created_at DESC LIMIT 200`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invitation
	for rows.Next() {
		var inv Invitation
		if err := scanInvitation(rows, &inv); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}
