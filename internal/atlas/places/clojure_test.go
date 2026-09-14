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
