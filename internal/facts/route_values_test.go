package facts

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A method that calls itself on a field of its own receiver (`n.next.walk()`)
// makes the receiver's origin that same field one level deeper. Reading it is
// recursion and ends as unknown instead of growing the field path forever.
func TestRouteValuesStopAtRecursiveReceiverFields(t *testing.T) {
	owner := sourcevalue.Anchor{Path: "list.go", Line: 3, Column: 1}
	receiver := sourcevalue.Value{Kind: "receiver", Owner: &owner}
	next := sourcevalue.Value{Kind: "field", Text: "next", Parts: []sourcevalue.Value{receiver}}
	index := programindex.Index{
		Objects: []programindex.Object{{
			ID: "walk", Kind: programindex.ObjectMethod, Name: "walk",
			Location: &programindex.Location{Path: "list.go", Line: 3, Column: 1},
		}},
		Relations: []programindex.Relation{{
			ID: "self", Kind: programindex.RelationCalls, FromID: "walk", ToIDs: []string{"walk"},
			Resolution: programindex.ResolutionExact,
			Patterns: []programindex.RelationPattern{{
				ID: "p", Form: programindex.PatternCall, Selector: "walk",
				Location:      &programindex.Location{Path: "list.go", Line: 5, Column: 2},
				ReceiverValue: &next,
			}},
		}},
	}
	target, err := newTargetContext(TargetInput{Index: index, Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan []routeLiteral, 1)
	go func() {
		done <- newRouteValueReader(target).value(&next, []string{"path"}, routeLiteral{}, make(map[string]bool), false)
	}()
	select {
	case got := <-done:
		if len(got) != 0 {
			t.Fatalf("recursive field read produced %v, want unknown", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recursive field read did not stop")
	}
}

// routeTestFunction is a function object at path:line, column 1.
func routeTestFunction(id, path string, line int) programindex.Object {
	return programindex.Object{ID: id, Kind: programindex.ObjectFunction, Name: id, Location: &programindex.Location{Path: path, Line: line, Column: 1}}
}

// routeTestCall is a call from one function to another at path:line,
// column 2, handing over the arguments.
func routeTestCall(from, to, path string, line int, arguments ...programindex.PatternArgument) programindex.Relation {
	id := from + "->" + to + "@" + strconv.Itoa(line)
	return programindex.Relation{
		ID: id, Kind: programindex.RelationCalls, FromID: from, ToIDs: []string{to}, Resolution: programindex.ResolutionExact,
		Patterns: []programindex.RelationPattern{{ID: id, Form: programindex.PatternCall, Selector: to, Location: &programindex.Location{Path: path, Line: line, Column: 2}, Arguments: arguments}},
	}
}

// routeTestParameter is the first parameter of the function at path:owner,
// used at path:line, column 4.
func routeTestParameter(path string, owner, line int) *sourcevalue.Value {
	return &sourcevalue.Value{Kind: "parameter", Text: "p", Position: 1, Anchor: &sourcevalue.Anchor{Path: path, Line: line, Column: 4}, Owner: &sourcevalue.Anchor{Path: path, Line: owner, Column: 1}}
}

func routeTestRead(t *testing.T, read func() []routeLiteral) []routeLiteral {
	t.Helper()
	done := make(chan []routeLiteral, 1)
	go func() { done <- read() }()
	select {
	case got := <-done:
		return got
	case <-time.After(30 * time.Second):
		t.Fatal("the read did not finish")
		return nil
	}
}

// Lua's allocator hands its block pointer on through every caller of the
// allocation functions, each of which has many callers in a call graph with
// cycles; the paths through them multiply although none ends in a literal.
// Here f0's parameter comes from f1 calling it twice with f1's parameter,
// f1's from f2 calling it twice, and so on: 2^40 paths, and f0 calls f40,
// closing a cycle. None of them reads a literal, which the walk now knows
// without following each path. A literal handed to the same parameter of
// the same graph is still read, with the evidence of its one path.
func TestRouteValuesSkipCallerPathsThatReachNoLiteral(t *testing.T) {
	const layers = 40
	var index programindex.Index
	for i := 0; i <= layers; i++ {
		index.Objects = append(index.Objects, routeTestFunction("f"+strconv.Itoa(i), "chain.c", 10*i+1))
	}
	for i := 1; i <= layers; i++ {
		for _, line := range []int{10*i + 2, 10*i + 3} {
			arguments := []programindex.PatternArgument{{ID: "p", Position: 1, Kind: programindex.PatternDynamic, Origin: routeTestParameter("chain.c", 10*i+1, line)}}
			if i == 1 && line == 12 {
				arguments = append(arguments, programindex.PatternArgument{ID: "q", Position: 2, Kind: programindex.PatternLiteralString, Value: "/health"})
			}
			index.Relations = append(index.Relations, routeTestCall("f"+strconv.Itoa(i), "f"+strconv.Itoa(i-1), "chain.c", line, arguments...))
		}
	}
	index.Relations = append(index.Relations, routeTestCall("f0", "f"+strconv.Itoa(layers), "chain.c", 5,
		programindex.PatternArgument{ID: "p", Position: 1, Kind: programindex.PatternDynamic, Origin: routeTestParameter("chain.c", 1, 5)}))
	target, err := newTargetContext(TargetInput{Index: index, Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	values := newRouteValueReader(target)
	pointer := programindex.PatternArgument{Kind: programindex.PatternDynamic, Origin: routeTestParameter("chain.c", 1, 7)}
	if got := routeTestRead(t, func() []routeLiteral { return values.argument(pointer) }); len(got) != 0 {
		t.Fatalf("f0's parameter read %+v, want nothing", got)
	}
	if routeTestRead(t, func() []routeLiteral {
		if resolvesToAddress(values, pointer) {
			return []routeLiteral{{}}
		}
		return nil
	}) != nil {
		t.Fatal("f0's parameter resolves to an address")
	}
	path := programindex.PatternArgument{Kind: programindex.PatternDynamic, Origin: &sourcevalue.Value{Kind: "parameter", Text: "q", Position: 2, Anchor: &sourcevalue.Anchor{Path: "chain.c", Line: 8, Column: 4}, Owner: &sourcevalue.Anchor{Path: "chain.c", Line: 1, Column: 1}}}
	got := routeTestRead(t, func() []routeLiteral { return values.argument(path) })
	want := []Anchor{{Path: "chain.c", Line: 8, Column: 4}, {Path: "chain.c", Line: 12, Column: 2}}
	if len(got) != 1 || got[0].text != "/health" || got[0].possible || !reflect.DeepEqual(got[0].evidence, want) || !resolvesToAddress(values, path) {
		t.Fatalf("f0's second parameter read %+v, want /health through chain.c:12", got)
	}
}

// a and b call each other with their parameter, and main calls a("/y").
// Reading a's parameter cuts the cycle where it comes back to a, so b's
// parameter reads nothing there; read on its own afterwards, b's parameter
// still reads "/y" through a. What a cut cycle found is not what an
// expression leads to.
func TestRouteValuesReadACycleFromEitherEnd(t *testing.T) {
	ap, bp := routeTestParameter("loop.c", 1, 2), routeTestParameter("loop.c", 5, 6)
	index := programindex.Index{
		Objects: []programindex.Object{routeTestFunction("a", "loop.c", 1), routeTestFunction("b", "loop.c", 5), routeTestFunction("main", "loop.c", 9)},
		Relations: []programindex.Relation{
			routeTestCall("a", "b", "loop.c", 2, programindex.PatternArgument{ID: "p", Position: 1, Kind: programindex.PatternDynamic, Origin: ap}),
			routeTestCall("b", "a", "loop.c", 6, programindex.PatternArgument{ID: "p", Position: 1, Kind: programindex.PatternDynamic, Origin: bp}),
			routeTestCall("main", "a", "loop.c", 10, programindex.PatternArgument{ID: "p", Position: 1, Kind: programindex.PatternLiteralString, Value: "/y"}),
		},
	}
	target, err := newTargetContext(TargetInput{Index: index, Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	values := newRouteValueReader(target)
	at := func(line, column int) Anchor { return Anchor{Path: "loop.c", Line: line, Column: column} }
	for _, read := range []struct {
		name  string
		value *sourcevalue.Value
		want  []Anchor
	}{
		{"a's parameter", ap, []Anchor{at(2, 4), at(10, 2)}},
		{"b's parameter", bp, []Anchor{at(6, 4), at(2, 2), at(2, 4), at(10, 2)}},
	} {
		argument := programindex.PatternArgument{Kind: programindex.PatternDynamic, Origin: read.value}
		got := routeTestRead(t, func() []routeLiteral { return values.argument(argument) })
		if len(got) != 1 || got[0].text != "/y" || got[0].possible || !reflect.DeepEqual(got[0].evidence, read.want) || !resolvesToAddress(values, argument) {
			t.Fatalf("%s read %+v, want /y through %v", read.name, got, read.want)
		}
	}
}

// An address argument may also carry diagnostics through a branching caller
// graph. Reading the address must not enumerate 2^40 diagnostic paths after
// the admission walk already established which branches can carry addresses.
// Both written address sites and concatenations keep their original evidence;
// a direct command name still names the registration whatever its spelling.
func TestRegistrationAddressReadSkipsNonAddressCallerBranches(t *testing.T) {
	const layers = 40
	var index programindex.Index
	for i := 0; i <= layers; i++ {
		index.Objects = append(index.Objects, routeTestFunction("f"+strconv.Itoa(i), "branch.c", 10*i+1))
	}
	for i := 1; i <= layers; i++ {
		for _, line := range []int{10*i + 2, 10*i + 3} {
			index.Relations = append(index.Relations, routeTestCall("f"+strconv.Itoa(i), "f"+strconv.Itoa(i-1), "branch.c", line,
				programindex.PatternArgument{Position: 1, Kind: programindex.PatternDynamic, Origin: routeTestParameter("branch.c", 10*i+1, line)}))
		}
	}
	index.Objects = append(index.Objects, routeTestFunction("main", "branch.c", 500))
	index.Relations = append(index.Relations, routeTestCall("main", "f"+strconv.Itoa(layers), "branch.c", 501,
		programindex.PatternArgument{Position: 1, Kind: programindex.PatternLiteralString, Value: "diagnostic"}))
	target, err := newTargetContext(TargetInput{Index: index, Root: "."})
	if err != nil {
		t.Fatal(err)
	}
	at := func(line int) *sourcevalue.Anchor {
		return &sourcevalue.Anchor{Path: "branch.c", Line: line, Column: 4}
	}
	root := &sourcevalue.Value{Kind: "alternatives", Parts: []sourcevalue.Value{
		{Kind: "literal", Text: "/health", Anchor: at(510)},
		*routeTestParameter("branch.c", 1, 511),
		{Kind: "literal", Text: "/health", Anchor: at(512)},
		{Kind: "concat", Parts: []sourcevalue.Value{{Kind: "literal", Text: "/", Anchor: at(513)}, {Kind: "literal", Text: "items", Anchor: at(514)}}},
	}}
	values := newRouteValueReader(target)
	got := routeTestRead(t, func() []routeLiteral {
		return addressLiterals(values, programindex.PatternArgument{Kind: programindex.PatternDynamic, Origin: root})
	})
	anchors := func(lines ...int) []Anchor {
		var result []Anchor
		for _, line := range lines {
			result = append(result, Anchor{Path: "branch.c", Line: line, Column: 4})
		}
		return result
	}
	if len(got) != 2 || got[0].text != "/health" || !got[0].possible || !reflect.DeepEqual(got[0].evidence, anchors(510, 512)) ||
		got[1].text != "/items" || !got[1].possible || !reflect.DeepEqual(got[1].evidence, anchors(513, 514)) {
		t.Fatalf("address read lost a real branch or its evidence: %+v", got)
	}
	named := addressLiterals(values, programindex.PatternArgument{Kind: programindex.PatternLiteralString, Value: "orders.created"})
	if len(named) != 1 || named[0].text != "orders.created" || named[0].possible {
		t.Fatalf("a direct registration name changed: %+v", named)
	}
}
