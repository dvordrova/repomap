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
	root := preparedCompilerProject(t)
	const path = "src/stored-callbacks.ts"
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
