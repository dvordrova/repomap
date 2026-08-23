package llm

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeJSONAcceptsOneUnambiguousObjectOrArray(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want string
	}{
		"whitespace object": {
			raw: "  \n{\"value\":1}\t ", want: `{"value":1}`,
		},
		"array": {
			raw: "\n[1,{\"value\":2}]\n", want: `[1,{"value":2}]`,
		},
		"json fence": {
			raw: "```json\n{\"value\":1}\n```", want: `{"value":1}`,
		},
		"plain fence with preamble": {
			raw:  "Here is the bounded result:\n```\n[{\"value\":1}]\n```\n",
			want: `[{"value":1}]`,
		},
		"unclosed fence with complete root": {
			raw: "```json\n{\"value\":1}", want: `{"value":1}`,
		},
		"leading prose": {
			raw: "The result follows:\n{\"value\":1}\n", want: `{"value":1}`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			normalized, err := NormalizeJSON([]byte(test.raw))
			if err != nil {
				t.Fatal(err)
			}
			if string(normalized) != test.want {
				t.Fatalf("normalized = %q, want %q", normalized, test.want)
			}
		})
	}
}

func TestNormalizeJSONRejectsAmbiguityGarbageAndTruncation(t *testing.T) {
	tests := map[string]string{
		"scalar":                   `"value"`,
		"multiple objects":         `{"first":1} {"second":2}`,
		"trailing prose":           `{"value":1} done`,
		"trailing delimiter":       `{"value":1}}`,
		"truncated outer":          `{"outer":{"value":1}`,
		"unclosed truncated fence": "```json\n{\"value\":1",
		"fence trailing prose":     "```json\n{\"value\":1}\n```\ndone",
		"two fenced values":        "```json\n{}\n```\n```json\n[]\n```",
		"competing prefix value":   "{\"first\":1}\n```json\n{\"second\":2}\n```",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if normalized, err := NormalizeJSON([]byte(raw)); err == nil {
				t.Fatalf("NormalizeJSON() = %q, want rejection", normalized)
			}
		})
	}
}

func TestNormalizeJSONDoesNotRepairRefsSchemaOrValues(t *testing.T) {
	raw := []byte(`{"unknown_ref":"t999","extra":{"label":"verbatim"}}`)
	normalized, err := NormalizeJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(normalized, raw) {
		t.Fatalf("syntax boundary changed semantic bytes:\n got %s\nwant %s", normalized, raw)
	}

	type refsOnly struct {
		Ref string `json:"ref"`
	}
	if _, err := DecodeJSON[refsOnly](nil)(normalized); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("default decoder repaired unknown schema: %v", err)
	}
	decode := DecodeJSON(func(value refsOnly) error {
		if value.Ref != "t1" {
			return errors.New("unknown request-local ref")
		}
		return nil
	})
	if _, err := decode([]byte(`{"ref":"t999"}`)); err == nil {
		t.Fatal("validator repaired unknown ref")
	}
}
