package pythontarget

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCumulativeLaunchFormsRetainRootsAndShareCallableEvidence(t *testing.T) {
	catalog, _ := cumulativeCheckedPythonCatalog(t)
	command := targetBySelector(t, catalog, "python:.:script:repomap-fixture")
	module := targetBySelector(t, catalog, "python:.:module:fixture_app")
	guard := targetBySelector(t, catalog, "python:.:guard:src/fixture_app/cli")
	entry, ok := LaunchEntry(command)
	if !ok {
		t.Fatal("console script lost exact callable")
	}
	for _, launch := range []Target{module, guard} {
		got, ok := LaunchEntry(launch)
		if !ok || got != entry || !CanSeed(command, launch) || !CanSeed(launch, command) {
			t.Fatalf("launch identity not restored: %+v", launch)
		}
		if launch.Roots[0].Kind == RootCallable || len(launch.LaunchCalls) != 1 || launch.LaunchCalls[0].Line < 1 {
			t.Fatalf("launch anchor replaced: %+v", launch)
		}
		copy := launch.Snapshot()
		copy.LaunchCalls[0].Entry.Qualname = "changed"
		if launch.LaunchCalls[0].Entry != entry {
			t.Fatal("snapshot shares mutable launch evidence")
		}
		if err := copy.Validate(); err == nil {
			t.Fatal("changed launch evidence retained sealed authority")
		}
	}
	if CanSeed(command, command) {
		t.Fatal("self seed advertised")
	}
	wire, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	var restored Catalog
	if err = json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if got, _ := LaunchEntry(targetBySelector(t, restored, module.Selector)); got != entry {
		t.Fatal("launch identity lost on persistence")
	}
	for _, target := range catalog.Entries {
		if target.Selector == command.Selector || target.Selector == module.Selector || target.Selector == guard.Selector {
			continue
		}
		if target.Kind == KindExecutable && CanSeed(command, target) {
			t.Fatalf("unrelated launch promoted by shared modules: %s", target.Selector)
		}
	}
}

func TestLaunchFormsRequireSimpleArgumentFreeCallAndExplicitImport(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{"main()", true}, {"main(['worker'])", false}, {"main(mode='worker')", false}, {"main(); other()", false}, {"if enabled:\n        main()", false}, {"factory()()", false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			// Exercise variants of the same cumulative repository, retaining
			// its complete inventory and independent worker/control targets.
			_, sourceFile, _, _ := runtime.Caller(0)
			sourceRoot := filepath.Join(filepath.Dir(sourceFile), "../../testdata/repositories/python")
			_, original := cumulativeCheckedPythonCatalog(t)
			files := make(map[string]string)
			for _, entry := range original.Entries() {
				raw, err := os.ReadFile(filepath.Join(sourceRoot, entry.Path))
				if err != nil {
					t.Fatal(err)
				}
				files[entry.Path] = string(raw)
			}
			files["src/fixture_app/cli.py"] = strings.Replace(files["src/fixture_app/cli.py"], "if __name__ == \"__main__\":\n    main()", "if __name__ == \"__main__\":\n    "+tc.body, 1)
			repository := fixture(t, files)
			catalog, err := Discover(t.Context(), repository)
			if err != nil {
				t.Fatal(err)
			}
			command := targetBySelector(t, catalog, "python:.:script:repomap-fixture")
			guard := targetBySelector(t, catalog, "python:.:guard:src/fixture_app/cli")
			if got := CanSeed(command, guard); got != tc.want {
				t.Fatalf("CanSeed=%v want%v", got, tc.want)
			}
		})
	}
}
