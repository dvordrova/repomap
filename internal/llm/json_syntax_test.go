package llm

import (
	"bytes"
	"encoding/json"
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
		"thinking then trailing prose": {
			raw: "<think>draft</think>\n{}\ndone", want: `{}`,
		},
		"thinking then fence tagged as another language": {
			raw: "<think>draft</think>\n```python\n{}\n```", want: `{}`,
		},
		"identical second fence": {
			raw:  "```json\n{\"value\":1}\n```\nThe same again:\n```json\n{ \"value\": 1 }\n```\n",
			want: `{"value":1}`,
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

// Each relaxed form is paired with the nearest answer that stays refused:
// the relaxation discards or joins only what cannot change a value.
func TestNormalizeJSONRelaxedFormsKeepTheirRefusals(t *testing.T) {
	for _, test := range []struct{ name, accepted, want, refused string }{
		{"crossed closer after the last row",
			`{"rows":[{"key":"r1"}}]}`, `{"rows":[{"key":"r1"}]}`,
			// Deleting the stray } gives [{"a":[1,2]}]; closing [ first gives [{"a":[1]},2].
			`[{"a":[1}, 2]`},
		{"crossed closer at EOF",
			`{"rows":[{"key":"r1"}}`, `{"rows":[{"key":"r1"}]}`,
			// Deleting the closer leaves no valid reading.
			`{"a":[1},"b":2]`},
		{"crossed object closer inside an array",
			`{"a":[1}`, `{"a":[1]}`,
			// A closer with no opener of its kind still never merges tokens.
			`[1}2]`},
		{"crossed array closer inside an object",
			`{"a":[{"b":1]}`, `{"a":[{"b":1}]}`,
			`{"a":[{"b":1],"c":2}`},
		{"prose after the root",
			"{\"a\":1}\nDone.", `{"a":1}`,
			// A structural tail could hide a dropped field.
			`{"a":1}], "b":2}`},
		{"repeated identical root",
			"{\"a\":1} {\"a\" : 1}\n", `{"a":1}`,
			`{"a":1} {"b":2}`},
		{"bracketed prose before a fence",
			"Rows for [target]:\n```json\n{\"a\":1}\n```", `{"a":1}`,
			"{\"first\":1}\n```json\n{\"a\":1}\n```"},
		{"inline fence",
			"```json{\"a\":1}```", `{"a":1}`,
			"```json {\"a\":1} {\"b\":2}```"},
		{"fence tag other than json",
			"```jsonc\n{\"a\":1}\n```", `{"a":1}`,
			"```python\nvalue = {\"a\": 1}\n```"},
		{"prose after the closing fence",
			"```json\n{\"a\":1}\n```\nHope this helps.", `{"a":1}`,
			"```json\n{\"a\":1}\n```\n```json\n{\"a\":2}\n```"},
		{"prose after a fence that closes an unfinished root",
			"```json\n{\"a\":[1,2\n```\nDone.", `{"a":[1,2]}`,
			// The fence may belong to the unfinished string.
			"```json\n{\"v\":\"a ``` b\"}\n```"},
		{"repeated complete thinking blocks",
			"<think>a</think><think>b</think>\n{\"a\":1}", `{"a":1}`,
			"<think>a</think><think>b\n{\"a\":1}"},
		{"nested complete thinking blocks",
			"<think>a<think>b</think>c</think>{\"a\":1}", `{"a":1}`,
			"<think><think>draft</think>\n{\"a\":1}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			normalized, err := NormalizeJSON([]byte(test.accepted))
			if err != nil || compactJSON(t, normalized) != test.want {
				t.Fatalf("NormalizeJSON(%q) = %q, %v; want %s", test.accepted, normalized, err, test.want)
			}
			if normalized, err := NormalizeJSON([]byte(test.refused)); err == nil {
				t.Fatalf("NormalizeJSON(%q) = %q, want rejection", test.refused, normalized)
			}
		})
	}
}

// The journaled answers of 2026-09 that were refused for a doubled closer,
// anonymized to their row keys. In all but one, deleting the closer and closing
// the inner array before it give the same rows. The operations answer places
// "terms" inside or beside "result" depending on the reading, so it stays refused.
func TestNormalizeJSONJournaledDoubledClosers(t *testing.T) {
	for i, fixture := range []struct {
		stage, raw string
		accepted   bool
	}{
		{"atlas_operations", `{"result":{"rows":[{"key":"r1","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r2","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r3","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r4","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r5","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r6","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r7","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r8","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r9","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r10","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r11","entry":"w","activation":"w","name":"w","description":"w"},{"key":"r12","entry":"none","activation":"none","name":"none","description":"none"}}],"terms":[]}}`, false},
		{"atlas_question", `{"rows":[{"key":"r1","relevance":"context","anchors":"a22","why":"w"},{"key":"r2","relevance":"context","anchors":"w","why":"w"},{"key":"r3","relevance":"none","anchors":"none","why":"w"},{"key":"r4","relevance":"none","anchors":"none","why":"w"},{"key":"r5","relevance":"none","anchors":"none","why":"w"},{"key":"r6","relevance":"context","anchors":"a12","why":"w"},{"key":"r7","relevance":"none","anchors":"none","why":"w"},{"key":"r8","relevance":"none","anchors":"none","why":"w"},{"key":"r9","relevance":"none","anchors":"none","why":"w"},{"key":"r10","relevance":"direct","anchors":"w","why":"w"},{"key":"r11","relevance":"direct","anchors":"w","why":"w"},{"key":"r12","relevance":"context","anchors":"a5","why":"w"}}]}`, true},
		{"atlas_question", `{"rows":[{"key":"r1","relevance":"none","anchors":"none","why":"w"},{"key":"r2","relevance":"none","anchors":"none","why":"w"},{"key":"r3","relevance":"none","anchors":"none","why":"w"},{"key":"r4","relevance":"none","anchors":"none","why":"w"},{"key":"r5","relevance":"none","anchors":"none","why":"w"},{"key":"r6","relevance":"none","anchors":"none","why":"w"}}]}`, true},
		{"atlas_question", `{"rows":[{"key":"r1","relevance":"none","anchors":"none","why":"w"},{"key":"r2","relevance":"none","anchors":"none","why":"w"},{"key":"r3","relevance":"direct","anchors":"a1","why":"w"},{"key":"r4","relevance":"context","anchors":"a1","why":"w"},{"key":"r5","relevance":"none","anchors":"none","why":"w"},{"key":"r6","relevance":"none","anchors":"none","why":"w"},{"key":"r7","relevance":"direct","anchors":"a1","why":"w"}}]`, true},
		{"atlas_question", `{"rows":[{"key":"r1","relevance":"context","anchors":"w","why":"w"},{"key":"r2","relevance":"none","anchors":"none","why":"w"},{"key":"r3","relevance":"context","anchors":"w","why":"w"},{"key":"r4","relevance":"context","anchors":"w","why":"w"},{"key":"r5","relevance":"direct","anchors":"w","why":"w"},{"key":"r6","relevance":"direct","anchors":"w","why":"w"}}]}`, true},
		{"atlas_question", `{"rows":[{"key":"r1","relevance":"direct","anchors":"w","why":"w"},{"key":"r2","relevance":"context","anchors":"w","why":"w"},{"key":"r3","relevance":"none","anchors":"none","why":"w"},{"key":"r4","relevance":"none","anchors":"none","why":"w"},{"key":"r5","relevance":"none","anchors":"none","why":"w"},{"key":"r6","relevance":"direct","anchors":"w","why":"w"},{"key":"r7","relevance":"none","anchors":"none","why":"w"},{"key":"r8","relevance":"none","anchors":"none","why":"w"},{"key":"r9","relevance":"none","anchors":"none","why":"w"},{"key":"r10","relevance":"context","anchors":"w","why":"w"},{"key":"r11","relevance":"direct","anchors":"w","why":"w"},{"key":"r12","relevance":"context","anchors":"w","why":"w"}}]}`, true},
		{"atlas_question", `{"rows":[{"key":"r1","relevance":"none","anchor":"none","why":"w"},{"key":"r2","relevance":"none","anchor":"none","why":"w"},{"key":"r3","relevance":"none","anchor":"none","why":"w"},{"key":"r4","relevance":"none","anchor":"none","why":"w"},{"key":"r5","relevance":"none","anchor":"none","why":"w"},{"key":"r6","relevance":"none","anchor":"none","why":"w"},{"key":"r7","relevance":"context","anchor":"a4","why":"w"},{"key":"r8","relevance":"none","anchor":"none","why":"w"},{"key":"r9","relevance":"none","anchor":"none","why":"w"},{"key":"r10","relevance":"none","anchor":"none","why":"w"},{"key":"r11","relevance":"none","anchor":"none","why":"w"},{"key":"r12","relevance":"direct","anchor":"a15","why":"w"}}]}`, true},
		{"atlas_question", `{"rows":[{"key":"r1","relevance":"none","anchor":"none","why":"w"},{"key":"r2","relevance":"none","anchor":"none","why":"w"},{"key":"r3","relevance":"none","anchor":"none","why":"w"},{"key":"r4","relevance":"none","anchor":"none","why":"w"},{"key":"r5","relevance":"none","anchor":"none","why":"w"},{"key":"r6","relevance":"none","anchor":"none","why":"w"},{"key":"r7","relevance":"none","anchor":"none","why":"w"},{"key":"r8","relevance":"none","anchor":"none","why":"w"},{"key":"r9","relevance":"none","anchor":"none","why":"w"},{"key":"r10","relevance":"none","anchor":"none","why":"w"},{"key":"r11","relevance":"none","anchor":"none","why":"w"},{"key":"r12","relevance":"direct","anchor":"a6","why":"w"}}]}`, true},
		{"atlas_route", `{"rows":[{"key":"r1","order":"w","open_question":"w"}}]}`, true},
		{"atlas_route", `{"rows":[{"key":"r1","order":"w","summary":"w","open_question":"w"}}]}`, true},
		{"atlas_route", `{"rows":[{"key":"r1","order":"w","summary":"w","open_question":"w"}}]}`, true},
		{"atlas_route", `{"rows":[{"key":"r1","order":"w","summary":"w","open_question":"w"}}`, true},
	} {
		normalized, err := NormalizeJSON([]byte(fixture.raw))
		if (err == nil) != fixture.accepted {
			t.Fatalf("%d %s: accepted=%t (%v), want %t", i, fixture.stage, err == nil, err, fixture.accepted)
		}
		if err != nil {
			continue
		}
		// Only the doubled closer goes and the missing closers are appended:
		// every row survives with its own fields.
		want := strings.Replace(fixture.raw, "}}", "}", 1)
		want = strings.TrimSuffix(strings.TrimSuffix(want, "]}"), "]") + "]}"
		if got := compactJSON(t, normalized); got != compactJSON(t, []byte(want)) {
			t.Fatalf("%d %s: normalized = %s, want %s", i, fixture.stage, got, want)
		}
	}
}

func compactJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return "invalid: " + err.Error()
	}
	return out.String()
}

func TestNormalizeJSONRejectsAmbiguityGarbageAndTruncation(t *testing.T) {
	tests := map[string]string{
		"scalar":                       `"value"`,
		"multiple different objects":   `{"first":1} {"second":2}`,
		"cut repeated root":            `{"value":1} {"value":1`,
		"member continued after root":  `{"value":1}, "other":2`,
		"string after root":            `{"value":1} "other"`,
		"member continued after fence": "```json\n{\"value\":1}\n```\n, \"other\":2",
		"two different fenced values":  "```json\n{}\n```\n```json\n[]\n```",
		"competing prefix value":       "{\"first\":1}\n```json\n{\"second\":2}\n```",
		"bracket after fence":          "```json\n{}\n```\nSee [1].",
		"unclosed thinking with JSON":  "<think>Consider:\n{\"value\":1}",
		"wrong thinking close":         "<think>Consider [2].<think/>\n{\"value\":1}",
		"thinking without answer":      "<think>{\"draft\":1}</think>",
		"unbalanced nested thinking":   "<think><think>draft</think>\n{\"value\":1}",
		"thinking then two values":     "<think>draft</think>\n{}\n[]",
		"thinking then non-JSON fence": "<think>draft</think>\n```python\nprint({})\n```",
		"incomplete inline fence":      "```json",
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
		"extra root closer": {`{"value":1}}`, `{"value":1}`},
		"extra array closer inside object": {
			`{"result":{"rows":[{"key":"r1","line":"x"}]}],"terms":[]}`,
			`{"result":{"rows":[{"key":"r1","line":"x"}]} ,"terms":[]}`,
		},
		"missing outer closer":      {`{"outer":{"value":1}`, `{"outer":{"value":1}}`},
		"missing mixed closers":     {`{"rows":[{"values":[1,true,null`, `{"rows":[{"values":[1,true,null]}]}`},
		"extra and missing closers": {`{"a":1],"b":[2`, `{"a":1 ,"b":[2]}`},
		"empty nested structures":   {`{"rows":[{`, `{"rows":[{}]}`},
		"fenced":                    {"```json\n{\"value\":1\n```", "{\"value\":1\n}"},
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
		`{"a":`, `[tru`, `[1e`, `{"a":"unfinished\`, `{"a":1,`,
		`[1}2]`, `[t}rue]`, `[1}e2]`, `[n}ull]`,
		`{"a":1 "b":2`, `{"a":1}]} done`, `{}]}[]`,
		`[{"a":[1}, 2]`, `{"a":[1},"b":2]`,
	} {
		t.Run(raw, func(t *testing.T) {
			if normalized, err := NormalizeJSON([]byte(raw)); err == nil {
				t.Fatalf("accepted %q as %q", raw, normalized)
			}
		})
	}
	// Several crossed closers are each deleted when both readings agree. Each
	// one doubles the readings; past the bound the answer is refused instead
	// of searched.
	crossed := func(n int) string { return `{"a":[` + strings.Repeat(`{"b":[1}]},`, n) + `{}]}` }
	normalized, err := NormalizeJSON([]byte(crossed(3)))
	if err != nil || compactJSON(t, normalized) != `{"a":[{"b":[1]},{"b":[1]},{"b":[1]},{}]}` {
		t.Fatalf("three crossed closers = %q, %v", normalized, err)
	}
	if normalized, err := NormalizeJSON([]byte(crossed(maxCrossedReadings))); err == nil {
		t.Fatalf("searched %d crossed closers: %q", maxCrossedReadings, normalized)
	}
}

func TestNormalizeJSONClosesOnlyAnEOFString(t *testing.T) {
	for _, raw := range []string{
		`{"value":"text  `,
		"<think>draft</think>\n{\"value\":\"text  ",
		"```json\n{\"value\":\"text  ",
	} {
		value, err := DecodeJSON[struct{ Value string }](nil)([]byte(raw))
		if err != nil || value.Value != "text  " {
			t.Fatalf("lost unfinished string bytes in %q: %#v, %v", raw, value, err)
		}
	}
	for _, raw := range []string{
		`{"value":"`, `{"value":"brackets }]`, `{"value":"escaped \"quote`,
		`{"value":"backslash \\`, `{"value":"unicode \u0041`,
	} {
		normalized, err := NormalizeJSON([]byte(raw))
		if err != nil || string(normalized) != raw+`"}` {
			t.Fatalf("EOF normalization of %q = %q, %v", raw, normalized, err)
		}
	}
	for _, raw := range []string{
		`{"value":"escape\`, `{"value":"unicode \u12`, `{"value":"bad \q`,
		`{"key`, `{"value":"text "other":1}`, `{"value":"text" "other":1}`,
		"{\"value\":\"text\n", "<think>draft</think>\n{\"value\":\"text\n",
		"```json\n{\"value\":\"text\n```",
	} {
		if normalized, err := NormalizeJSON([]byte(raw)); err == nil {
			t.Fatalf("guessed missing contents or interior punctuation in %q: %q", raw, normalized)
		}
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
	for _, raw := range []string{`{"ref":"t999"}`, `{"ref":"t999"`, `{"ref":"t999`, `{`} {
		if _, err := decode([]byte(raw)); err == nil {
			t.Fatalf("validator repaired unknown or missing ref in %q", raw)
		}
	}
}

func TestExecuteJSONBalancesDelimitersWithoutChangingRawCache(t *testing.T) {
	for name, response := range map[string]string{
		"brackets": `{"value":"ok"}]`,
		"quote":    `{"value":"ok`,
	} {
		t.Run(name, func(t *testing.T) {
			provider := baseTestProvider()
			raw := []byte(response)
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
			provider.responses = [][]byte{[]byte(`{"value":"ok`)}
			provider.finishReasons = []FinishReason{FinishLength}
			call.Prompt.User = "provider reported truncation"
			if _, err := ExecuteJSON(t.Context(), executor, provider, call); err == nil {
				t.Fatal("bracket balancing overrode the provider's output limit")
			}
		})
	}
}
