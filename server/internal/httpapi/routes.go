package httpapi

import (
	"net/http"
)

// routes registers every endpoint. The table is the API's index; keep it
// in step with api/openapi.yaml.
func (s *Server) routes(route func(pattern string, level access, h handlerFunc)) {
	// First run and authentication.
	route("GET /v1/setup", public, s.getSetup)
	route("POST /v1/setup", public, s.postSetup)
	route("POST /v1/auth/login", public, s.login)
	route("POST /v1/auth/logout", session, s.logout)
	route("GET /v1/auth/me", session, s.me)
	route("PATCH /v1/auth/me", session, s.updateMe)
	route("POST /v1/auth/password", session, s.changePassword)
	route("GET /v1/invitations/token/{token}", public, s.lookupInvitation)
	route("POST /v1/invitations/accept", public, s.acceptInvitation)

	// Agent API.
	route("POST /v1/actions", agent, s.submitAction)
	route("GET /v1/actions/{id}", anyAuth, s.getActionResult)
	route("POST /v1/actions/{id}/outcome", anyAuth, s.reportOutcome)
	route("POST /v1/actions/{id}/execute", agent, s.executeAction)
	route("POST /v1/grants/verify", apiKey, s.verifyGrant)
	route("GET /v1/grants/keys", anyAuth, s.grantKeys)

	// MCP proxy.
	route("POST /mcp/{server}", agent, s.mcpProxy)
	route("GET /mcp/{server}", agent, s.mcpProxy)
	route("DELETE /mcp/{server}", agent, s.mcpProxy)

	// Console: activity.
	route("GET /v1/overview", console, s.overview)
	route("GET /v1/calibration", console, s.calibration)
	route("GET /v1/actions", console, s.listActions)
	route("GET /v1/actions/{id}/detail", console, s.actionDetail)
	route("POST /v1/actions/{id}/review", console, s.reviewAction)
	route("GET /v1/audits", console, s.listAudits)
	route("POST /v1/audits/{id}", console, s.recordAudit)
	route("GET /v1/events", console, s.events)
	route("GET /v1/meta", console, s.meta)

	// Console: agents and keys.
	route("GET /v1/agents", console, s.listAgents)
	route("POST /v1/agents", console, s.createAgent)
	route("GET /v1/agents/{id}", console, s.getAgent)
	route("PATCH /v1/agents/{id}", console, s.updateAgent)
	route("POST /v1/agents/{id}/keys", console, s.issueAgentKey)
	route("GET /v1/keys", console, s.listKeys)
	route("POST /v1/keys", session, s.issueAdminKey)
	route("DELETE /v1/keys/{id}", console, s.revokeKey)

	// Console: tools, policies, packs.
	route("GET /v1/tools", console, s.listTools)
	route("GET /v1/tools/{name...}", console, s.getTool)
	route("PUT /v1/tools/{name...}", console, s.saveTool)
	route("DELETE /v1/tools/{name...}", console, s.deleteTool)
	route("POST /v1/safer/preview", console, s.previewSafer)
	route("GET /v1/policies", console, s.listPolicies)
	route("GET /v1/policies/active", console, s.activePolicy)
	route("GET /v1/policies/{version}", console, s.getPolicy)
	route("POST /v1/policies", console, s.savePolicy)
	route("POST /v1/policies/check", console, s.checkPolicy)
	route("POST /v1/policies/{version}/activate", console, s.activatePolicy)
	route("GET /v1/packs", console, s.listPacks)
	route("GET /v1/packs/{name}/versions", console, s.packVersions)
	route("PUT /v1/packs/{name}", console, s.savePack)
	route("DELETE /v1/packs/{name}", console, s.resetPack)
	route("POST /v1/simulate", console, s.simulate)

	// Console: connections and integrations.
	route("GET /v1/connections", console, s.listConnections)
	route("POST /v1/connections", console, s.createConnection)
	route("PATCH /v1/connections/{id}", console, s.updateConnection)
	route("DELETE /v1/connections/{id}", console, s.deleteConnection)
	route("GET /v1/mcp-servers", console, s.listMCPServers)
	route("POST /v1/mcp-servers", console, s.createMCPServer)
	route("PATCH /v1/mcp-servers/{id}", console, s.updateMCPServer)
	route("DELETE /v1/mcp-servers/{id}", console, s.deleteMCPServer)
	route("GET /v1/webhooks", console, s.listWebhooks)
	route("POST /v1/webhooks", console, s.createWebhook)
	route("PATCH /v1/webhooks/{id}", console, s.updateWebhook)
	route("DELETE /v1/webhooks/{id}", console, s.deleteWebhook)
	route("POST /v1/webhooks/{id}/test", console, s.testWebhook)
	route("POST /v1/webhooks/{id}/rotate", console, s.rotateWebhook)
	route("GET /v1/webhooks/{id}/deliveries", console, s.listDeliveries)
	route("GET /v1/integrations/slack", console, s.getSlack)
	route("PUT /v1/integrations/slack", console, s.putSlack)
	route("DELETE /v1/integrations/slack", console, s.deleteSlack)
	route("POST /v1/integrations/slack/interactions/{workspace}", public, s.slackInteraction)

	// Console: workspace, members, ledger.
	route("GET /v1/workspace", console, s.getWorkspace)
	route("PATCH /v1/workspace", console, s.updateWorkspace)
	route("GET /v1/members", console, s.listMembers)
	route("PATCH /v1/members/{id}", console, s.setRole)
	route("DELETE /v1/members/{id}", console, s.removeMember)
	route("GET /v1/invitations", console, s.listInvitations)
	route("POST /v1/invitations", console, s.invite)
	route("DELETE /v1/invitations/{id}", console, s.revokeInvitation)
	route("GET /v1/ledger", console, s.ledger)
	route("POST /v1/ledger/verify", console, s.verifyLedger)
	route("GET /v1/status", console, s.status)
	route("POST /v1/signing-keys/rotate", session, s.rotateSigningKey)
}

func ok(w http.ResponseWriter, v any) error {
	writeJSON(w, http.StatusOK, v)
	return nil
}

func created(w http.ResponseWriter, v any) error {
	writeJSON(w, http.StatusCreated, v)
	return nil
}

func noContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}
