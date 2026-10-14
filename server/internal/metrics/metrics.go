// Package metrics exposes counters, gauges and histograms in the Prometheus
// text format (version 0.0.4). It is deliberately small: Veto needs labeled
// counters and histograms and nothing else, and a dependency-free
// implementation keeps the security-sensitive binary lean.
package metrics

import (
	"crypto/subtle"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Registry holds every metric and renders them.
type Registry struct {
	mu      sync.Mutex
	metrics []metric
}

type metric interface {
	name() string
	write(w io.Writer)
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) register(m metric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics = append(r.metrics, m)
	sort.Slice(r.metrics, func(i, j int) bool { return r.metrics[i].name() < r.metrics[j].name() })
}

// Write renders every metric.
func (r *Registry) Write(w io.Writer) {
	r.mu.Lock()
	ms := append([]metric(nil), r.metrics...)
	r.mu.Unlock()
	for _, m := range ms {
		m.write(w)
	}
}

// Handler serves the metrics. With a non-empty token, requests must carry
// "Authorization: Bearer <token>".
func (r *Registry) Handler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if token != "" {
			got := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		r.Write(w)
	})
}

type vecKey string

func key(values []string) vecKey { return vecKey(strings.Join(values, "\xff")) }

// CounterVec is a counter partitioned by label values.
type CounterVec struct {
	n, help string
	labels  []string
	mu      sync.Mutex
	values  map[vecKey]*counterEntry
}

type counterEntry struct {
	labels []string
	v      float64
}

// Counter registers a counter.
func (r *Registry) Counter(name, help string, labels ...string) *CounterVec {
	c := &CounterVec{n: name, help: help, labels: labels, values: map[vecKey]*counterEntry{}}
	r.register(c)
	return c
}

// Add increases the counter for the label values by v, which must not be
// negative.
func (c *CounterVec) Add(v float64, labelValues ...string) {
	if v < 0 || len(labelValues) != len(c.labels) {
		return
	}
	k := key(labelValues)
	c.mu.Lock()
	e, ok := c.values[k]
	if !ok {
		e = &counterEntry{labels: append([]string(nil), labelValues...)}
		c.values[k] = e
	}
	e.v += v
	c.mu.Unlock()
}

// Inc increases the counter by one.
func (c *CounterVec) Inc(labelValues ...string) { c.Add(1, labelValues...) }

func (c *CounterVec) name() string { return c.n }

func (c *CounterVec) write(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.n, escapeHelp(c.help), c.n)
	c.mu.Lock()
	entries := sortedEntries(c.values)
	for _, e := range entries {
		fmt.Fprintf(w, "%s%s %s\n", c.n, labelString(c.labels, e.labels, "", ""), formatFloat(e.v))
	}
	c.mu.Unlock()
}

// HistogramVec is a histogram partitioned by label values.
type HistogramVec struct {
	n, help string
	labels  []string
	buckets []float64
	mu      sync.Mutex
	values  map[vecKey]*histEntry
}

type histEntry struct {
	labels []string
	counts []uint64
	sum    float64
	count  uint64
}

// DefaultLatencyBuckets suit request and model latencies in seconds.
var DefaultLatencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Histogram registers a histogram with the given upper bounds.
func (r *Registry) Histogram(name, help string, buckets []float64, labels ...string) *HistogramVec {
	b := append([]float64(nil), buckets...)
	sort.Float64s(b)
	h := &HistogramVec{n: name, help: help, labels: labels, buckets: b, values: map[vecKey]*histEntry{}}
	r.register(h)
	return h
}

// Observe records a value for the label values.
func (h *HistogramVec) Observe(v float64, labelValues ...string) {
	if len(labelValues) != len(h.labels) || math.IsNaN(v) {
		return
	}
	k := key(labelValues)
	h.mu.Lock()
	e, ok := h.values[k]
	if !ok {
		e = &histEntry{labels: append([]string(nil), labelValues...), counts: make([]uint64, len(h.buckets))}
		h.values[k] = e
	}
	for i, ub := range h.buckets {
		if v <= ub {
			e.counts[i]++
		}
	}
	e.sum += v
	e.count++
	h.mu.Unlock()
}

func (h *HistogramVec) name() string { return h.n }

func (h *HistogramVec) write(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.n, escapeHelp(h.help), h.n)
	h.mu.Lock()
	keys := make([]vecKey, 0, len(h.values))
	for k := range h.values {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		e := h.values[k]
		for i, ub := range h.buckets {
			fmt.Fprintf(w, "%s_bucket%s %d\n", h.n, labelString(h.labels, e.labels, "le", formatFloat(ub)), e.counts[i])
		}
		fmt.Fprintf(w, "%s_bucket%s %d\n", h.n, labelString(h.labels, e.labels, "le", "+Inf"), e.count)
		fmt.Fprintf(w, "%s_sum%s %s\n", h.n, labelString(h.labels, e.labels, "", ""), formatFloat(e.sum))
		fmt.Fprintf(w, "%s_count%s %d\n", h.n, labelString(h.labels, e.labels, "", ""), e.count)
	}
	h.mu.Unlock()
}

type gaugeFunc struct {
	n, help string
	fn      func() float64
}

// GaugeFunc registers a gauge whose value is read when metrics are scraped.
func (r *Registry) GaugeFunc(name, help string, fn func() float64) {
	r.register(&gaugeFunc{n: name, help: help, fn: fn})
}

func (g *gaugeFunc) name() string { return g.n }

func (g *gaugeFunc) write(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %s\n", g.n, escapeHelp(g.help), g.n, g.n, formatFloat(g.fn()))
}

func sortedEntries(m map[vecKey]*counterEntry) []*counterEntry {
	keys := make([]vecKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]*counterEntry, len(keys))
	for i, k := range keys {
		out[i] = m[k]
	}
	return out
}

func labelString(names, values []string, extraName, extraValue string) string {
	if len(names) == 0 && extraName == "" {
		return ""
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, n := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `%s="%s"`, n, escapeLabel(values[i]))
	}
	if extraName != "" {
		if len(names) > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `%s="%s"`, extraName, extraValue)
	}
	b.WriteByte('}')
	return b.String()
}

func escapeLabel(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(s)
}

func escapeHelp(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`).Replace(s)
}

func formatFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "+Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}
