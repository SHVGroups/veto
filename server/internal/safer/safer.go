// Package safer builds option B, the constrained variant of a proposed call.
//
// Each tool carries an ordered list of rules. Rules are plain data so admins
// can edit them in the console, and every rule produces a one-line
// description of what it changed so reviewers see the difference without
// reading JSON. No model is involved: option B is always built by code.
package safer

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"slices"
	"strings"

	"github.com/shvgroups/veto/server/internal/canonical"
	"github.com/shvgroups/veto/server/internal/domain"
)

// Op names a rule type.
type Op string

const (
	// OpSwapTool runs a different, safer tool with the same arguments, such
	// as creating a draft instead of sending.
	OpSwapTool Op = "swap_tool"
	// OpCapNumber lowers a numeric argument to a maximum.
	OpCapNumber Op = "cap_number"
	// OpFilterDomains removes email addresses outside the allowed domains
	// from a string or list argument.
	OpFilterDomains Op = "filter_domains"
	// OpSet sets an argument to a fixed value.
	OpSet Op = "set"
	// OpRemove deletes an argument.
	OpRemove Op = "remove"
	// OpLimitItems keeps only the first N items of a list argument.
	OpLimitItems Op = "limit_items"
)

// Rule is one step in a tool's safer-variant recipe.
type Rule struct {
	Op    Op              `json:"op"`
	Tool  string          `json:"tool,omitempty"`
	Path  string          `json:"path,omitempty"`
	Max   json.Number     `json:"max,omitempty"`
	Allow []string        `json:"allow,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// ParseRules decodes and validates a rule list.
func ParseRules(raw []byte) ([]Rule, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var rules []Rule
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&rules); err != nil {
		return nil, fmt.Errorf("safer: invalid rules: %w", err)
	}
	for i, r := range rules {
		if err := r.validate(); err != nil {
			return nil, fmt.Errorf("safer: rule %d: %w", i+1, err)
		}
	}
	return rules, nil
}

func (r Rule) validate() error {
	needPath := func() error {
		if r.Path == "" {
			return fmt.Errorf("%s needs a path", r.Op)
		}
		for _, seg := range strings.Split(r.Path, ".") {
			if seg == "" {
				return fmt.Errorf("path %q has an empty segment", r.Path)
			}
		}
		return nil
	}
	switch r.Op {
	case OpSwapTool:
		if r.Tool == "" {
			return errors.New("swap_tool needs a tool")
		}
	case OpCapNumber:
		if err := needPath(); err != nil {
			return err
		}
		if _, ok := new(big.Float).SetString(r.Max.String()); !ok || r.Max == "" {
			return errors.New("cap_number needs a numeric max")
		}
	case OpFilterDomains:
		if err := needPath(); err != nil {
			return err
		}
		if len(r.Allow) == 0 {
			return errors.New("filter_domains needs at least one allowed domain")
		}
	case OpSet:
		if err := needPath(); err != nil {
			return err
		}
		if len(r.Value) == 0 {
			return errors.New("set needs a value")
		}
	case OpRemove:
		return needPath()
	case OpLimitItems:
		if err := needPath(); err != nil {
			return err
		}
		n, err := r.Max.Int64()
		if err != nil || n < 0 {
			return errors.New("limit_items needs a non-negative integer max")
		}
	default:
		return fmt.Errorf("unknown op %q", r.Op)
	}
	return nil
}

// Build applies the rules to a proposed call and returns option B. Option B
// is unavailable when there are no rules or when the rules change nothing.
func Build(tool string, args json.RawMessage, rules []Rule) (domain.OptionSpec, error) {
	spec := domain.OptionSpec{Key: domain.OptionProceedSafer}
	if len(rules) == 0 {
		return spec, nil
	}
	v, err := canonical.Decode(args)
	if err != nil {
		return spec, fmt.Errorf("safer: decode args: %w", err)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return spec, errors.New("safer: args must be a JSON object")
	}
	outTool := tool
	var changes []string
	for _, r := range rules {
		change, err := apply(r, &outTool, obj)
		if err != nil {
			return spec, err
		}
		if change != "" {
			changes = append(changes, change)
		}
	}
	if len(changes) == 0 {
		return spec, nil
	}
	outArgs, err := canonical.Marshal(obj)
	if err != nil {
		return spec, err
	}
	same, err := sameCall(tool, args, outTool, outArgs)
	if err != nil || same {
		return spec, err
	}
	spec.Available = true
	spec.Tool = outTool
	spec.Args = outArgs
	spec.Changes = changes
	return spec, nil
}

func sameCall(toolA string, argsA []byte, toolB string, argsB []byte) (bool, error) {
	if toolA != toolB {
		return false, nil
	}
	ha, err := canonical.ArgsHashJSON(toolA, argsA)
	if err != nil {
		return false, err
	}
	hb, err := canonical.ArgsHashJSON(toolB, argsB)
	if err != nil {
		return false, err
	}
	return string(ha) == string(hb), nil
}

func apply(r Rule, tool *string, obj map[string]any) (string, error) {
	switch r.Op {
	case OpSwapTool:
		if *tool == r.Tool {
			return "", nil
		}
		change := fmt.Sprintf("Runs %s instead of %s", r.Tool, *tool)
		*tool = r.Tool
		return change, nil

	case OpCapNumber:
		cur, ok := lookup(obj, r.Path)
		if !ok {
			return "", nil
		}
		curNum, ok := asBigFloat(cur)
		if !ok {
			return "", nil
		}
		maxNum, _ := new(big.Float).SetString(r.Max.String())
		if curNum.Cmp(maxNum) <= 0 {
			return "", nil
		}
		assign(obj, r.Path, r.Max)
		return fmt.Sprintf("Caps %s at %s (was %s)", r.Path, r.Max, format(cur)), nil

	case OpFilterDomains:
		cur, ok := lookup(obj, r.Path)
		if !ok {
			return "", nil
		}
		kept, removed := filterDomains(cur, r.Allow)
		if len(removed) == 0 {
			return "", nil
		}
		assign(obj, r.Path, kept)
		noun := "address"
		if len(removed) > 1 {
			noun = "addresses"
		}
		return fmt.Sprintf("Removes %d %s from %s outside %s: %s",
			len(removed), noun, r.Path, strings.Join(r.Allow, ", "), strings.Join(removed, ", ")), nil

	case OpSet:
		v, err := canonical.Decode(r.Value)
		if err != nil {
			return "", fmt.Errorf("safer: set %s: %w", r.Path, err)
		}
		cur, existed := lookup(obj, r.Path)
		if existed && equalJSON(cur, v) {
			return "", nil
		}
		assign(obj, r.Path, v)
		if existed {
			return fmt.Sprintf("Sets %s to %s (was %s)", r.Path, format(v), format(cur)), nil
		}
		return fmt.Sprintf("Sets %s to %s", r.Path, format(v)), nil

	case OpRemove:
		if _, ok := lookup(obj, r.Path); !ok {
			return "", nil
		}
		remove(obj, r.Path)
		return fmt.Sprintf("Removes %s", r.Path), nil

	case OpLimitItems:
		cur, ok := lookup(obj, r.Path)
		if !ok {
			return "", nil
		}
		list, ok := cur.([]any)
		limit, _ := r.Max.Int64()
		if !ok || int64(len(list)) <= limit {
			return "", nil
		}
		assign(obj, r.Path, slices.Clone(list[:limit]))
		return fmt.Sprintf("Keeps the first %d of %d items in %s", limit, len(list), r.Path), nil
	}
	return "", fmt.Errorf("safer: unknown op %q", r.Op)
}

func lookup(obj map[string]any, path string) (any, bool) {
	segs := strings.Split(path, ".")
	cur := any(obj)
	for _, s := range segs {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[s]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func assign(obj map[string]any, path string, v any) {
	segs := strings.Split(path, ".")
	m := obj
	for _, s := range segs[:len(segs)-1] {
		next, ok := m[s].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[s] = next
		}
		m = next
	}
	m[segs[len(segs)-1]] = v
}

func remove(obj map[string]any, path string) {
	segs := strings.Split(path, ".")
	m := obj
	for _, s := range segs[:len(segs)-1] {
		next, ok := m[s].(map[string]any)
		if !ok {
			return
		}
		m = next
	}
	delete(m, segs[len(segs)-1])
}

func asBigFloat(v any) (*big.Float, bool) {
	switch n := v.(type) {
	case json.Number:
		f, ok := new(big.Float).SetString(n.String())
		return f, ok
	case float64:
		return big.NewFloat(n), true
	}
	return nil, false
}

// filterDomains keeps addresses whose domain is allowed. It accepts a single
// string, a comma-separated string or a list of strings; other values are
// left untouched.
func filterDomains(v any, allow []string) (any, []string) {
	switch x := v.(type) {
	case string:
		parts := strings.Split(x, ",")
		var kept, removed []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if domainAllowed(p, allow) {
				kept = append(kept, p)
			} else {
				removed = append(removed, p)
			}
		}
		return strings.Join(kept, ", "), removed
	case []any:
		kept := make([]any, 0, len(x))
		var removed []string
		for _, e := range x {
			s, ok := e.(string)
			if ok && !domainAllowed(s, allow) {
				removed = append(removed, s)
				continue
			}
			kept = append(kept, e)
		}
		return kept, removed
	}
	return v, nil
}

func domainAllowed(addr string, allow []string) bool {
	if parsed, err := mail.ParseAddress(addr); err == nil {
		addr = parsed.Address
	}
	at := strings.LastIndexByte(addr, '@')
	if at < 0 {
		return false
	}
	d := strings.ToLower(strings.TrimSpace(addr[at+1:]))
	for _, a := range allow {
		a = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(a), "@"))
		if d == a || strings.HasSuffix(d, "."+a) {
			return true
		}
	}
	return false
}

func equalJSON(a, b any) bool {
	ea, err1 := canonical.Marshal(a)
	eb, err2 := canonical.Marshal(b)
	return err1 == nil && err2 == nil && string(ea) == string(eb)
}

func format(v any) string {
	b, err := canonical.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	s := string(b)
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}
