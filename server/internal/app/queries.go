package app

import (
	"context"

	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/store"
)

// The read operations below check the view permission and pass through to
// the store. They exist so the HTTP layer never touches the store directly.

// ListActions lists actions with filters and pagination.
func (a *App) ListActions(ctx context.Context, actor Actor, f store.ActionFilter) ([]store.ActionSummary, string, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, "", err
	}
	return a.Store.ListActions(ctx, actor.WorkspaceID, f)
}

// ListAgents lists agents with seven-day activity.
func (a *App) ListAgents(ctx context.Context, actor Actor) ([]store.AgentStats, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.Store.ListAgents(ctx, actor.WorkspaceID)
}

// ListKeys lists API keys, optionally for one agent.
func (a *App) ListKeys(ctx context.Context, actor Actor, agentID string) ([]store.APIKey, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.Store.ListAPIKeys(ctx, actor.WorkspaceID, agentID)
}

// ListTools lists tool profiles with seven-day activity.
func (a *App) ListTools(ctx context.Context, actor Actor) ([]store.ToolStats, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.Store.ListTools(ctx, actor.WorkspaceID)
}

// GetTool returns one tool profile.
func (a *App) GetTool(ctx context.Context, actor Actor, name string) (domain.Tool, error) {
	if err := require(actor, domain.PermView); err != nil {
		return domain.Tool{}, err
	}
	t, err := a.Store.GetTool(ctx, actor.WorkspaceID, name)
	return t, mapStoreErr(err, "tool")
}

// ListPolicyVersions lists policy revisions, newest first.
func (a *App) ListPolicyVersions(ctx context.Context, actor Actor) ([]store.PolicyVersion, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.Store.ListPolicyVersions(ctx, actor.WorkspaceID)
}

// GetPolicyVersion returns one policy revision with its source.
func (a *App) GetPolicyVersion(ctx context.Context, actor Actor, version int) (store.PolicyVersion, error) {
	if err := require(actor, domain.PermView); err != nil {
		return store.PolicyVersion{}, err
	}
	p, err := a.Store.GetPolicyVersion(ctx, actor.WorkspaceID, version)
	return p, mapStoreErr(err, "policy version")
}

// ListPackVersions lists the saved revisions of a pack.
func (a *App) ListPackVersions(ctx context.Context, actor Actor, name string) ([]store.PackVersion, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.Store.ListPackVersions(ctx, actor.WorkspaceID, name)
}

// ListConnections lists connections without their secrets.
func (a *App) ListConnections(ctx context.Context, actor Actor) ([]store.Connection, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.Store.ListConnections(ctx, actor.WorkspaceID)
}

// ListMCPServers lists proxied MCP servers.
func (a *App) ListMCPServers(ctx context.Context, actor Actor) ([]store.MCPServer, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.Store.ListMCPServers(ctx, actor.WorkspaceID)
}

// ListWebhooks lists webhooks.
func (a *App) ListWebhooks(ctx context.Context, actor Actor) ([]store.Webhook, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.Store.ListWebhooks(ctx, actor.WorkspaceID)
}

// ListDeliveries lists a webhook's recent deliveries.
func (a *App) ListDeliveries(ctx context.Context, actor Actor, webhookID string) ([]store.Delivery, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	if _, err := a.Store.GetWebhook(ctx, actor.WorkspaceID, webhookID); err != nil {
		return nil, mapStoreErr(err, "webhook")
	}
	return a.Store.ListDeliveries(ctx, actor.WorkspaceID, webhookID, 100)
}

// ListAudits lists open or reviewed audits.
func (a *App) ListAudits(ctx context.Context, actor Actor, open bool) ([]store.AuditItem, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.Store.ListAudits(ctx, actor.WorkspaceID, open, 200)
}

// ListMembers lists workspace members.
func (a *App) ListMembers(ctx context.Context, actor Actor) ([]store.Member, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	return a.Store.ListMembers(ctx, actor.WorkspaceID)
}

// ListInvitations lists invitations.
func (a *App) ListInvitations(ctx context.Context, actor Actor) ([]store.Invitation, error) {
	if err := require(actor, domain.PermManageMembers); err != nil {
		return nil, err
	}
	return a.Store.ListInvitations(ctx, actor.WorkspaceID)
}

// GetWorkspace returns the workspace with its settings.
func (a *App) GetWorkspace(ctx context.Context, actor Actor) (store.Workspace, error) {
	if err := require(actor, domain.PermView); err != nil {
		return store.Workspace{}, err
	}
	return a.Store.GetWorkspace(ctx, actor.WorkspaceID)
}
