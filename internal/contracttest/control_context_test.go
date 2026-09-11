package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

func TestCumulativeGoWorkerControlContext(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	writePublishedGoFixtureModule(t, root)
	authorities := analyzeGoFixture(t, root, repository, goFixtureRootPackage+"/cmd/worker", "cumulative-go-worker-control")
	input, err := goadapter.BuildInput(repository, authorities.target, authorities.origins, authorities.direct,
		authorities.external, authorities.core, authorities.dynamic, authorities.tests)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	assertProgramIndexRoundTrip(t, index)
	adaptertest.AssertSharedArtifact(t, input, index)
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.AssertCallControls(t, index, graph, "cmd/worker/main.go", "processPendingJobs", map[int][]adaptertest.Control{
		11: nil,
		13: {{Line: 12, Kind: "range body over channel"}},
		19: {{Line: 18, Kind: "range body"}},
		29: {{Line: 24, Kind: "for body without condition"}, {Line: 25, Kind: "select without default"}},
		38: nil,
	})
	assertEvidenceVocabulary(t, graph, "control_context", "range body over channel", "for body without condition", "select without default")
}
