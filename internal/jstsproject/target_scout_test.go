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

func TestScoutTargetsKeepEveryNestedPackageBeforeCompiler(t *testing.T) {
	repository := newTargetScoutCorpus(t, map[string]string{
		"admin/package.json": `{"name":"admin-app"}`,
		"admin/src/main.ts":  "export const admin = true\n",
		"front/package.json": `{"name":"front-app"}`,
		"front/src/main.ts":  "export const front = true\n",
		"package.json":       `{"name":"workspace","workspaces":["admin","front"]}`,
	})
	defer repository.Close()

	if _, err := Scout(context.Background(), repository); err == nil ||
		!strings.Contains(err.Error(), "exact-one compatibility helper") ||
		!strings.Contains(err.Error(), "use ScoutTargets") {
		t.Fatalf("exact-one compatibility scout error = %v", err)
	}
	targets, err := ScoutTargets(context.Background(), repository, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].Selector != "jsts:admin/package.json" ||
		targets[1].Selector != "jsts:front/package.json" {
		t.Fatalf("repository package targets = %#v", targets)
	}
	for _, target := range targets {
		exact, exactErr := ScoutTargets(context.Background(), repository, target.Selector)
		if exactErr != nil {
			t.Fatalf("replay %s: %v", target.Selector, exactErr)
		}
		if len(exact) != 1 || exact[0] != target {
			t.Fatalf("replay %s = %#v, want %#v", target.Selector, exact, target)
		}
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

func TestScoutTargetsKeepSourceOwningRootAndNestedPackage(t *testing.T) {
	repository := newTargetScoutCorpus(t, map[string]string{
		"package.json":       `{"name":"root-app"}`,
		"src/main.ts":        "export const root = true\n",
		"front/package.json": `{"name":"front-app"}`,
		"front/src/main.ts":  "export const front = true\n",
	})
	defer repository.Close()

	targets, err := ScoutTargets(context.Background(), repository, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].Selector != "jsts:front/package.json" ||
		targets[1].Selector != "jsts:package.json" {
		t.Fatalf("root and nested package targets = %#v", targets)
	}
	if _, err := Scout(context.Background(), repository); err == nil ||
		!strings.Contains(err.Error(), "use ScoutTargets") {
		t.Fatalf("exact-one compatibility scout error = %v", err)
	}
	for _, target := range targets {
		exact, exactErr := ScoutTargets(context.Background(), repository, target.Selector)
		if exactErr != nil {
			t.Fatalf("replay %s: %v", target.Selector, exactErr)
		}
		if len(exact) != 1 || exact[0] != target {
			t.Fatalf("replay %s = %#v, want %#v", target.Selector, exact, target)
		}
	}
}

func TestScoutDoesNotLetSourceLessRootSuppressOwnedNestedPackage(t *testing.T) {
	repository := newTargetScoutCorpus(t, map[string]string{
		"package.json":       `{"private":true,"scripts":{"dev":"bun run --cwd ../.. dev","start":"bun run --cwd=../.. start"}}`,
		"front/package.json": `{"name":"front-app"}`,
		"front/src/main.ts":  "export const front = true\n",
	})
	defer repository.Close()

	singleton, err := Scout(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if singleton.Selector != "jsts:front/package.json" || singleton.ProjectDir != "front" {
		t.Fatalf("exact-one target = %#v, want the source-owning nested package", singleton)
	}
	exact, err := ScoutSelected(context.Background(), repository, singleton.Selector)
	if err != nil {
		t.Fatalf("materialization selector rejected the exact-one scout target: %v", err)
	}
	if exact != singleton {
		t.Fatalf("exact scout target = %#v, want exact-one target %#v", exact, singleton)
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
