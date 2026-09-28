package reading

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// launchSource is one Go file whose main starts programs: litestream with
// a subcommand and options (another program's command line), git through
// a shell, a program whose path a variable holds, git built on either
// branch, and a hook's command built on one branch only; four calls act on
// what a launching call returned.
const launchSource = `func main() {
	out, err := exec.CommandContext(ctx, "litestream", "restore", "-config", path, "-o", db).CombinedOutput()
	cmd := exec.Command("sh", "-c", "git status")
	cmd.Run()
	exec.Command(binary, args...)
	if ref != "" {
		built = exec.CommandContext(ctx, "git", "rev-parse", ref)
	} else {
		built = exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	}
	built.CombinedOutput()
	if hook != "" {
		held = exec.Command(hook)
	}
	held.Run()
}
`

// launchCall is a call of an outside Go symbol at the first occurrence of
// mark on line, given values; on names the call whose result it is made on.
func launchCall(t *testing.T, symbol string, line int, mark string, on *atlas.SymbolCall, values ...string) atlas.SymbolCall {
	t.Helper()
	text := strings.Split(launchSource, "\n")[line-1]
	column := strings.Index(text, mark) + 1
	if column < 1 {
		t.Fatalf("no %q on line %d", mark, line)
	}
	dot := strings.LastIndex(symbol, ".")
	api := &atlas.CallAPI{Package: symbol[:dot], Name: symbol[dot+1:]}
	if strings.HasPrefix(symbol, "os/exec.Cmd.") {
		api = &atlas.CallAPI{Package: "os/exec", Receiver: "Cmd", Name: symbol[dot+1:]}
	}
	call := atlas.SymbolCall{Kind: string(programindex.RelationInvokesExternal), Name: symbol, Line: line, Column: column, Values: values, API: api}
	if on != nil {
		call.ReceiverValue = &sourcevalue.Value{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "main.go", Line: on.Line, Column: on.Column}}
	}
	return call
}

// programAsking answers the program question by the call it shows and
// records every call it was asked about with the words it offered.
type programAsking struct {
	mu     sync.Mutex
	offers map[string][]string
	choose map[string]string
}

func (p *programAsking) categorizer() *typesafetest.Categorizer {
	return &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		if !strings.HasSuffix(key, "|program") {
			return llm.Verdict{}, false
		}
		usage, _ := question.Item["usage"].(string)
		var offered []string
		for _, option := range question.Options {
			offered = append(offered, option.Name)
			if option.Criteria == nil {
				return llm.Verdict{}, false
			}
		}
		sort.Strings(offered)
		p.mu.Lock()
		defer p.mu.Unlock()
		p.offers[usage] = offered
		choice, ok := p.choose[usage]
		if !ok {
			return llm.Verdict{}, false
		}
		return typesafetest.Choose(choice), true
	}}
}

// A call that starts another program is one outgoing boundary, named by
// the word the model chooses among that call's own words, as written: a
// symbol that starts litestream at one site and git through a shell at
// another is asked once per call, and the shell's command line, not the
// shell, is what Jev picks here. A call on what a launching call returned
// (CombinedOutput, Run) is the same program, not another boundary, and a
// call on a command built on either branch is both branches' launch, each
// naming git; a command one branch leaves nil is not only a launch's
// result, so the call on it stays its own boundary. A call given no word
// is not asked and its program stays not established (no recorded word is
// no evidence that none names it). No launch is an entry of this program.
func TestACallThatStartsAProgramIsNamedByTheWordItsCallWrote(t *testing.T) {
	restore := launchCall(t, "os/exec.CommandContext", 2, "exec.CommandContext", nil, "litestream", "restore", "-config", "-o")
	output := launchCall(t, "os/exec.Cmd.CombinedOutput", 2, ".CombinedOutput", &restore)
	shell := launchCall(t, "os/exec.Command", 3, "exec.Command", nil, "sh", "-c", "git status")
	run := launchCall(t, "os/exec.Cmd.Run", 4, "cmd.Run", &shell)
	unnamed := launchCall(t, "os/exec.Command", 5, "exec.Command", nil)
	byRef := launchCall(t, "os/exec.CommandContext", 7, "exec.CommandContext", nil, "git", "rev-parse")
	atHead := launchCall(t, "os/exec.CommandContext", 9, "exec.CommandContext", nil, "git", "rev-parse", "HEAD")
	either := launchCall(t, "os/exec.Cmd.CombinedOutput", 11, ".CombinedOutput", nil)
	either.ReceiverValue = &sourcevalue.Value{Kind: "alternatives", Parts: []sourcevalue.Value{
		{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "main.go", Line: byRef.Line, Column: byRef.Column}},
		{Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "main.go", Line: atHead.Line, Column: atHead.Column}}}}
	hook := launchCall(t, "os/exec.Command", 13, "exec.Command", nil)
	held := launchCall(t, "os/exec.Cmd.Run", 15, ".Run", nil)
	held.ReceiverValue = &sourcevalue.Value{Kind: "alternatives", Parts: []sourcevalue.Value{
		{Kind: "unknown"}, {Kind: "call_result", Anchor: &sourcevalue.Anchor{Path: "main.go", Line: hook.Line, Column: hook.Column}}}}
	places := apiGraph(restore, output, shell, run, unnamed, byRef, atHead, either, hook, held)
	places[0].Path, places[1].Path = "main.go", "main.go"
	asking := &programAsking{offers: map[string][]string{}, choose: map[string]string{
		`exec.CommandContext(ctx, "litestream", "restore", "-config", path, "-o", db)`: "litestream",
		`exec.Command("sh", "-c", "git status")`:                                       "git status",
		`exec.CommandContext(ctx, "git", "rev-parse", ref)`:                            "git",
		`exec.CommandContext(ctx, "git", "rev-parse", "HEAD")`:                         "git",
	}}
	r := apiReader(t, t.TempDir(), places, asking.categorizer())
	r.opts.ReadSource = func(string) ([]byte, error) { return []byte(launchSource), nil }
	r.boundaries = map[string]*boundaryState{}
	for _, symbol := range []string{"os/exec.CommandContext", "os/exec.Command", "os/exec.Cmd.CombinedOutput", "os/exec.Cmd.Run"} {
		r.api[symbol] = apiRole{talks: atlas.BoundaryRunsProgram}
	}
	r.bindInterpretedBoundaries()
	if err := r.readPrograms(t.Context()); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, state := range r.boundaries {
		b := state.place.Boundary
		got = append(got, strings.Join([]string{fmt.Sprint(state.place.LineNo), b.Direction, state.kind, state.apiSymbol, strings.Join(b.Values, "|"), state.destination, map[bool]string{true: "not named", false: "-"}[state.programNotNamed]}, " "))
	}
	sort.Strings(got)
	want := []string{
		"13 out runs_program os/exec.Command   -",
		"15 out runs_program os/exec.Cmd.Run   -",
		"2 out runs_program os/exec.CommandContext litestream|restore|-config|-o litestream -",
		"3 out runs_program os/exec.Command sh|-c|git status git status -",
		"5 out runs_program os/exec.Command   -",
		"7 out runs_program os/exec.CommandContext git|rev-parse git -",
		"9 out runs_program os/exec.CommandContext git|rev-parse|HEAD git -",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("boundaries = %q\nwant %q", got, want)
	}
	wantOffers := map[string][]string{
		`exec.CommandContext(ctx, "litestream", "restore", "-config", path, "-o", db)`: {"-config", "-o", "litestream", lines.ProgramNotNamed, "restore"},
		`exec.Command("sh", "-c", "git status")`:                                       {"-c", "git status", lines.ProgramNotNamed, "sh"},
		`exec.CommandContext(ctx, "git", "rev-parse", ref)`:                            {"git", lines.ProgramNotNamed, "rev-parse"},
		`exec.CommandContext(ctx, "git", "rev-parse", "HEAD")`:                         {"HEAD", "git", lines.ProgramNotNamed, "rev-parse"},
	}
	if !reflect.DeepEqual(asking.offers, wantOffers) {
		t.Fatalf("the program question offered %v\nwant %v", asking.offers, wantOffers)
	}
	for _, state := range r.boundaries {
		if state.place.Boundary.Direction == atlas.DirectionIn || slices.Contains([]string{atlas.BoundaryCommand}, state.kind) {
			t.Fatalf("a launch made an entry: %+v", state.place.Boundary)
		}
	}
}
