package terminology

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestNamedReviewAndSourceRowsKeepOnlyAcceptedTextOrigins(t *testing.T) {
	for _, test := range []struct {
		name     string
		result   any
		accepted string
	}{
		{"learning intents", map[string]any{"key": "run", "reviews": []any{
			map[string]any{"intent": "purpose", "key": "run", "reason": "RejectedTerm", "questions": []any{map[string]any{"key": "run", "question": "NestedRejectedTerm"}}},
			map[string]any{"intent": "run", "reason": "AcceptedTerm"},
		}}, "run"},
		{"guidance files", []any{
			map[string]any{"file_ref": "f1", "key": "f2", "classifications": []any{map[string]any{"ref": "f2", "hypotheses": []string{"RejectedTerm", "NestedRejectedTerm"}}}},
			map[string]any{"file_ref": "f2", "classifications": []any{map[string]any{"hypotheses": []string{"AcceptedTerm"}}}},
		}, "f2"},
		{"document sources", map[string]any{"key": "d2", "overview": "GlobalTerm", "sources": []any{
			map[string]any{"ref": "d1", "key": "d2", "claims": []any{"RejectedTerm", map[string]any{"ref": "d2", "text": "NestedRejectedTerm"}}},
			map[string]any{"ref": "d2", "claims": []string{"AcceptedTerm"}},
		}}, "d2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			collector := NewCollector([]string{"README.md"})
			wrapped, request := prepareForTest(t, collector, `{"path":"README.md"}`)
			ref := sourceRef(t, collector, request, "README.md", "")
			response := responseJSON(test.result,
				termJSON("AcceptedTerm", "A retained interpretation.", ref),
				termJSON("RejectedTerm", "A refused interpretation.", ref),
				termJSON("NestedRejectedTerm", "Nested refused interpretation.", ref),
				termJSON("GlobalTerm", "An unaccepted overview.", ref))
			adapted, err := llm.AdaptResponse(wrapped, request, response)
			if err != nil {
				t.Fatal(err)
			}
			adapted.Accepted([]string{test.accepted})
			adapted.Accepted([]string{test.accepted})
			terms := collector.Snapshot()
			if len(terms) != 1 || terms[0].Name != "AcceptedTerm" || len(terms[0].Origins) != 1 || terms[0].Origins[0].Row != test.accepted || !reflect.DeepEqual(terms[0].Sources, []Source{{Path: "README.md"}}) {
				t.Fatalf("named row acceptance acquired rejected text or lost exact origin: %+v", terms)
			}
		})
	}
}
