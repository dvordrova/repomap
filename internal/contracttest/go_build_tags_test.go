package contracttest

import (
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

// cmd/vfs and root_vfs.go build only with the fixturevfs tag, which the
// fixture's Makefile passes to `go build ./cmd/vfs` through VFS_TAGS, as
// litestream's Makefile builds cmd/litestream-vfs. The run's own load sees
// no program there; target discovery reads the recipe, so the catalogue
// holds cmd/vfs with its tag, the recipe line and the run's platform, and
// the program is analysed with the tag: it reaches OpenReplica, which only
// root_vfs.go declares, and reads FIXTURE_VFS_REPLICA_URL.
func TestCumulativeGoProgramBuiltWithTagsFromItsMakefile(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	platform := runtime.GOOS + "/" + runtime.GOARCH
	deferred, err := snapshot.BuildContext(t.Context(), snapshot.Options{RepoPath: root, GoTarget: platform, RepositoryCorpus: repository})
	if err != nil {
		t.Fatal(err)
	}
	for _, entrypoint := range deferred.GoFacts.EntrypointPackages {
		if entrypoint.PackageDir == "cmd/vfs" {
			t.Fatal("the run's own load made cmd/vfs a program without its tag")
		}
	}
	var vfs *analysistarget.TargetCatalogEntry
	for index, entry := range deferred.TargetCatalog.Entries {
		if entry.Candidate.Target.PackagePath == goFixtureRootPackage+"/cmd/vfs" {
			vfs = &deferred.TargetCatalog.Entries[index]
		}
	}
	if vfs == nil {
		t.Fatal("cmd/vfs is not a target")
	}
	target := vfs.Candidate.Target
	if vfs.Candidate.Key != goFixtureRootPackage+"@.::"+goFixtureRootPackage+"/cmd/vfs" ||
		!slices.Equal(target.BuildTags, []string{"fixturevfs"}) || target.BuildPlatform != platform ||
		!reflect.DeepEqual(target.BuildSources, []gofacts.BuildTagSource{{Path: "Makefile", Line: 7}}) {
		t.Fatalf("cmd/vfs: key %s, tags %v for %s from %v", vfs.Candidate.Key, target.BuildTags, target.BuildPlatform, target.BuildSources)
	}
	// The target portfolio reads the tags beside the main function.
	candidates, _, err := analysistarget.DiscoverGoTargetFilesWithResolver(repository, *deferred.GoFacts, *deferred.TargetCatalog)
	if err != nil {
		t.Fatal(err)
	}
	mainRef, _ := repository.ID("cmd/vfs/main.go")
	hypotheses := ""
	for _, candidate := range candidates {
		if candidate.FileRef == mainRef {
			hypotheses = strings.Join(candidate.Hypotheses, "; ")
		}
	}
	if !strings.Contains(hypotheses, "builds only with Go build tags fixturevfs, as Makefile:7 builds it") {
		t.Fatalf("cmd/vfs/main.go hypotheses: %q", hypotheses)
	}

	scoped, err := snapshot.ScopeAnalysisTarget(deferred, target.Ref)
	if err != nil {
		t.Fatal(err)
	}
	var rootFiles []string
	for _, pkg := range scoped.GoFacts.Packages {
		if pkg.CanonicalPath == goFixtureRootPackage {
			rootFiles = pkg.Files
		}
	}
	if !slices.Contains(rootFiles, "root_vfs.go") {
		t.Fatalf("the tagged program's root package files %v lack root_vfs.go", rootFiles)
	}
	input, err := goadapter.AnalysisInput(scoped.GoFacts, scoped.AnalysisTarget)
	if err != nil {
		t.Fatal(err)
	}
	options := surfacediscovery.DefaultOptions(root, platform)
	options.BuildTags = target.BuildTags
	options.CaptureExternalCallIndex = true
	options.CaptureCoreObjectIndex = true
	options.CaptureDynamicHandoffIndex = true
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(), options, input)
	if err != nil {
		t.Fatal(err)
	}
	index, err := goadapter.Build(repository, scoped.AnalysisTarget.Snapshot(), scoped.GoFacts.PackageOrigins,
		result.DirectCallIndex.Snapshot(), result.ExternalCallIndex.Snapshot(), result.CoreObjectIndex.Snapshot(),
		result.DynamicHandoffIndex.Snapshot(), scoped.GoFacts.TestSources)
	if err != nil {
		t.Fatal(err)
	}
	openReplica := programIndexObjectNamed(t, index, programindex.ObjectFunction, "OpenReplica", "root_vfs.go")
	called := false
	for _, relation := range index.Relations {
		called = called || relation.Kind == programindex.RelationCalls && slices.Contains(relation.ToIDs, openReplica.ID)
	}
	if !called {
		t.Fatal("main does not reach OpenReplica, which only the tagged build declares")
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	reads := false
	for _, fact := range layer.OfKind(facts.KindConfigRead) {
		reads = reads || fact.Key == "FIXTURE_VFS_REPLICA_URL"
	}
	if !reads {
		t.Fatal("the tagged program's environment read is no fact")
	}
}
