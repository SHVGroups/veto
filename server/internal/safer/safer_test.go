package safer

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func mustRules(t *testing.T, s string) []Rule {
	t.Helper()
	r, err := ParseRules([]byte(s))
	if err != nil {
		t.Fatalf("ParseRules: %v", err)
	}
	return r
}

func TestBuildEmailDraftToInternalOnly(t *testing.T) {
	rules := mustRules(t, `[
		{"op":"swap_tool","tool":"gmail.create_draft"},
		{"op":"filter_domains","path":"to","allow":["acme.com"]}
	]`)
	args := json.RawMessage(`{"to":["ap@acme.com","CFO <cfo@rival.io>","ops@eu.acme.com"],"subject":"Invoice"}`)
	spec, err := Build("gmail.send", args, rules)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Available || spec.Tool != "gmail.create_draft" {
		t.Fatalf("unexpected spec %+v", spec)
	}
	var out map[string]any
	_ = json.Unmarshal(spec.Args, &out)
	if got := out["to"]; !reflect.DeepEqual(got, []any{"ap@acme.com", "ops@eu.acme.com"}) {
		t.Errorf("to = %v", got)
	}
	want := []string{
		"Runs gmail.create_draft instead of gmail.send",
		"Removes 1 address from to outside acme.com: CFO <cfo@rival.io>",
	}
	if !reflect.DeepEqual(spec.Changes, want) {
		t.Errorf("changes = %q", spec.Changes)
	}
}

func TestBuildCapNumber(t *testing.T) {
	rules := mustRules(t, `[{"op":"cap_number","path":"payment.amount_cents","max":50000}]`)
	spec, err := Build("stripe.refund", json.RawMessage(`{"payment":{"amount_cents":120000,"currency":"usd"}}`), rules)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Available || string(spec.Args) != `{"payment":{"amount_cents":50000,"currency":"usd"}}` {
		t.Fatalf("got %+v args %s", spec, spec.Args)
	}
	if spec.Changes[0] != "Caps payment.amount_cents at 50000 (was 120000)" {
		t.Errorf("change %q", spec.Changes[0])
	}
}

func TestBuildUnavailableWhenNothingChanges(t *testing.T) {
	rules := mustRules(t, `[{"op":"cap_number","path":"amount","max":500},{"op":"filter_domains","path":"to","allow":["acme.com"]}]`)
	spec, err := Build("pay", json.RawMessage(`{"amount":100,"to":"a@acme.com"}`), rules)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Available {
		t.Fatalf("expected unavailable, got %+v", spec)
	}
}

func TestBuildSetRemoveLimit(t *testing.T) {
	rules := mustRules(t, `[
		{"op":"set","path":"visibility","value":"internal"},
		{"op":"remove","path":"bcc"},
		{"op":"limit_items","path":"files","max":2},
		{"op":"set","path":"dry_run","value":true}
	]`)
	spec, err := Build("drive.share", json.RawMessage(`{"visibility":"public","bcc":["x@y.z"],"files":[1,2,3,4]}`), rules)
	if err != nil {
		t.Fatal(err)
	}
	if string(spec.Args) != `{"dry_run":true,"files":[1,2],"visibility":"internal"}` {
		t.Errorf("args %s", spec.Args)
	}
	if len(spec.Changes) != 4 || !strings.HasPrefix(spec.Changes[0], `Sets visibility to "internal" (was "public")`) {
		t.Errorf("changes %q", spec.Changes)
	}
}

func TestParseRulesRejectsBadInput(t *testing.T) {
	bad := []string{
		`[{"op":"explode"}]`,
		`[{"op":"cap_number","path":"a"}]`,
		`[{"op":"filter_domains","path":"to"}]`,
		`[{"op":"swap_tool"}]`,
		`[{"op":"remove","path":"a..b"}]`,
		`[{"op":"set","path":"a","value":1,"extra":true}]`,
	}
	for _, s := range bad {
		if _, err := ParseRules([]byte(s)); err == nil {
			t.Errorf("ParseRules(%s) succeeded, want error", s)
		}
	}
}
