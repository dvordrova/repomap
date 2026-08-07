package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

// Long-horizon program Phase 1B (Asynq): external/GOROOT/module-cache
// pseudo-paths (<external>/...) — and any absolute host path — can never
// become mandatory repository source reads. They stay typed external
// frontier evidence in their own artifacts and never enter the authorized
// openable catalog, so the report source-coverage stage cannot abort a run
// because an external path is not a regular repository file.
func TestCollectOpenablePathsExcludesExternalPseudoPaths(t *testing.T) {
	t.Parallel()
	grounding := &ArchitectureGrounding{
		BehaviorAnchors: []ArchitectureBehaviorAnchor{
			{Location: evidence.Location{Path: "internal/rdb/inspect.go", Line: 610}},
			{Location: evidence.Location{Path: "<external>/inspect.go", Line: 610}},
			{Location: evidence.Location{Path: "/absolute/host/path.go", Line: 1}},
		},
		EntryHandoffs: []ArchitectureEntryHandoff{
			{
				ProcessEntrypoint:      ArchitectureAnchorMember{Location: evidence.Location{Path: "<external>/main.go", Line: 1}},
				RepresentativeCallsite: evidence.Location{Path: "internal/worker/run.go", Line: 12},
			},
		},
	}
	data := &ReportData{
		OpenablePaths:         []string{"<external>/leaked.go", "cmd/server/main.go"},
		ArchitectureGrounding: grounding,
		RepositoryAtlas:       &repositoryatlas.Atlas{},
	}
	collectOpenablePaths(data)
	for _, path := range data.OpenablePaths {
		if strings.HasPrefix(path, "<external>/") {
			t.Fatalf("external pseudo-path leaked into openable catalog: %q", path)
		}
		if strings.HasPrefix(path, "/") {
			t.Fatalf("absolute host path leaked into openable catalog: %q", path)
		}
	}
	want := map[string]bool{
		"cmd/server/main.go":      true,
		"internal/rdb/inspect.go": true,
		"internal/worker/run.go":  true,
	}
	for _, path := range data.OpenablePaths {
		if !want[path] {
			t.Fatalf("unexpected openable path %q (want only repository-relative paths)", path)
		}
	}
	if len(data.OpenablePaths) != len(want) {
		t.Fatalf("openable paths = %#v, want %#v", data.OpenablePaths, want)
	}
}
