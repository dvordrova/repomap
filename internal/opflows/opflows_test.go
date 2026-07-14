package opflows

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

func TestDiscoverEvidenceThresholds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		signals  []sourcesignals.Signal
		wantSize int
	}{
		{
			name: "two weak signals across two files",
			signals: []sourcesignals.Signal{
				testSignal("background_loop", "internal/lease/reaper.go", 20),
				testSignal("background_loop", "internal/lease/queue.go", 20),
			},
			wantSize: 1,
		},
		{
			name: "one weak signal",
			signals: []sourcesignals.Signal{
				testSignal("background_loop", "internal/lease/reaper.go", 34),
			},
			wantSize: 0,
		},
		{
			name: "one strong signal",
			signals: []sourcesignals.Signal{
				testSignal("background_loop", "internal/lease/reaper.go", 35),
			},
			wantSize: 1,
		},
		{
			name:     "no signals",
			signals:  nil,
			wantSize: 0,
		},
		{
			name: "request signal is excluded",
			signals: []sourcesignals.Signal{
				testSignal("request_handler", "internal/api/handler.go", 40),
			},
			wantSize: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidates, warnings := Discover(test.signals, testEntrypoints())
			if len(warnings) != 0 {
				t.Fatalf("warnings = %v, want none", warnings)
			}
			if len(candidates) != test.wantSize {
				t.Fatalf("candidate count = %d, want %d: %#v", len(candidates), test.wantSize, candidates)
			}
		})
	}
}

func TestDiscoverBuildsBoundedOperationalCandidate(t *testing.T) {
	t.Parallel()

	signals := []sourcesignals.Signal{
		testSignal("admin_maintenance", "internal/admin/a.go", 20),
		testSignal("admin_maintenance", "internal/admin/b.go", 45),
		testSignal("admin_maintenance", "internal/admin/c.go", 25),
		testSignal("admin_maintenance", "internal/admin/d.go", 30),
		testSignal("admin_maintenance", "internal/admin/a.go", 15),
	}
	signals[1].Match = "compactMaintenance"

	candidates, warnings := Discover(signals, testEntrypoints())

	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one", candidates)
	}
	candidate := candidates[0]
	if candidate.Kind != "signal_flow" {
		t.Fatalf("kind = %q, want signal_flow", candidate.Kind)
	}
	if candidate.Priority != 4 {
		t.Fatalf("priority = %d, want 4", candidate.Priority)
	}
	if candidate.EntrypointPackage != "" {
		t.Fatalf("entrypoint package = %q, want no unproved executable relation", candidate.EntrypointPackage)
	}
	if len(candidate.OpenFiles) != 4 || candidate.OpenFiles[0] != "internal/admin/b.go" {
		t.Fatalf("open files = %v, want four strongest-first files", candidate.OpenFiles)
	}
	if !strings.Contains(candidate.Why, `matched "compactMaintenance"`) {
		t.Fatalf("why = %q, want compact matched evidence", candidate.Why)
	}
	if strings.Contains(candidate.Why, "internal/admin/b.go") {
		t.Fatalf("why contains an unvalidated repository path: %q", candidate.Why)
	}
	if strings.Contains(candidate.Why, signals[1].Snippet) {
		t.Fatalf("why contains full source snippet: %q", candidate.Why)
	}
}

func TestDiscoverAppliesSingleFilePriorityPenalty(t *testing.T) {
	t.Parallel()

	signals := []sourcesignals.Signal{
		testSignal("threshold_limit", "internal/quota/quota.go", 40),
		testSignal("threshold_limit", "internal/quota/quota.go", 21),
		testSignal("threshold_limit", "internal/quota/quota.go", 22),
	}

	candidates, _ := Discover(signals, testEntrypoints())

	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one", candidates)
	}
	if candidates[0].Priority != 2 {
		t.Fatalf("priority = %d, want 2 after single-file penalty", candidates[0].Priority)
	}
}

func TestDiscoverGroupsMixedOperationalCategories(t *testing.T) {
	t.Parallel()

	signals := []sourcesignals.Signal{
		testSignal("background_loop", "internal/loop/a.go", 40),
		testSignal("storage_durability", "internal/storage/sync.go", 40),
		testSignal("request_handler", "internal/api/handler.go", 40),
	}

	candidates, _ := Discover(signals, testEntrypoints())

	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2: %#v", len(candidates), candidates)
	}
	for _, candidate := range candidates {
		if candidate.Kind != "signal_flow" {
			t.Fatalf("candidate kind = %q, want signal_flow", candidate.Kind)
		}
	}
}

func TestDiscoverReturnsAtMostTopFiveCandidates(t *testing.T) {
	t.Parallel()

	signals := make([]sourcesignals.Signal, 0, len(operationalCategories)*12)
	for _, category := range operationalCategories {
		for index := range 12 {
			signal := testSignal(category, "internal/"+category+"/file"+string(rune('a'+index))+".go", 40-index)
			signals = append(signals, signal)
		}
	}

	candidates, _ := Discover(signals, testEntrypoints())

	if len(candidates) != 5 {
		t.Fatalf("candidate count = %d, want bounded top 5", len(candidates))
	}
	for _, candidate := range candidates {
		if candidate.Priority != 5 {
			t.Fatalf("priority = %d, want capped priority 5", candidate.Priority)
		}
		if len(candidate.OpenFiles) > maxOpenFiles {
			t.Fatalf("open files = %d, want at most %d", len(candidate.OpenFiles), maxOpenFiles)
		}
	}
}

func TestDiscoverWarnsAndSkipsMalformedSignals(t *testing.T) {
	t.Parallel()

	signals := []sourcesignals.Signal{
		{Category: "background_loop", Line: 1, Weight: 40},
		{Path: "internal/loop.go", Line: 1, Weight: 40},
		{Category: "background_loop", Path: "/private/secret.go", Line: 1, Weight: 40},
		testSignal("background_loop", "internal/valid.go", 40),
	}

	candidates, warnings := Discover(signals, testEntrypoints())

	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one valid candidate", candidates)
	}
	if len(warnings) != 3 {
		t.Fatalf("warnings = %v, want three", warnings)
	}
	for _, warning := range warnings {
		if strings.Contains(warning, "secret") {
			t.Fatalf("warning echoes malformed path: %q", warning)
		}
	}
}

func testSignal(category, path string, weight int) sourcesignals.Signal {
	return sourcesignals.Signal{
		Path:     path,
		Line:     1,
		Category: category,
		Match:    "matched-token",
		Snippet:  "full source snippet must stay out of candidate summaries",
		Weight:   weight,
	}
}

func testEntrypoints() []gofacts.Entrypoint {
	return []gofacts.Entrypoint{
		{ImportPath: "example.com/project/cmd/tool", Kind: "tool"},
		{ImportPath: "example.com/project/cmd/server", Kind: "primary_binary"},
	}
}
