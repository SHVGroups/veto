// Package grant issues and verifies the permission tokens attached to
// approved actions. A grant names one tool and the hash of its exact
// arguments, expires quickly, and is signed with the workspace's Ed25519 key.
// A tool server that receives a grant can verify it offline with the public
// key, and can consume it through the API so it is honored only once.
package grant

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shvgroups/veto/server/internal/domain"
)

const (
	tokenPrefix   = "vg1"
	signingDomain = "veto.grant.v1\n"
	// DefaultTTL is how long a grant stays valid after issue.
	DefaultTTL = 5 * time.Minute
)

// Errors returned by Verify.
var (
	ErrMalformed = errors.New("grant: malformed token")
	ErrSignature = errors.New("grant: signature does not verify")
	ErrExpired   = errors.New("grant: token has expired")
	ErrMismatch  = errors.New("grant: token does not match this call")
)

// Claims are the signed contents of a grant.
type Claims struct {
	ID          string        `json:"gid"`
	ActionID    string        `json:"aid"`
	WorkspaceID string        `json:"ws"`
	Tool        string        `json:"tool"`
	ArgsHash    string        `json:"ah"`
	Option      domain.Option `json:"opt"`
	KeyID       string        `json:"kid"`
	IssuedAt    int64         `json:"iat"`
	ExpiresAt   int64         `json:"exp"`
}

// Expires returns the expiry time.
func (c Claims) Expires() time.Time { return time.Unix(c.ExpiresAt, 0) }

// Sign encodes and signs claims.
func Sign(c Claims, key ed25519.PrivateKey) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(key, append([]byte(signingDomain), payload...))
	return tokenPrefix + "." + b64(payload) + "." + b64(sig), nil
}

// Parse decodes a token without verifying it, so the caller can look up the
// signing key named in the claims.
func Parse(token string) (Claims, error) {
	payload, _, err := split(token)
	if err != nil {
		return Claims{}, err
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Claims{}, ErrMalformed
	}
	return c, nil
}

// Verify checks the signature and expiry, and that the grant covers exactly
// this tool and argument hash.
func Verify(token string, key ed25519.PublicKey, tool string, argsHash []byte, now time.Time) (Claims, error) {
	payload, sig, err := split(token)
	if err != nil {
		return Claims{}, err
	}
	if len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, append([]byte(signingDomain), payload...), sig) {
		return Claims{}, ErrSignature
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Claims{}, ErrMalformed
	}
	if now.Unix() >= c.ExpiresAt {
		return c, ErrExpired
	}
	if c.Tool != tool || c.ArgsHash != hex.EncodeToString(argsHash) {
		return c, ErrMismatch
	}
	return c, nil
}

func split(token string) (payload, sig []byte, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != tokenPrefix {
		return nil, nil, ErrMalformed
	}
	payload, err = base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, ErrMalformed
	}
	sig, err = base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, nil, ErrMalformed
	}
	return payload, sig, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// KeyID derives a short identifier for a public key.
func KeyID(pub ed25519.PublicKey) string {
	return fmt.Sprintf("%x", pub[:8])
}
