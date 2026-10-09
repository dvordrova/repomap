package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Use the cumulative fixture's actual native result, never synthetic locations.
func assertNativeAdjacentCommentOwners(t *testing.T, root string, repository *corpus.Corpus, index programindex.Index, source, first, second string) {
	t.Helper()
	runFixtureGit(t, root, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "--no-gpg-sign", "-m", "fixture")
	quoted, err := claims.Extract(t.Context(), claims.Input{Repository: repository, RepoPath: root, Revision: "HEAD", ReadIndexes: []func() (programindex.Index, error){func() (programindex.Index, error) { return index, nil }}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}, Claims: quoted})
	if err != nil {
		t.Fatal(err)
	}
	var owner, neighbor *programindex.Location
	for _, object := range index.Objects {
		if object.Location == nil || object.Location.Path != source {
			continue
		}
		switch object.Name {
		case first:
			owner = object.Location
		case second:
			neighbor = object.Location
		}
	}
	if owner == nil || neighbor == nil || owner.Line != neighbor.Line || owner.Column == neighbor.Column {
		t.Fatalf("native adjacent declarations: %+v %+v", owner, neighbor)
	}
	const text = "The first function keeps its own description."
	foundQuote := false
	for _, quote := range quoted.Claims {
		if quote.Path == source && quote.Text == text {
			foundQuote = true
			if quote.DeclarationLine != owner.Line || quote.DeclarationColumn != owner.Column {
				t.Fatalf("quote changed native declaration tuple: %+v, owner %+v", quote, owner)
			}
		}
	}
	if !foundQuote {
		t.Fatal("original author quote lost")
	}
	if index.Target.Language == "c" {
		foundPrototype := false
		for _, quote := range quoted.Claims {
			if quote.Path == source && quote.Text == "The prototype owns this distinct author contract." {
				foundPrototype = true
				if !quote.DeclarationUnresolved || quote.DeclarationColumn != 0 {
					t.Fatalf("prototype borrowed a later native definition: %+v", quote)
				}
			}
		}
		if !foundPrototype {
			t.Fatal("unbound prototype author quote lost")
		}
		for _, place := range graph.Places {
			if place.Path == source && place.Symbol != nil && place.Symbol.Decl.Name == "prototypeNeighbor" && place.Symbol.Decl.Doc != "" {
				t.Fatalf("prototype neighbor inherited comment: %+v", place.Symbol.Decl)
			}
		}
	}
	seen := map[string]bool{}
	for _, place := range graph.Places {
		if place.Path != source || place.Symbol == nil {
			continue
		}
		name := place.Symbol.Decl.Name
		if name != first && name != second {
			continue
		}
		seen[name] = true
		want := ""
		if name == first {
			want = text
		}
		if place.Symbol.Decl.Doc != want {
			t.Errorf("%s owns %q, want %q", name, place.Symbol.Decl.Doc, want)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("ordinary declarations absent: %v", seen)
	}
}
