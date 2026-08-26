package jstsproject

import (
	"context"
	"path"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestScoutNamelessRootPackageUsesLockfileIdentityWithoutCompiler(t *testing.T) {
	repository := newTargetScoutCorpus(t, map[string]string{
		"package-lock.json": `{"name":"meetup","lockfileVersion":3}`,
		"package.json":      `{"private":true,"devDependencies":{"tailwindcss":"4.1.0"}}`,
		"static/main.js":    "document.body.dataset.ready = 'true'\n",
	})
	defer repository.Close()

	target, err := Scout(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	manifestID, ok := repository.ID("package.json")
	if !ok {
		t.Fatal("package.json is absent from corpus")
	}
	if target.Version != TargetVersion || target.CorpusSHA256 != repository.SHA256() ||
		target.Ref != "project:root-package" || target.Name != "meetup" ||
		target.Selector != "jsts:package.json" || target.ProjectDir != "." ||
		target.ManifestPath != "package.json" || target.ManifestFileRef != string(manifestID) {
		t.Fatalf("scouted root target = %#v", target)
	}
	if err := target.ValidateAgainst(repository); err != nil {
		t.Fatal(err)
	}
}

func TestScoutSelectedBindsExactNestedPackageBeforeCompiler(t *testing.T) {
	repository := newTargetScoutCorpus(t, map[string]string{
		"admin/package.json": `{"name":"admin-app"}`,
		"admin/src/main.ts":  "export const admin = true\n",
		"front/package.json": `{"name":"front-app"}`,
		"front/src/main.ts":  "export const front = true\n",
		"package.json":       `{"name":"workspace","workspaces":["admin","front"]}`,
	})
	defer repository.Close()

	if _, err := Scout(context.Background(), repository); err == nil ||
		!strings.Contains(err.Error(), "jsts:admin/package.json") ||
		!strings.Contains(err.Error(), "jsts:front/package.json") {
		t.Fatalf("ambiguous automatic scout error = %v", err)
	}
	target, err := ScoutSelected(
		context.Background(), repository, "jsts:front/package.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	manifestID, ok := repository.ID("front/package.json")
	if !ok {
		t.Fatal("front/package.json is absent from corpus")
	}
	if target.Ref != "project:package:"+string(manifestID) || target.Name != "front-app" ||
		target.Selector != "jsts:front/package.json" || target.ProjectDir != "front" ||
		target.ManifestPath != "front/package.json" || target.ManifestFileRef != string(manifestID) {
		t.Fatalf("scouted nested target = %#v", target)
	}
}

func TestScoutDoesNotLetSourceLessRootSuppressOwnedNestedPackage(t *testing.T) {
	repository := newTargetScoutCorpus(t, map[string]string{
		"package.json":       `{"private":true,"scripts":{"dev":"bun run --cwd ../.. dev","start":"bun run --cwd=../.. start"}}`,
		"front/package.json": `{"name":"front-app"}`,
		"front/src/main.ts":  "export const front = true\n",
	})
	defer repository.Close()

	automatic, err := Scout(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if automatic.Selector != "jsts:front/package.json" || automatic.ProjectDir != "front" {
		t.Fatalf("automatic target = %#v, want the source-owning nested package", automatic)
	}
	exact, err := ScoutSelected(context.Background(), repository, automatic.Selector)
	if err != nil {
		t.Fatalf("materialization selector rejected the automatic scout target: %v", err)
	}
	if exact != automatic {
		t.Fatalf("exact scout target = %#v, want automatic target %#v", exact, automatic)
	}
}

func TestTargetFromResultPreservesExactScoutIdentity(t *testing.T) {
	result := minimalResult(t, "typescript")
	target, err := TargetFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if target.Version != TargetVersion || target.CorpusSHA256 != result.CorpusSHA256 ||
		target.Ref != result.Project.Ref || target.Name != result.Project.Name ||
		target.Selector != result.Project.Selector || target.ProjectDir != "." ||
		target.ManifestPath != result.Project.ManifestPath ||
		target.ManifestFileRef != result.Project.ManifestFileRef {
		t.Fatalf("target projected from result = %#v", target)
	}

	tampered := target
	tampered.ManifestPath = "nested/package.json"
	if err := tampered.Validate(); err == nil {
		t.Fatal("target with mismatched project directory was accepted")
	}
}

func TestTargetMaterializationAllowsOnlyContentDerivedNameDrift(t *testing.T) {
	target := Target{
		Version: TargetVersion, CorpusSHA256: strings.Repeat("a", 64),
		Ref: "project:root-package", Name: "before-change",
		Selector: "jsts:package.json", ProjectDir: ".",
		ManifestPath: "package.json", ManifestFileRef: "f1",
	}
	materialized := target
	materialized.Name = "after-change"
	if err := target.ValidateMaterialization(materialized); err != nil {
		t.Fatalf("content-derived package name drift was rejected: %v", err)
	}

	drifted := materialized
	drifted.ManifestFileRef = "f2"
	if err := target.ValidateMaterialization(drifted); err == nil {
		t.Fatal("materialization with a different manifest authority was accepted")
	}
}

func newTargetScoutCorpus(t *testing.T, files map[string]string) *corpus.Corpus {
	t.Helper()
	root := t.TempDir()
	paths := make([]string, 0, len(files))
	for filePath, content := range files {
		if filePath == "" || path.IsAbs(filePath) {
			t.Fatalf("invalid test path %q", filePath)
		}
		writeTestFile(t, root, filePath, content)
		paths = append(paths, filePath)
	}
	repository, err := corpus.New(
		context.Background(),
		root,
		gitfiles.Listing{Paths: paths, RegularPaths: append([]string(nil), paths...)},
	)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
