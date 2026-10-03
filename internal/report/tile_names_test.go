package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A part whose declarations share one namespace (Clojure's ns/name) names
// its tiles without it; one of another namespace, or with none, keeps every
// name whole.
func TestAPartsSharedNamespaceIsSaidOnce(t *testing.T) {
	symbols := func(names ...string) []pageNodeSymbol {
		var out []pageNodeSymbol
		for _, name := range names {
			out = append(out, pageNodeSymbol{Name: name, Kind: "function"})
		}
		return out
	}
	if got := sharedNamespace(symbols("othello.ui.host/on-press", "othello.ui.host/on-key")); got != "othello.ui.host/" {
		t.Fatalf("shared namespace = %q", got)
	}
	for _, names := range [][]string{{"othello.ui.host/on-press", "othello.ui.draw/draw-state"}, {"othello.ui.host/on-press", "main"}, {"processCommand"}} {
		if got := sharedNamespace(symbols(names...)); got != "" {
			t.Fatalf("%q share %q", names, got)
		}
	}
	// A field of a record keeps the part's namespace question to its
	// declarations.
	withField := append(symbols("othello.board/make-board"), pageNodeSymbol{Name: "cells", Kind: "field"})
	if got := sharedNamespace(withField); got != "othello.board/" {
		t.Fatalf("a field decided the namespace: %q", got)
	}
	// Most of a part's declarations in one namespace are named without it;
	// the one of another keeps its whole name.
	if got := sharedNamespace(symbols("othello.ai/move", "othello.ai.search/choose", "othello.ai.search/negamax")); got != "othello.ai.search/" {
		t.Fatalf("the namespace most share = %q", got)
	}
}

// A callable written inline is read by the name GroupsIndex gives it,
// never by its compiler's number, and still stands on no tile: GroupsIndex
// says it is one (ObjectFacts.Anonymous). A name with a dollar sign is no
// such word: JavaScript's `export function price$()` is a tile and a
// member like any declaration (external review, 2026-10-03).
func TestAnInlineCallableIsReadByItsInlineName(t *testing.T) {
	builder := &pageBuilder{links: pageLinks{repositoryURL: "https://github.com/o/r", blobPrefix: "/blob/", revision: "abc"}}
	closure := groupindex.Subject{ID: "n42", Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{
		Name: "Run$1", Kind: programindex.ObjectFunction, Inline: "ReplicateCommand.Run (inline)", Anonymous: true,
		Location: &programindex.Location{Path: "cmd/litestream/replicate.go", Line: 183, Column: 7}}}
	if name, _ := builder.subjectDisplay(closure); name != "ReplicateCommand.Run (inline)" {
		t.Fatalf("the closure reads %q", name)
	}
	if !builder.inline(closure) {
		t.Fatal("a compiler-numbered callable would stand on a tile")
	}
	named := groupindex.Subject{ID: "n12", Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: "Run", Kind: programindex.ObjectMethod,
		Location: &programindex.Location{Path: "cmd/litestream/replicate.go", Line: 90, Column: 1}}}
	if name, _ := builder.subjectDisplay(named); name != "Run" || builder.inline(named) {
		t.Fatalf("a named method reads %q, inline %v", name, builder.inline(named))
	}
	for _, name := range []string{"price$", "active$", "price$1"} {
		dollar := groupindex.Subject{ID: "n7", Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityPublic,
			Location: &programindex.Location{Path: "src/price.js", Line: 1, Column: 17}}}
		if builder.inline(dollar) {
			t.Fatalf("JavaScript's %s stands on no tile", name)
		}
	}
}
