package vault

import (
	"errors"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	k, err := ParseKey(GenerateKey())
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	kr, err := NewKeyring(mustKey(t))
	if err != nil {
		t.Fatal(err)
	}
	sealed := kr.Seal([]byte("sk_live_123"), "connection:con_1")
	got, err := kr.Open(sealed, "connection:con_1")
	if err != nil || string(got) != "sk_live_123" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := kr.Open(sealed, "connection:con_2"); err == nil {
		t.Fatal("opened with the wrong associated data")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := kr.Open(sealed, "connection:con_1"); err == nil {
		t.Fatal("opened a tampered value")
	}
}

func TestRotation(t *testing.T) {
	oldKey, newKey := mustKey(t), mustKey(t)
	oldRing, _ := NewKeyring(oldKey)
	sealed := oldRing.Seal([]byte("secret"), "a")

	newOnly, _ := NewKeyring(newKey)
	if _, err := newOnly.Open(sealed, "a"); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("expected ErrUnknownKey, got %v", err)
	}

	rotating, _ := NewKeyring(newKey, oldKey)
	if rotating.SealedWithCurrent(sealed) {
		t.Fatal("old value reported as current")
	}
	resealed, err := rotating.Reseal(sealed, "a")
	if err != nil {
		t.Fatal(err)
	}
	if !rotating.SealedWithCurrent(resealed) {
		t.Fatal("resealed value not under current key")
	}
	if got, err := newOnly.Open(resealed, "a"); err != nil || string(got) != "secret" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestParseKeyRejectsWrongLength(t *testing.T) {
	if _, err := ParseKey("c2hvcnQ="); err == nil {
		t.Fatal("accepted a short key")
	}
}
