// Package canonical produces a deterministic JSON encoding so that the same
// tool call always hashes to the same value, regardless of key order or
// number formatting in the request that carried it.
//
// The encoding follows RFC 8785 (JSON Canonicalization Scheme) with one
// deliberate difference: integer literals are kept exact at any size instead
// of being rounded through IEEE 754 doubles, because tool arguments such as
// amounts in minor units or 64-bit identifiers must never change value.
package canonical

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// argsHashDomain separates argument hashes from any other SHA-256 use.
const argsHashDomain = "veto.args.v1\n"

// Decode parses JSON into the generic form Marshal accepts, keeping numbers as
// json.Number so no precision is lost. Trailing data after the first value is
// an error.
func Decode(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("canonical: unexpected data after JSON value")
	}
	return v, nil
}

// Marshal returns the canonical encoding of v. v must be composed of the types
// produced by Decode: map[string]any, []any, string, json.Number, bool, nil.
// float64 and the integer types are accepted for convenience.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := encode(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ArgsHash returns the SHA-256 digest that binds a tool name to its exact
// arguments. Grants, idempotency and the audit ledger all refer to calls by
// this hash.
func ArgsHash(tool string, args any) ([]byte, error) {
	body, err := Marshal(args)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write([]byte(argsHashDomain))
	h.Write([]byte(tool))
	h.Write([]byte{'\n'})
	h.Write(body)
	return h.Sum(nil), nil
}

// ArgsHashJSON decodes raw JSON arguments and returns their ArgsHash.
func ArgsHashJSON(tool string, raw []byte) ([]byte, error) {
	v, err := Decode(raw)
	if err != nil {
		return nil, err
	}
	return ArgsHash(tool, v)
}

func encode(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if x {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		encodeString(buf, x)
	case json.Number:
		s, err := normalizeNumber(string(x))
		if err != nil {
			return err
		}
		buf.WriteString(s)
	case float64:
		s, err := formatFloat(x)
		if err != nil {
			return err
		}
		buf.WriteString(s)
	case int:
		buf.WriteString(strconv.Itoa(x))
	case int64:
		buf.WriteString(strconv.FormatInt(x, 10))
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encode(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, compareUTF16)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			encodeString(buf, k)
			buf.WriteByte(':')
			if err := encode(buf, x[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("canonical: unsupported type %T", v)
	}
	return nil
}

// compareUTF16 orders strings by UTF-16 code units, as RFC 8785 requires.
func compareUTF16(a, b string) int {
	return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
}

func encodeString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			buf.WriteString(`\"`)
		case r == '\\':
			buf.WriteString(`\\`)
		case r == '\b':
			buf.WriteString(`\b`)
		case r == '\f':
			buf.WriteString(`\f`)
		case r == '\n':
			buf.WriteString(`\n`)
		case r == '\r':
			buf.WriteString(`\r`)
		case r == '\t':
			buf.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(buf, `\u%04x`, r)
		case r == utf8.RuneError:
			buf.WriteString("�")
		default:
			buf.WriteRune(r)
		}
	}
	buf.WriteByte('"')
}

// normalizeNumber renders a JSON number literal canonically. Integer literals
// are reproduced exactly; anything with a fraction or exponent is formatted
// the way ECMAScript formats doubles.
func normalizeNumber(lit string) (string, error) {
	if !strings.ContainsAny(lit, ".eE") {
		n, ok := new(big.Int).SetString(lit, 10)
		if !ok {
			return "", fmt.Errorf("canonical: invalid number %q", lit)
		}
		return n.String(), nil
	}
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil {
		return "", fmt.Errorf("canonical: invalid number %q: %w", lit, err)
	}
	return formatFloat(f)
}

func formatFloat(f float64) (string, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", errors.New("canonical: NaN and Inf are not valid JSON")
	}
	if f == 0 {
		return "0", nil
	}
	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	mantissa, exp, _ := strings.Cut(s, "e")
	sign := exp[0]
	digits := strings.TrimLeft(exp[1:], "0")
	return mantissa + "e" + string(sign) + digits, nil
}
