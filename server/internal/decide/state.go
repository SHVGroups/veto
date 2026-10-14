package decide

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/shvgroups/veto/server/internal/canonical"
	"github.com/shvgroups/veto/server/internal/domain"
)

// Size limits that keep the state inside Jev's 32k-token budget for the
// state plus its longest question, with room to spare. Jev also loses
// accuracy as irrelevant state grows, so less is better.
const (
	maxArgsChars      = 24000
	maxRequestChars   = 4000
	defaultUntrusted  = 12000
	maxUntrustedItems = 20
)

// Subject is everything the model is told about one proposed call.
type Subject struct {
	ActionID          string
	Request           string
	Agent             string
	Tool              domain.Tool
	Args              json.RawMessage
	Safer             domain.OptionSpec
	Untrusted         []domain.Untrusted
	UntrustedMaxChars int
}

// BuildState returns the state object sent to the model and the set of
// state keys it contains. Signals whose required keys are missing are not
// asked, so a call without a stated request is never judged on whether it
// matches one.
func BuildState(s Subject) (map[string]any, map[string]bool) {
	state := map[string]any{}
	present := map[string]bool{}

	tool := map[string]any{"name": s.Tool.Name}
	if s.Tool.Description != "" {
		tool["description"] = s.Tool.Description
	}
	state[KeyTool] = tool
	present[KeyTool] = true

	if s.Agent != "" {
		state[KeyAgent] = s.Agent
		present[KeyAgent] = true
	}
	if s.Request != "" {
		state[KeyRequest] = truncate(s.Request, maxRequestChars)
		present[KeyRequest] = true
	}

	state[KeyAction] = map[string]any{"tool": s.Tool.Name, "args": argsForState(s.Args)}
	present[KeyAction] = true

	if s.Safer.Available {
		state[KeySaferVariant] = map[string]any{
			"tool":    s.Safer.Tool,
			"args":    argsForState(s.Safer.Args),
			"changes": s.Safer.Changes,
		}
		present[KeySaferVariant] = true
	}

	limit := s.UntrustedMaxChars
	if limit <= 0 {
		limit = defaultUntrusted
	}
	if items := untrustedForState(s.Untrusted, limit); len(items) > 0 {
		state[KeyUntrusted] = items
		present[KeyUntrusted] = true
	}
	return state, present
}

func argsForState(raw json.RawMessage) any {
	if len(raw) > maxArgsChars {
		return map[string]any{
			"truncated": true,
			"preview":   truncate(string(raw), maxArgsChars),
		}
	}
	v, err := canonical.Decode(raw)
	if err != nil {
		return string(raw)
	}
	return v
}

func untrustedForState(in []domain.Untrusted, limit int) []map[string]any {
	var out []map[string]any
	remaining := limit
	for _, u := range in {
		if remaining <= 0 || len(out) >= maxUntrustedItems {
			break
		}
		content := truncate(u.Content, remaining)
		remaining -= utf8.RuneCountInString(content)
		item := map[string]any{"source": u.Source, "content": content}
		if content != u.Content {
			item["truncated"] = true
		}
		out = append(out, item)
	}
	return out
}

// truncate shortens s to at most n runes.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n])
}
