package jstsproject

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The TypeScript equivalent of C's conditional callee
// (src/stored-callbacks.ts): a callee a condition chooses among the functions
// its branches name calls one of them, alternatives through a function value.
// A class a condition chooses constructs one of them, each by its own
// constructor, never the one the compiler's union signature picks. A branch
// naming anything else leaves the call open.
func TestCumulativeJSTSACalleeChosenByAConditionCallsOneOfItsFunctions(t *testing.T) {
	const path = "src/stored-callbacks.ts"
	index, names := storedCallbacksIndex(t, path)
	want := map[int]string{
		101: "watchTick->tickMillis,tickSeconds alternatives function_value",
		// A nested condition and an asserted, parenthesised name.
		102: "watchTick->tickMillis,tickSeconds,tickTenths alternatives function_value",
		// A parameter is no function the index knows: the call stays open.
		103: "watchTick-> unresolved ",
		// A literal condition takes its branch; one function is the call.
		107: "watchTickAgain->tickSeconds exact ",
		108: "watchTickAgain->tickMillis exact ",
		109: "watchTickAgain->MillisClock.constructor,SecondsClock.constructor alternatives function_value",
		110: "watchTickAgain-> unresolved ",
	}
	got := map[int][]string{}
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != path || relation.Kind != programindex.RelationCalls || want[relation.Location.Line] == "" {
			continue
		}
		var to []string
		for _, id := range relation.ToIDs {
			to = append(to, names[id])
		}
		slices.Sort(to)
		line := relation.Location.Line
		got[line] = append(got[line], fmt.Sprintf("%s->%s %s %s", names[relation.FromID], strings.Join(to, ","), relation.Resolution, relation.Dispatch))
		chose := slices.ContainsFunc(relation.Witnesses, func(witness programindex.Witness) bool {
			return witness.Kind == "typescript_conditional_callee" && witness.Location != nil && witness.Location.Line == line &&
				strings.HasSuffix(witness.Detail, ", as its condition decides")
		})
		if chose != (relation.Resolution == programindex.ResolutionAlternatives) {
			t.Fatalf("line %d: conditional callee witness present=%v for a %s call: %+v", line, chose, relation.Resolution, relation)
		}
	}
	for line, expected := range want {
		if !slices.Equal(got[line], []string{expected}) {
			t.Fatalf("line %d: calls %q, want [%q]", line, got[line], expected)
		}
	}
}

// storedCallbacksIndex builds the cumulative fixture's stored-callbacks
// example with the project files it needs.
func storedCallbacksIndex(t *testing.T, path string) (programindex.Index, map[string]string) {
	t.Helper()
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "vitest.config.ts", "src/test-setup.ts", "src/market.test.ts", path}
	for _, file := range tracked {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, file, string(contents))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, index, _, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, object := range index.Objects {
		names[object.ID] = object.Name
	}
	return index, names
}

// A call through a variable calls what the stores that may reach it put
// there (owner, 2026-09-30), the TypeScript equivalent of Python's name a
// branch reassigns and Go's phi of a function value (src/stored-callbacks.ts).
// Several targets are alternatives through a function value, each reaching
// store a witness; an open call names every store.
func TestCumulativeJSTSACallThroughAVariableCallsWhatItsStoresPutThere(t *testing.T) {
	const path = "src/stored-callbacks.ts"
	index, names := storedCallbacksIndex(t, path)
	want := map[int]string{
		128: "callConstant -> acceptClient exact ",
		135: "callChosenLet -> acceptClient,flushReplies alternatives function_value | acceptClient stored in handler under a condition@133, flushReplies stored in handler under a condition@134",
		141: "callTypedLet -> acceptClient,flushReplies alternatives function_value | flushReplies stored in handler@139, acceptClient stored in handler under a condition@140",
		// The store no branch skips overwrites what came before it.
		148: "callOverwritten -> flushReplies exact ",
		// A later store reaches the call around the loop.
		154: "callInLoop -> acceptClient,flushReplies alternatives function_value | flushReplies stored in handler@152, acceptClient stored in handler under a condition@155",
		// A closure runs at a time the index does not know: any store.
		162: "callFromClosure.returned_handler -> acceptClient,flushReplies alternatives function_value | flushReplies stored in handler@160, acceptClient stored in handler under a condition@161",
		// A logical assignment is a write the index does not follow.
		168: "callAfterCompound ->  unresolved  | acceptClient stored in handler@166",
		// A factory's result is no plain function: the call of the constant stays.
		173: "callFactory -> callFactory.handler exact ",
		// A callback is such a closure, an array's too, though map runs it
		// before the later store: the limit JSTS.md records.
		182: "callInMapOfConstant -> acceptClient exact ",
		187: "callInMapBeforeStore -> acceptClient exact ",
		// A store of no plain function leaves a closure's call open.
		194: "callFromClosureOverFactory.returned_handler ->  unresolved  | acceptClient stored in handler@192",
	}
	for _, relation := range index.Relations {
		line := lineOrZero(relation.Location)
		expected, ok := want[line]
		if !ok || relation.Location.Path != path || relation.Kind != programindex.RelationCalls || !strings.HasPrefix(expected, names[relation.FromID]+" ->") {
			continue
		}
		var to, stores []string
		for _, id := range relation.ToIDs {
			to = append(to, names[id])
		}
		slices.Sort(to)
		for _, witness := range relation.Witnesses {
			if witness.Kind == "function_value_store" && witness.Location != nil && names[witness.ObjectID] != "" {
				stores = append(stores, fmt.Sprintf("%s@%d", witness.Detail, witness.Location.Line))
			}
		}
		got := fmt.Sprintf("%s -> %s %s %s", names[relation.FromID], strings.Join(to, ","), relation.Resolution, relation.Dispatch)
		if len(stores) > 0 {
			got += " | " + strings.Join(stores, ", ")
		}
		if got != expected {
			t.Fatalf("line %d: call = %q, want %q", line, got, expected)
		}
		delete(want, line)
	}
	if len(want) != 0 {
		t.Fatalf("calls not seen: %v", want)
	}
}
