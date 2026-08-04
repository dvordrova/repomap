package atlasstudy

import (
	"testing"
)

// TestMockResponseValidForAnyCompiledProduct verifies the provider-free mock
// fixture is always a replayable response: every emitted direction passes
// item-local validation (zero rejected siblings), the status is accepted, and
// every direction title is restored from the request's own span questions.
func TestMockResponseValidForAnyCompiledProduct(t *testing.T) {
	for _, language := range []Language{LanguageEnglish, LanguageRussian} {
		t.Run(string(language), func(t *testing.T) {
			product := mustCompileArtifactTestProduct(t, language)
			record := mustRequestRecord(t, product)

			raw, err := MockResponse(record)
			if err != nil {
				t.Fatalf("MockResponse: %v", err)
			}
			if len(raw) == 0 {
				t.Fatalf("MockResponse returned empty bytes")
			}

			result, status, diagnostics, err := ReplayResponseRecord(record, raw)
			if err != nil {
				t.Fatalf("ReplayResponseRecord(mock): %v", err)
			}
			if diagnostics.DirectionsRejected != 0 {
				t.Fatalf("mock fixture produced rejected siblings: %#v", diagnostics.Issues)
			}
			if len(result.Directions) == 0 || len(result.Directions) > MaxDirections {
				t.Fatalf("mock directions = %d, want 1..%d", len(result.Directions), MaxDirections)
			}
			if status.State != ProductStateAccepted {
				t.Fatalf("mock replay status = %q, want accepted", status.State)
			}
			if status.DirectionCount != len(result.Directions) {
				t.Fatalf("status direction count %d != result %d", status.DirectionCount, len(result.Directions))
			}
			for index, direction := range result.Directions {
				if direction.Question == "" {
					t.Fatalf("direction %d has an empty question", index)
				}
				span, ok := product.byCanonical[direction.Span]
				if !ok {
					t.Fatalf("direction %d references unknown span %#v", index, direction.Span)
				}
				if direction.Question != span.Question {
					t.Fatalf("direction %d question %q != span question %q",
						index, direction.Question, span.Question)
				}
			}
			if status.PortfolioTargetMet != (len(result.Directions) >= MinPortfolioDirections) {
				t.Fatalf("portfolio flag inconsistent with direction count %d", len(result.Directions))
			}
		})
	}
}

// TestMockResponseRejectsTamperedRequest verifies the fixture refuses a
// request whose catalog no longer matches its own shas (same protection as
// the replay path).
func TestMockResponseRejectsTamperedRequest(t *testing.T) {
	product := mustCompileArtifactTestProduct(t, LanguageEnglish)
	record := mustRequestRecord(t, product)
	record.Catalog[0].Question = "tampered"
	if _, err := MockResponse(record); err == nil {
		t.Fatalf("MockResponse accepted a tampered request")
	}
}
