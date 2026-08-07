package componentmap

import (
	"encoding/json"
	"strings"
	"testing"
)

// Decision 235 / Archive 12 P0 (Chatto): a provider response that is one
// COMPLETE valid JSON proposal object followed by exactly a bounded
// sequence of unmatched closing delimiters (`]}`) must normalize
// deterministically, run ordinary semantic validation, and publish a
// diagnostic — the run continues instead of dying under
// response_validation.
func TestTrailingClosingDelimiterNormalizationChattoCase(t *testing.T) {
	proposal := `{"records":[{"kind":"subsystem","ref":"g1","name":"first subsystem","description":"first purpose"},{"kind":"component","subsystem_ref":"g1","name":"first component","description":"first responsibility","member_refs":[{"ref":"p1"}],"anchor_refs":[{"ref":"a1"}]},{"kind":"subsystem","ref":"g2","name":"second subsystem","description":"second purpose"},{"kind":"component","subsystem_ref":"g2","name":"second component","description":"second responsibility","member_refs":[{"ref":"s1"}],"anchor_refs":[]}]}`
	raw := []byte(proposal + "]}")
	normalized, ok := normalizeTrailingClosingDelimiters(raw)
	if !ok {
		t.Fatalf("normalizeTrailingClosingDelimiters rejected the Chatto case (complete object + ]})")
	}
	if string(normalized) != proposal {
		t.Fatalf("normalized bytes differ: got %d bytes want %d", len(normalized), len(proposal))
	}
	if !json.Valid(normalized) {
		t.Fatalf("normalized bytes are not valid JSON")
	}
}

// The normalization must fail closed on any NON-delimiter trailing content:
// letters, digits, a second object fragment, or extra OPENING brackets.
func TestTrailingClosingDelimiterNormalizationFailsClosed(t *testing.T) {
	proposal := `{"records":[]}`
	bad := []string{
		proposal + `]}` + `x`,   // trailing letter
		proposal + `]}` + `1`,   // trailing digit
		proposal + `]}` + `{`,   // extra opening bracket
		proposal + `]}` + `"x"`, // trailing string
		proposal + `[{]}` + `]`, // garbage interior
	}
	for _, candidate := range bad {
		if normalized, ok := normalizeTrailingClosingDelimiters([]byte(candidate)); ok {
			t.Fatalf("normalizeTrailingClosingDelimiters accepted %q → %q (must fail closed)", candidate, normalized)
		}
	}
}

// A bounded sequence of trailing closing delimiters is the ONLY accepted
// trailing form; more than the cap fails closed.
func TestTrailingClosingDelimiterNormalizationBound(t *testing.T) {
	proposal := `{"records":[]}`
	within := proposal + strings.Repeat("]", 8)
	if _, ok := normalizeTrailingClosingDelimiters([]byte(within)); !ok {
		t.Fatalf("8 trailing closing delimiters must be accepted (bounded)")
	}
	over := proposal + strings.Repeat("]", 9)
	if _, ok := normalizeTrailingClosingDelimiters([]byte(over)); ok {
		t.Fatalf("9 trailing closing delimiters must fail closed (bound is 8)")
	}
}
