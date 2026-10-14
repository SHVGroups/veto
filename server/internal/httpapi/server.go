// Package httpapi is Veto's HTTP interface: the agent API, the console's
// management API, the MCP proxy route, the live event stream, health checks
// and metrics. Handlers translate HTTP to app operations and back; they hold
// no business logic.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/config"
	"github.com/shvgroups/veto/server/internal/mcp"
	"github.com/shvgroups/veto/server/internal/metrics"
)

// SessionCookie holds the console session token.
const SessionCookie = "veto_session"

// CSRFHeader must accompany every cookie-authenticated request that changes
// state. Browsers cannot add custom headers to cross-site requests without a
// CORS preflight, which Veto never grants.
const CSRFHeader = "X-Veto-Requested-With"

// Server holds the HTTP layer's dependencies.
type Server struct {
	app     *app.App
	cfg     *config.Config
	log     *slog.Logger
	mcp     *mcp.Proxy
	reg     *metrics.Registry
	limiter *limiter
	logins  *limiter

	requests *metrics.CounterVec
	latency  *metrics.HistogramVec
}

// New returns a Server.
func New(a *app.App, cfg *config.Config, log *slog.Logger, proxy *mcp.Proxy, reg *metrics.Registry) *Server {
	return &Server{
		app: a, cfg: cfg, log: log, mcp: proxy, reg: reg,
		limiter:  newLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst),
		logins:   newLimiter(0.2, 10),
		requests: reg.Counter("veto_http_requests_total", "HTTP requests, by route and status class.", "route", "code"),
		latency:  reg.Histogram("veto_http_request_seconds", "HTTP request latency, by route.", metrics.DefaultLatencyBuckets, "route"),
	}
}

// Auth levels for routes.
type access int

const (
	public  access = iota
	anyAuth        // session or any API key
	console        // session or admin key
	session        // session only
	agent          // agent key only
	apiKey         // any API key
)

type handlerFunc func(w http.ResponseWriter, r *http.Request, actor app.Actor) error

// Handler returns the root handler with every route registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern string, level access, h handlerFunc) {
		mux.Handle(pattern, s.wrap(pattern, level, h))
	}
	s.routes(route)
	mux.Handle("GET /metrics", s.reg.Handler(s.cfg.MetricsToken))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": app.Version})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.app.Store.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "reason": "database unreachable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.writeError(w, r, &app.Error{Code: app.CodeNotFound, Message: "no route for " + r.Method + " " + r.URL.Path})
	})
	return s.base(mux)
}

// base applies request ids, panic recovery, security headers and logging to
// every request.
func (s *Server) base(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := r.Header.Get("X-Request-Id")
		if rid == "" || len(rid) > 64 {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			rid = "req_" + hex.EncodeToString(b)
		}
		r = r.WithContext(context.WithValue(r.Context(), ctxRequestID{}, rid))
		h := w.Header()
		h.Set("X-Request-Id", rid)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cache-Control", "no-store")
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if p := recover(); p != nil {
				s.log.Error("panic serving request", "request_id", rid, "panic", p, "stack", string(debug.Stack()))
				if !rec.wrote {
					s.writeError(rec, r, errors.New("panic"))
				}
			}
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			} else if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
				level = slog.LevelDebug
			}
			s.log.Log(r.Context(), level, "request",
				"request_id", rid, "method", r.Method, "path", r.URL.Path, "status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(), "ip", s.clientIP(r))
		}()
		next.ServeHTTP(rec, r)
	})
}

// wrap authenticates, authorizes, rate limits and instruments one route.
func (s *Server) wrap(pattern string, level access, h handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec, ok := w.(*statusRecorder)
		if !ok {
			rec = &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		}
		defer func() {
			s.requests.Inc(pattern, statusClass(rec.status))
			if !strings.HasSuffix(pattern, "/events") {
				s.latency.Observe(time.Since(start).Seconds(), pattern)
			}
		}()
		actor, err := s.authenticate(r, level)
		if err != nil {
			s.writeError(rec, r, err)
			return
		}
		if actor.Kind == app.ActorAgent || actor.Kind == app.ActorKey {
			if !s.limiter.allow(actor.String()) {
				s.writeError(rec, r, &app.Error{Code: app.CodeRateLimited, Message: "rate limit exceeded; slow down"})
				return
			}
		}
		if err := h(rec, r, actor); err != nil {
			s.writeError(rec, r, err)
		}
	})
}

func (s *Server) authenticate(r *http.Request, level access) (app.Actor, error) {
	if level == public {
		return app.Actor{}, nil
	}
	unauthorized := &app.Error{Code: app.CodeUnauthorized, Message: "authentication required"}
	if authz := r.Header.Get("Authorization"); authz != "" {
		token, ok := strings.CutPrefix(authz, "Bearer ")
		if !ok {
			return app.Actor{}, &app.Error{Code: app.CodeUnauthorized, Message: "Authorization header must be: Bearer <api key>"}
		}
		if level == session {
			return app.Actor{}, &app.Error{Code: app.CodeForbidden, Message: "this endpoint needs a signed-in user, not an API key"}
		}
		actor, err := s.app.AuthenticateKey(r.Context(), strings.TrimSpace(token))
		if err != nil {
			return app.Actor{}, err
		}
		switch {
		case level == agent && actor.Kind != app.ActorAgent:
			return app.Actor{}, &app.Error{Code: app.CodeForbidden, Message: "this endpoint needs an agent API key (veto_sk_...)"}
		case level == console && actor.Kind != app.ActorKey:
			return app.Actor{}, &app.Error{Code: app.CodeForbidden, Message: "this endpoint needs a signed-in user or an admin API key (veto_ak_...)"}
		}
		return actor, nil
	}
	if level == agent || level == apiKey {
		return app.Actor{}, &app.Error{Code: app.CodeUnauthorized, Message: "send an API key as: Authorization: Bearer <key>"}
	}
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" {
		return app.Actor{}, unauthorized
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if r.Header.Get(CSRFHeader) == "" {
			return app.Actor{}, &app.Error{Code: app.CodeForbidden, Message: "missing " + CSRFHeader + " header"}
		}
		if origin := r.Header.Get("Origin"); origin != "" && !s.originAllowed(r, origin) {
			return app.Actor{}, &app.Error{Code: app.CodeForbidden, Message: "request origin is not allowed"}
		}
	}
	return s.app.Authenticate(r.Context(), c.Value)
}

// originAllowed accepts the configured origins. When none are configured it
// accepts the origin of VETO_PUBLIC_URL, where the console is served, and
// the request's own host.
func (s *Server) originAllowed(r *http.Request, origin string) bool {
	origin = strings.TrimRight(origin, "/")
	if len(s.cfg.AllowedOrigins) > 0 {
		return slices.Contains(s.cfg.AllowedOrigins, origin)
	}
	if s.cfg.PublicURL != "" {
		if u, err := url.Parse(s.cfg.PublicURL); err == nil && u.Scheme+"://"+u.Host == origin {
			return true
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" || !s.trustedPeer(r) {
		host = r.Host
	}
	return strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://") == host
}

// clientIP returns the caller's address, honoring X-Forwarded-For only from
// trusted proxies.
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !s.trustedPeer(r) {
		return host
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(hops[i]))
		if ip == nil {
			continue
		}
		if !s.trusted(ip) {
			return ip.String()
		}
	}
	return host
}

func (s *Server) trustedPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && s.trusted(ip)
}

func (s *Server) trusted(ip net.IP) bool {
	for _, n := range s.cfg.TrustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

type ctxRequestID struct{}

func requestID(ctx context.Context) string {
	v, _ := ctx.Value(ctxRequestID{}).(string)
	return v
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// Flush supports streaming responses through the recorder.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func statusClass(code int) string {
	return string(rune('0'+code/100)) + "xx"
}

// limiter is a token bucket per key. It is per replica: with N replicas the
// effective limit is up to N times higher, which is documented.
type limiter struct {
	rate, burst float64
	mu          sync.Mutex
	buckets     map[string]*bucket
	lastSweep   time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(rps float64, burst int) *limiter {
	return &limiter{rate: rps, burst: float64(burst), buckets: map[string]*bucket{}, lastSweep: time.Now()}
}

func (l *limiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) > 10*time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
