package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

const pythonFixtureSelector = "python:.:script:repomap-fixture"

func TestCumulativePythonRepositoryDiscoveryAndProgramIndexContract(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatalf("discover cumulative Python fixture: %v", err)
	}
	if err := catalog.Validate(); err != nil {
		t.Fatalf("validate cumulative Python target catalog: %v", err)
	}
	target := pythonFixtureTarget(t, catalog)
	indexes, err := pythonprogramindex.BuildMany(t.Context(), repository, []pythontarget.Target{target})
	if err != nil {
		t.Fatalf("build Python ProgramIndex: %v", err)
	}
	if len(indexes) != 1 {
		t.Fatalf("Python ProgramIndex count = %d, want one exact target", len(indexes))
	}
	index := indexes[0]
	assertProgramIndexRoundTrip(t, index)
	if index.Target.Language != "python" || index.Target.Selector != pythonFixtureSelector {
		t.Fatalf("Python ProgramIndex target = %#v", index.Target)
	}
	if len(index.Target.Seeds) != 1 || index.Target.Seeds[0].Kind != programindex.SeedCallable {
		t.Fatalf("Python script target seeds = %#v, want one exact callable seed", index.Target.Seeds)
	}
	seed := programIndexObjectByID(index, index.Target.Seeds[0].ObjectID)
	if seed.ID == "" || seed.Kind != programindex.ObjectFunction || seed.Name != "main" {
		t.Fatalf("Python script seed object = %#v, want exact main function", seed)
	}
}

func pythonFixtureTarget(t *testing.T, catalog pythontarget.Catalog) pythontarget.Target {
	t.Helper()
	for _, target := range catalog.Entries {
		if target.Selector == pythonFixtureSelector {
			return target
		}
	}
	t.Fatalf("Python fixture target %q is absent from catalog %#v", pythonFixtureSelector, catalog.Entries)
	return pythontarget.Target{}
}

func programIndexObjectByID(index programindex.Index, id string) programindex.Object {
	for _, object := range index.Objects {
		if object.ID == id {
			return object
		}
	}
	return programindex.Object{}
}
