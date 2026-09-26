package cproject

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// TestRealRepositoryIndex projects every program of a real C repository when
// REPOMAP_C_PROBE names it, and reports what the reader gets: objects and
// relations, the functions main reaches with and without calls through
// function pointers, and table rows that name their function.
func TestRealRepositoryIndex(t *testing.T) {
	root := os.Getenv("REPOMAP_C_PROBE")
	if root == "" {
		t.Skip("optional real repository probe")
	}
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	project, err := Discover(t.Context(), root, repository)
	if err != nil || project == nil {
		t.Fatalf("discover: %v", err)
	}
	store := NewStore()
	var indexes []programindex.Index
	for _, program := range project.Programs {
		parsed, err := Parse(t.Context(), root, repository, program, store)
		if err != nil {
			t.Fatalf("%s: %v", program.Selector, err)
		}
		result, err := Index(repository, parsed)
		if err != nil {
			t.Fatalf("%s: %v", program.Selector, err)
		}
		index, err := programindex.New(result.Input)
		if err != nil {
			t.Fatalf("%s: %v", program.Selector, err)
		}
		indexes = append(indexes, index)
		kinds := map[string]int{}
		for _, object := range index.Objects {
			kinds[string(object.Kind)]++
		}
		relations := map[string]int{}
		for _, relation := range index.Relations {
			if relation.WitnessesOmitted > 0 || relation.PatternsOmitted > 0 {
				t.Errorf("%s: relation %s omits evidence: %+v", program.Selector, relation.ID, relation)
			}
			key := string(relation.Kind) + "/" + string(relation.Resolution)
			if relation.Dispatch != "" {
				key += "/" + relation.Dispatch
			}
			relations[key]++
		}
		packages := map[string]int{}
		for _, object := range index.Objects {
			if object.External != nil {
				packages[string(object.External.AuthorityKind)+":"+object.External.PackagePath]++
			}
		}
		t.Logf("%s: external symbols by package %v", program.Selector, packages)
		direct, pointers, all, functions := reach(index)
		t.Logf("%s: %d objects %v; %d relations %v; %d dependencies", program.Selector, len(index.Objects), kinds, len(index.Relations), relations, len(result.Dependencies.Dependencies))
		t.Logf("%s: main reaches %d of %d functions by direct calls, %d adding calls through function pointers, %d adding callbacks", program.Selector, direct, functions, pointers, all)
		var rows []string
		for _, relation := range index.Relations {
			if relation.Invocation != programindex.InvocationConstruct {
				continue
			}
			for _, pattern := range relation.Patterns {
				var parts []string
				for _, argument := range pattern.Arguments {
					value := argument.Value
					for _, id := range argument.ObjectIDs {
						value = objectName(index, id)
					}
					parts = append(parts, argument.Keyword+"="+value)
				}
				rows = append(rows, pattern.Selector+"{"+strings.Join(parts, ", ")+"}")
			}
		}
		t.Logf("%s: %d constructions: %s", program.Selector, len(rows), strings.Join(rows, " "))
		for _, relation := range index.Relations {
			if relation.Dispatch != programindex.DispatchFunctionValue {
				continue
			}
			var targets []string
			for _, id := range relation.ToIDs {
				targets = append(targets, objectName(index, id))
			}
			if len(targets) > 6 {
				targets = append(targets[:6], "...")
			}
			candidates := 0
			for _, witness := range relation.Witnesses {
				if witness.Kind == "c_function_pointer_store" {
					candidates++
				}
			}
			t.Logf("  %s:%d %s -> %s %s %v (%d store witnesses)", relation.Location.Path, relation.Location.Line, objectName(index, relation.FromID), relation.Patterns[0].Selector, relation.Resolution, targets, candidates)
		}
	}
	layer, graph := buildPlaces(t, repository, indexes...)
	registrations := map[string]int{}
	for _, fact := range layer.OfKind(facts.KindRegistration) {
		registrations[fact.TargetID+" "+fact.Key]++
	}
	boundaries, bindings, literal := 0, 0, 0
	for _, place := range graph.Places {
		if place.Boundary != nil && place.Boundary.Source == "fact" {
			boundaries++
		}
		if place.Symbol != nil {
			for _, binding := range place.Symbol.Bindings {
				bindings++
				if len(binding.Arguments) > 0 {
					literal++
				}
			}
		}
	}
	t.Logf("registrations by target and word %v; %d fact boundaries; %d binding rows, %d with registration literals", registrations, boundaries, bindings, literal)
}

func objectName(index programindex.Index, id string) string {
	for _, object := range index.Objects {
		if object.ID == id {
			return object.Name
		}
	}
	return id
}

// reach counts the functions the seeds reach through direct calls alone,
// adding the targets of calls through function pointers, and adding
// callbacks handed over.
func reach(index programindex.Index) (direct, pointers, all, functions int) {
	isFunction := map[string]bool{}
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectFunction {
			isFunction[object.ID] = true
		}
	}
	count := func(follow func(programindex.Relation) bool) int {
		seen := map[string]bool{}
		var queue []string
		for _, seed := range index.Target.Seeds {
			queue = append(queue, seed.ObjectID)
		}
		edges := map[string][]string{}
		for _, relation := range index.Relations {
			if follow(relation) {
				edges[relation.FromID] = append(edges[relation.FromID], relation.ToIDs...)
			}
		}
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if seen[id] {
				continue
			}
			seen[id] = true
			queue = append(queue, edges[id]...)
		}
		n := 0
		for id := range seen {
			if isFunction[id] {
				n++
			}
		}
		return n
	}
	direct = count(func(relation programindex.Relation) bool {
		return relation.Kind == programindex.RelationCalls && relation.Dispatch == ""
	})
	pointers = count(func(relation programindex.Relation) bool {
		return relation.Kind == programindex.RelationCalls
	})
	all = count(func(relation programindex.Relation) bool {
		return relation.Kind == programindex.RelationCalls || relation.Kind == programindex.RelationPassesCallback
	})
	keys := make([]string, 0, len(isFunction))
	for id := range isFunction {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	return direct, pointers, all, len(keys)
}
