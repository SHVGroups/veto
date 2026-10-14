package app

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shvgroups/veto/server/internal/auth"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/id"
	"github.com/shvgroups/veto/server/internal/ledger"
	"github.com/shvgroups/veto/server/internal/policy"
	"github.com/shvgroups/veto/server/internal/store"
)

// SetupRequest creates the first workspace and its owner.
type SetupRequest struct {
	WorkspaceName string `json:"workspace_name"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Password      string `json:"password"`
	SetupToken    string `json:"setup_token,omitempty"`
}

// Session is a signed-in console session returned to the HTTP layer.
type Session struct {
	Token     string
	ExpiresAt time.Time
	User      store.User
	Role      domain.Role
	Workspace string
}

// NeedsSetup reports whether the installation has no users yet.
func (a *App) NeedsSetup(ctx context.Context) (bool, error) {
	n, err := a.Store.CountUsers(ctx)
	return n == 0, err
}

// Setup creates the first workspace, its owner, default policies and a
// signing key, and signs the owner in. It works only once.
func (a *App) Setup(ctx context.Context, req SetupRequest, requiredToken, ip, userAgent string) (*Session, error) {
	if requiredToken != "" && req.SetupToken != requiredToken {
		return nil, forbiddenf("setup token is missing or incorrect")
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return nil, err
	}
	req.Name, req.WorkspaceName = strings.TrimSpace(req.Name), strings.TrimSpace(req.WorkspaceName)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 100 {
		return nil, invalidf("name must be 1 to 100 characters")
	}
	if req.WorkspaceName == "" || utf8.RuneCountInString(req.WorkspaceName) > 100 {
		return nil, invalidf("workspace name must be 1 to 100 characters")
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		return nil, invalidf("%v", err)
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	wsID, userID := id.New(id.Workspace), id.New(id.User)
	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		// Serialize concurrent setup attempts; only the first may proceed.
		if err := q.LockForTransaction(ctx, store.LockSetup); err != nil {
			return err
		}
		n, err := q.CountUsers(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			return conflictf("setup has already been completed")
		}
		if err := q.CreateWorkspace(ctx, wsID, req.WorkspaceName, domain.DefaultSettings()); err != nil {
			return err
		}
		if err := q.CreateUser(ctx, store.User{ID: userID, Email: email, Name: req.Name, PasswordHash: hash}); err != nil {
			return mapStoreErr(err, "user")
		}
		if err := q.AddMembership(ctx, wsID, userID, domain.RoleOwner); err != nil {
			return err
		}
		owner := Actor{Kind: ActorUser, ID: userID, Name: req.Name, WorkspaceID: wsID, Role: domain.RoleOwner}
		if err := record(ctx, q, wsID, ledger.KindWorkspaceCreated, "", owner, map[string]any{"name": req.WorkspaceName, "owner": userID}); err != nil {
			return err
		}
		if err := a.seedWorkspace(ctx, q, wsID, owner); err != nil {
			return err
		}
		return a.createSigningKey(ctx, q, wsID)
	})
	if err != nil {
		return nil, err
	}
	return a.startSession(ctx, userID, wsID, ip, userAgent)
}

// seedWorkspace installs the default policy set as version 1.
func (a *App) seedWorkspace(ctx context.Context, q *store.Queries, wsID string, owner Actor) error {
	v, err := q.CreatePolicyVersion(ctx, wsID, policy.DefaultSource, "Default policies", &owner.ID)
	if err != nil {
		return err
	}
	if err := q.ActivatePolicyVersion(ctx, wsID, v); err != nil {
		return err
	}
	return record(ctx, q, wsID, ledger.KindPolicyActivated, "", owner, map[string]any{"version": v, "note": "Default policies"})
}

// Login checks credentials and starts a session.
func (a *App) Login(ctx context.Context, email, password, ip, userAgent string) (*Session, error) {
	invalid := &Error{Code: CodeUnauthorized, Message: "email or password is incorrect"}
	u, err := a.Store.GetUserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Spend the same time as a real check so response timing does not
			// reveal which emails have accounts.
			_, _ = auth.CheckPassword(password, dummyHash)
			return nil, invalid
		}
		return nil, err
	}
	rehash, err := auth.CheckPassword(password, u.PasswordHash)
	if err != nil || u.DisabledAt != nil {
		return nil, invalid
	}
	if rehash {
		if h, err := auth.HashPassword(password); err == nil {
			_ = a.Store.UpdateUserPassword(ctx, u.ID, h)
		}
	}
	ws, _, err := a.Store.FirstMembership(ctx, u.ID)
	if err != nil {
		return nil, &Error{Code: CodeForbidden, Message: "this account is not a member of any workspace"}
	}
	return a.startSession(ctx, u.ID, ws, ip, userAgent)
}

// dummyHash is a valid hash of a random password, used to equalize timing.
var dummyHash = func() string {
	h, err := auth.HashPassword(auth.NewToken("dummy"))
	if err != nil {
		panic(err)
	}
	return h
}()

func (a *App) startSession(ctx context.Context, userID, wsID, ip, userAgent string) (*Session, error) {
	token := auth.NewToken(auth.PrefixSession)
	expires := a.now().Add(a.SessionTTL)
	var s *Session
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.CreateSession(ctx, auth.HashToken(token), userID, wsID, expires, ip, userAgent); err != nil {
			return err
		}
		if err := q.TouchLogin(ctx, userID); err != nil {
			return err
		}
		m, err := q.GetMember(ctx, wsID, userID)
		if err != nil {
			return err
		}
		s = &Session{Token: token, ExpiresAt: expires, User: m.User, Role: m.Role, Workspace: wsID}
		actor := Actor{Kind: ActorUser, ID: userID, WorkspaceID: wsID}
		return record(ctx, q, wsID, ledger.KindSessionCreated, "", actor, map[string]any{"ip": ip, "user_agent": truncate(userAgent, 200)})
	})
	return s, err
}

// Authenticate resolves a session token to an actor, sliding the session's
// expiry forward when more than a day has passed since it was last extended.
func (a *App) Authenticate(ctx context.Context, token string) (Actor, error) {
	s, err := a.Store.GetSession(ctx, auth.HashToken(token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Actor{}, &Error{Code: CodeUnauthorized, Message: "session expired; sign in again"}
		}
		return Actor{}, err
	}
	if a.now().Sub(s.LastSeenAt) > time.Hour {
		_ = a.Store.ExtendSession(ctx, s.TokenHash, a.now().Add(a.SessionTTL))
	}
	return Actor{Kind: ActorUser, ID: s.UserID, Name: s.UserName, WorkspaceID: s.WorkspaceID, Role: s.Role, SessionHash: s.TokenHash}, nil
}

// AuthenticateKey resolves an API key secret to an actor.
func (a *App) AuthenticateKey(ctx context.Context, secret string) (Actor, error) {
	k, err := a.Store.GetAPIKeyBySecret(ctx, auth.HashToken(secret))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Actor{}, &Error{Code: CodeUnauthorized, Message: "API key is invalid or revoked"}
		}
		return Actor{}, err
	}
	_ = a.Store.TouchAPIKey(ctx, k.ID)
	if k.Kind == "agent" {
		name := ""
		if k.AgentName != nil {
			name = *k.AgentName
		}
		return Actor{Kind: ActorAgent, ID: *k.AgentID, AgentID: *k.AgentID, Name: name, WorkspaceID: k.WorkspaceID, AgentDisabled: k.AgentDisabled}, nil
	}
	return Actor{Kind: ActorKey, ID: k.ID, Name: k.Name, WorkspaceID: k.WorkspaceID, Role: domain.RoleAdmin}, nil
}

// Logout ends a session.
func (a *App) Logout(ctx context.Context, actor Actor) error {
	if actor.SessionHash == nil {
		return nil
	}
	return a.Store.DeleteSession(ctx, actor.SessionHash)
}

// ChangePassword replaces the signed-in user's password and signs out every
// other session.
func (a *App) ChangePassword(ctx context.Context, actor Actor, current, next string) error {
	if actor.Kind != ActorUser {
		return forbiddenf("only users have passwords")
	}
	u, err := a.Store.GetUser(ctx, actor.ID)
	if err != nil {
		return err
	}
	if _, err := auth.CheckPassword(current, u.PasswordHash); err != nil {
		return &Error{Code: CodeForbidden, Message: "current password is incorrect"}
	}
	if err := auth.ValidatePassword(next); err != nil {
		return invalidf("%v", err)
	}
	h, err := auth.HashPassword(next)
	if err != nil {
		return err
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.UpdateUserPassword(ctx, u.ID, h); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, u.ID, actor.SessionHash); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindMemberChanged, "", actor, map[string]any{"user_id": u.ID, "change": "password"})
	})
}

// UpdateProfile changes the signed-in user's display name.
func (a *App) UpdateProfile(ctx context.Context, actor Actor, name string) error {
	name = strings.TrimSpace(name)
	if actor.Kind != ActorUser {
		return forbiddenf("only users have profiles")
	}
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return invalidf("name must be 1 to 100 characters")
	}
	return a.Store.UpdateUserProfile(ctx, actor.ID, name)
}

// Me returns the signed-in user and their workspace.
func (a *App) Me(ctx context.Context, actor Actor) (store.Member, store.Workspace, error) {
	if actor.Kind != ActorUser {
		return store.Member{}, store.Workspace{}, forbiddenf("only users have a profile")
	}
	m, err := a.Store.GetMember(ctx, actor.WorkspaceID, actor.ID)
	if err != nil {
		return m, store.Workspace{}, err
	}
	ws, err := a.Store.GetWorkspace(ctx, actor.WorkspaceID)
	return m, ws, err
}

// InviteRequest invites someone to the workspace.
type InviteRequest struct {
	Email string      `json:"email"`
	Role  domain.Role `json:"role"`
}

// Invite creates an invitation and returns its one-time token. Veto sends no
// email; the inviter shares the link.
func (a *App) Invite(ctx context.Context, actor Actor, req InviteRequest) (store.Invitation, string, error) {
	if err := require(actor, domain.PermManageMembers); err != nil {
		return store.Invitation{}, "", err
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return store.Invitation{}, "", err
	}
	if !req.Role.Valid() {
		return store.Invitation{}, "", invalidf("role must be owner, admin, reviewer or viewer")
	}
	if req.Role == domain.RoleOwner && !actor.Can(domain.PermOwn) {
		return store.Invitation{}, "", forbiddenf("only owners can invite owners")
	}
	token := auth.NewToken(auth.PrefixInvite)
	creator := actor.ID
	inv := store.Invitation{
		ID: id.New(id.Invitation), WorkspaceID: actor.WorkspaceID, Email: email, Role: req.Role,
		CreatedBy: &creator, ExpiresAt: a.now().Add(7 * 24 * time.Hour), CreatedAt: a.now(),
	}
	err = a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.CreateInvitation(ctx, inv, auth.HashToken(token)); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindMemberChanged, "", actor, map[string]any{
			"change": "invited", "email": email, "role": req.Role, "invitation_id": inv.ID,
		})
	})
	return inv, token, err
}

// InvitationInfo describes an invitation to the person opening its link.
type InvitationInfo struct {
	Email         string      `json:"email"`
	Role          domain.Role `json:"role"`
	WorkspaceName string      `json:"workspace_name"`
	ExpiresAt     time.Time   `json:"expires_at"`
	AccountExists bool        `json:"account_exists"`
}

// LookupInvitation returns what an invitation token is for.
func (a *App) LookupInvitation(ctx context.Context, token string) (*InvitationInfo, error) {
	inv, err := a.validInvitation(ctx, a.Store.Queries, token)
	if err != nil {
		return nil, err
	}
	ws, err := a.Store.GetWorkspace(ctx, inv.WorkspaceID)
	if err != nil {
		return nil, err
	}
	_, uerr := a.Store.GetUserByEmail(ctx, inv.Email)
	return &InvitationInfo{Email: inv.Email, Role: inv.Role, WorkspaceName: ws.Name, ExpiresAt: inv.ExpiresAt, AccountExists: uerr == nil}, nil
}

func (a *App) validInvitation(ctx context.Context, q *store.Queries, token string) (store.Invitation, error) {
	inv, err := q.GetInvitationByToken(ctx, auth.HashToken(token))
	if err != nil {
		return inv, notFoundf("invitation not found")
	}
	if inv.AcceptedAt != nil || inv.RevokedAt != nil || !inv.ExpiresAt.After(a.now()) {
		return inv, &Error{Code: CodeConflict, Message: "invitation is no longer valid"}
	}
	return inv, nil
}

// AcceptRequest accepts an invitation.
type AcceptRequest struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// AcceptInvitation creates the account (or checks the existing one's
// password), adds the membership, and signs the person in.
func (a *App) AcceptInvitation(ctx context.Context, req AcceptRequest, ip, userAgent string) (*Session, error) {
	var userID, wsID string
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		inv, err := a.validInvitation(ctx, q, req.Token)
		if err != nil {
			return err
		}
		wsID = inv.WorkspaceID
		existing, err := q.GetUserByEmail(ctx, inv.Email)
		switch {
		case err == nil:
			if _, err := auth.CheckPassword(req.Password, existing.PasswordHash); err != nil {
				return &Error{Code: CodeUnauthorized, Message: "an account with this email exists; enter its password"}
			}
			userID = existing.ID
		case errors.Is(err, store.ErrNotFound):
			name := strings.TrimSpace(req.Name)
			if name == "" || utf8.RuneCountInString(name) > 100 {
				return invalidf("name must be 1 to 100 characters")
			}
			if err := auth.ValidatePassword(req.Password); err != nil {
				return invalidf("%v", err)
			}
			h, err := auth.HashPassword(req.Password)
			if err != nil {
				return err
			}
			userID = id.New(id.User)
			if err := q.CreateUser(ctx, store.User{ID: userID, Email: inv.Email, Name: name, PasswordHash: h}); err != nil {
				return mapStoreErr(err, "user")
			}
		default:
			return err
		}
		if err := q.AcceptInvitation(ctx, inv.ID); err != nil {
			return &Error{Code: CodeConflict, Message: "invitation is no longer valid"}
		}
		if err := q.AddMembership(ctx, wsID, userID, inv.Role); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return conflictf("you are already a member of this workspace")
			}
			return err
		}
		actor := Actor{Kind: ActorUser, ID: userID, WorkspaceID: wsID}
		return record(ctx, q, wsID, ledger.KindMemberChanged, "", actor, map[string]any{
			"change": "joined", "user_id": userID, "role": inv.Role, "invitation_id": inv.ID,
		})
	})
	if err != nil {
		return nil, err
	}
	return a.startSession(ctx, userID, wsID, ip, userAgent)
}

// RevokeInvitation cancels a pending invitation.
func (a *App) RevokeInvitation(ctx context.Context, actor Actor, invitationID string) error {
	if err := require(actor, domain.PermManageMembers); err != nil {
		return err
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.RevokeInvitation(ctx, actor.WorkspaceID, invitationID); err != nil {
			return mapStoreErr(err, "invitation")
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindMemberChanged, "", actor, map[string]any{"change": "invitation_revoked", "invitation_id": invitationID})
	})
}

// SetRole changes a member's role. The last owner cannot be demoted.
func (a *App) SetRole(ctx context.Context, actor Actor, userID string, role domain.Role) error {
	if err := require(actor, domain.PermManageMembers); err != nil {
		return err
	}
	if !role.Valid() {
		return invalidf("role must be owner, admin, reviewer or viewer")
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		m, err := q.GetMember(ctx, actor.WorkspaceID, userID)
		if err != nil {
			return mapStoreErr(err, "member")
		}
		if (m.Role == domain.RoleOwner || role == domain.RoleOwner) && !actor.Can(domain.PermOwn) {
			return forbiddenf("only owners can change owner roles")
		}
		if m.Role == domain.RoleOwner && role != domain.RoleOwner {
			n, err := q.CountOwners(ctx, actor.WorkspaceID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return conflictf("a workspace needs at least one owner")
			}
		}
		if err := q.UpdateMembershipRole(ctx, actor.WorkspaceID, userID, role); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindMemberChanged, "", actor, map[string]any{
			"change": "role", "user_id": userID, "from": m.Role, "to": role,
		})
	})
}

// RemoveMember removes someone from the workspace and signs them out.
func (a *App) RemoveMember(ctx context.Context, actor Actor, userID string) error {
	if err := require(actor, domain.PermManageMembers); err != nil {
		return err
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		m, err := q.GetMember(ctx, actor.WorkspaceID, userID)
		if err != nil {
			return mapStoreErr(err, "member")
		}
		if m.Role == domain.RoleOwner {
			if !actor.Can(domain.PermOwn) {
				return forbiddenf("only owners can remove owners")
			}
			n, err := q.CountOwners(ctx, actor.WorkspaceID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return conflictf("a workspace needs at least one owner")
			}
		}
		if err := q.RemoveMembership(ctx, actor.WorkspaceID, userID); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, userID, nil); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindMemberChanged, "", actor, map[string]any{"change": "removed", "user_id": userID, "role": m.Role})
	})
}

func normalizeEmail(s string) (string, error) {
	s = strings.TrimSpace(s)
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || len(s) > 254 {
		return "", invalidf("email address is not valid")
	}
	return strings.ToLower(s), nil
}
