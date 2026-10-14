// Package config loads vetod's configuration from environment variables.
// Every variable is documented in docs/reference/configuration.md; keep the
// two in step.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shvgroups/veto/server/internal/jev"
	"github.com/shvgroups/veto/server/internal/vault"
)

// Config is the complete runtime configuration.
type Config struct {
	DatabaseURL string
	ListenAddr  string
	// PublicURL is where people reach the console. It is used to build review
	// links in notifications and to check the Origin of console requests.
	PublicURL string

	MasterKey         []byte
	PreviousMasterKey [][]byte

	Jev             []jev.Endpoint
	DecisionTimeout time.Duration

	SessionTTL     time.Duration
	CookieSecure   bool
	SetupToken     string
	AllowedOrigins []string

	LogLevel  slog.Level
	LogFormat string

	RunWorkers      bool
	MaxWait         time.Duration
	RateLimitRPS    float64
	RateLimitBurst  int
	MetricsToken    string
	TrustedProxies  []*net.IPNet
	ConnectorPublic bool
	MCPHoldTimeout  time.Duration
}

// Load reads configuration using getenv, which is os.Getenv in production
// and a map lookup in tests.
func Load(getenv func(string) string) (*Config, error) {
	var errs []error
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	dur := func(key string, def time.Duration) time.Duration {
		v := get(key, "")
		if v == "" {
			return def
		}
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("%s: invalid duration %q", key, v))
			return def
		}
		return d
	}
	boolean := func(key string, def bool) bool {
		v := get(key, "")
		if v == "" {
			return def
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: invalid boolean %q", key, v))
			return def
		}
		return b
	}

	c := &Config{
		DatabaseURL:     get("VETO_DATABASE_URL", ""),
		ListenAddr:      get("VETO_LISTEN_ADDR", defaultListenAddr(get)),
		PublicURL:       strings.TrimRight(get("VETO_PUBLIC_URL", ""), "/"),
		DecisionTimeout: dur("VETO_DECISION_TIMEOUT", 2500*time.Millisecond),
		SessionTTL:      dur("VETO_SESSION_TTL", 14*24*time.Hour),
		CookieSecure:    boolean("VETO_COOKIE_SECURE", true),
		SetupToken:      get("VETO_SETUP_TOKEN", ""),
		LogFormat:       get("VETO_LOG_FORMAT", "json"),
		RunWorkers:      boolean("VETO_WORKERS", true),
		MaxWait:         dur("VETO_MAX_WAIT", 60*time.Second),
		MetricsToken:    get("VETO_METRICS_TOKEN", ""),
		ConnectorPublic: !boolean("VETO_CONNECTOR_ALLOW_PRIVATE", false),
		MCPHoldTimeout:  dur("VETO_MCP_HOLD_TIMEOUT", 50*time.Second),
	}

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("VETO_DATABASE_URL is required"))
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("VETO_PUBLIC_URL: %q is not an absolute http(s) URL", c.PublicURL))
		} else {
			c.AllowedOrigins = append(c.AllowedOrigins, u.Scheme+"://"+u.Host)
		}
	}
	for _, o := range strings.Split(get("VETO_ALLOWED_ORIGINS", ""), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}

	if mk := get("VETO_MASTER_KEY", ""); mk == "" {
		errs = append(errs, errors.New("VETO_MASTER_KEY is required; generate one with: vetod keygen"))
	} else if k, err := vault.ParseKey(mk); err != nil {
		errs = append(errs, fmt.Errorf("VETO_MASTER_KEY: %w", err))
	} else {
		c.MasterKey = k
	}
	for _, p := range strings.Split(get("VETO_MASTER_KEY_PREVIOUS", ""), ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		k, err := vault.ParseKey(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("VETO_MASTER_KEY_PREVIOUS: %w", err))
			continue
		}
		c.PreviousMasterKey = append(c.PreviousMasterKey, k)
	}

	if key := get("VETO_JEV_API_KEY", ""); key != "" {
		c.Jev = append(c.Jev, jev.Endpoint{
			Name:    "typesafe",
			BaseURL: get("VETO_JEV_BASE_URL", jev.DefaultBaseURL),
			APIKey:  key,
			Model:   get("VETO_JEV_MODEL", "jev-1.13.0"),
		})
	}
	if raw := get("VETO_JEV_FALLBACKS", ""); raw != "" {
		var fallbacks []struct {
			Name    string `json:"name"`
			BaseURL string `json:"base_url"`
			APIKey  string `json:"api_key"`
			Model   string `json:"model"`
		}
		if err := json.Unmarshal([]byte(raw), &fallbacks); err != nil {
			errs = append(errs, fmt.Errorf("VETO_JEV_FALLBACKS: %w", err))
		}
		for i, f := range fallbacks {
			if f.BaseURL == "" || f.Model == "" {
				errs = append(errs, fmt.Errorf("VETO_JEV_FALLBACKS[%d]: base_url and model are required", i))
				continue
			}
			if f.Name == "" {
				f.Name = fmt.Sprintf("fallback-%d", i+1)
			}
			c.Jev = append(c.Jev, jev.Endpoint{Name: f.Name, BaseURL: f.BaseURL, APIKey: f.APIKey, Model: f.Model})
		}
	}

	switch strings.ToLower(get("VETO_LOG_LEVEL", "info")) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		errs = append(errs, errors.New("VETO_LOG_LEVEL must be debug, info, warn or error"))
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		errs = append(errs, errors.New("VETO_LOG_FORMAT must be json or text"))
	}

	rps, err := strconv.ParseFloat(get("VETO_RATE_LIMIT_RPS", "50"), 64)
	if err != nil || rps <= 0 {
		errs = append(errs, errors.New("VETO_RATE_LIMIT_RPS must be a positive number"))
	}
	c.RateLimitRPS = rps
	burst, err := strconv.Atoi(get("VETO_RATE_LIMIT_BURST", "100"))
	if err != nil || burst <= 0 {
		errs = append(errs, errors.New("VETO_RATE_LIMIT_BURST must be a positive integer"))
	}
	c.RateLimitBurst = burst

	for _, cidr := range strings.Split(get("VETO_TRUSTED_PROXIES", ""), ",") {
		if cidr = strings.TrimSpace(cidr); cidr == "" {
			continue
		}
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			errs = append(errs, fmt.Errorf("VETO_TRUSTED_PROXIES: invalid CIDR %q", cidr))
			continue
		}
		c.TrustedProxies = append(c.TrustedProxies, n)
	}

	if c.MaxWait > 5*time.Minute {
		errs = append(errs, errors.New("VETO_MAX_WAIT must be at most 5m"))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return c, nil
}

// ModelConfigured reports whether a decision model endpoint is set.
func (c *Config) ModelConfigured() bool { return len(c.Jev) > 0 }

// defaultListenAddr is :8080, or :$PORT when a platform or a local proxy
// such as portless assigns the port.
func defaultListenAddr(get func(string, string) string) string {
	if p := get("PORT", ""); p != "" {
		return ":" + p
	}
	return ":8080"
}
