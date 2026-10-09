package places

import (
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"os"
	"testing"
)

func TestClojureDocstringFollowsItsOwnDeclaration(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/repositories/clojure/src/example/service.cljc")
	if err != nil {
		t.Fatal(err)
	}
	file := "src/example/service.cljc"
	b := builder{docs: map[string][]claims.Claim{}}
	for _, doc := range clojureproject.Docstrings(data) {
		b.docs[file] = append(b.docs[file], claims.Claim{Line: doc.Line, Text: doc.Text})
	}
	decls := []atlas.Decl{{LineNo: 4}, {LineNo: 9}}
	if got := b.docstringFor(file, 4, decls); got != "Build the greeting before delivery." {
		t.Fatalf("docstring = %q", got)
	}
	if got := b.docstringFor(file, 9, decls); got != "" {
		t.Fatalf("previous declaration's docstring leaked: %q", got)
	}
}

func TestClojureNamespaceQuoteDoesNotDescribeSameLineFunction(t *testing.T) {
	const file = "sample.clj"
	data := []byte(`(ns sample "Namespace description.") (defn silent [] 1)`)
	b := builder{docs: map[string][]claims.Claim{}}
	for _, doc := range clojureproject.Docstrings(data) {
		b.docs[file] = append(b.docs[file], claims.Claim{Line: doc.Line, Column: doc.Column, DeclarationLine: doc.DeclarationLine, DeclarationColumn: doc.DeclarationColumn, Text: doc.Text})
	}
	if len(b.docs[file]) != 1 || b.docs[file][0].Text != "Namespace description." {
		t.Fatal("namespace original quote lost")
	}
	if got := b.docstringFor(file, 1, []atlas.Decl{{LineNo: 1, Column: 37}}, 37); got != "" {
		t.Fatalf("namespace quote became neighboring function doc: %q", got)
	}
}
