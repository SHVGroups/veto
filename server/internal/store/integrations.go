package store

import (
	"context"
	"encoding/json"
	"time"
)

// Connection is an HTTP API whose credential Veto holds on the agent's
// behalf. Agents call it through Veto and never see the secret.
type Connection struct {
	ID           string     `json:"id"`
	WorkspaceID  string     `json:"-"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	BaseURL      string     `json:"base_url"`
	AuthScheme   string     `json:"auth_scheme"`
	HeaderName   string     `json:"header_name"`
	SecretEnc    []byte     `json:"-"`
	HasSecret    bool       `json:"has_secret"`
	AllowMethods []string   `json:"allow_methods"`
	AllowPrivate bool       `json:"allow_private"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	RotatedAt    *time.Time `json:"rotated_at"`
	DisabledAt   *time.Time `json:"disabled_at"`
}

const connectionColumns = `id, workspace_id, name, description, base_url, auth_scheme, header_name, secret_enc,
	allow_methods, allow_private, created_at, updated_at, rotated_at, disabled_at`

func scanConnection(row interface{ Scan(...any) error }, c *Connection) error {
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Description, &c.BaseURL, &c.AuthScheme, &c.HeaderName,
		&c.SecretEnc, &c.AllowMethods, &c.AllowPrivate, &c.CreatedAt, &c.UpdatedAt, &c.RotatedAt, &c.DisabledAt)
	c.HasSecret = len(c.SecretEnc) > 0
	return err
}

// CreateConnection inserts a connection.
func (q *Queries) CreateConnection(ctx context.Context, c Connection) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO connections (id, workspace_id, name, description, base_url, auth_scheme, header_name, secret_enc,
		                         allow_methods, allow_private)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		c.ID, c.WorkspaceID, c.Name, c.Description, c.BaseURL, c.AuthScheme, c.HeaderName, c.SecretEnc,
		c.AllowMethods, c.AllowPrivate)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// UpdateConnection replaces a connection's settings. A nil secret keeps the
// current one.
func (q *Queries) UpdateConnection(ctx context.Context, c Connection, newSecret []byte) error {
	tag, err := q.db.Exec(ctx, `
		UPDATE connections SET description = $3, base_url = $4, auth_scheme = $5, header_name = $6,
		       allow_methods = $7, allow_private = $8, disabled_at = $9, updated_at = now(),
		       secret_enc = CASE WHEN $10::bytea IS NULL THEN secret_enc ELSE $10 END,
		       rotated_at = CASE WHEN $10::bytea IS NULL THEN rotated_at ELSE now() END
		WHERE workspace_id = $1 AND id = $2`,
		c.WorkspaceID, c.ID, c.Description, c.BaseURL, c.AuthScheme, c.HeaderName, c.AllowMethods,
		c.AllowPrivate, c.DisabledAt, newSecret)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteConnection removes a connection.
func (q *Queries) DeleteConnection(ctx context.Context, workspaceID, id string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM connections WHERE workspace_id = $1 AND id = $2`, workspaceID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// GetConnection loads a connection by id.
func (q *Queries) GetConnection(ctx context.Context, workspaceID, id string) (Connection, error) {
	var c Connection
	err := scanConnection(q.db.QueryRow(ctx, `SELECT `+connectionColumns+` FROM connections WHERE workspace_id = $1 AND id = $2`, workspaceID, id), &c)
	return c, notFound(err)
}

// GetConnectionByName loads a connection by name.
func (q *Queries) GetConnectionByName(ctx context.Context, workspaceID, name string) (Connection, error) {
	var c Connection
	err := scanConnection(q.db.QueryRow(ctx, `SELECT `+connectionColumns+` FROM connections WHERE workspace_id = $1 AND name = $2`, workspaceID, name), &c)
	return c, notFound(err)
}

// ListConnections returns a workspace's connections by name.
func (q *Queries) ListConnections(ctx context.Context, workspaceID string) ([]Connection, error) {
	rows, err := q.db.Query(ctx, `SELECT `+connectionColumns+` FROM connections WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		var c Connection
		if err := scanConnection(rows, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AllConnectionSecrets returns every sealed connection secret, for master
// key rotation.
func (q *Queries) AllConnectionSecrets(ctx context.Context) ([]Connection, error) {
	rows, err := q.db.Query(ctx, `SELECT `+connectionColumns+` FROM connections WHERE secret_enc IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		var c Connection
		if err := scanConnection(rows, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateConnectionSecret replaces a sealed secret without touching rotated_at.
func (q *Queries) UpdateConnectionSecret(ctx context.Context, id string, sealed []byte) error {
	_, err := q.db.Exec(ctx, `UPDATE connections SET secret_enc = $2 WHERE id = $1`, id, sealed)
	return err
}

// MCPServer is an upstream MCP server proxied through Veto.
type MCPServer struct {
	ID           string     `json:"id"`
	WorkspaceID  string     `json:"-"`
	Name         string     `json:"name"`
	UpstreamURL  string     `json:"upstream_url"`
	ConnectionID *string    `json:"connection_id"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DisabledAt   *time.Time `json:"disabled_at"`
}

const mcpColumns = `id, workspace_id, name, upstream_url, connection_id, created_at, updated_at, disabled_at`

func scanMCP(row interface{ Scan(...any) error }, m *MCPServer) error {
	return row.Scan(&m.ID, &m.WorkspaceID, &m.Name, &m.UpstreamURL, &m.ConnectionID, &m.CreatedAt, &m.UpdatedAt, &m.DisabledAt)
}

// CreateMCPServer inserts an MCP server.
func (q *Queries) CreateMCPServer(ctx context.Context, m MCPServer) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO mcp_servers (id, workspace_id, name, upstream_url, connection_id) VALUES ($1, $2, $3, $4, $5)`,
		m.ID, m.WorkspaceID, m.Name, m.UpstreamURL, m.ConnectionID)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// UpdateMCPServer replaces an MCP server's settings.
func (q *Queries) UpdateMCPServer(ctx context.Context, m MCPServer) error {
	tag, err := q.db.Exec(ctx, `
		UPDATE mcp_servers SET upstream_url = $3, connection_id = $4, disabled_at = $5, updated_at = now()
		WHERE workspace_id = $1 AND id = $2`, m.WorkspaceID, m.ID, m.UpstreamURL, m.ConnectionID, m.DisabledAt)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteMCPServer removes an MCP server.
func (q *Queries) DeleteMCPServer(ctx context.Context, workspaceID, id string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM mcp_servers WHERE workspace_id = $1 AND id = $2`, workspaceID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// GetMCPServer loads an MCP server by id.
func (q *Queries) GetMCPServer(ctx context.Context, workspaceID, id string) (MCPServer, error) {
	var m MCPServer
	err := scanMCP(q.db.QueryRow(ctx, `SELECT `+mcpColumns+` FROM mcp_servers WHERE workspace_id = $1 AND id = $2`, workspaceID, id), &m)
	return m, notFound(err)
}

// GetMCPServerByName loads an MCP server by name.
func (q *Queries) GetMCPServerByName(ctx context.Context, workspaceID, name string) (MCPServer, error) {
	var m MCPServer
	err := scanMCP(q.db.QueryRow(ctx, `SELECT `+mcpColumns+` FROM mcp_servers WHERE workspace_id = $1 AND name = $2`, workspaceID, name), &m)
	return m, notFound(err)
}

// ListMCPServers returns a workspace's MCP servers by name.
func (q *Queries) ListMCPServers(ctx context.Context, workspaceID string) ([]MCPServer, error) {
	rows, err := q.db.Query(ctx, `SELECT `+mcpColumns+` FROM mcp_servers WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MCPServer
	for rows.Next() {
		var m MCPServer
		if err := scanMCP(rows, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Webhook is an HTTPS endpoint that receives signed event notifications.
type Webhook struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"-"`
	URL         string     `json:"url"`
	Description string     `json:"description"`
	Events      []string   `json:"events"`
	SecretEnc   []byte     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	DisabledAt  *time.Time `json:"disabled_at"`
}

const webhookColumns = `id, workspace_id, url, description, events, secret_enc, created_at, disabled_at`

func scanWebhook(row interface{ Scan(...any) error }, w *Webhook) error {
	return row.Scan(&w.ID, &w.WorkspaceID, &w.URL, &w.Description, &w.Events, &w.SecretEnc, &w.CreatedAt, &w.DisabledAt)
}

// CreateWebhook inserts a webhook.
func (q *Queries) CreateWebhook(ctx context.Context, w Webhook) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO webhooks (id, workspace_id, url, description, events, secret_enc) VALUES ($1, $2, $3, $4, $5, $6)`,
		w.ID, w.WorkspaceID, w.URL, w.Description, w.Events, w.SecretEnc)
	return err
}

// UpdateWebhook replaces a webhook's settings.
func (q *Queries) UpdateWebhook(ctx context.Context, w Webhook) error {
	tag, err := q.db.Exec(ctx, `
		UPDATE webhooks SET url = $3, description = $4, events = $5, disabled_at = $6
		WHERE workspace_id = $1 AND id = $2`, w.WorkspaceID, w.ID, w.URL, w.Description, w.Events, w.DisabledAt)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteWebhook removes a webhook and its delivery history.
func (q *Queries) DeleteWebhook(ctx context.Context, workspaceID, id string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM webhooks WHERE workspace_id = $1 AND id = $2`, workspaceID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// GetWebhook loads a webhook.
func (q *Queries) GetWebhook(ctx context.Context, workspaceID, id string) (Webhook, error) {
	var w Webhook
	err := scanWebhook(q.db.QueryRow(ctx, `SELECT `+webhookColumns+` FROM webhooks WHERE workspace_id = $1 AND id = $2`, workspaceID, id), &w)
	return w, notFound(err)
}

// ListWebhooks returns a workspace's webhooks.
func (q *Queries) ListWebhooks(ctx context.Context, workspaceID string) ([]Webhook, error) {
	rows, err := q.db.Query(ctx, `SELECT `+webhookColumns+` FROM webhooks WHERE workspace_id = $1 ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		var w Webhook
		if err := scanWebhook(rows, &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// AllWebhookSecrets returns every webhook, for master key rotation.
func (q *Queries) AllWebhookSecrets(ctx context.Context) ([]Webhook, error) {
	rows, err := q.db.Query(ctx, `SELECT `+webhookColumns+` FROM webhooks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		var w Webhook
		if err := scanWebhook(rows, &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// UpdateWebhookSecret replaces a webhook's sealed secret.
func (q *Queries) UpdateWebhookSecret(ctx context.Context, id string, sealed []byte) error {
	_, err := q.db.Exec(ctx, `UPDATE webhooks SET secret_enc = $2 WHERE id = $1`, id, sealed)
	return err
}

// Delivery is one attempt to notify a webhook of an event.
type Delivery struct {
	ID             string          `json:"id"`
	WebhookID      string          `json:"webhook_id"`
	WorkspaceID    string          `json:"-"`
	Event          string          `json:"event"`
	Payload        json.RawMessage `json:"payload"`
	Status         string          `json:"status"`
	Attempts       int             `json:"attempts"`
	ResponseStatus *int            `json:"response_status"`
	LastError      string          `json:"last_error"`
	NextAttemptAt  time.Time       `json:"next_attempt_at"`
	CreatedAt      time.Time       `json:"created_at"`
	DeliveredAt    *time.Time      `json:"delivered_at"`
}

// EnqueueDeliveries queues an event for every enabled webhook subscribed to
// it. Webhooks subscribed to "*" receive everything.
func (q *Queries) EnqueueDeliveries(ctx context.Context, workspaceID, event string, payload []byte, newID func() string) (int, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id FROM webhooks
		WHERE workspace_id = $1 AND disabled_at IS NULL AND ($2 = ANY(events) OR '*' = ANY(events))`, workspaceID, event)
	if err != nil {
		return 0, err
	}
	var hooks []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		hooks = append(hooks, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, h := range hooks {
		if _, err := q.db.Exec(ctx, `
			INSERT INTO webhook_deliveries (id, webhook_id, workspace_id, event, payload, status)
			VALUES ($1, $2, $3, $4, $5, 'pending')`, newID(), h, workspaceID, event, payload); err != nil {
			return 0, err
		}
	}
	return len(hooks), nil
}

// DueDelivery is a pending delivery with what is needed to send it.
type DueDelivery struct {
	Delivery
	URL       string
	SecretEnc []byte
	Disabled  bool
}

// LeaseDueDeliveries claims up to limit due deliveries by pushing their next
// attempt time out by lease, and returns them. A replica that crashes
// mid-send leaves its deliveries to be retried when the lease runs out; rows
// claimed by another replica are skipped.
func (q *Queries) LeaseDueDeliveries(ctx context.Context, limit int, lease time.Duration) ([]DueDelivery, error) {
	rows, err := q.db.Query(ctx, `
		WITH due AS (
		    SELECT id FROM webhook_deliveries
		    WHERE status = 'pending' AND next_attempt_at <= now()
		    ORDER BY next_attempt_at
		    LIMIT $1
		    FOR UPDATE SKIP LOCKED
		)
		UPDATE webhook_deliveries d SET next_attempt_at = now() + make_interval(secs => $2)
		FROM due, webhooks w
		WHERE d.id = due.id AND w.id = d.webhook_id
		RETURNING d.id, d.webhook_id, d.workspace_id, d.event, d.payload, d.status, d.attempts, d.response_status,
		          d.last_error, d.next_attempt_at, d.created_at, d.delivered_at, w.url, w.secret_enc,
		          w.disabled_at IS NOT NULL`, limit, lease.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DueDelivery
	for rows.Next() {
		var d DueDelivery
		var payload []byte
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.WorkspaceID, &d.Event, &payload, &d.Status, &d.Attempts, &d.ResponseStatus,
			&d.LastError, &d.NextAttemptAt, &d.CreatedAt, &d.DeliveredAt, &d.URL, &d.SecretEnc, &d.Disabled); err != nil {
			return nil, err
		}
		d.Payload = payload
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateDelivery records the result of an attempt.
func (q *Queries) UpdateDelivery(ctx context.Context, d Delivery) error {
	_, err := q.db.Exec(ctx, `
		UPDATE webhook_deliveries SET status = $2, attempts = $3, response_status = $4, last_error = $5,
		       next_attempt_at = $6, delivered_at = $7
		WHERE id = $1`, d.ID, d.Status, d.Attempts, d.ResponseStatus, d.LastError, d.NextAttemptAt, d.DeliveredAt)
	return err
}

// ListDeliveries returns a webhook's recent deliveries, newest first.
func (q *Queries) ListDeliveries(ctx context.Context, workspaceID, webhookID string, limit int) ([]Delivery, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, webhook_id, workspace_id, event, payload, status, attempts, response_status, last_error,
		       next_attempt_at, created_at, delivered_at
		FROM webhook_deliveries WHERE workspace_id = $1 AND webhook_id = $2
		ORDER BY created_at DESC LIMIT $3`, workspaceID, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		var d Delivery
		var payload []byte
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.WorkspaceID, &d.Event, &payload, &d.Status, &d.Attempts, &d.ResponseStatus,
			&d.LastError, &d.NextAttemptAt, &d.CreatedAt, &d.DeliveredAt); err != nil {
			return nil, err
		}
		d.Payload = payload
		out = append(out, d)
	}
	return out, rows.Err()
}

// Integration is a third-party integration's configuration.
type Integration struct {
	Kind      string          `json:"kind"`
	Config    json.RawMessage `json:"config"`
	SecretEnc []byte          `json:"-"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// GetIntegration loads an integration's settings.
func (q *Queries) GetIntegration(ctx context.Context, workspaceID, kind string) (Integration, error) {
	var in Integration
	var cfg []byte
	err := q.db.QueryRow(ctx,
		`SELECT kind, config, secret_enc, updated_at FROM integrations WHERE workspace_id = $1 AND kind = $2`, workspaceID, kind,
	).Scan(&in.Kind, &cfg, &in.SecretEnc, &in.UpdatedAt)
	in.Config = cfg
	return in, notFound(err)
}

// PutIntegration creates or replaces an integration. A nil secret keeps the
// stored one.
func (q *Queries) PutIntegration(ctx context.Context, workspaceID, kind string, config []byte, secret []byte) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO integrations (workspace_id, kind, config, secret_enc) VALUES ($1, $2, $3, $4)
		ON CONFLICT (workspace_id, kind) DO UPDATE SET config = excluded.config,
		    secret_enc = coalesce(excluded.secret_enc, integrations.secret_enc), updated_at = now()`,
		workspaceID, kind, config, secret)
	return err
}

// DeleteIntegration removes an integration.
func (q *Queries) DeleteIntegration(ctx context.Context, workspaceID, kind string) error {
	_, err := q.db.Exec(ctx, `DELETE FROM integrations WHERE workspace_id = $1 AND kind = $2`, workspaceID, kind)
	return err
}

// AllIntegrationSecrets returns every integration with a sealed secret, for
// master key rotation.
func (q *Queries) AllIntegrationSecrets(ctx context.Context) ([]struct {
	WorkspaceID string
	Integration
}, error) {
	rows, err := q.db.Query(ctx, `SELECT workspace_id, kind, config, secret_enc, updated_at FROM integrations WHERE secret_enc IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		WorkspaceID string
		Integration
	}
	for rows.Next() {
		var r struct {
			WorkspaceID string
			Integration
		}
		var cfg []byte
		if err := rows.Scan(&r.WorkspaceID, &r.Kind, &cfg, &r.SecretEnc, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Config = cfg
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateIntegrationSecret replaces an integration's sealed secret.
func (q *Queries) UpdateIntegrationSecret(ctx context.Context, workspaceID, kind string, sealed []byte) error {
	_, err := q.db.Exec(ctx, `UPDATE integrations SET secret_enc = $3 WHERE workspace_id = $1 AND kind = $2`, workspaceID, kind, sealed)
	return err
}

// SaveSlackMessage remembers the Slack message posted for a held action.
func (q *Queries) SaveSlackMessage(ctx context.Context, workspaceID, actionID, channel, ts string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO slack_messages (action_id, workspace_id, channel, ts) VALUES ($1, $2, $3, $4)
		ON CONFLICT (action_id) DO UPDATE SET channel = excluded.channel, ts = excluded.ts`, actionID, workspaceID, channel, ts)
	return err
}

// GetSlackMessage returns the Slack message posted for an action.
func (q *Queries) GetSlackMessage(ctx context.Context, workspaceID, actionID string) (channel, ts string, err error) {
	err = q.db.QueryRow(ctx, `SELECT channel, ts FROM slack_messages WHERE workspace_id = $1 AND action_id = $2`,
		workspaceID, actionID).Scan(&channel, &ts)
	return channel, ts, notFound(err)
}

// EnqueueDirectDelivery queues an event for one webhook regardless of its
// subscriptions. It is used for test deliveries.
func (q *Queries) EnqueueDirectDelivery(ctx context.Context, workspaceID, webhookID, event string, payload []byte, deliveryID string) error {
	_, err := q.db.Exec(ctx, `
		INSERT INTO webhook_deliveries (id, webhook_id, workspace_id, event, payload, status)
		VALUES ($1, $2, $3, $4, $5, 'pending')`, deliveryID, webhookID, workspaceID, event, payload)
	return err
}
