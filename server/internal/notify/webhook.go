// Package notify tells people and systems about events: signed webhooks to
// any HTTPS endpoint, and Slack messages with buttons reviewers can use to
// decide held actions.
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Webhook event names a subscriber can choose.
const (
	EventActionPending  = "action.pending_review"
	EventActionDecided  = "action.decided"
	EventActionExpired  = "action.expired"
	EventOutcome        = "outcome.reported"
	EventAuditSampled   = "audit.sampled"
	EventToolDiscovered = "tool.discovered"
	EventLedgerBreak    = "ledger.verification_failed"
	EventTest           = "webhook.test"
)

// Events lists every event a webhook can subscribe to.
var Events = []string{
	EventActionPending, EventActionDecided, EventActionExpired, EventOutcome,
	EventAuditSampled, EventToolDiscovered, EventLedgerBreak,
}

// SignatureHeader carries the HMAC signature of a delivery.
const SignatureHeader = "Veto-Signature"

// Sign returns the signature header value for a body sent at time ts:
// "t=<unix seconds>,v1=<hex HMAC-SHA256 of "<t>.<body>">".
func Sign(secret []byte, ts time.Time, body []byte) string {
	t := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(t))
	mac.Write([]byte{'.'})
	mac.Write(body)
	return "t=" + t + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a signature header against a body, rejecting timestamps
// older than tolerance. Receivers can use the same algorithm; it is
// documented in docs/guides/webhooks.md.
func Verify(secret []byte, header string, body []byte, now time.Time, tolerance time.Duration) error {
	var ts int64
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts, _ = strconv.ParseInt(v, 10, 64)
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == 0 || len(sigs) == 0 {
		return errors.New("notify: malformed signature header")
	}
	if d := now.Sub(time.Unix(ts, 0)); d > tolerance || d < -tolerance {
		return errors.New("notify: signature timestamp outside tolerance")
	}
	want := Sign(secret, time.Unix(ts, 0), body)
	_, wantSig, _ := strings.Cut(want, ",v1=")
	for _, s := range sigs {
		if hmac.Equal([]byte(s), []byte(wantSig)) {
			return nil
		}
	}
	return errors.New("notify: signature does not match")
}

// DeliveryTimeout bounds one delivery attempt, from connecting to reading
// the response.
const DeliveryTimeout = 20 * time.Second

// Sender posts webhook deliveries.
type Sender struct {
	http *http.Client
	now  func() time.Time
}

// NewSender returns a sender using the given client.
func NewSender(c *http.Client) *Sender { return &Sender{http: c, now: time.Now} }

// Send posts one delivery and returns the response status. Any 2xx status
// is success.
func (s *Sender) Send(ctx context.Context, url, deliveryID, event string, secret, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "veto-webhooks/1")
	req.Header.Set("Veto-Event", event)
	req.Header.Set("Veto-Delivery", deliveryID)
	req.Header.Set(SignatureHeader, Sign(secret, s.now(), body))
	resp, err := s.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)) //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Errorf("notify: endpoint returned %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// RetryDelay returns how long to wait before the given attempt number (1 is
// the first retry). Delays grow from 30 seconds to 6 hours.
func RetryDelay(attempt int) time.Duration {
	delays := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour, 3 * time.Hour, 6 * time.Hour}
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(delays) {
		return delays[len(delays)-1]
	}
	return delays[attempt-1]
}

// MaxAttempts is how many times a delivery is tried before it is marked
// failed.
const MaxAttempts = 8
