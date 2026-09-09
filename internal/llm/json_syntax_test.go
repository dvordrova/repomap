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
		"thinking with code fence": {
			raw:  "<think>\n```python\nvalue = 1\n```\n</think>\n```json\n{\"value\":1}\n```",
			want: `{"value":1}`,
		},
		"thinking with draft JSON": {
			raw:  " \n<think>Consider {\"value\":0} and [2].</think>\n{\"value\":1}",
			want: `{"value":1}`,
		},
		"empty thinking": {
			raw: "<think>\n\n</think>\n[1,2]", want: `[1,2]`,
		},
		"thinking then leading prose": {
			raw:  "<think>Consider [2].</think>\nThe result follows:\n{\"value\":1}",
			want: `{"value":1}`,
		},
		"literal thinking tags in JSON": {
			raw: `{"value":"<think>literal</think>"}`, want: `{"value":"<think>literal</think>"}`,
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
		"scalar":                       `"value"`,
		"multiple objects":             `{"first":1} {"second":2}`,
		"trailing prose":               `{"value":1} done`,
		"fence trailing prose":         "```json\n{\"value\":1}\n```\ndone",
		"two fenced values":            "```json\n{}\n```\n```json\n[]\n```",
		"competing prefix value":       "{\"first\":1}\n```json\n{\"second\":2}\n```",
		"unclosed thinking with JSON":  "<think>Consider:\n{\"value\":1}",
		"wrong thinking close":         "<think>Consider [2].<think/>\n{\"value\":1}",
		"thinking without answer":      "<think>{\"draft\":1}</think>",
		"nested thinking":              "<think><think>draft</think>\n{\"value\":1}",
		"repeated thinking":            "<think>first</think><think>second</think>\n{\"value\":1}",
		"thinking then two values":     "<think>draft</think>\n{}\n[]",
		"thinking then trailing prose": "<think>draft</think>\n{}\ndone",
		"thinking then non-JSON fence": "<think>draft</think>\n```python\n{}\n```",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if normalized, err := NormalizeJSON([]byte(raw)); err == nil {
				t.Fatalf("NormalizeJSON() = %q, want rejection", normalized)
			}
		})
	}
}

func TestNormalizeJSONBalancesOnlyStructuralBrackets(t *testing.T) {
	tests := map[string]struct{ raw, want string }{
		"extra root closer": {`{"value":1}}`, `{"value":1} `},
		"extra array closer inside object": {
			`{"result":{"rows":[{"key":"r1","line":"x"}]}],"terms":[]}`,
			`{"result":{"rows":[{"key":"r1","line":"x"}]} ,"terms":[]}`,
		},
		"missing outer closer":      {`{"outer":{"value":1}`, `{"outer":{"value":1}}`},
		"missing mixed closers":     {`{"rows":[{"values":[1,true,null`, `{"rows":[{"values":[1,true,null]}]}`},
		"extra and missing closers": {`{"a":1],"b":[2`, `{"a":1 ,"b":[2]}`},
		"empty nested structures":   {`{"rows":[{`, `{"rows":[{}]}`},
		"fenced":                    {"```json\n{\"value\":1\n```", "{\"value\":1}"},
		"unclosed fence":            {"```json\n{\"value\":1", `{"value":1}`},
		"thinking":                  {"<think>{\"draft\":1}</think>\n{\"value\":1", `{"value":1}`},
		"literal brackets and escaped quotes": {
			`{"value":"[}] \\\" \\"`,
			`{"value":"[}] \\\" \\"}`,
		},
		"literal backticks": {"{\"value\":\"```\"", "{\"value\":\"```\"}"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			raw := []byte(test.raw)
			normalized, err := NormalizeJSON(raw)
			if err != nil || string(normalized) != test.want {
				t.Fatalf("normalized = %q, %v; want %q", normalized, err, test.want)
			}
			if string(raw) != test.raw {
				t.Fatal("changed original provider response")
			}
		})
	}
}

func TestNormalizeJSONBracketBalanceCannotInventValues(t *testing.T) {
	for _, raw := range []string{
		`{"a":`, `[tru`, `[1e`, `{"a":"unfinished`, `{"a":"unfinished\`,
		`{"a":[1}`, `{"a":[{"b":1]}`, `{"a":1,`,
		`[1}2]`, `[t}rue]`, `[1}e2]`, `[n}ull]`,
		`{"a":1 "b":2`, `{"a":1}]} done`, `{}]}[]`,
	} {
		t.Run(raw, func(t *testing.T) {
			if normalized, err := NormalizeJSON([]byte(raw)); err == nil {
				t.Fatalf("accepted %q as %q", raw, normalized)
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
	for _, raw := range []string{`{"ref":"t999"}`, `{"ref":"t999"`, `{`} {
		if _, err := decode([]byte(raw)); err == nil {
			t.Fatalf("validator repaired unknown or missing ref in %q", raw)
		}
	}
}

func TestExecuteJSONBalancesBracketsWithoutChangingRawCache(t *testing.T) {
	provider := baseTestProvider()
	raw := []byte(`{"value":"ok"}]`)
	provider.responses = [][]byte{raw}
	call := baseTestCall("brackets", "complete value with an extra closer")
	executor := Executor{RootDir: t.TempDir(), Enabled: true}
	for i := range 2 {
		out, err := ExecuteJSON(t.Context(), executor, provider, call)
		if err != nil || out.Value.Value != "ok" || out.Cached != (i == 1) {
			t.Fatalf("run %d: %#v, %v", i, out, err)
		}
		if !bytes.Equal(out.Response, raw) {
			t.Fatalf("run %d changed the original response: %q", i, out.Response)
		}
	}
	if provider.completeCalls != 1 {
		t.Fatalf("syntax normalization needed %d provider calls", provider.completeCalls)
	}
	// A valid bracket shape cannot supply a missing required value.
	provider.responses = [][]byte{[]byte(`{`)}
	call.Prompt.User = "missing required value"
	if _, err := ExecuteJSON(t.Context(), executor, provider, call); err == nil {
		t.Fatal("accepted missing value after closing its root")
	}
	// A provider-reported output cutoff is not a successful completion.
	provider.responses = [][]byte{[]byte(`{"value":"ok"`)}
	provider.finishReasons = []FinishReason{FinishLength}
	call.Prompt.User = "provider reported truncation"
	if _, err := ExecuteJSON(t.Context(), executor, provider, call); err == nil {
		t.Fatal("bracket balancing overrode the provider's output limit")
	}
}
