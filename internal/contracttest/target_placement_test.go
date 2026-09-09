package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCumulativePythonDeclaredOwnershipAndSeedProjection(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	var owner, demo, api, worker, excluded pythontarget.Target
	for _, target := range catalog.Entries {
		if target.Kind == pythontarget.KindLibrary && target.ProjectDir == "." {
			owner = target
		}
		for _, root := range target.Roots {
			switch root.Path {
			case "src/acme/demo.py":
				demo = target
			case "src/acme/api.py":
				api = target
			case "src/acme/worker.py":
				worker = target
			case "src/acme/excluded/check.py":
				excluded = target
			}
		}
	}
	if owner.Ref == "" || demo.Ref == "" || api.Ref == "" || worker.Ref == "" || excluded.Ref == "" {
		t.Fatal("cumulative candidate missing")
	}
	if len(owner.DeclaredPackages) != 2 {
		t.Fatalf("want TOML and cfg declarations, got %#v", owner.DeclaredPackages)
	}
	for _, declaration := range owner.DeclaredPackages {
		if declaration.Kind != "find" || declaration.Line < 1 || len(declaration.Include) != 3 || len(declaration.Exclude) != 2 {
			t.Fatalf("incomplete package declaration: %#v", declaration)
		}
	}
	if !pythontarget.CanSeed(owner, demo) || !pythontarget.CanSeed(owner, api) || !pythontarget.CanSeed(owner, worker) || pythontarget.CanSeed(owner, excluded) {
		t.Fatal("membership must allow a decision, while excluding only declared exclusions")
	}
	// Explicitly select only the demonstration as a seed. Both documented
	// services still have independent native targets and complete module views.
	input, err := pythonprogramindex.BuildInput(t.Context(), repository, owner, demo)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	if index.Target.Kind != "library" || len(index.Target.Seeds) != 1 || index.Target.Seeds[0].Location.Path != "src/acme/demo.py" || index.Target.Seeds[0].Location.Line != 5 {
		t.Fatalf("library API or seed source lost: %#v", index.Target)
	}
	for _, name := range []string{"greeting", "process_pending_jobs", "Handler"} {
		found := false
		for _, object := range index.Objects {
			if object.Name == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("owner lost declaration %s", name)
		}
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Seeds) != 1 {
		t.Fatalf("seed did not reach atlas: %v", graph.Seeds)
	}
	if _, err := pythonprogramindex.BuildInput(t.Context(), repository, owner, excluded); err == nil {
		t.Fatal("excluded module absorbed into library")
	}
}
