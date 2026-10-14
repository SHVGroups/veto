package config

import (
	"strings"
	"testing"
	"time"

	"github.com/shvgroups/veto/server/internal/vault"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{
		"VETO_DATABASE_URL": "postgres://x",
		"VETO_MASTER_KEY":   vault.GenerateKey(),
		"VETO_PUBLIC_URL":   "https://veto.example.com/",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":8080" || c.DecisionTimeout != 2500*time.Millisecond || !c.CookieSecure || c.ModelConfigured() {
		t.Errorf("defaults %+v", c)
	}
	if c.PublicURL != "https://veto.example.com" || c.AllowedOrigins[0] != "https://veto.example.com" {
		t.Errorf("public url %q origins %v", c.PublicURL, c.AllowedOrigins)
	}
}

func TestLoadJev(t *testing.T) {
	c, err := Load(env(map[string]string{
		"VETO_DATABASE_URL":  "postgres://x",
		"VETO_MASTER_KEY":    vault.GenerateKey(),
		"VETO_JEV_API_KEY":   "ts_123",
		"VETO_JEV_FALLBACKS": `[{"name":"gateway","base_url":"https://gw.example.com","api_key":"k","model":"jev-1.13.0"}]`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Jev) != 2 || c.Jev[0].Model != "jev-1.13.0" || c.Jev[1].Name != "gateway" {
		t.Errorf("jev %+v", c.Jev)
	}
}

func TestLoadReportsEveryProblem(t *testing.T) {
	_, err := Load(env(map[string]string{
		"VETO_MASTER_KEY":       "short",
		"VETO_DECISION_TIMEOUT": "soon",
		"VETO_LOG_LEVEL":        "loud",
		"VETO_PUBLIC_URL":       "veto.example.com",
	}))
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"VETO_DATABASE_URL", "VETO_MASTER_KEY", "VETO_DECISION_TIMEOUT", "VETO_LOG_LEVEL", "VETO_PUBLIC_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %v", want, err)
		}
	}
}
