package report

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSnapshotExtractsOnlyMaterialGoPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "snapshot.json")
	writeTestFile(t, dir, "snapshot.json", `{
		"repo_name":"project",
		"go_facts":{
			"modules":[{"module_dir":"."}],
			"packages":[{"files":["internal/server/server.go"]}]
		}
	}`)
	data := &ReportData{}
	if err := parseSnapshot(snapshotPath, data); err != nil {
		t.Fatal(err)
	}
	want := []string{"go.mod", "go.sum", "internal/server/server.go"}
	if strings.Join(data.materialInputPaths, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("material inputs = %#v, want %#v", data.materialInputPaths, want)
	}
}

func TestSnapshotMaterialInputsIncludeEveryModuleBoundary(t *testing.T) {
	got, err := snapshotMaterialInputPaths(&snapshotGoFactsJSON{
		Modules: []snapshotModuleJSON{{ModuleDir: "tools"}, {ModuleDir: "."}},
		Packages: []snapshotPackageJSON{
			{Files: []string{"tools/cmd/check/main.go", "internal/app/app.go"}},
			{Files: []string{"internal/app/app.go"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"go.mod", "go.sum", "internal/app/app.go", "tools/cmd/check/main.go",
		"tools/go.mod", "tools/go.sum",
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("material inputs = %#v, want sorted unique %#v", got, want)
	}
}
