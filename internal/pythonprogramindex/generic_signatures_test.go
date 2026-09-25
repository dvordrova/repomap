package pythonprogramindex

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// Type parameters are part of a generic declaration in both spellings: the
// Generic[T] base and PEP 695 brackets after the name.
func TestCumulativePythonGenericDeclarationsKeepTypeParameters(t *testing.T) {
	const path = "src/fixture_app/generic_types.py"
	repository := pythonCorpus(t, cumulativePythonSources(t, path))
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Revision: strings.Repeat("a", 40), Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.AssertDeclarationSignatures(t, graph, path, map[string]string{
		"Box":   "class Box(Generic[T])",
		"Crate": "class Crate[T]",
		"Keyed": "class Keyed[K: str, V: (int, str)](Box[V])",
		"first": "first[T](items: list[T]) -> T",
	})
}
