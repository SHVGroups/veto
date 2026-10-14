// Package ledger defines Veto's tamper-evident audit log. Every event that
// changes what Veto will do, or records what it did, is appended to a
// per-workspace hash chain: each entry's hash covers its contents and the
// previous entry's hash, so editing or deleting any entry breaks every hash
// after it. Signed checkpoints let a third party confirm the head of the
// chain without trusting the database.
package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shvgroups/veto/server/internal/canonical"
)

// Event kinds. The console and webhooks use the same names.
const (
	KindActionReceived    = "action.received"
	KindGateEvaluated     = "gate.evaluated"
	KindModelEvaluated    = "model.evaluated"
	KindActionHeld        = "action.held"
	KindActionDecided     = "action.decided"
	KindActionExpired     = "action.expired"
	KindGrantIssued       = "grant.issued"
	KindGrantConsumed     = "grant.consumed"
	KindActionExecuted    = "action.executed"
	KindOutcomeReported   = "outcome.reported"
	KindAuditSampled      = "audit.sampled"
	KindAuditRecorded     = "audit.recorded"
	KindPolicyCreated     = "policy.created"
	KindPolicyActivated   = "policy.activated"
	KindPackUpdated       = "pack.updated"
	KindToolUpdated       = "tool.updated"
	KindToolDeleted       = "tool.deleted"
	KindAgentCreated      = "agent.created"
	KindAgentUpdated      = "agent.updated"
	KindKeyCreated        = "key.created"
	KindKeyRevoked        = "key.revoked"
	KindConnectionChanged = "connection.changed"
	KindMCPServerChanged  = "mcp_server.changed"
	KindWebhookChanged    = "webhook.changed"
	KindIntegration       = "integration.changed"
	KindMemberChanged     = "member.changed"
	KindSettingsUpdated   = "settings.updated"
	KindSessionCreated    = "session.created"
	KindWorkspaceCreated  = "workspace.created"
	KindSigningKeyRotated = "signing_key.rotated"
)

// Entry is one link of the chain.
type Entry struct {
	WorkspaceID string          `json:"-"`
	Seq         int64           `json:"seq"`
	Kind        string          `json:"kind"`
	ActionID    string          `json:"action_id,omitempty"`
	Actor       string          `json:"actor"`
	Payload     json.RawMessage `json:"payload"`
	PrevHash    []byte          `json:"-"`
	Hash        []byte          `json:"-"`
	CreatedAt   time.Time       `json:"created_at"`
}

// MarshalJSON adds hex-encoded hashes for API consumers.
func (e Entry) MarshalJSON() ([]byte, error) {
	type alias Entry
	return json.Marshal(struct {
		alias
		PrevHash string `json:"prev_hash"`
		Hash     string `json:"hash"`
	}{alias(e), hex.EncodeToString(e.PrevHash), hex.EncodeToString(e.Hash)})
}

// Genesis is the previous-hash value of a workspace's first entry.
func Genesis(workspaceID string) []byte {
	sum := sha256.Sum256([]byte("veto.ledger.genesis\n" + workspaceID))
	return sum[:]
}

// Normalize returns the time with the precision Postgres stores, so the hash
// computed at write time matches the one recomputed from the database.
func Normalize(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// Hash computes an entry's hash from its contents and the previous hash.
// The payload is canonicalized first, so storage that reorders keys or
// reformats numbers does not change the result.
func Hash(e Entry) ([]byte, error) {
	v, err := canonical.Decode(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("ledger: payload: %w", err)
	}
	payload, err := canonical.Marshal(v)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	write := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	write([]byte("veto.ledger.v1"))
	write([]byte(e.WorkspaceID))
	var seq [8]byte
	binary.BigEndian.PutUint64(seq[:], uint64(e.Seq))
	write(seq[:])
	write([]byte(e.Kind))
	write([]byte(e.ActionID))
	write([]byte(e.Actor))
	write(payload)
	write([]byte(Normalize(e.CreatedAt).Format(time.RFC3339Nano)))
	write(e.PrevHash)
	return h.Sum(nil), nil
}

// Break describes the first place a chain fails to verify.
type Break struct {
	Seq    int64  `json:"seq"`
	Reason string `json:"reason"`
}

// Verify checks a contiguous run of entries that starts after prevHash at
// sequence startSeq. It returns the hash of the last entry, or the first
// break found.
func Verify(entries []Entry, startSeq int64, prevHash []byte) ([]byte, *Break) {
	expectSeq := startSeq
	prev := prevHash
	for _, e := range entries {
		if e.Seq != expectSeq {
			return prev, &Break{Seq: expectSeq, Reason: fmt.Sprintf("entry %d is missing", expectSeq)}
		}
		if !bytes.Equal(e.PrevHash, prev) {
			return prev, &Break{Seq: e.Seq, Reason: "previous hash does not match the prior entry"}
		}
		h, err := Hash(e)
		if err != nil {
			return prev, &Break{Seq: e.Seq, Reason: err.Error()}
		}
		if !bytes.Equal(h, e.Hash) {
			return prev, &Break{Seq: e.Seq, Reason: "contents do not match the recorded hash"}
		}
		prev = e.Hash
		expectSeq++
	}
	return prev, nil
}

// CheckpointMessage is the byte string signed for a checkpoint.
func CheckpointMessage(workspaceID string, seq int64, hash []byte) []byte {
	return []byte(fmt.Sprintf("veto.ledger.checkpoint.v1\n%s\n%d\n%x", workspaceID, seq, hash))
}
