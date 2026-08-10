package orient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestAnalysisTargetHandoffOwnsSnapshotAndBindsMetadata(t *testing.T) {
	target := analysistarget.Target{
		Version: 1, Ref: "at-placeholder", Kind: analysistarget.KindExecutablePackage,
		ModuleID: "module-root", ModulePath: "example.com/app", ModuleDir: ".",
		PackagePath: "example.com/app/cmd/app", PackageDir: "cmd/app",
		RootBoundary: analysistarget.RootBoundaryExactPackageMains,
		Roots:        []analysistarget.Root{{Path: "cmd/app/main.go", Line: 10}},
	}
	// Resolve supplies the canonical self-seal; the handoff must preserve it.
	// This test intentionally focuses the live seam, not resolver internals.
	resolution, err := analysistarget.Resolve(syntheticOrientTargetFacts(), analysistarget.Options{})
	if err != nil {
		t.Fatal(err)
	}
	target = resolution.Selected.Snapshot()
	var received analysistarget.Target
	deliverAnalysisTarget(Options{AnalysisTargetSink: func(got analysistarget.Target) {
		received = got
		got.Roots[0].Path = "mutated"
	}}, &target)
	if received.Ref != target.Ref || target.Roots[0].Path == "mutated" {
		t.Fatalf("handoff did not own its copy: got %#v source %#v", received, target)
	}
	meta := debugdump.RunMeta{}
	bindRunMetaAnalysisTarget(&meta, &target)
	if meta.AnalysisTargetRef != target.Ref || meta.AnalysisTargetKind != string(target.Kind) ||
		meta.AnalysisTargetModule != target.ModulePath ||
		meta.AnalysisTargetDisplayPath != target.DisplayPath() ||
		meta.AnalysisTargetPackage != target.PackagePath {
		t.Fatalf("metadata target = %#v", meta)
	}
	effective := effectiveOptions(Options{
		AnalysisTargetOverride: "cmd/app",
		EffectiveOptions:       debugdump.EffectiveOptions{Offline: true},
	})
	if effective.AnalysisTargetOverride != "cmd/app" || !effective.Offline {
		t.Fatalf("effective options = %#v", effective)
	}
}

func TestModuleLibraryMetadataKeepsModuleAndDisplayWithoutInventingPackage(t *testing.T) {
	const modulePath = "example.com/library"
	catalog, err := analysistarget.BuildCatalog(gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: modulePath, ModuleDir: ".", Main: true,
			PackagesCount: 1, RetainedPackagesCount: 1,
			Coverage: gofacts.ModuleCoverage{PackagesDiscovered: 1, PackagesRetained: 1},
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: modulePath, Name: "library", ModuleID: "module-root", ModulePath: modulePath,
			PackageDir: ".", ModuleRelativeDir: ".", Locality: "local", DeclarationsScanned: true,
			LoadCompleteness: completeSurfacePackageLoad(),
			Declarations:     []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: "Open"}},
		}},
	})
	if err != nil || len(catalog.Entries) != 1 {
		t.Fatalf("module-library catalog = %#v / %v", catalog, err)
	}
	target := catalog.Entries[0].Candidate.Target
	var meta debugdump.RunMeta
	bindRunMetaAnalysisTarget(&meta, &target)
	wire, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	var restored debugdump.RunMeta
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.AnalysisTargetRef != target.Ref ||
		restored.AnalysisTargetKind != string(analysistarget.KindModuleLibrary) ||
		restored.AnalysisTargetModule != modulePath || restored.AnalysisTargetDisplayPath != "." ||
		restored.AnalysisTargetPackage != "" {
		t.Fatalf("module-library metadata = %#v", restored)
	}
}

func TestAnalysisTargetRefSeparatesResearchCacheIdentity(t *testing.T) {
	const digest = "0000000000000000000000000000000000000000000000000000000000000000"
	base := modelresearch.FingerprintInput{
		Repository: modelresearch.RepositoryContext{Identity: "repo", Revision: "rev", Scenario: "go-default", AnalysisTargetRef: "at-one"},
		Stage:      "orientation", PromptVersion: "v1", Model: "model",
		ProviderEndpointSHA256: digest, RequestSHA256: digest, EvidenceBundleHash: digest,
	}
	first, err := modelresearch.CacheKey(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Repository.AnalysisTargetRef = "at-two"
	second, err := modelresearch.CacheKey(base)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("cross-target cache key reused: %q", first)
	}
}

func TestTargetRunContainerHandoffPersistsAndProjectsTwoTargetsProviderFree(t *testing.T) {
	repository := t.TempDir()
	for _, directory := range []string{"cmd/app", "cmd/helper", "internal/core"} {
		if err := os.MkdirAll(filepath.Join(repository, filepath.FromSlash(directory)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeSurfaceTestFile(t, repository, "go.mod", "module example.com/multi\n\ngo 1.24\n")
	writeSurfaceTestFile(t, repository, "cmd/app/main.go", "package main\nimport _ \"example.com/multi/internal/core\"\nfunc main() {}\n")
	writeSurfaceTestFile(t, repository, "cmd/helper/main.go", "package main\nfunc main() {}\n")
	writeSurfaceTestFile(t, repository, "internal/core/core.go", "package core\n")
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "go.mod", "cmd/app/main.go", "cmd/helper/main.go", "internal/core/core.go")

	debugDirectory := t.TempDir()
	var received snapshot.TargetRunContainer
	selectorCalls := 0
	_, err := Run(context.Background(), Options{
		RepoPath: repository, AtlasFirst: true, OutputJSON: true,
		DebugDir: debugDirectory, RunID: "multi-target", RequireArtifacts: true, DumpRedacted: true,
		MaxReadmeBytes: 1024, MaxTreeLines: 50, MaxInterestingFiles: 50,
		AnalysisTargetSelector: func(
			_ context.Context,
			_ string,
			catalog analysistarget.TargetCatalog,
		) (snapshot.TargetRunSelection, error) {
			selectorCalls++
			refs := map[string]string{}
			for _, entry := range catalog.Entries {
				refs[entry.DisplayPath] = entry.Candidate.Target.Ref
			}
			return snapshot.TargetRunSelection{
				DefaultTargetRef: refs["cmd/app"],
				TargetRefs:       []string{refs["cmd/helper"], refs["cmd/app"]},
			}, nil
		},
		TargetRunContainerSink: func(container snapshot.TargetRunContainer) {
			received = container
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selectorCalls != 1 || len(received.Targets) != 2 ||
		received.Targets[0].DisplayPath != "cmd/app" ||
		received.Targets[1].DisplayPath != "cmd/helper" {
		t.Fatalf("selector/container = %d / %#v", selectorCalls, received)
	}
	for _, projection := range received.Targets {
		page, err := received.ScopedSnapshot(projection.Target.Ref)
		if err != nil {
			t.Fatalf("project %s: %v", projection.DisplayPath, err)
		}
		if page.AnalysisTarget == nil || page.AnalysisTarget.Ref != projection.Target.Ref ||
			page.TargetCatalog != nil {
			t.Fatalf("target page %s = %#v", projection.DisplayPath, page)
		}
	}

	runDirectory := filepath.Join(debugDirectory, "multi-target")
	artifact, err := os.ReadFile(filepath.Join(runDirectory, snapshot.TargetRunContainerArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := snapshot.DecodeTargetRunContainer(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SHA256 != received.SHA256 || decoded.DefaultTargetRef != received.DefaultTargetRef ||
		len(decoded.Targets) != 2 {
		t.Fatalf("persisted container = %#v", decoded)
	}
	snapshotBytes, err := os.ReadFile(filepath.Join(runDirectory, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	var defaultSnapshot snapshot.Snapshot
	if err := json.Unmarshal(snapshotBytes, &defaultSnapshot); err != nil {
		t.Fatal(err)
	}
	if defaultSnapshot.AnalysisTarget == nil || defaultSnapshot.AnalysisTarget.PackageDir != "cmd/app" {
		t.Fatalf("ordinary downstream snapshot target = %#v", defaultSnapshot.AnalysisTarget)
	}
}

func syntheticOrientTargetFacts() gofacts.Facts {
	return gofacts.Facts{
		Modules:            []gofacts.ModuleFact{{ID: "module-root", ModulePath: "example.com/app", ModuleDir: ".", Main: true}},
		Packages:           []gofacts.PackageFact{{CanonicalPath: "example.com/app/cmd/app", Name: "main", ModuleID: "module-root", ModulePath: "example.com/app", PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app"}},
		EntrypointPackages: []gofacts.Entrypoint{{ModulePath: "example.com/app", ImportPath: "example.com/app/cmd/app", PackageDir: "cmd/app", ModuleDir: ".", Anchors: []gofacts.EntrypointAnchor{{Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain, Path: "cmd/app/main.go", Line: 10}}}},
	}
}
