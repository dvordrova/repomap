package pythonprogramindex

import (
	"fmt"
	"slices"
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

// A `@typing.overload` stub, known by what its decorator resolves to, is a
// signature of the implementation that follows it in its statement list or
// one enclosing it: one declaration keeping each stub's signature, place
// and lines, and every call reaches the implementation alone (beets's
// BeatportClient.search had listed three times).
func TestCumulativePythonOverloadStubsFoldIntoTheirImplementation(t *testing.T) {
	const path = "src/fixture_app/generic_types.py"
	repository := pythonCorpus(t, cumulativePythonSources(t, path))
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string][]programindex.Object{}
	names := map[string]string{}
	for _, object := range index.Objects {
		names[object.ID] = object.Name
		if object.Location != nil && object.Location.Path == path {
			byName[object.Name] = append(byName[object.Name], object)
		}
	}
	want := map[string][]string{
		"pick":         {"28 pick(items: list[str]) -> str", "30 pick(items: list[int]) -> int"},
		"choose":       {"57 choose(self, items: list[str]) -> str", "59 choose(self, items: list[int]) -> int"},
		"checked_pick": {"66 checked_pick(items: list[str]) -> str"},
	}
	for name, overloads := range want {
		if len(byName[name]) != 1 {
			t.Fatalf("%s is %d declarations, want one: %+v", name, len(byName[name]), byName[name])
		}
		var said []string
		for _, overload := range byName[name][0].Overloads {
			said = append(said, fmt.Sprintf("%d %s", overload.Location.Line, overload.Signature))
		}
		if !slices.Equal(said, overloads) {
			t.Fatalf("%s overloads %q, want %q", name, said, overloads)
		}
	}
	if overload := byName["pick"][0].Overloads[0]; overload.CodeLines != 1 || len(overload.Parameters) != 1 || overload.Parameters[0].Type != "list[str]" {
		t.Fatalf("a stub lost its lines or its typed values: %+v", overload)
	}
	// Each call from pick_all reaches one callable, the implementation.
	var calls []string
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationCalls && names[relation.FromID] == "pick_all" {
			var targets []string
			for _, id := range relation.ToIDs {
				targets = append(targets, names[id])
			}
			calls = append(calls, strings.Join(targets, "|"))
		}
	}
	slices.Sort(calls)
	if !slices.Equal(calls, []string{"checked_pick", "choose", "pick"}) {
		t.Fatalf("pick_all calls %q", calls)
	}
}
