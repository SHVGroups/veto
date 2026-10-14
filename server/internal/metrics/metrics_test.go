package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExposition(t *testing.T) {
	r := NewRegistry()
	c := r.Counter("veto_actions_total", "Actions decided.", "outcome")
	c.Inc("approved")
	c.Add(2, "denied")
	c.Inc(`we"ird`)
	h := r.Histogram("veto_model_latency_seconds", "Model latency.", []float64{0.1, 0.5}, "endpoint")
	h.Observe(0.05, "typesafe")
	h.Observe(0.3, "typesafe")
	h.Observe(2, "typesafe")
	r.GaugeFunc("veto_up", "Always 1.", func() float64 { return 1 })

	var b strings.Builder
	r.Write(&b)
	out := b.String()
	for _, want := range []string{
		"# TYPE veto_actions_total counter",
		`veto_actions_total{outcome="approved"} 1`,
		`veto_actions_total{outcome="denied"} 2`,
		`veto_actions_total{outcome="we\"ird"} 1`,
		`veto_model_latency_seconds_bucket{endpoint="typesafe",le="0.1"} 1`,
		`veto_model_latency_seconds_bucket{endpoint="typesafe",le="0.5"} 2`,
		`veto_model_latency_seconds_bucket{endpoint="typesafe",le="+Inf"} 3`,
		`veto_model_latency_seconds_sum{endpoint="typesafe"} 2.35`,
		`veto_model_latency_seconds_count{endpoint="typesafe"} 3`,
		"veto_up 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestHandlerToken(t *testing.T) {
	r := NewRegistry()
	h := r.Handler("s3cret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
}
