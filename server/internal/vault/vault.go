// Package vault encrypts the secrets Veto holds on behalf of a workspace:
// connection credentials, Slack tokens, webhook secrets and grant signing
// keys. Secrets are sealed with AES-256-GCM under a master key supplied by
// the operator and never stored in plaintext.
//
// Each sealed value records which key sealed it, so the master key can be
// rotated: configure the new key as current and the old one as previous, run
// "vetod vault rekey", then drop the previous key.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	formatV1 = 0x01
	keyIDLen = 4
	keyLen   = 32
)

// ErrUnknownKey means a value was sealed with a key the keyring does not have.
var ErrUnknownKey = errors.New("vault: value was sealed with an unknown key")

type key struct {
	id   [keyIDLen]byte
	aead cipher.AEAD
}

// Keyring seals with its current key and opens with any of its keys.
type Keyring struct {
	current  key
	previous []key
}

// ParseKey decodes a base64 master key and checks its length.
func ParseKey(encoded string) ([]byte, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		k, err = base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	}
	if err != nil {
		return nil, errors.New("vault: master key must be base64")
	}
	if len(k) != keyLen {
		return nil, fmt.Errorf("vault: master key must be %d bytes, got %d", keyLen, len(k))
	}
	return k, nil
}

// GenerateKey returns a new random master key, base64 encoded.
func GenerateKey() string {
	k := make([]byte, keyLen)
	if _, err := rand.Read(k); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(k)
}

// NewKeyring builds a keyring from a current key and any previous keys.
func NewKeyring(current []byte, previous ...[]byte) (*Keyring, error) {
	c, err := newKey(current)
	if err != nil {
		return nil, err
	}
	kr := &Keyring{current: c}
	for _, p := range previous {
		k, err := newKey(p)
		if err != nil {
			return nil, err
		}
		kr.previous = append(kr.previous, k)
	}
	return kr, nil
}

func newKey(raw []byte) (key, error) {
	if len(raw) != keyLen {
		return key{}, fmt.Errorf("vault: key must be %d bytes", keyLen)
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return key{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return key{}, err
	}
	sum := sha256.Sum256(append([]byte("veto.vault.keyid\n"), raw...))
	var k key
	copy(k.id[:], sum[:keyIDLen])
	k.aead = aead
	return k, nil
}

// Seal encrypts plaintext. The associated data binds the ciphertext to its
// purpose, such as "connection:con_123", so a sealed value copied to another
// record fails to open.
func (k *Keyring) Seal(plaintext []byte, associated string) []byte {
	nonce := make([]byte, k.current.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	out := make([]byte, 0, 1+keyIDLen+len(nonce)+len(plaintext)+k.current.aead.Overhead())
	out = append(out, formatV1)
	out = append(out, k.current.id[:]...)
	out = append(out, nonce...)
	return k.current.aead.Seal(out, nonce, plaintext, []byte(associated))
}

// Open decrypts a value produced by Seal with the same associated data.
func (k *Keyring) Open(sealed []byte, associated string) ([]byte, error) {
	kk, body, err := k.keyFor(sealed)
	if err != nil {
		return nil, err
	}
	ns := kk.aead.NonceSize()
	if len(body) < ns+kk.aead.Overhead() {
		return nil, errors.New("vault: sealed value is truncated")
	}
	pt, err := kk.aead.Open(nil, body[:ns], body[ns:], []byte(associated))
	if err != nil {
		return nil, errors.New("vault: sealed value failed authentication")
	}
	return pt, nil
}

// SealedWithCurrent reports whether a value was sealed with the current key.
func (k *Keyring) SealedWithCurrent(sealed []byte) bool {
	return len(sealed) > 1+keyIDLen && sealed[0] == formatV1 && [keyIDLen]byte(sealed[1:1+keyIDLen]) == k.current.id
}

// Reseal opens a value with whichever key sealed it and seals it again with
// the current key.
func (k *Keyring) Reseal(sealed []byte, associated string) ([]byte, error) {
	pt, err := k.Open(sealed, associated)
	if err != nil {
		return nil, err
	}
	return k.Seal(pt, associated), nil
}

func (k *Keyring) keyFor(sealed []byte) (key, []byte, error) {
	if len(sealed) < 1+keyIDLen || sealed[0] != formatV1 {
		return key{}, nil, errors.New("vault: unrecognized sealed value")
	}
	id := [keyIDLen]byte(sealed[1 : 1+keyIDLen])
	body := sealed[1+keyIDLen:]
	if id == k.current.id {
		return k.current, body, nil
	}
	for _, p := range k.previous {
		if id == p.id {
			return p, body, nil
		}
	}
	return key{}, nil, ErrUnknownKey
}
