package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SlackAPI is Slack's Web API base URL.
const SlackAPI = "https://slack.com/api"

// SlackSettings is the non-secret part of a workspace's Slack integration.
type SlackSettings struct {
	// Channel is the id of the channel held actions are posted to.
	Channel string `json:"channel"`
	// MentionHeld adds an @here mention to messages about held actions.
	MentionHeld bool `json:"mention_held"`
}

// SlackSecrets are sealed in the vault.
type SlackSecrets struct {
	BotToken      string `json:"bot_token"`
	SigningSecret string `json:"signing_secret"`
}

// Slack talks to Slack's Web API.
type Slack struct {
	http    *http.Client
	baseURL string
}

// NewSlack returns a client. baseURL is SlackAPI except in tests.
func NewSlack(c *http.Client, baseURL string) *Slack {
	return &Slack{http: c, baseURL: strings.TrimRight(baseURL, "/")}
}

// SlackOption is one button on a held-action message.
type SlackOption struct {
	Key         string
	Label       string
	Changes     []string
	Recommended bool
}

// HeldMessage describes a held action for Slack.
type HeldMessage struct {
	ActionID  string
	AgentName string
	Tool      string
	Principal string
	Request   string
	Reasons   []string
	Options   []SlackOption
	ReviewURL string
	Deadline  time.Time
	Mention   bool
}

// Button action ids. The value of every button is the Veto action id.
const (
	SlackActionPrefix = "veto_"
	slackOpenAction   = "veto_open"
)

func (m HeldMessage) blocks(resolution string) []map[string]any {
	headline := fmt.Sprintf("*%s* wants to call `%s`", mrkdwnEscape(m.AgentName), mrkdwnEscape(m.Tool))
	if m.Principal != "" {
		headline += " for " + mrkdwnEscape(m.Principal)
	}
	if m.Mention && resolution == "" {
		headline = "<!here> " + headline
	}
	blocks := []map[string]any{
		{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": headline}},
	}
	if m.Request != "" {
		blocks = append(blocks, map[string]any{"type": "section", "text": map[string]any{
			"type": "mrkdwn", "text": "> " + mrkdwnEscape(truncateRunes(m.Request, 600)),
		}})
	}
	if len(m.Reasons) > 0 {
		blocks = append(blocks, map[string]any{"type": "context", "elements": []map[string]any{
			{"type": "mrkdwn", "text": "Held because: " + mrkdwnEscape(strings.Join(m.Reasons, " · "))},
		}})
	}
	for _, o := range m.Options {
		if len(o.Changes) == 0 {
			continue
		}
		blocks = append(blocks, map[string]any{"type": "context", "elements": []map[string]any{
			{"type": "mrkdwn", "text": "*" + mrkdwnEscape(o.Label) + "*: " + mrkdwnEscape(strings.Join(o.Changes, "; "))},
		}})
	}
	if resolution != "" {
		blocks = append(blocks, map[string]any{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": resolution}})
		if m.ReviewURL != "" {
			blocks = append(blocks, map[string]any{"type": "actions", "elements": []map[string]any{
				{"type": "button", "action_id": slackOpenAction, "text": plain("Open in Veto"), "url": m.ReviewURL},
			}})
		}
		return blocks
	}
	var buttons []map[string]any
	for _, o := range m.Options {
		b := map[string]any{
			"type": "button", "action_id": SlackActionPrefix + o.Key, "value": m.ActionID,
			"text": plain(truncateRunes(o.Label, 70)),
		}
		switch {
		case o.Key == "stop":
			b["style"] = "danger"
		case o.Recommended:
			b["style"] = "primary"
		}
		buttons = append(buttons, b)
	}
	if m.ReviewURL != "" {
		buttons = append(buttons, map[string]any{"type": "button", "action_id": slackOpenAction, "text": plain("Open in Veto"), "url": m.ReviewURL})
	}
	blocks = append(blocks, map[string]any{"type": "actions", "elements": buttons})
	if !m.Deadline.IsZero() {
		blocks = append(blocks, map[string]any{"type": "context", "elements": []map[string]any{
			{"type": "mrkdwn", "text": fmt.Sprintf("Stops automatically <!date^%d^{date_short_pretty} at {time}|%s> if nobody decides.",
				m.Deadline.Unix(), m.Deadline.UTC().Format(time.RFC1123))},
		}})
	}
	return blocks
}

func plain(s string) map[string]any { return map[string]any{"type": "plain_text", "text": s} }

// PostHeld posts a held action and returns the message timestamp, which
// identifies it for later updates.
func (s *Slack) PostHeld(ctx context.Context, token, channel string, m HeldMessage) (string, error) {
	var resp struct {
		TS string `json:"ts"`
	}
	err := s.call(ctx, token, "chat.postMessage", map[string]any{
		"channel": channel,
		"text":    fmt.Sprintf("%s wants to call %s", m.AgentName, m.Tool),
		"blocks":  m.blocks(""),
	}, &resp)
	return resp.TS, err
}

// UpdateResolved replaces the buttons of a held-action message with how it
// was resolved.
func (s *Slack) UpdateResolved(ctx context.Context, token, channel, ts string, m HeldMessage, resolution string) error {
	return s.call(ctx, token, "chat.update", map[string]any{
		"channel": channel,
		"ts":      ts,
		"text":    resolution,
		"blocks":  m.blocks(resolution),
	}, nil)
}

// UserEmail returns the email address on a Slack user's profile. The
// integration needs the users:read.email scope.
func (s *Slack) UserEmail(ctx context.Context, token, userID string) (string, error) {
	var resp struct {
		User struct {
			Profile struct {
				Email string `json:"email"`
			} `json:"profile"`
		} `json:"user"`
	}
	if err := s.callGet(ctx, token, "users.info", url.Values{"user": {userID}}, &resp); err != nil {
		return "", err
	}
	if resp.User.Profile.Email == "" {
		return "", errors.New("notify: Slack user has no email; add the users:read.email scope")
	}
	return resp.User.Profile.Email, nil
}

// AuthTest checks a bot token and returns the team name.
func (s *Slack) AuthTest(ctx context.Context, token string) (string, error) {
	var resp struct {
		Team string `json:"team"`
	}
	err := s.call(ctx, token, "auth.test", map[string]any{}, &resp)
	return resp.Team, err
}

// Reply posts an ephemeral message through an interaction's response URL.
func (s *Slack) Reply(ctx context.Context, responseURL, text string) error {
	body, _ := json.Marshal(map[string]any{"response_type": "ephemeral", "replace_original": false, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, responseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *Slack) call(ctx context.Context, token, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)
	return s.do(req, method, out)
}

func (s *Slack) callGet(ctx context.Context, token, method string, query url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/"+method+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return s.do(req, method, out)
}

func (s *Slack) do(req *http.Request, method string, out any) error {
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("notify: slack %s: %w", method, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("notify: slack %s: unexpected response (HTTP %d)", method, resp.StatusCode)
	}
	if !envelope.OK {
		return fmt.Errorf("notify: slack %s: %s", method, envelope.Error)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// VerifySlackRequest checks the signature Slack puts on interaction
// requests, rejecting anything older than five minutes.
func VerifySlackRequest(signingSecret string, h http.Header, body []byte, now time.Time) error {
	ts := h.Get("X-Slack-Request-Timestamp")
	sig := h.Get("X-Slack-Signature")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || sig == "" {
		return errors.New("notify: missing Slack signature")
	}
	if d := now.Sub(time.Unix(sec, 0)); d > 5*time.Minute || d < -5*time.Minute {
		return errors.New("notify: Slack request timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte("v0:" + ts + ":"))
	mac.Write(body)
	want := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return errors.New("notify: Slack signature does not match")
	}
	return nil
}

// Interaction is a button press on a Veto message.
type Interaction struct {
	UserID      string
	TeamID      string
	ActionID    string
	Option      string
	ResponseURL string
	Channel     string
	MessageTS   string
}

// ParseInteraction reads a block_actions payload. ok is false for payloads
// that are not a Veto decision button, such as the "Open in Veto" link.
func ParseInteraction(form url.Values) (Interaction, bool, error) {
	var p struct {
		Type string `json:"type"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		Team struct {
			ID string `json:"id"`
		} `json:"team"`
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
		Container struct {
			MessageTS string `json:"message_ts"`
		} `json:"container"`
		ResponseURL string `json:"response_url"`
		Actions     []struct {
			ActionID string `json:"action_id"`
			Value    string `json:"value"`
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(form.Get("payload")), &p); err != nil {
		return Interaction{}, false, fmt.Errorf("notify: invalid Slack payload: %w", err)
	}
	if p.Type != "block_actions" || len(p.Actions) == 0 {
		return Interaction{}, false, nil
	}
	a := p.Actions[0]
	if !strings.HasPrefix(a.ActionID, SlackActionPrefix) || a.ActionID == slackOpenAction {
		return Interaction{}, false, nil
	}
	return Interaction{
		UserID:      p.User.ID,
		TeamID:      p.Team.ID,
		ActionID:    a.Value,
		Option:      strings.TrimPrefix(a.ActionID, SlackActionPrefix),
		ResponseURL: p.ResponseURL,
		Channel:     p.Channel.ID,
		MessageTS:   p.Container.MessageTS,
	}, true, nil
}

func mrkdwnEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
