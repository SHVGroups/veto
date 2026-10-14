package grant

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
)

func TestSignVerify(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	hash := []byte{1, 2, 3, 4}
	now := time.Unix(1_800_000_000, 0)
	c := Claims{
		ID: "grt_1", ActionID: "act_1", WorkspaceID: "ws_1", Tool: "gmail.create_draft",
		ArgsHash: hex.EncodeToString(hash), Option: domain.OptionProceedSafer, KeyID: KeyID(pub),
		IssuedAt: now.Unix(), ExpiresAt: now.Add(DefaultTTL).Unix(),
	}
	tok, err := Sign(c, priv)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "vg1.") {
		t.Fatalf("token %s", tok)
	}
	got, err := Verify(tok, pub, "gmail.create_draft", hash, now)
	if err != nil || got.ActionID != "act_1" {
		t.Fatalf("verify: %+v %v", got, err)
	}
	if _, err := Verify(tok, pub, "gmail.send", hash, now); !errors.Is(err, ErrMismatch) {
		t.Errorf("tool mismatch: %v", err)
	}
	if _, err := Verify(tok, pub, "gmail.create_draft", []byte{9}, now); !errors.Is(err, ErrMismatch) {
		t.Errorf("args mismatch: %v", err)
	}
	if _, err := Verify(tok, pub, "gmail.create_draft", hash, now.Add(DefaultTTL)); !errors.Is(err, ErrExpired) {
		t.Errorf("expiry: %v", err)
	}
	otherPub, _, _ := ed25519.GenerateKey(nil)
	if _, err := Verify(tok, otherPub, "gmail.create_draft", hash, now); !errors.Is(err, ErrSignature) {
		t.Errorf("wrong key: %v", err)
	}
	parts := strings.Split(tok, ".")
	forged := parts[0] + "." + b64([]byte(`{"gid":"x","tool":"gmail.create_draft","exp":9999999999}`)) + "." + parts[2]
	if _, err := Verify(forged, pub, "gmail.create_draft", hash, now); !errors.Is(err, ErrSignature) {
		t.Errorf("forged payload: %v", err)
	}
	if parsed, err := Parse(tok); err != nil || parsed.KeyID != KeyID(pub) {
		t.Errorf("parse: %+v %v", parsed, err)
	}
}
