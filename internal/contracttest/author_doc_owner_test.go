package contracttest

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Reuse the cumulative test's actual Go compiler/index, then consume its
// exact declaration locations beside the ordinary extracted author claims.
func assertGoAttachedAuthorCommentOwnership(t *testing.T, root string, repository *corpus.Corpus, index programindex.Index) {
	t.Helper()
	runFixtureGit(t, root, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "--no-gpg-sign", "-m", "fixture")
	quoted, err := claims.Extract(t.Context(), claims.Input{Repository: repository, RepoPath: root, Revision: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}, Claims: quoted})
	if err != nil {
		t.Fatal(err)
	}
	const source = "internal/storefixture/author_doc.go"
	const purpose = "DocumentedEntry retains an author's description beside its declaration."
	var attached claims.Claim
	for _, claim := range quoted.Claims {
		if claim.Path == source && strings.HasPrefix(claim.Text, purpose) {
			attached = claim
		}
	}
	if attached.DeclarationLine-attached.Line <= 12 {
		t.Fatalf("long author quote lost its exact declaration: %+v", attached)
	}
	want := map[string]string{
		"DocumentedEntry": attached.Text, "UndocumentedEntry": "",
		"DocumentedEntry.Label": "Label returns the authored entry name.", "DocumentedEntry.UndocumentedLabel": "",
		"Package":            "Package holds a name in a source declaration, not a package description.",
		"SameLineDocumented": "SameLineDocumented keeps its own author statement.", "SameLineUndocumented": "",
	}
	seen := map[string]bool{}
	for _, place := range graph.Places {
		if place.Path != source {
			continue
		}
		if place.File != nil && place.File.Doc != "" {
			t.Fatalf("attached author quote became module documentation: %q", place.File.Doc)
		}
		if place.Symbol == nil {
			continue
		}
		if doc, check := want[place.Symbol.Decl.Name]; check {
			seen[place.Symbol.Decl.Name] = true
			if place.Symbol.Decl.Doc != doc {
				t.Errorf("%s: quote %q, want %q", place.Symbol.Decl.Name, place.Symbol.Decl.Doc, doc)
			}
			if place.Symbol.Decl.Name == "DocumentedEntry" && (place.Symbol.Decl.LineNo != attached.DeclarationLine || place.Symbol.Decl.Column != attached.DeclarationColumn) {
				t.Errorf("author quote names declaration %d, native type is at %d", attached.DeclarationLine, place.Symbol.Decl.LineNo)
			}
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("native declarations missing: %v", seen)
	}
	var first, second *programindex.Location
	for _, object := range index.Objects {
		if object.Location != nil && object.Location.Path == source {
			switch object.Name {
			case "SameLineDocumented":
				first = object.Location
			case "SameLineUndocumented":
				second = object.Location
			}
		}
	}
	if first == nil || second == nil || first.Line != second.Line || first.Column == second.Column {
		t.Fatalf("same-line Go declarations lost native name positions: %+v %+v", first, second)
	}
	for _, claim := range quoted.Claims {
		if claim.Path == source && claim.Text == "SameLineDocumented keeps its own author statement." && (claim.DeclarationLine != first.Line || claim.DeclarationColumn != first.Column) {
			t.Fatalf("Go AST author ownership changed native position: %+v", claim)
		}
	}
}
