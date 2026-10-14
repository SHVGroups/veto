// Package id generates the prefixed, time-sortable identifiers used for every
// record Veto stores, such as "act_01JA2B..." for an action.
//
// The suffix is a ULID: 48 bits of millisecond timestamp followed by 80 bits
// of randomness, encoded in Crockford base32. Identifiers therefore sort by
// creation time, which keeps B-tree inserts append-mostly and makes ids
// readable in logs.
package id

import (
	"crypto/rand"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Prefixes for each record kind. Keeping them in one place documents every
// identifier shape the system can emit.
const (
	Workspace  = "ws"
	User       = "usr"
	Invitation = "inv"
	Agent      = "agt"
	APIKey     = "key"
	Action     = "act"
	Evaluation = "evl"
	Grant      = "grt"
	Outcome    = "out"
	Audit      = "aud"
	SigningKey = "sgk"
	Connection = "con"
	MCPServer  = "mcp"
	Webhook    = "whk"
	Delivery   = "dlv"
)

var (
	mu      sync.Mutex
	entropy = ulid.Monotonic(rand.Reader, 0)
)

// New returns a new identifier with the given prefix.
func New(prefix string) string {
	return prefix + "_" + strings.ToLower(newULID(time.Now()).String())
}

// HasPrefix reports whether s is an identifier of the given kind.
func HasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix+"_") && len(s) == len(prefix)+1+ulid.EncodedSize
}

func newULID(t time.Time) ulid.ULID {
	mu.Lock()
	defer mu.Unlock()
	return ulid.MustNew(ulid.Timestamp(t), entropy)
}
