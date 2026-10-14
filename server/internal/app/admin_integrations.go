package app

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/shvgroups/veto/server/internal/auth"
	"github.com/shvgroups/veto/server/internal/connector"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/id"
	"github.com/shvgroups/veto/server/internal/ledger"
	"github.com/shvgroups/veto/server/internal/notify"
	"github.com/shvgroups/veto/server/internal/store"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

var httpMethods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"}

// ConnectionInput creates or updates a connection. Secret is write-only;
// leave it nil on update to keep the stored one.
type ConnectionInput struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	BaseURL      string   `json:"base_url"`
	AuthScheme   string   `json:"auth_scheme"`
	HeaderName   string   `json:"header_name"`
	Secret       *string  `json:"secret,omitempty"`
	AllowMethods []string `json:"allow_methods"`
	AllowPrivate bool     `json:"allow_private"`
	Disabled     bool     `json:"disabled"`
}

func (a *App) validateConnection(in *ConnectionInput, creating bool) error {
	if !slugPattern.MatchString(in.Name) {
		return invalidf("connection name must be 1 to 63 lowercase letters, digits, - and _")
	}
	if utf8.RuneCountInString(in.Description) > 500 {
		return invalidf("description must be at most 500 characters")
	}
	if err := connector.ValidateBaseURL(in.BaseURL); err != nil {
		return invalidf("%v", err)
	}
	switch in.AuthScheme {
	case "none":
	case "bearer", "basic":
		if creating && (in.Secret == nil || *in.Secret == "") {
			return invalidf("%s authentication needs a secret", in.AuthScheme)
		}
	case "header":
		if !regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`).MatchString(in.HeaderName) {
			return invalidf("header authentication needs a valid header name")
		}
		if creating && (in.Secret == nil || *in.Secret == "") {
			return invalidf("header authentication needs a secret")
		}
	default:
		return invalidf("auth_scheme must be none, bearer, header or basic")
	}
	if in.Secret != nil && len(*in.Secret) > 8192 {
		return invalidf("secret must be at most 8192 bytes")
	}
	if len(in.AllowMethods) == 0 {
		in.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	}
	for i, m := range in.AllowMethods {
		in.AllowMethods[i] = strings.ToUpper(m)
		if !slices.Contains(httpMethods, in.AllowMethods[i]) {
			return invalidf("method %q is not supported", m)
		}
	}
	if in.AllowPrivate && a.PrivateConnector == nil {
		return invalidf("this server does not allow connections to private addresses (set VETO_CONNECTOR_ALLOW_PRIVATE)")
	}
	return nil
}

// CreateConnection stores a connection and registers its tool, so calls to
// it are classified from the start.
func (a *App) CreateConnection(ctx context.Context, actor Actor, in ConnectionInput) (store.Connection, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return store.Connection{}, err
	}
	if err := a.validateConnection(&in, true); err != nil {
		return store.Connection{}, err
	}
	c := store.Connection{
		ID: id.New(id.Connection), WorkspaceID: actor.WorkspaceID, Name: in.Name, Description: in.Description,
		BaseURL: in.BaseURL, AuthScheme: in.AuthScheme, HeaderName: in.HeaderName,
		AllowMethods: in.AllowMethods, AllowPrivate: in.AllowPrivate,
	}
	if in.Secret != nil && *in.Secret != "" {
		c.SecretEnc = a.Keyring.Seal([]byte(*in.Secret), "connection:"+c.ID)
	}
	toolName := connector.ToolPrefix + c.Name
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.CreateConnection(ctx, c); err != nil {
			return mapStoreErr(err, "a connection with this name")
		}
		if _, err := q.GetTool(ctx, actor.WorkspaceID, toolName); err != nil {
			if err := q.UpsertTool(ctx, domain.Tool{
				WorkspaceID: actor.WorkspaceID, Name: toolName, RiskClass: domain.RiskMedium, Pack: "general",
				Description: "HTTP calls through the " + c.Name + " connection", Safer: json.RawMessage("[]"), FailMode: domain.FailClosed,
			}); err != nil {
				return err
			}
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindConnectionChanged, "", actor, map[string]any{
			"change": "created", "connection_id": c.ID, "name": c.Name, "base_url": c.BaseURL, "auth_scheme": c.AuthScheme,
		})
	})
	c.HasSecret = len(c.SecretEnc) > 0
	return c, err
}

// UpdateConnection changes a connection; a new secret rotates it.
func (a *App) UpdateConnection(ctx context.Context, actor Actor, connID string, in ConnectionInput) (store.Connection, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return store.Connection{}, err
	}
	var out store.Connection
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		c, err := q.GetConnection(ctx, actor.WorkspaceID, connID)
		if err != nil {
			return mapStoreErr(err, "connection")
		}
		in.Name = c.Name
		if err := a.validateConnection(&in, false); err != nil {
			return err
		}
		c.Description, c.BaseURL, c.AuthScheme, c.HeaderName = in.Description, in.BaseURL, in.AuthScheme, in.HeaderName
		c.AllowMethods, c.AllowPrivate = in.AllowMethods, in.AllowPrivate
		c.DisabledAt = nil
		if in.Disabled {
			now := a.now()
			c.DisabledAt = &now
		}
		var sealed []byte
		if in.Secret != nil && *in.Secret != "" {
			sealed = a.Keyring.Seal([]byte(*in.Secret), "connection:"+c.ID)
		}
		if err := q.UpdateConnection(ctx, c, sealed); err != nil {
			return err
		}
		if err := record(ctx, q, actor.WorkspaceID, ledger.KindConnectionChanged, "", actor, map[string]any{
			"change": "updated", "connection_id": c.ID, "base_url": c.BaseURL, "secret_rotated": sealed != nil, "disabled": in.Disabled,
		}); err != nil {
			return err
		}
		out, err = q.GetConnection(ctx, actor.WorkspaceID, connID)
		return err
	})
	return out, err
}

// DeleteConnection removes a connection. Approved calls through it can no
// longer be executed.
func (a *App) DeleteConnection(ctx context.Context, actor Actor, connID string) error {
	if err := require(actor, domain.PermConfigure); err != nil {
		return err
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		c, err := q.GetConnection(ctx, actor.WorkspaceID, connID)
		if err != nil {
			return mapStoreErr(err, "connection")
		}
		if err := q.DeleteConnection(ctx, actor.WorkspaceID, connID); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindConnectionChanged, "", actor, map[string]any{"change": "deleted", "connection_id": connID, "name": c.Name})
	})
}

// MCPServerInput creates or updates an MCP server.
type MCPServerInput struct {
	Name         string  `json:"name"`
	UpstreamURL  string  `json:"upstream_url"`
	ConnectionID *string `json:"connection_id"`
	Disabled     bool    `json:"disabled"`
}

func validateMCP(in MCPServerInput) error {
	if !slugPattern.MatchString(in.Name) {
		return invalidf("server name must be 1 to 63 lowercase letters, digits, - and _")
	}
	u, err := url.Parse(in.UpstreamURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return invalidf("upstream_url must be an absolute http or https URL without credentials")
	}
	return nil
}

// CreateMCPServer registers an upstream MCP server to proxy.
func (a *App) CreateMCPServer(ctx context.Context, actor Actor, in MCPServerInput) (store.MCPServer, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return store.MCPServer{}, err
	}
	if err := validateMCP(in); err != nil {
		return store.MCPServer{}, err
	}
	m := store.MCPServer{ID: id.New(id.MCPServer), WorkspaceID: actor.WorkspaceID, Name: in.Name, UpstreamURL: in.UpstreamURL, ConnectionID: in.ConnectionID}
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if in.ConnectionID != nil {
			if _, err := q.GetConnection(ctx, actor.WorkspaceID, *in.ConnectionID); err != nil {
				return mapStoreErr(err, "connection")
			}
		}
		if err := q.CreateMCPServer(ctx, m); err != nil {
			return mapStoreErr(err, "an MCP server with this name")
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindMCPServerChanged, "", actor, map[string]any{"change": "created", "server_id": m.ID, "name": m.Name, "upstream_url": m.UpstreamURL})
	})
	if err != nil {
		return store.MCPServer{}, err
	}
	return a.Store.GetMCPServer(ctx, actor.WorkspaceID, m.ID)
}

// UpdateMCPServer changes an MCP server.
func (a *App) UpdateMCPServer(ctx context.Context, actor Actor, serverID string, in MCPServerInput) (store.MCPServer, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return store.MCPServer{}, err
	}
	var out store.MCPServer
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		m, err := q.GetMCPServer(ctx, actor.WorkspaceID, serverID)
		if err != nil {
			return mapStoreErr(err, "MCP server")
		}
		in.Name = m.Name
		if err := validateMCP(in); err != nil {
			return err
		}
		if in.ConnectionID != nil {
			if _, err := q.GetConnection(ctx, actor.WorkspaceID, *in.ConnectionID); err != nil {
				return mapStoreErr(err, "connection")
			}
		}
		m.UpstreamURL, m.ConnectionID, m.DisabledAt = in.UpstreamURL, in.ConnectionID, nil
		if in.Disabled {
			now := a.now()
			m.DisabledAt = &now
		}
		if err := q.UpdateMCPServer(ctx, m); err != nil {
			return err
		}
		if err := record(ctx, q, actor.WorkspaceID, ledger.KindMCPServerChanged, "", actor, map[string]any{"change": "updated", "server_id": m.ID, "upstream_url": m.UpstreamURL, "disabled": in.Disabled}); err != nil {
			return err
		}
		out, err = q.GetMCPServer(ctx, actor.WorkspaceID, serverID)
		return err
	})
	return out, err
}

// DeleteMCPServer stops proxying an MCP server.
func (a *App) DeleteMCPServer(ctx context.Context, actor Actor, serverID string) error {
	if err := require(actor, domain.PermConfigure); err != nil {
		return err
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.DeleteMCPServer(ctx, actor.WorkspaceID, serverID); err != nil {
			return mapStoreErr(err, "MCP server")
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindMCPServerChanged, "", actor, map[string]any{"change": "deleted", "server_id": serverID})
	})
}

// WebhookInput creates or updates a webhook.
type WebhookInput struct {
	URL         string   `json:"url"`
	Description string   `json:"description"`
	Events      []string `json:"events"`
	Disabled    bool     `json:"disabled"`
}

func validateWebhook(in *WebhookInput) error {
	u, err := url.Parse(in.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return invalidf("url must be an absolute http or https URL without credentials")
	}
	if utf8.RuneCountInString(in.Description) > 500 {
		return invalidf("description must be at most 500 characters")
	}
	if len(in.Events) == 0 {
		return invalidf("choose at least one event, or * for all")
	}
	for _, e := range in.Events {
		if e != "*" && !slices.Contains(notify.Events, e) {
			return invalidf("unknown event %q", e)
		}
	}
	slices.Sort(in.Events)
	in.Events = slices.Compact(in.Events)
	return nil
}

// CreatedWebhook includes the signing secret, shown once.
type CreatedWebhook struct {
	store.Webhook
	Secret string `json:"secret"`
}

// CreateWebhook registers an endpoint and returns its signing secret.
func (a *App) CreateWebhook(ctx context.Context, actor Actor, in WebhookInput) (*CreatedWebhook, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return nil, err
	}
	if err := validateWebhook(&in); err != nil {
		return nil, err
	}
	secret := auth.NewToken("whsec_")
	w := store.Webhook{ID: id.New(id.Webhook), WorkspaceID: actor.WorkspaceID, URL: in.URL, Description: in.Description, Events: in.Events, CreatedAt: a.now()}
	w.SecretEnc = a.Keyring.Seal([]byte(secret), "webhook:"+w.ID)
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.CreateWebhook(ctx, w); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindWebhookChanged, "", actor, map[string]any{"change": "created", "webhook_id": w.ID, "url": w.URL, "events": w.Events})
	})
	if err != nil {
		return nil, err
	}
	return &CreatedWebhook{Webhook: w, Secret: secret}, nil
}

// UpdateWebhook changes a webhook's URL, events or state.
func (a *App) UpdateWebhook(ctx context.Context, actor Actor, webhookID string, in WebhookInput) (store.Webhook, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return store.Webhook{}, err
	}
	if err := validateWebhook(&in); err != nil {
		return store.Webhook{}, err
	}
	var out store.Webhook
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		w, err := q.GetWebhook(ctx, actor.WorkspaceID, webhookID)
		if err != nil {
			return mapStoreErr(err, "webhook")
		}
		w.URL, w.Description, w.Events, w.DisabledAt = in.URL, in.Description, in.Events, nil
		if in.Disabled {
			now := a.now()
			w.DisabledAt = &now
		}
		if err := q.UpdateWebhook(ctx, w); err != nil {
			return err
		}
		out = w
		return record(ctx, q, actor.WorkspaceID, ledger.KindWebhookChanged, "", actor, map[string]any{"change": "updated", "webhook_id": w.ID, "url": w.URL, "events": w.Events, "disabled": in.Disabled})
	})
	return out, err
}

// RotateWebhookSecret replaces a webhook's signing secret.
func (a *App) RotateWebhookSecret(ctx context.Context, actor Actor, webhookID string) (string, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return "", err
	}
	secret := auth.NewToken("whsec_")
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if _, err := q.GetWebhook(ctx, actor.WorkspaceID, webhookID); err != nil {
			return mapStoreErr(err, "webhook")
		}
		if err := q.UpdateWebhookSecret(ctx, webhookID, a.Keyring.Seal([]byte(secret), "webhook:"+webhookID)); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindWebhookChanged, "", actor, map[string]any{"change": "secret_rotated", "webhook_id": webhookID})
	})
	return secret, err
}

// DeleteWebhook removes a webhook and its delivery history.
func (a *App) DeleteWebhook(ctx context.Context, actor Actor, webhookID string) error {
	if err := require(actor, domain.PermConfigure); err != nil {
		return err
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.DeleteWebhook(ctx, actor.WorkspaceID, webhookID); err != nil {
			return mapStoreErr(err, "webhook")
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindWebhookChanged, "", actor, map[string]any{"change": "deleted", "webhook_id": webhookID})
	})
}

// TestWebhook queues a webhook.test delivery to one endpoint.
func (a *App) TestWebhook(ctx context.Context, actor Actor, webhookID string) error {
	if err := require(actor, domain.PermConfigure); err != nil {
		return err
	}
	w, err := a.Store.GetWebhook(ctx, actor.WorkspaceID, webhookID)
	if err != nil {
		return mapStoreErr(err, "webhook")
	}
	body, _ := json.Marshal(map[string]any{
		"id": id.New(id.Delivery), "event": notify.EventTest, "workspace_id": actor.WorkspaceID,
		"created_at": a.now().UTC(), "data": map[string]any{"message": "Test delivery from Veto", "webhook_id": w.ID},
	})
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		return q.EnqueueDirectDelivery(ctx, actor.WorkspaceID, w.ID, notify.EventTest, body, id.New(id.Delivery))
	})
}

// SlackInput configures the Slack integration. Leave Secrets nil to keep
// the stored credentials.
type SlackInput struct {
	Settings notify.SlackSettings `json:"settings"`
	Secrets  *notify.SlackSecrets `json:"secrets,omitempty"`
}

// SlackStatus describes the Slack integration without its secrets.
type SlackStatus struct {
	Configured     bool                 `json:"configured"`
	Settings       notify.SlackSettings `json:"settings"`
	HasSecrets     bool                 `json:"has_secrets"`
	InteractionURL string               `json:"interaction_url"`
}

// GetSlack returns the Slack integration's settings.
func (a *App) GetSlack(ctx context.Context, actor Actor) (*SlackStatus, error) {
	if err := require(actor, domain.PermView); err != nil {
		return nil, err
	}
	st := &SlackStatus{}
	if a.PublicURL != "" {
		st.InteractionURL = a.PublicURL + "/api/v1/integrations/slack/interactions/" + actor.WorkspaceID
	}
	integ, err := a.Store.GetIntegration(ctx, actor.WorkspaceID, "slack")
	if err != nil {
		return st, nil
	}
	st.Configured, st.HasSecrets = true, len(integ.SecretEnc) > 0
	_ = json.Unmarshal(integ.Config, &st.Settings)
	return st, nil
}

// PutSlack saves the Slack integration, checking new credentials with
// Slack first.
func (a *App) PutSlack(ctx context.Context, actor Actor, in SlackInput) (*SlackStatus, error) {
	if err := require(actor, domain.PermConfigure); err != nil {
		return nil, err
	}
	if !regexp.MustCompile(`^[A-Z0-9]{8,20}$`).MatchString(in.Settings.Channel) {
		return nil, invalidf("channel must be a Slack channel id such as C0123456789")
	}
	var sealed []byte
	if in.Secrets != nil {
		if !strings.HasPrefix(in.Secrets.BotToken, "xoxb-") || in.Secrets.SigningSecret == "" {
			return nil, invalidf("a bot token (xoxb-...) and a signing secret are required")
		}
		if a.Slack != nil {
			if _, err := a.Slack.AuthTest(ctx, in.Secrets.BotToken); err != nil {
				return nil, invalidf("Slack rejected the bot token: %v", err)
			}
		}
		raw, _ := json.Marshal(in.Secrets)
		sealed = a.Keyring.Seal(raw, "integration:"+actor.WorkspaceID+":slack")
	} else if _, err := a.Store.GetIntegration(ctx, actor.WorkspaceID, "slack"); err != nil {
		return nil, invalidf("credentials are required the first time Slack is configured")
	}
	cfg, _ := json.Marshal(in.Settings)
	err := a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.PutIntegration(ctx, actor.WorkspaceID, "slack", cfg, sealed); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindIntegration, "", actor, map[string]any{"integration": "slack", "change": "saved", "channel": in.Settings.Channel, "secrets_changed": sealed != nil})
	})
	if err != nil {
		return nil, err
	}
	return a.GetSlack(ctx, actor)
}

// DeleteSlack removes the Slack integration.
func (a *App) DeleteSlack(ctx context.Context, actor Actor) error {
	if err := require(actor, domain.PermConfigure); err != nil {
		return err
	}
	return a.Store.Tx(ctx, func(q *store.Queries) error {
		if err := q.DeleteIntegration(ctx, actor.WorkspaceID, "slack"); err != nil {
			return err
		}
		return record(ctx, q, actor.WorkspaceID, ledger.KindIntegration, "", actor, map[string]any{"integration": "slack", "change": "deleted"})
	})
}
