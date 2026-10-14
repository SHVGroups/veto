package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/store"
)

func (s *Server) submitAction(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	wait, err := waitParam(r, s.cfg.MaxWait)
	if err != nil {
		return err
	}
	var req app.SubmitRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	res, err := s.app.Submit(r.Context(), actor, domain.SourceAPI, req, wait)
	if err != nil {
		return err
	}
	status := http.StatusOK
	if res.Status == domain.StatusPendingReview {
		status = http.StatusAccepted
	}
	writeJSON(w, status, res)
	return nil
}

func (s *Server) getActionResult(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	wait, err := waitParam(r, s.cfg.MaxWait)
	if err != nil {
		return err
	}
	res, err := s.app.GetResult(r.Context(), actor, r.PathValue("id"), wait)
	if err != nil {
		return err
	}
	return ok(w, res)
}

func (s *Server) reportOutcome(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var req app.OutcomeRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	o, err := s.app.ReportOutcome(r.Context(), actor, r.PathValue("id"), req)
	if err != nil {
		return err
	}
	return created(w, o)
}

func (s *Server) executeAction(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	resp, err := s.app.Execute(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, resp)
}

func (s *Server) verifyGrant(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var req app.VerifyRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	res, err := s.app.VerifyGrant(r.Context(), actor, req)
	if err != nil {
		return err
	}
	return ok(w, res)
}

func (s *Server) grantKeys(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	keys, err := s.app.PublicKeys(r.Context(), actor.WorkspaceID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"workspace_id": actor.WorkspaceID, "algorithm": "Ed25519", "keys": keys})
}

func (s *Server) mcpProxy(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	s.mcp.Serve(w, r, actor, r.PathValue("server"))
	return nil
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	o, err := s.app.Overview(r.Context(), actor, app.Range(r.URL.Query().Get("range")))
	if err != nil {
		return err
	}
	return ok(w, o)
}

func (s *Server) calibration(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	q := r.URL.Query()
	c, err := s.app.Calibration(r.Context(), actor, app.Range(q.Get("range")), q.Get("tool"))
	if err != nil {
		return err
	}
	return ok(w, c)
}

func (s *Server) listActions(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	q := r.URL.Query()
	limit, err := intParam(r, "limit", 50)
	if err != nil {
		return err
	}
	f := store.ActionFilter{
		AgentID:   q.Get("agent"),
		Tool:      q.Get("tool"),
		DecidedBy: q.Get("decided_by"),
		Option:    q.Get("option"),
		Source:    q.Get("source"),
		Query:     q.Get("q"),
		HeldOnly:  q.Get("held") == "true",
		Cursor:    q.Get("cursor"),
		Limit:     limit,
	}
	if st := q.Get("status"); st != "" {
		f.Statuses = strings.Split(st, ",")
	}
	for _, p := range []struct {
		name string
		dst  **time.Time
	}{{"since", &f.Since}, {"until", &f.Until}} {
		if v := q.Get(p.name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return &decodeError{p.name + " must be an RFC 3339 timestamp"}
			}
			*p.dst = &t
		}
	}
	items, next, err := s.app.ListActions(r.Context(), actor, f)
	if err != nil {
		return err
	}
	if items == nil {
		items = []store.ActionSummary{}
	}
	return ok(w, map[string]any{"items": items, "next_cursor": next})
}

func (s *Server) actionDetail(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	d, err := s.app.GetActionDetail(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, d)
}

func (s *Server) reviewAction(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var req app.ReviewRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	channel := app.ChannelConsole
	if actor.Kind == app.ActorKey {
		channel = app.ChannelAPI
	}
	if _, err := s.app.Review(r.Context(), actor, r.PathValue("id"), req, channel); err != nil {
		return err
	}
	d, err := s.app.GetActionDetail(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, d)
}

func (s *Server) listAudits(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	open := r.URL.Query().Get("state") != "reviewed"
	items, err := s.app.ListAudits(r.Context(), actor, open)
	if err != nil {
		return err
	}
	if items == nil {
		items = []store.AuditItem{}
	}
	return ok(w, map[string]any{"items": items})
}

func (s *Server) recordAudit(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var req struct {
		Verdict domain.AuditVerdict `json:"verdict"`
		Note    string              `json:"note"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.app.RecordAudit(r.Context(), actor, r.PathValue("id"), req.Verdict, req.Note)
	if err != nil {
		return err
	}
	return ok(w, a)
}
