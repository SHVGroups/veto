package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/decide"
	"github.com/shvgroups/veto/server/internal/domain"
	"github.com/shvgroups/veto/server/internal/notify"
	"github.com/shvgroups/veto/server/internal/safer"
)

// events streams workspace events to the console with Server-Sent Events.
// Each event names what changed; the console refetches what it shows.
func (s *Server) events(w http.ResponseWriter, r *http.Request, actor app.Actor) error {
	if err := actorCanView(actor); err != nil {
		return err
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("httpapi: response writer does not support streaming")
	}
	ch, cancel := s.app.Bus.Subscribe(actor.WorkspaceID, nil)
	defer cancel()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	// no-transform stops proxies from compressing the stream. A gzip layer
	// (the console's Next.js server compresses by default) buffers events
	// until its window fills, so the browser would see none of them.
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "retry: 3000\n: connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-heartbeat.C:
			if _, err := fmt.Fprintf(w, ": heartbeat %d\n\n", time.Now().Unix()); err != nil {
				return nil
			}
			flusher.Flush()
		case e := <-ch:
			data, _ := json.Marshal(e)
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Kind, data); err != nil {
				return nil
			}
			flusher.Flush()
		}
	}
}

func actorCanView(actor app.Actor) error {
	if !actor.Can(domain.PermView) {
		return &app.Error{Code: app.CodeForbidden, Message: "your role does not allow this"}
	}
	return nil
}

// meta returns the vocabulary the console renders: risk classes with their
// default thresholds, reason and option labels, packs, events and rule ops.
func (s *Server) meta(w http.ResponseWriter, _ *http.Request, actor app.Actor) error {
	if err := actorCanView(actor); err != nil {
		return err
	}
	type riskClass struct {
		Name       domain.RiskClass  `json:"name"`
		Thresholds domain.Thresholds `json:"thresholds"`
	}
	classes := make([]riskClass, 0, len(domain.RiskClasses))
	for _, c := range domain.RiskClasses {
		classes = append(classes, riskClass{Name: c, Thresholds: domain.DefaultThresholds(c)})
	}
	reasons := map[string]string{}
	for _, code := range []string{
		domain.ReasonPolicyForbid, domain.ReasonPolicyReview, domain.ReasonPolicyAllow, domain.ReasonRateLimited,
		domain.ReasonModelUnavailable, domain.ReasonModelDisabled, domain.ReasonFailOpen, domain.ReasonLowConfidence,
		domain.ReasonRiskAboveCeiling, domain.ReasonGuard, domain.ReasonRecheckDisagreed, domain.ReasonCriticalTool,
		domain.ReasonUnregisteredTool, domain.ReasonConfident, domain.ReasonAgentDisabled, domain.ReasonReviewerDecision,
		domain.ReasonReviewExpired, domain.ReasonSaferUnavailable, domain.ReasonOptionUnavailable,
	} {
		reasons[code] = app.ReasonLabel(domain.Reason{Code: code})
	}
	options := map[domain.Option]string{}
	for _, o := range domain.Options {
		options[o] = app.OptionLabel(o)
	}
	return ok(w, map[string]any{
		"version":        app.Version,
		"risk_classes":   classes,
		"reasons":        reasons,
		"options":        options,
		"packs":          decide.BuiltinNames(),
		"webhook_events": notify.Events,
		"safer_ops":      []safer.Op{safer.OpSwapTool, safer.OpCapNumber, safer.OpFilterDomains, safer.OpSet, safer.OpRemove, safer.OpLimitItems},
		"roles":          []domain.Role{domain.RoleOwner, domain.RoleAdmin, domain.RoleReviewer, domain.RoleViewer},
		"model":          app.ModelStatus{Configured: s.app.ModelConfigured(), Name: s.app.ModelName},
	})
}
