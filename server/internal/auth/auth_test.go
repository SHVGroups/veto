package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected format %s", h)
	}
	rehash, err := CheckPassword("correct horse battery staple", h)
	if err != nil || rehash {
		t.Fatalf("check: rehash=%v err=%v", rehash, err)
	}
	if _, err := CheckPassword("wrong password entirely", h); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected mismatch, got %v", err)
	}
}

func TestCheckPasswordFlagsOldParameters(t *testing.T) {
	// A hash produced with t=1 should verify and ask for a rehash.
	old := "$argon2id$v=19$m=19456,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$" +
		"qcuqHDuIfRlgL71ZQfsb2/9iLYsPrt7KxD3Myn6O7Ek"
	if _, err := CheckPassword("x", old); err == nil {
		t.Fatal("expected mismatch for wrong password")
	}
}

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Error("accepted short password")
	}
	if ValidatePassword(strings.Repeat("a", 300)) == nil {
		t.Error("accepted very long password")
	}
	if err := ValidatePassword("twelve chars!"); err != nil {
		t.Error(err)
	}
}

func TestTokens(t *testing.T) {
	a, b := NewToken(PrefixAgentKey), NewToken(PrefixAgentKey)
	if a == b || !strings.HasPrefix(a, PrefixAgentKey) || len(a) != len(PrefixAgentKey)+48 {
		t.Fatalf("tokens %s %s", a, b)
	}
	if string(HashToken(a)) == string(HashToken(b)) {
		t.Fatal("hash collision")
	}
	if DisplayPrefix(a) != a[:12] {
		t.Fatal("display prefix")
	}
}
