// Package auth holds the credential primitives: password hashing for console
// users, and generation and hashing of opaque tokens for sessions, API keys
// and invitations. Tokens are random and only their SHA-256 digests are
// stored, so a database leak does not reveal usable credentials.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Token prefixes make credentials recognizable to secret scanners and to
// people reading logs.
const (
	PrefixAgentKey  = "veto_sk_"
	PrefixAdminKey  = "veto_ak_"
	PrefixInvite    = "veto_inv_"
	PrefixSession   = "veto_ses_"
	PrefixDisplayed = 12
)

// Argon2id parameters from the OWASP password storage recommendations.
const (
	argonTime    = 2
	argonMemory  = 19 * 1024
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// Password length limits. The upper bound stops very long inputs from being
// used to burn CPU.
const (
	MinPasswordLen = 12
	MaxPasswordLen = 256
)

var tokenEncoding = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)

// ErrPasswordMismatch is returned when a password does not match its hash.
var ErrPasswordMismatch = errors.New("auth: password does not match")

// ValidatePassword checks a new password against the length policy.
func ValidatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < MinPasswordLen {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLen)
	}
	if len(pw) > MaxPasswordLen {
		return fmt.Errorf("password must be at most %d bytes", MaxPasswordLen)
	}
	return nil
}

// HashPassword returns an encoded Argon2id hash in PHC string format.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum)), nil
}

// CheckPassword compares a password to an encoded hash in constant time.
// needsRehash is true when the hash uses older parameters and should be
// replaced after a successful login.
func CheckPassword(pw, encoded string) (needsRehash bool, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("auth: unsupported password hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("auth: unsupported argon2 version")
	}
	var mem uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return false, errors.New("auth: malformed password hash parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errors.New("auth: malformed salt")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errors.New("auth: malformed hash")
	}
	got := argon2.IDKey([]byte(pw), salt, t, mem, p, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return false, ErrPasswordMismatch
	}
	return mem != argonMemory || t != argonTime || p != argonThreads, nil
}

// NewToken returns a random token with the given prefix.
func NewToken(prefix string) string {
	b := make([]byte, 30)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + tokenEncoding.EncodeToString(b)
}

// HashToken returns the digest stored in place of a token.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// DisplayPrefix returns the leading characters of a token that are safe to
// show so people can tell keys apart.
func DisplayPrefix(token string) string {
	if len(token) <= PrefixDisplayed {
		return token
	}
	return token[:PrefixDisplayed]
}
