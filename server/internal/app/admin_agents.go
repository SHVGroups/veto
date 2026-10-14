package app

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/shvgroups/veto/server/internal/auth"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/id"
	"github.com/shvgroups/veto/server/internal/ledger"
	"github.com/shvgroups/veto/server/internal/store"
)

var agentNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)

// AgentInput creates or updates an agent.
type AgentInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Vendor      string `json:"vendor"`
	Disabled    *bool  `json:"disabled,omitempty"`
}

func (in *AgentInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if !agentNamePattern.MatchString(in.Name) {
		return invalidf("agent name must be 1 to 64 characters of letters, digits, spaces and . _ -")
	}
	if utf8.RuneCountInString(in.Description) > 500 {
		return invalidf("description must be at most 500 characters")
	}
	if utf8.RuneCountInString(in.Vendor) > 100 {
		return invalidf("vendor must be at most 100 characters")
	}
	return nil
}

// CreateAgent registers an agent and issues its first API key.
func (a *App) CreateAgent(ctx context.Context, actor Actor, in AgentInput) (store.Agent, *IssuedKey, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return store.Agent{}, nil, err
	}
	if err := in.validate(); err != nil {
		return store.Agent{}, nil, err
	}
	ag := store.Agent{ID: id.New(id.Agent), WorkspaceID: actor.WorkspaceID, Name: in.Name, Description: in.Description, Vendor: in.Vendor, CreatedAt: a.now()}
	var key *IssuedKey
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.CreateAgent(ctx, ag); err != nil {
			return mapStoreErr(err, "an agent with this name")
		}
		if err := record(ctx, q, actor.WorkspaceID, ledger.KindAgentCreated, "", actor, map[string]any{
			"agent_id": ag.ID, "name": ag.Name, "vendor": ag.Vendor,
		}); err != nil {
			return err
		}
		var err error
		key, err = a.issueKey(ctx, q, actor, "agent", &ag.ID, "default")
		return err
	})
	return ag, key, err
}

// UpdateAgent changes an agent's details or disables it. Actions from a
// disabled agent are recorded and stopped.
func (a *App) UpdateAgent(ctx context.Context, actor Actor, agentID string, in AgentInput) (store.Agent, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return store.Agent{}, err
	}
	if err := in.validate(); err != nil {
		return store.Agent{}, err
	}
	var ag store.Agent
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		var err error
		ag, err = q.GetAgent(ctx, actor.WorkspaceID, agentID)
		if err != nil {
			return mapStoreErr(err, "agent")
		}
		before := ag
		ag.Name, ag.Description, ag.Vendor = in.Name, in.Description, in.Vendor
		if in.Disabled != nil {
			if *in.Disabled && ag.DisabledAt == nil {
				now := a.now()
				ag.DisabledAt = &now
			} else if !*in.Disabled {
				ag.DisabledAt = nil
			}
		}
		if err := q.UpdateAgent(ctx, ag); err != nil {
			return mapStoreErr(err, "an agent with this name")
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindAgentUpdated, "", actor, map[string]any{
			"agent_id": ag.ID, "name": ag.Name, "previous_name": before.Name, "disabled": ag.DisabledAt != nil,
		})
	})
	return ag, err
}

// IssuedKey is a newly created API key. Secret is shown once.
type IssuedKey struct {
	store.APIKey
	Secret string `json:"secret"`
}

// IssueAgentKey creates another key for an agent, for rotation without
// downtime.
func (a *App) IssueAgentKey(ctx context.Context, actor Actor, agentID, name string) (*IssuedKey, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return nil, err
	}
	var key *IssuedKey
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if _, err := q.GetAgent(ctx, actor.WorkspaceID, agentID); err != nil {
			return mapStoreErr(err, "agent")
		}
		var err error
		key, err = a.issueKey(ctx, q, actor, "agent", &agentID, name)
		return err
	})
	return key, err
}

// IssueAdminKey creates a key for automation and tool servers. Admin keys
// can call the management API with the admin role and verify grants.
func (a *App) IssueAdminKey(ctx context.Context, actor Actor, name string) (*IssuedKey, error) {
	if err := require(actor, domain.PermOwn); err != nil {
		return nil, err
	}
	var key *IssuedKey
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		var err error
		key, err = a.issueKey(ctx, q, actor, "admin", nil, name)
		return err
	})
	return key, err
}

func (a *App) issueKey(ctx context.Context, q *store.Queries, actor Actor, kind string, agentID *string, name string) (*IssuedKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	if utf8.RuneCountInString(name) > 100 {
		return nil, invalidf("key name must be at most 100 characters")
	}
	prefix := auth.PrefixAgentKey
	if kind == "admin" {
		prefix = auth.PrefixAdminKey
	}
	secret := auth.NewToken(prefix)
	k := store.APIKey{
		ID: id.New(id.APIKey), WorkspaceID: actor.WorkspaceID, AgentID: agentID, Name: name, Kind: kind,
		Prefix: auth.DisplayPrefix(secret), CreatedAt: a.now(),
	}
	if actor.Kind == ActorUser {
		uid := actor.ID
		k.CreatedBy = &uid
	}
	if err := q.CreateAPIKey(ctx, k, auth.HashToken(secret)); err != nil {
		return nil, err
	}
	if err := record(ctx, q, actor.WorkspaceID, ledger.KindKeyCreated, "", actor, map[string]any{
		"key_id": k.ID, "kind": kind, "agent_id": agentID, "name": name, "prefix": k.Prefix,
	}); err != nil {
		return nil, err
	}
	return &IssuedKey{APIKey: k, Secret: secret}, nil
}

// RevokeKey stops a key from working immediately.
func (a *App) RevokeKey(ctx context.Context, actor Actor, keyID string) error {
	if err := require(actor, domain.PermConfigure); err != nil {
		return err
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		k, err := q.GetAPIKey(ctx, actor.WorkspaceID, keyID)
		if err != nil {
			return mapStoreErr(err, "key")
		}
		if k.Kind == "admin" && !actor.Can(domain.PermOwn) {
			return forbiddenf("only owners can revoke admin keys")
		}
		if err := q.RevokeAPIKey(ctx, actor.WorkspaceID, keyID); err != nil {
			return mapStoreErr(err, "active key")
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindKeyRevoked, "", actor, map[string]any{"key_id": keyID, "kind": k.Kind, "agent_id": k.AgentID})
	})
}

// AgentDetail is an agent with its keys and recent actions.
type AgentDetail struct {
	store.AgentStats
	Keys   []store.APIKey        `json:"keys"`
	Recent []store.ActionSummary `json:"recent"`
}

// GetAgentDetail returns everything the agent page shows.
func (a *App) GetAgentDetail(ctx context.Context, actor Actor, agentID string) (*AgentDetail, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	agents, err := a.Store.ListAgents(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	var d *AgentDetail
	for _, s := range agents {
		if s.ID == agentID {
			d = &AgentDetail{AgentStats: s}
		}
	}
	if d == nil {
		return nil, notFoundf("agent not found")
	}
	if d.Keys, err = a.Store.ListAPIKeys(ctx, actor.WorkspaceID, agentID); err != nil {
		return nil, err
	}
	if d.Recent, _, err = a.Store.ListActions(ctx, actor.WorkspaceID, store.ActionFilter{AgentID: agentID, Limit: 25}); err != nil {
		return nil, err
	}
	return d, nil
}
