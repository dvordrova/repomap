package adaptertest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// AssertDeclarationSignatures checks the signature each named declaration of
// one fixture file carries into that file's declarations, the text a model
// reads. A generic declaration keeps its whole type-parameter list; a type's
// body, fields and field tags are declarations of their own, never signature
// text.
func AssertDeclarationSignatures(t testing.TB, graph atlas.Graph, path string, want map[string]string) {
	t.Helper()
	got := make(map[string]string)
	for _, place := range graph.Places {
		if place.Kind != atlas.PlaceFile || place.Path != path || place.File == nil {
			continue
		}
		for _, decl := range place.File.Decls {
			got[decl.Name] = decl.Signature
		}
	}
	for name, signature := range want {
		if actual, ok := got[name]; !ok || actual != signature {
			t.Errorf("%s declaration %s signature = %q (declared: %t), want %q", path, name, actual, ok, signature)
		}
	}
}
