package pythonprogramindex

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/programindex"
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
		"Box":           "class Box(Generic[T])",
		"Crate":         "class Crate[T]",
		"Keyed":         "class Keyed[K: str, V: (int, str)](Box[V])",
		"first":         "first[T](items: list[T]) -> T",
		"Checked":       "class Checked[T: Annotated[object, lambda value: value is not None]]",
		"first_checked": "first_checked[T: Annotated[object, lambda value: value is not None]](items: list[T]) -> T",
	})
	// A lambda in a type parameter's bound belongs to the defining scope.
	objects := make(map[string]programindex.Object)
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	bounds := 0
	for _, object := range index.Objects {
		if object.Kind != programindex.ObjectLambda || object.Location == nil || object.Location.Path != path {
			continue
		}
		if container := objects[object.ContainerID]; container.Kind != programindex.ObjectModule || container.Name != "fixture_app.generic_types" {
			t.Fatalf("type parameter lambda %s belongs to %#v", object.Name, container)
		}
		bounds++
	}
	if bounds != 2 {
		t.Fatalf("type parameter lambdas = %d, want 2", bounds)
	}
}
