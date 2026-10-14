package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/store"
)

func list[T any](items []T) map[string]any {
	if items == nil {
		items = []T{}
	}
	return map[string]any{"items": items}
}

func versionParam(r *http.Request) (int, error) {
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || v < 1 {
		return 0, &decodeError{"version must be a positive integer"}
	}
	return v, nil
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListAgents(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.AgentInput
	if err := decode(r, &in); err != nil {
		return err
	}
	ag, key, err := s.app.CreateAgent(r.Context(), actor, in)
	if err != nil {
		return err
	}
	return created(w, map[string]any{"agent": ag, "key": key})
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	d, err := s.app.GetAgentDetail(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, d)
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.AgentInput
	if err := decode(r, &in); err != nil {
		return err
	}
	ag, err := s.app.UpdateAgent(r.Context(), actor, r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return ok(w, ag)
}

func (s *Server) issueAgentKey(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	key, err := s.app.IssueAgentKey(r.Context(), actor, r.PathValue("id"), in.Name)
	if err != nil {
		return err
	}
	return created(w, key)
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListKeys(r.Context(), actor, r.URL.Query().Get("agent"))
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) issueAdminKey(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	key, err := s.app.IssueAdminKey(r.Context(), actor, in.Name)
	if err != nil {
		return err
	}
	return created(w, key)
}

func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.RevokeKey(r.Context(), actor, r.PathValue("id")); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) listTools(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListTools(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) getTool(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	t, err := s.app.GetTool(r.Context(), actor, r.PathValue("name"))
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"tool": t, "effective": t.Thresholds()})
}

func (s *Server) saveTool(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.ToolInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Name = r.PathValue("name")
	t, err := s.app.SaveTool(r.Context(), actor, in)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"tool": t, "effective": t.Thresholds()})
}

func (s *Server) deleteTool(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.DeleteTool(r.Context(), actor, r.PathValue("name")); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) previewSafer(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in struct {
		Tool  string          `json:"tool"`
		Args  json.RawMessage `json:"args"`
		Rules json.RawMessage `json:"rules"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	spec, err := s.app.PreviewSafer(actor, in.Tool, in.Args, in.Rules)
	if err != nil {
		return err
	}
	return ok(w, spec)
}

func (s *Server) listPolicies(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListPolicyVersions(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) activePolicy(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	ws, err := s.app.GetWorkspace(r.Context(), actor)
	if err != nil {
		return err
	}
	if ws.ActivePolicyVersion == nil {
		return &app.Error{Code: app.CodeNotFound, Message: "no policy version is active"}
	}
	p, err := s.app.GetPolicyVersion(r.Context(), actor, *ws.ActivePolicyVersion)
	if err != nil {
		return err
	}
	check, err := s.app.CheckPolicy(actor, p.Source)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"version": p, "policies": check.Policies})
}

func (s *Server) getPolicy(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	v, err := versionParam(r)
	if err != nil {
		return err
	}
	p, err := s.app.GetPolicyVersion(r.Context(), actor, v)
	if err != nil {
		return err
	}
	return ok(w, p)
}

func (s *Server) savePolicy(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in struct {
		Source   string `json:"source"`
		Note     string `json:"note"`
		Activate bool   `json:"activate"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	v, err := s.app.SavePolicy(r.Context(), actor, in.Source, in.Note, in.Activate)
	if err != nil {
		return err
	}
	p, err := s.app.GetPolicyVersion(r.Context(), actor, v)
	if err != nil {
		return err
	}
	return created(w, p)
}

func (s *Server) checkPolicy(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in struct {
		Source string `json:"source"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	c, err := s.app.CheckPolicy(actor, in.Source)
	if err != nil {
		return err
	}
	return ok(w, c)
}

func (s *Server) activatePolicy(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	v, err := versionParam(r)
	if err != nil {
		return err
	}
	if err := s.app.ActivatePolicy(r.Context(), actor, v); err != nil {
		return err
	}
	p, err := s.app.GetPolicyVersion(r.Context(), actor, v)
	if err != nil {
		return err
	}
	return ok(w, p)
}

func (s *Server) listPacks(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListPacks(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) packVersions(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListPackVersions(r.Context(), actor, r.PathValue("name"))
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) savePack(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBody))
	if err != nil {
		return &decodeError{"request body could not be read"}
	}
	v, err := s.app.SavePack(r.Context(), actor, r.PathValue("name"), raw)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"name": r.PathValue("name"), "version": v})
}

func (s *Server) resetPack(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.ResetPack(r.Context(), actor, r.PathValue("name")); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) simulate(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.SimulateRequest
	if err := decode(r, &in); err != nil {
		return err
	}
	res, err := s.app.Simulate(r.Context(), actor, in)
	if err != nil {
		return err
	}
	return ok(w, res)
}

func (s *Server) listConnections(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListConnections(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) createConnection(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.ConnectionInput
	if err := decode(r, &in); err != nil {
		return err
	}
	c, err := s.app.CreateConnection(r.Context(), actor, in)
	if err != nil {
		return err
	}
	return created(w, c)
}

func (s *Server) updateConnection(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.ConnectionInput
	if err := decode(r, &in); err != nil {
		return err
	}
	c, err := s.app.UpdateConnection(r.Context(), actor, r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return ok(w, c)
}

func (s *Server) deleteConnection(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.DeleteConnection(r.Context(), actor, r.PathValue("id")); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) listMCPServers(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListMCPServers(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) createMCPServer(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.MCPServerInput
	if err := decode(r, &in); err != nil {
		return err
	}
	m, err := s.app.CreateMCPServer(r.Context(), actor, in)
	if err != nil {
		return err
	}
	return created(w, m)
}

func (s *Server) updateMCPServer(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.MCPServerInput
	if err := decode(r, &in); err != nil {
		return err
	}
	m, err := s.app.UpdateMCPServer(r.Context(), actor, r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return ok(w, m)
}

func (s *Server) deleteMCPServer(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.DeleteMCPServer(r.Context(), actor, r.PathValue("id")); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListWebhooks(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.WebhookInput
	if err := decode(r, &in); err != nil {
		return err
	}
	wh, err := s.app.CreateWebhook(r.Context(), actor, in)
	if err != nil {
		return err
	}
	return created(w, wh)
}

func (s *Server) updateWebhook(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.WebhookInput
	if err := decode(r, &in); err != nil {
		return err
	}
	wh, err := s.app.UpdateWebhook(r.Context(), actor, r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return ok(w, wh)
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.DeleteWebhook(r.Context(), actor, r.PathValue("id")); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) testWebhook(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.TestWebhook(r.Context(), actor, r.PathValue("id")); err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
	return nil
}

func (s *Server) rotateWebhook(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	secret, err := s.app.RotateWebhookSecret(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, map[string]string{"secret": secret})
}

func (s *Server) listDeliveries(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListDeliveries(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) getSlack(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	st, err := s.app.GetSlack(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, st)
}

func (s *Server) putSlack(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.SlackInput
	if err := decode(r, &in); err != nil {
		return err
	}
	st, err := s.app.PutSlack(r.Context(), actor, in)
	if err != nil {
		return err
	}
	return ok(w, st)
}

func (s *Server) deleteSlack(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.DeleteSlack(r.Context(), actor); err != nil {
		return err
	}
	return noContent(w)
}

// slackInteraction answers Slack within its three-second window. Problems
// the reviewer should know about go back as an ephemeral message.
func (s *Server) slackInteraction(w http.ResponseWriter, r *http.Request, _ app.Actor) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return &decodeError{"request body could not be read"}
	}
	msg, err := s.app.SlackInteraction(r.Context(), r.PathValue("workspace"), r.Header, body)
	if err != nil {
		return err
	}
	if msg != "" {
		return ok(w, map[string]any{"response_type": "ephemeral", "replace_original": false, "text": msg})
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	ws, err := s.app.GetWorkspace(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, ws)
}

func (s *Server) updateWorkspace(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.WorkspaceInput
	if err := decode(r, &in); err != nil {
		return err
	}
	ws, err := s.app.UpdateWorkspace(r.Context(), actor, in)
	if err != nil {
		return err
	}
	return ok(w, ws)
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListMembers(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) setRole(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in struct {
		Role domain.Role `json:"role"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.app.SetRole(r.Context(), actor, r.PathValue("id"), in.Role); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.RemoveMember(r.Context(), actor, r.PathValue("id")); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) listInvitations(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	items, err := s.app.ListInvitations(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, list(items))
}

func (s *Server) invite(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	var in app.InviteRequest
	if err := decode(r, &in); err != nil {
		return err
	}
	inv, token, err := s.app.Invite(r.Context(), actor, in)
	if err != nil {
		return err
	}
	link := ""
	if s.cfg.PublicURL != "" {
		link = s.cfg.PublicURL + "/invite/" + token
	}
	return created(w, map[string]any{"invitation": inv, "token": token, "link": link})
}

func (s *Server) revokeInvitation(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.RevokeInvitation(r.Context(), actor, r.PathValue("id")); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) ledger(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	q := r.URL.Query()
	before, err := intParam(r, "before", 0)
	if err != nil {
		return err
	}
	limit, err := intParam(r, "limit", 100)
	if err != nil {
		return err
	}
	page, err := s.app.Ledger(r.Context(), actor, store.LedgerFilter{Kind: q.Get("kind"), ActionID: q.Get("action"), BeforeSeq: int64(before), Limit: limit})
	if err != nil {
		return err
	}
	return ok(w, page)
}

func (s *Server) verifyLedger(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	rep, err := s.app.VerifyLedger(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, rep)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	st, err := s.app.Status(r.Context(), actor)
	if err != nil {
		return err
	}
	return ok(w, st)
}

func (s *Server) rotateSigningKey(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := s.app.RotateSigningKey(r.Context(), actor); err != nil {
		return err
	}
	return s.status(w, r, actor)
}
