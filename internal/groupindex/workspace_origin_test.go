package groupindex

import (
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestObjectFactsKeepCanonicalWorkspaceOrigin(t *testing.T) {
	external := &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage,
		PackagePath: "got", Name: "get", RepositoryPath: "shared"}
	facts := ObjectFacts{Name: "got.get", Kind: programindex.ObjectExternalSymbol, Visibility: programindex.VisibilityPublic,
		SymbolLinkIdentities: []programindex.SymbolLinkIdentity{}, External: cloneExternal(external)}
	external.RepositoryPath = "changed"
	if facts.External.RepositoryPath != "shared" {
		t.Fatal("group facts did not copy the compiler-owned origin")
	}
	for _, directory := range []string{"", ".", "shared", "한글/shared"} {
		facts.External.RepositoryPath = directory
		if !validObjectFacts(facts) {
			t.Fatalf("valid workspace directory %q rejected", directory)
		}
	}
	for _, directory := range []string{"..", "../shared", "/shared", "a//b", "a/../b", "a\\b", " shared", "a\nb"} {
		facts.External.RepositoryPath = directory
		if validObjectFacts(facts) {
			t.Fatalf("invalid workspace directory %q accepted", directory)
		}
	}
	facts.External.RepositoryPath = "."
	facts.External.AuthorityKind = programindex.ExternalAuthorityPlatform
	if validObjectFacts(facts) {
		t.Fatal("platform acquired repository package authority")
	}
}
