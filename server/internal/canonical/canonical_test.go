package canonical

import (
	"bytes"
	"testing"
)

func TestMarshalCanonicalForms(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"key order", `{"b":1,"a":2}`, `{"a":2,"b":1}`},
		{"nested", `{"z":{"y":[3,{"b":true,"a":null}]},"a":"x"}`, `{"a":"x","z":{"y":[3,{"a":null,"b":true}]}}`},
		{"whitespace", "{ \"a\" :\n [ 1 , 2 ] }", `{"a":[1,2]}`},
		{"integer exact beyond float64", `{"n":12345678901234567890123}`, `{"n":12345678901234567890123}`},
		{"negative zero integer", `-0`, `0`},
		{"float trailing zero", `1.50`, `1.5`},
		{"float integral", `2.0`, `2`},
		{"small float uses exponent", `0.0000001`, `1e-7`},
		{"boundary decimal", `0.000001`, `0.000001`},
		{"large float uses exponent", `1.5e21`, `1.5e+21`},
		{"escapes", `"a\"b\\c\n\u0001 <>&"`, "\"a\\\"b\\\\c\\n\\u0001 <>&\""},
		// U+10000 sorts after U+FFFF by code point but before it in UTF-16,
		// which is the order RFC 8785 requires.
		{"utf16 key order", "{\"\uffff\":2,\"\U00010000\":1}", "{\"\U00010000\":1,\"\uffff\":2}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := Decode([]byte(tt.in))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			got, err := Marshal(v)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal(%s)\n got  %s\n want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestDecodeRejectsTrailingData(t *testing.T) {
	if _, err := Decode([]byte(`{"a":1} {"b":2}`)); err == nil {
		t.Fatal("expected error for trailing data")
	}
}

func TestArgsHashIgnoresFormattingButNotValues(t *testing.T) {
	h1, err := ArgsHashJSON("gmail.send", []byte(`{"to":["a@x.io"],"subject":"Hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	h2, err := ArgsHashJSON("gmail.send", []byte(`{ "subject" : "Hi", "to" : [ "a@x.io" ] }`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(h1, h2) {
		t.Fatal("hash changed with formatting")
	}
	h3, _ := ArgsHashJSON("gmail.send", []byte(`{"to":["b@x.io"],"subject":"Hi"}`))
	if bytes.Equal(h1, h3) {
		t.Fatal("hash did not change with a different argument")
	}
	h4, _ := ArgsHashJSON("gmail.create_draft", []byte(`{"to":["a@x.io"],"subject":"Hi"}`))
	if bytes.Equal(h1, h4) {
		t.Fatal("hash did not change with a different tool")
	}
}
