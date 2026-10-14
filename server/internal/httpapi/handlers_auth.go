package httpapi

import (
	"net/http"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
)

func (s *Server) setSessionCookie(w http.ResponseWriter, sess *app.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    sess.Token,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0),
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

type sessionView struct {
	User        any    `json:"user"`
	Role        string `json:"role"`
	WorkspaceID string `json:"workspace_id"`
	ExpiresAt   string `json:"expires_at"`
}

func viewSession(sess *app.Session) sessionView {
	return sessionView{User: sess.User, Role: string(sess.Role), WorkspaceID: sess.Workspace, ExpiresAt: sess.ExpiresAt.UTC().Format(time.RFC3339)}
}

func (s *Server) getSetup(w http.ResponseWriter, r *http.Request, _ app.Actor) error {
	needs, err := s.app.NeedsSetup(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"needs_setup": needs, "setup_token_required": needs && s.cfg.SetupToken != ""})
}

func (s *Server) postSetup(w http.ResponseWriter, r *http.Request, _ app.Actor) error {
	var req app.SetupRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if !s.logins.allow("setup:" + s.clientIP(r)) {
		return &app.Error{Code: app.CodeRateLimited, Message: "too many attempts; wait a minute"}
	}
	sess, err := s.app.Setup(r.Context(), req, s.cfg.SetupToken, s.clientIP(r), r.UserAgent())
	if err != nil {
		return err
	}
	s.setSessionCookie(w, sess)
	return created(w, viewSession(sess))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request, _ app.Actor) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if !s.logins.allow("login:"+s.clientIP(r)) || !s.logins.allow("login-email:"+req.Email) {
		return &app.Error{Code: app.CodeRateLimited, Message: "too many sign-in attempts; wait a minute and try again"}
	}
	sess, err := s.app.Login(r.Context(), req.Email, req.Password, s.clientIP(r), r.UserAgent())
	if err != nil {
		return err
	}
	s.setSessionCookie(w, sess)
	return ok(w, viewSession(sess))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.Logout(r.Context(), actor); err != nil {
		return err
	}
	s.clearSessionCookie(w)
	return noContent(w)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	m, ws, err := s.app.Me(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"user": m.User, "role": m.Role, "workspace": ws})
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := s.app.UpdateProfile(r.Context(), actor, req.Name); err != nil {
		return err
	}
	return s.me(w, r, actor)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var req struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := s.app.ChangePassword(r.Context(), actor, req.Current, req.New); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) lookupInvitation(w http.ResponseWriter, r *http.Request, _ app.Actor) error {
	info, err := s.app.LookupInvitation(r.Context(), r.PathValue("token"))
	if err != nil {
		return err
	}
	return ok(w, info)
}

func (s *Server) acceptInvitation(w http.ResponseWriter, r *http.Request, _ app.Actor) error {
	var req app.AcceptRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if !s.logins.allow("invite:" + s.clientIP(r)) {
		return &app.Error{Code: app.CodeRateLimited, Message: "too many attempts; wait a minute"}
	}
	sess, err := s.app.AcceptInvitation(r.Context(), req, s.clientIP(r), r.UserAgent())
	if err != nil {
		return err
	}
	s.setSessionCookie(w, sess)
	return created(w, viewSession(sess))
}
