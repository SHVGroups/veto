package ledger

import (
	"encoding/json"
	"testing"
	"time"
)

func chain(t *testing.T, ws string, n int) []Entry {
	t.Helper()
	prev := Genesis(ws)
	var out []Entry
	base := time.Date(2026, 10, 14, 9, 0, 0, 123456789, time.UTC)
	for i := 1; i <= n; i++ {
		e := Entry{
			WorkspaceID: ws, Seq: int64(i), Kind: KindActionDecided, ActionID: "act_x", Actor: "model",
			Payload:  json.RawMessage(`{"option":"proceed","confidence":0.91,"n":` + string(rune('0'+i)) + `}`),
			PrevHash: prev, CreatedAt: base.Add(time.Duration(i) * time.Second),
		}
		h, err := Hash(e)
		if err != nil {
			t.Fatal(err)
		}
		e.Hash = h
		prev = h
		out = append(out, e)
	}
	return out
}

func TestVerifyIntactChain(t *testing.T) {
	entries := chain(t, "ws_1", 5)
	last, brk := Verify(entries, 1, Genesis("ws_1"))
	if brk != nil {
		t.Fatalf("break %+v", brk)
	}
	if string(last) != string(entries[4].Hash) {
		t.Fatal("wrong head hash")
	}
}

func TestVerifyIgnoresStorageReformatting(t *testing.T) {
	entries := chain(t, "ws_1", 2)
	// Postgres jsonb reorders keys and keeps the precision written.
	entries[0].Payload = json.RawMessage(`{"n": 1, "option": "proceed", "confidence": 0.910}`)
	if _, brk := Verify(entries, 1, Genesis("ws_1")); brk != nil {
		t.Fatalf("break %+v", brk)
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	tests := map[string]func([]Entry) []Entry{
		"edited payload": func(e []Entry) []Entry {
			e[2].Payload = json.RawMessage(`{"option":"stop","confidence":0.91,"n":3}`)
			return e
		},
		"deleted entry": func(e []Entry) []Entry { return append(e[:2], e[3:]...) },
		"edited actor":  func(e []Entry) []Entry { e[1].Actor = "user:usr_evil"; return e },
		"reordered":     func(e []Entry) []Entry { e[1], e[2] = e[2], e[1]; return e },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, brk := Verify(mutate(chain(t, "ws_1", 5)), 1, Genesis("ws_1")); brk == nil {
				t.Fatal("tampering not detected")
			}
		})
	}
}

func TestChainsAreWorkspaceSpecific(t *testing.T) {
	entries := chain(t, "ws_1", 2)
	for i := range entries {
		entries[i].WorkspaceID = "ws_2"
	}
	if _, brk := Verify(entries, 1, Genesis("ws_1")); brk == nil {
		t.Fatal("entries moved between workspaces still verify")
	}
}
