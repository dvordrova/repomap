package contracttest

import (
	"fmt"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
)

// callWords are, for each call of a file whose selector is one of the
// given, its string literals as "selector(word@position)" or
// "selector(keyword=word)", and "on <line>" when it is made on what the
// call at that line of the file returned.
func callWords(index programindex.Index, path string, selectors ...string) []string {
	var result []string
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Location == nil || pattern.Location.Path != path || !slices.Contains(selectors, pattern.Selector) {
				continue
			}
			for _, argument := range pattern.Arguments {
				if argument.Kind != programindex.PatternLiteralString {
					continue
				}
				word := fmt.Sprintf("%s(%s@%d)", pattern.Selector, argument.Value, argument.Position)
				if argument.Keyword != "" {
					word = fmt.Sprintf("%s(%s=%s)", pattern.Selector, argument.Keyword, argument.Value)
				}
				if receiver := pattern.ReceiverValue; receiver != nil && receiver.Kind == "call_result" && receiver.Anchor != nil && receiver.Anchor.Path == path {
					word += fmt.Sprintf(" on %d", receiver.Anchor.Line)
				}
				result = append(result, word)
			}
		}
	}
	slices.Sort(result)
	return result
}

// A command group's word is the call's own, given by position, and its
// subcommands are declared on what that call returned; the collection a
// chosen subcommand is kept in is given its word only under a parameter's
// name. GroupsIndex keeps the first and drops the second as an input
// (TestADestOfHandledSubcommandsIsNoInputButACommandGroupKeepsItsWord).
//
//   - JS: commander's program.command("remote"), with add and rm declared
//     on its result. commander's declarations are not installed in the
//     fixture, so the reading asks none of these calls (JSTS.md) and no
//     input is made of them.
//   - Python: argparse's add_parser("remote") is given its word by
//     position; each add_subparsers only dest= (the reading of
//     tool_cli.py: TestCumulativePythonInputsJoinAndCatalogue).
func TestACommandGroupsWordIsGivenByPositionAndADestsByName(t *testing.T) {
	t.Run("jsts", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "jsts")
		_, index, _, err := jstsproject.Build(t.Context(), repository, root)
		if err != nil {
			t.Fatal(err)
		}
		got := callWords(index, "src/cli.ts", "command")
		want := []string{"command(add@1) on 25", "command(init@1) on 13", "command(remote@1) on 13", "command(rm@1) on 25"}
		if !slices.Equal(got, want) {
			t.Fatalf("cli.ts's command words %q, want %q", got, want)
		}
	})
	t.Run("python", func(t *testing.T) {
		index := pythonLibraryIndex(t)
		got := callWords(index, "src/fixture_app/tool_cli.py", "add_parser", "add_subparsers")
		for _, word := range []string{"add_parser(remote@1) on 99", "add_subparsers(dest=remote_cmd) on 100", "add_parser(add@1) on 101", "add_subparsers(dest=cmd) on 98"} {
			if !slices.Contains(got, word) {
				t.Fatalf("tool_cli.py's words %q hold no %q", got, word)
			}
		}
	})
}
