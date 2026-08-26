package contracttest

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const goFixtureAppPackage = "example.com/repomap/cumulative-go-fixture/cmd/app"

type goFixtureAuthorities struct {
	target   analysistarget.Target
	direct   surfacediscovery.DirectCallIndex
	external surfacediscovery.ExternalCallIndex
	core     gocoreobject.Index
	dynamic  godynamichandoff.Index
}

func TestCumulativeGoRepositoryDiscoveryAndProgramIndexContract(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	repositoryPath, repository := materializeFixtureRepository(t, "go")
	authorities := analyzeGoFixture(t, repositoryPath, repository)
	assertUnusedPrivateMethodHasNoDanglingDirectNode(t, authorities)

	index, err := goadapter.Build(
		repository,
		authorities.target,
		authorities.direct,
		authorities.external,
		authorities.core,
		authorities.dynamic,
	)
	if err != nil {
		t.Fatalf("build Go ProgramIndex: %v", err)
	}
	assertProgramIndexRoundTrip(t, index)
	if index.Target.Language != "go" || index.Target.Selector == "" || index.Target.Name != goFixtureAppPackage {
		t.Fatalf("Go ProgramIndex target = %#v", index.Target)
	}
	if !programIndexHasObject(index, programindex.ObjectMethod, "recreateStore") {
		t.Fatal("Go ProgramIndex omitted unused private method recreateStore")
	}

}

func analyzeGoFixture(
	t *testing.T,
	repositoryPath string,
	repository *corpus.Corpus,
) goFixtureAuthorities {
	t.Helper()
	var authorities goFixtureAuthorities
	err := orient.Run(t.Context(), orient.Options{
		RepoPath: repositoryPath, GoTarget: runtime.GOOS + "/" + runtime.GOARCH, RepositoryCorpus: repository,
		DebugDir: t.TempDir(), RunID: "cumulative-go-contract", RequireArtifacts: true,
		AnalyzeGoProgram:       true,
		AnalysisTargetSelector: selectGoFixtureTarget,
		AnalysisTargetSink: func(value analysistarget.Target) {
			authorities.target = value
		},
		DirectCallIndexSink: func(value surfacediscovery.DirectCallIndex) {
			authorities.direct = value
		},
		ExternalCallIndexSink: func(value surfacediscovery.ExternalCallIndex) {
			authorities.external = value
		},
		CoreObjectIndexSink: func(value gocoreobject.Index) {
			authorities.core = value
		},
		DynamicHandoffIndexSink: func(value godynamichandoff.Index) {
			authorities.dynamic = value
		},
	})
	if err != nil {
		t.Fatalf("analyze cumulative Go fixture: %v", err)
	}
	if authorities.target.Ref == "" || authorities.direct.SHA256 == "" ||
		authorities.external.SHA256 == "" || authorities.core.SHA256 == "" ||
		authorities.dynamic.SHA256 == "" {
		t.Fatalf("Go fixture analysis omitted producer authority: %#v", authorities)
	}
	return authorities
}

func assertUnusedPrivateMethodHasNoDanglingDirectNode(t *testing.T, authorities goFixtureAuthorities) {
	t.Helper()
	var method gocoreobject.CallableDeclaration
	for _, callable := range authorities.core.Callables {
		if callable.Name == "recreateStore" && strings.HasSuffix(
			callable.Receiver, "/internal/storefixture.fileStoreTestBundle",
		) {
			method = callable
			break
		}
	}
	if method.ID == "" {
		t.Fatal("CoreObjectIndex omitted (*fileStoreTestBundle).recreateStore")
	}
	if method.DirectCallNodeID != "" {
		t.Fatalf("unused private method gained dangling direct-call node %q", method.DirectCallNodeID)
	}
	for _, node := range authorities.direct.Nodes {
		if node.Symbol.Name == "recreateStore" {
			t.Fatalf("unused private method unexpectedly entered DirectCallIndex: %#v", node)
		}
	}
}

func programIndexHasObject(index programindex.Index, kind programindex.ObjectKind, name string) bool {
	for _, object := range index.Objects {
		if object.Kind == kind && object.Name == name {
			return true
		}
	}
	return false
}

func selectGoFixtureTarget(
	_ context.Context,
	_ string,
	catalog analysistarget.TargetCatalog,
	_ gofacts.Facts,
) (snapshot.TargetRunSelection, error) {
	for _, entry := range catalog.Entries {
		target := entry.Candidate.Target
		if target.PackagePath == goFixtureAppPackage {
			return snapshot.TargetRunSelection{
				DefaultTargetRef: target.Ref,
				TargetRefs:       []string{target.Ref},
			}, nil
		}
	}
	return snapshot.TargetRunSelection{}, fmt.Errorf("cumulative Go fixture target %q is absent", goFixtureAppPackage)
}
