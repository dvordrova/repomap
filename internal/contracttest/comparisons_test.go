package contracttest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// comparisonsOf are the comparisons of the one object named name (or
// ending in "/"+name, a Clojure var) declared in path.
func comparisonsOf(t *testing.T, index programindex.Index, path, name string) []programindex.Comparison {
	t.Helper()
	var found []programindex.Object
	for _, object := range index.Objects {
		if (object.Name == name || strings.HasSuffix(object.Name, "/"+name)) && object.Kind != programindex.ObjectModule && object.Location != nil && object.Location.Path == path {
			found = append(found, object)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d objects named %s in %s", len(found), name, path)
	}
	return found[0].Comparisons
}

// caseWords are a comparison's cases as their forms and words.
func caseWords(comparison programindex.Comparison) []string {
	var result []string
	for _, item := range comparison.Cases {
		result = append(result, string(item.Form)+" "+strings.Join(item.Words, "|"))
	}
	return result
}

// expectOneComparison checks the one comparison name makes: the compared
// value as written and its cases, then that lone names none.
func expectOneComparison(t *testing.T, index programindex.Index, path, name, value string, cases []string, lone string) programindex.Comparison {
	t.Helper()
	comparisons := comparisonsOf(t, index, path, name)
	if len(comparisons) != 1 {
		t.Fatalf("%s makes %d comparisons: %+v", name, len(comparisons), comparisons)
	}
	comparison := comparisons[0]
	if comparison.Value != value || !reflect.DeepEqual(caseWords(comparison), cases) {
		t.Fatalf("%s compares %q with %q, want %q with %q", name, comparison.Value, caseWords(comparison), value, cases)
	}
	// Each case selects lines: a case's from its label, an if condition's
	// the block it guards (Python's starts on the next line).
	for _, item := range comparison.Cases {
		if item.Location == nil || item.Branch == nil || item.Branch.EndLine < item.Location.Line || item.Branch.Line > item.Location.Line+1 {
			t.Fatalf("%s's case %v selects no lines at it: %+v", name, item.Words, item.Branch)
		}
	}
	if lone != "" {
		if got := comparisonsOf(t, index, path, lone); len(got) != 0 {
			t.Fatalf("a lone comparison in %s is recorded: %+v", lone, got)
		}
	}
	return comparison
}

// firstElement reports an origin that is element 0 of the declaration's
// parameter named parameter, alone or beside other values.
func firstElement(origin *sourcevalue.Value, parameter string) bool {
	if origin == nil {
		return false
	}
	if origin.Kind == "alternatives" {
		for i := range origin.Parts {
			if firstElement(&origin.Parts[i], parameter) {
				return true
			}
		}
		return false
	}
	return origin.Kind == "index" && len(origin.Parts) == 2 && origin.Parts[0].Kind == "parameter" && origin.Parts[0].Text == parameter &&
		origin.Parts[1].Kind == "literal" && origin.Parts[1].Text == "0"
}

// A multi-way dispatch is one comparison fact in every language: a value
// compared with two or more different words in two or more cases, each
// case its words and the lines it selects. Go and Python follow the
// parallel assignment `cmd, args = args[0], args[1:]` to the first element
// of the parameter; TypeScript's switch on process.argv[2] and its default
// branch's === comparisons are one comparison; Clojure's case form; C's
// switch on a character. A lone comparison of one word is none. C
// compares strings by calls (strcasecmp) and Clojure by `=` calls: each
// stays a call asked on its own (TestCumulativeClojureInputsAreAskedPerCall),
// not a comparison, a recorded difference.
func TestEveryLanguageRecordsAMultiWayDispatchAsOneComparison(t *testing.T) {
	t.Run("go", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "0")
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOWORK", "off")
		root, repository := materializeFixtureRepository(t, "go")
		writePublishedGoFixtureModule(t, root)
		authorities := sharedGoFixtureAuthorities(t, root, repository, goFixtureAppPackage, "comparisons")
		index, err := goadapter.Build(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
		if err != nil {
			t.Fatal(err)
		}
		comparison := expectOneComparison(t, index, "internal/storefixture/tool_cli.go", "RunSubcommand", "cmd",
			[]string{"case serve", "case check|verify", "equals help|-h"}, "IsDefaultLevel")
		if !firstElement(comparison.Origin, "args") {
			t.Fatalf("cmd's origin is not args[0]: %+v", comparison.Origin)
		}
		// ToolCommand's two EqualFold calls compare with one word each:
		// calls, never a comparison.
		if got := comparisonsOf(t, index, "internal/storefixture/tool_cli.go", "ToolCommand"); len(got) != 0 {
			t.Fatalf("ToolCommand's calls made a comparison: %+v", got)
		}
	})
	t.Run("python", func(t *testing.T) {
		index := pythonLibraryIndex(t)
		comparison := expectOneComparison(t, index, "src/fixture_app/dispatch.py", "dispatch", "command",
			[]string{"equals init", "equals serve|run", "equals help"}, "is_default")
		if !firstElement(comparison.Origin, "argv") {
			t.Fatalf("command's origin is not argv[0]: %+v", comparison.Origin)
		}
		expectOneComparison(t, index, "src/fixture_app/dispatch.py", "describe", "status", []string{"case up", "case down|stopped"}, "")
	})
	t.Run("jsts", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "jsts")
		_, index, _, err := sharedJSTSFixture(t, repository, root)
		if err != nil {
			t.Fatal(err)
		}
		expectOneComparison(t, index, "src/dispatch.ts", "dispatch", "process.argv[2]",
			[]string{"case build", "case check|verify", "equals help|-h"}, "isDefault")
	})
	t.Run("clojure", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "clojure")
		targets, err := clojureproject.Scout(repository, "clojure")
		if err != nil || len(targets) != 1 {
			t.Fatalf("Clojure discovery: %v %v", targets, err)
		}
		result, err := sharedClojureFixture(t, root, repository, targets[0])
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(result.Input)
		if err != nil {
			t.Fatal(err)
		}
		expectOneComparison(t, index, "src/example/core.clj", "run-command", "(first args)", []string{"case serve", "case check|verify"}, "shouted?")
	})
	t.Run("c", func(t *testing.T) {
		index := buildCIndex(t, sharedCFixture(t), "c:kvcli")
		expectOneComparison(t, index, "kvcli.c", "shortOption", "arg[1]", []string{"case h|?", "case V"}, "main")
	})
}

// pythonLibraryIndex is the Python fixture's library target, every module
// of its package.
func pythonLibraryIndex(t *testing.T) programindex.Index {
	t.Helper()
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range catalog.Entries {
		if candidate.Kind != pythontarget.KindLibrary || candidate.ProjectDir != "." {
			continue
		}
		input, err := sharedPythonFixtureInput(t, repository, candidate)
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		return index
	}
	t.Fatal("the Python fixture has no library target")
	return programindex.Index{}
}

// A Python module-level table whose elements share one shape is a table of
// names when a function outside the tests reads it: OPTIONS, one Opt(...)
// row per key, each row its key and the call's words as written, and
// REQUIRED, a list of names. FORMATS, which nothing reads, is none. Go,
// JS/TS and Clojure record no such table yet (PYTHON, GO, JSTS, CLOJURE).
func TestPythonTablesOfNamesAreTheOnesAFunctionReads(t *testing.T) {
	index := pythonLibraryIndex(t)
	rows := map[string][][]string{}
	for _, object := range index.Objects {
		if object.Location == nil || object.Location.Path != "src/fixture_app/dispatch.py" || len(object.Rows) == 0 {
			continue
		}
		for _, row := range object.Rows {
			var words []string
			for _, literal := range row.Literals {
				word := literal.Value
				if literal.Field != "" {
					word = literal.Field + "=" + word
				}
				words = append(words, word)
			}
			rows[object.Name] = append(rows[object.Name], words)
		}
	}
	want := map[string][][]string{
		"OPTIONS":    {{"verbose", "-v", "--verbose", "help=print more"}, {"force", "-f", "--force", "help=overwrite files"}},
		"REQUIRED":   {{"port"}, {"dbfile"}},
		"ARGS_SERVE": {{"verbose"}, {"force"}},
		"ARGS_INIT":  {{"force"}, {"verbose"}},
		"NO_CONFIG":  {{"init"}, {"help"}, {"prune"}},
		"KNOWN":      {{"serve"}, {"init"}},
		"HELP":       {{"serve", "run the server"}, {"init", "create the files"}},
		"COMMANDS":   {{"serve"}, {"init"}, {"status"}},
		"READ_ONLY":  {{"status"}, {"help"}},
		"WRITES":     {{"init"}, {"serve"}},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("tables of names: %q, want %q", rows, want)
	}
}
