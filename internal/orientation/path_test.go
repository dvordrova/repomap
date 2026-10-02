package orientation

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// flowProgram is a small program to walk: main calls start, which registers
// three callbacks with an outside loop (on_click, on_move, draw) and calls a
// helper; on_click runs play in the game's core part through the Game class
// (its constructor and move), play calls back into on_click (a cycle), and a
// test module drives start too.
func flowProgram() Input {
	subject := func(id, kind, path string, extra ...func(*groupindex.Subject)) groupindex.Subject {
		result := groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectKind(kind), Location: &programindex.Location{Path: path, Line: 1, Column: 1}}}
		for _, apply := range extra {
			apply(&result)
		}
		return result
	}
	helper := func(s *groupindex.Subject) {
		s.Interpretation = &groupindex.Interpretation{Line: "Formats a message.", Helper: true}
	}
	line := func(text string) func(*groupindex.Subject) {
		return func(s *groupindex.Subject) { s.Interpretation = &groupindex.Interpretation{Line: text} }
	}
	method := func(owner string) func(*groupindex.Subject) {
		return func(s *groupindex.Subject) { s.Object.OwnerID = owner }
	}
	edge := func(from, to string, kind programindex.RelationKind) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: kind, Resolution: programindex.ResolutionExact}
	}
	calls := programindex.RelationCalls
	index := groupindex.Index{
		Target: programindex.Target{ID: "t1", Name: "game", TestSources: []string{"test/start_test.py"},
			Seeds: []programindex.TargetSeed{{ObjectID: "main", Kind: programindex.SeedCallable}}},
		Subjects: []groupindex.Subject{
			subject("main", "function", "app/main.py"), subject("start", "function", "app/ui.py", line("Opens the window and registers its handlers.")),
			subject("format", "function", "app/ui.py", helper), subject("on_click", "function", "app/ui.py", line("Handles a click on the board.")),
			subject("on_move", "function", "app/ui.py"), subject("draw", "function", "app/ui.py"), subject("paint", "function", "app/ui.py"),
			subject("Game", "type", "app/game.py"), subject("Game.__init__", "method", "app/game.py", method("Game")),
			subject("Game.move", "method", "app/game.py", method("Game")), subject("play", "function", "app/game.py", line("Plays one move.")),
			subject("test_start", "function", "test/start_test.py"),
		},
		Groups: []groupindex.Group{
			{ID: "g1", Title: "User interface", MemberSubjectIDs: []string{"main", "start", "format", "on_click", "on_move", "draw", "paint"}},
			{ID: "g2", Title: "Game state", Core: true, MemberSubjectIDs: []string{"Game", "Game.__init__", "Game.move", "play"}},
		},
		StructuralEdges: []groupindex.StructuralEdge{
			edge("main", "start", calls), edge("start", "format", calls),
			edge("on_click", "Game.__init__", calls), edge("on_click", "Game.move", calls), edge("Game.move", "play", calls), edge("play", "on_click", calls),
			edge("draw", "paint", calls), edge("on_move", "Game.move", calls),
			edge("test_start", "start", calls), edge("test_start", "play", programindex.RelationPassesCallback),
		},
	}
	registration := func(object string) facts.Fact {
		return facts.Fact{Kind: facts.KindRegistration, TargetID: "t1", OwnerID: "start", ObjectID: object, Key: "loop.on", Text: "loop.on." + object}
	}
	return Input{Groups: []groupindex.Index{index}, Facts: facts.Result{Facts: []facts.Fact{
		registration("on_click"), registration("on_move"), registration("draw"),
		{Kind: facts.KindRegistration, TargetID: "t1", OwnerID: "test_start", ObjectID: "on_move", Text: "loop.on.move"},
	}}}
}

// flowNames are a walked flow's steps by declaration, each with how it is
// reached, then, where it parts, each way's steps numbered by the way
// ("2: on_move"), then its fork's unfollowed candidates ("? draw").
func flowNames(flow MainFlow) []string {
	var names []string
	var walk func(prefix string, steps []FlowStep)
	walk = func(prefix string, steps []FlowStep) {
		for _, step := range steps {
			names = append(names, prefix+step.SubjectID+" ("+step.Via+")")
			for number, path := range step.Paths {
				walk(fmt.Sprintf("%s%d: ", prefix, number+1), path.Steps)
			}
			for _, branch := range step.Branches {
				names = append(names, prefix+"? "+branch.SubjectID+" ("+branch.Via+")")
			}
		}
	}
	walk("", flow.Steps)
	return names
}

// A Main flow is walked by code: a step with one candidate is followed with
// no question, a split is one closed question whose options are the
// candidates (a helper, a unit whose closure enters no core part and test
// code are none), a class is one step of the members of it entered, and no
// declaration is on the path twice.
func TestAMainFlowIsWalkedByCodeAndAsksOneQuestionPerSplit(t *testing.T) {
	var asked []llm.Question
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		asked = append(asked, question)
		return llm.Verdict{Choice: "on_click", Probabilities: map[string]float64{"on_click": 0.8, "on_move": 0.2}}, true
	}}
	walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, flowProgram(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"main ()", "start (called)", "on_click (handed to loop.on.on_click)", "Game.move (called)", "play (called)"}
	if got := flowNames(walk.flow); !slices.Equal(got, want) {
		t.Fatalf("flow = %q, want %q", got, want)
	}
	if categorizer.Calls() != 1 || len(asked) != 1 {
		t.Fatalf("asked %d requests (%d questions), want the one split at start", categorizer.Calls(), len(asked))
	}
	var options []string
	for _, option := range asked[0].Options {
		options = append(options, option.Name)
		if option.Name == "on_click" && (!strings.Contains(option.Meaning, "role: Handles a click on the board.") || !strings.Contains(option.Meaning, "reached: handed to loop.on.on_click")) {
			t.Fatalf("an option lost its terms: %+v", option)
		}
	}
	// format is a helper, draw's closure enters no core part, and the test's
	// registration of on_move is no hand-over of the program's.
	if !slices.Equal(options, []string{"on_click", "on_move"}) || asked[0].Item["step"] != "start" {
		t.Fatalf("the split offered %v about %v", options, asked[0].Item)
	}
	if len(walk.asked) != 1 || !walk.asked[0].decided || walk.asked[0].lead < 0.59 {
		t.Fatalf("the split's decision was not kept: %+v", walk.asked)
	}
	if walk.flow.Title != "From main to play" || walk.flow.Steps[1].Explanation != "Opens the window and registers its handlers." {
		t.Fatalf("the flow's title or a step's accepted line is wrong: %+v", walk.flow)
	}
	// The decided split keeps the candidate it passed, as the step's own
	// (version 2): start goes on to on_click, past on_move.
	if passed := walk.flow.Steps[1].Passed; len(passed) != 1 || passed[0].SubjectID != "on_move" || passed[0].Via != "handed to loop.on.on_move" {
		t.Fatalf("start passed %+v, want on_move handed to loop.on.on_move", passed)
	}
	for position, step := range walk.flow.Steps {
		if position != 1 && len(step.Passed) > 0 {
			t.Fatalf("step %s passed %+v with no decided split", step.SubjectID, step.Passed)
		}
	}
	if _, err := Seal(Result{FactsSHA256: strings.Repeat("a", 64), ClaimsSHA256: strings.Repeat("b", 64), MainFlow: walk.flow}); err != nil {
		t.Fatalf("a flow with passed calls does not seal: %v", err)
	}
}

// A split the categorizer leaves under the margin parts the flow: each
// candidate within the margin of the leader is a way of its own, walked with
// its own visited set, so neither way walks back through the trunk or into
// the other's start (play calls on_click again); the split is journaled with
// its lead and the ways followed. A walk with no categorizer asks nothing
// and ends there, a named fork of the candidates.
func TestATornSplitPartsTheFlowIntoWays(t *testing.T) {
	torn := &typesafetest.Categorizer{Decide: func(_ string, question llm.Question) (llm.Verdict, bool) {
		if question.Item["step"] == "start" {
			return llm.Verdict{Choice: "on_click", Probabilities: map[string]float64{"on_click": 0.52, "on_move": 0.48}}, true
		}
		return typesafetest.Choose(question.Options[0].Name), true
	}}
	walk, err := walkFlow(t.Context(), llm.Executor{}, torn, flowProgram(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"main ()", "start (called)",
		"1: on_click (handed to loop.on.on_click)", "1: Game.move (called)", "1: play (called)",
		"2: on_move (handed to loop.on.on_move)", "2: Game.move (called)", "2: play (called)"}
	if got := flowNames(walk.flow); !slices.Equal(got, want) {
		t.Fatalf("flow = %q\nwant %q", got, want)
	}
	if walk.flow.Title != "From main to play" || validFlowSteps(walk.flow.Steps) != nil {
		t.Fatalf("title %q", walk.flow.Title)
	}
	var fork struct {
		Step       string
		Candidates []string
		Lead       float64
		Followed   []string
	}
	if len(walk.rejected) != 1 || walk.rejected[0].Section != sectionFlowFork || json.Unmarshal(walk.rejected[0].Raw, &fork) != nil ||
		fork.Step != "start" || len(fork.Candidates) != 2 || fork.Lead < 0.03 || fork.Lead > 0.05 || !slices.Equal(fork.Followed, []string{"on_click", "on_move"}) {
		t.Fatalf("the torn split was not journaled with its lead and ways: %+v", walk.rejected)
	}
	unasked, err := walkFlow(t.Context(), llm.Executor{}, nil, flowProgram(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got := flowNames(unasked.flow); !slices.Equal(got, []string{"main ()", "start (called)", "? on_click (handed to loop.on.on_click)", "? on_move (handed to loop.on.on_move)"}) ||
		len(unasked.rejected) != 0 || len(unasked.asked) != 0 {
		t.Fatalf("a walk with no categorizer = %q, %+v", got, unasked)
	}
}

// Only the candidates within the margin of the leader are ways; the rest
// stay the fork's folded candidates.
func TestWithinMarginFollowsOnlyTheCandidatesNearTheLeader(t *testing.T) {
	names := []string{"DB", "s3.ReplicaClient", "gs.ReplicaClient", "Pos"}
	verdict := llm.Verdict{Choice: "DB", Probabilities: map[string]float64{"DB": 0.40, "s3.ReplicaClient": 0.34, "c3": 0.31, "Pos": 0.05}}
	if got := withinMargin(verdict, names); !slices.Equal(got, []int{0, 1, 2}) {
		t.Fatalf("within the margin: %v", got)
	}
	if got := withinMargin(llm.Verdict{Probabilities: map[string]float64{"DB": 0.9, "Pos": 0.1}}, names); got != nil {
		t.Fatalf("a confident verdict parts: %v", got)
	}
}

// A chain whose every step has one candidate asks nothing.
func TestAChainOfSingleCandidatesAsksNothing(t *testing.T) {
	input := flowProgram()
	index := &input.Groups[0]
	// start registers on_click alone.
	input.Facts.Facts = input.Facts.Facts[:1]
	categorizer := &typesafetest.Categorizer{}
	walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, input, index.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := flowNames(walk.flow); len(got) != 5 || categorizer.Calls() != 0 {
		t.Fatalf("flow = %q after %d requests, want five steps and none", got, categorizer.Calls())
	}
}

// Candidates sharing a name are offered told apart (groupindex.TellApart),
// never as refs the categorizer cannot read: litestream's Sync had offered
// eight options titled ReplicaClient.
func TestASplitsSameNamedCandidatesAreToldApart(t *testing.T) {
	input := flowProgram()
	for position := range input.Groups[0].Subjects {
		if subject := &input.Groups[0].Subjects[position]; subject.ID == "on_move" {
			subject.Object.Name, subject.Object.Location.Path = "on_click", "app/touch/ui.py"
		}
	}
	var asked []llm.Question
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		asked = append(asked, question)
		return llm.Verdict{Choice: "app.on_click", Probabilities: map[string]float64{"app.on_click": 0.8, "touch.on_click": 0.2}}, true
	}}
	walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, input, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 {
		t.Fatalf("asked %d questions, want the split at start", len(asked))
	}
	var options []string
	for _, option := range asked[0].Options {
		options = append(options, option.Name)
		if !strings.HasPrefix(option.Meaning, option.Name+";") {
			t.Fatalf("an option's criteria do not name it as told apart: %+v", option)
		}
	}
	if !slices.Equal(options, []string{"app.on_click", "touch.on_click"}) || len(walk.asked) != 1 || walk.asked[0].chosen != "app.on_click" {
		t.Fatalf("the split offered %v and chose %+v", options, walk.asked)
	}
}

// A class member step's public calls of its own class's members are steps of
// their own, while its private helpers stay folded into it (FreqtradeBot's
// process had folded enter_positions and exit_positions, offering only the
// classes they call), and a member reads its own line, never its class's:
// the class's line on each of its members tells them nothing apart, while
// another class's member keeps its class's line.
func TestAClassStepsPublicCallsAreStepsAndItsHelpersFold(t *testing.T) {
	object := func(id, kind, owner string, visibility programindex.Visibility) groupindex.Subject {
		return groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id[strings.LastIndex(id, ".")+1:], Kind: programindex.ObjectKind(kind),
			OwnerID: owner, Visibility: visibility, Location: &programindex.Location{Path: "bot.py", Line: 1, Column: 1}}}
	}
	lined := func(subject groupindex.Subject, line string) groupindex.Subject {
		subject.Interpretation = &groupindex.Interpretation{Line: line}
		return subject
	}
	calls := func(from, to string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact}
	}
	public, private := programindex.VisibilityPublic, programindex.VisibilityInternal
	index := groupindex.Index{
		Target: programindex.Target{ID: "t1", Name: "bot", Seeds: []programindex.TargetSeed{{ObjectID: "main", Kind: programindex.SeedCallable}}},
		Subjects: []groupindex.Subject{
			object("main", "function", "", public),
			lined(object("Bot", "type", "", public), "The main class of the bot."),
			object("Bot.process", "method", "Bot", public), object("Bot.enter", "method", "Bot", public), object("Bot.exit", "method", "Bot", public),
			object("Bot._refresh", "method", "Bot", private),
			lined(object("Strategy", "type", "", public), "Decides entry and exit signals."),
			object("Strategy.analyze", "method", "Strategy", public), object("Exchange", "type", "", public), object("Exchange.place", "method", "Exchange", public),
		},
		Groups: []groupindex.Group{{ID: "g1", Title: "Bot", Core: true, MemberSubjectIDs: []string{"main", "Bot", "Bot.process", "Bot.enter", "Bot.exit", "Bot._refresh", "Strategy", "Strategy.analyze", "Exchange", "Exchange.place"}}},
		StructuralEdges: []groupindex.StructuralEdge{
			calls("main", "Bot.process"), calls("Bot.process", "Bot._refresh"), calls("Bot._refresh", "Strategy.analyze"),
			calls("Bot.process", "Bot.enter"), calls("Bot.process", "Bot.exit"), calls("Bot.enter", "Exchange.place"),
		},
	}
	var asked []llm.Question
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		asked = append(asked, question)
		return llm.Verdict{Choice: "enter", Probabilities: map[string]float64{"enter": 0.7, "exit": 0.2}}, true
	}}
	walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, Input{Groups: []groupindex.Index{index}}, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got := flowNames(walk.flow); !slices.Equal(got, []string{"main ()", "Bot.process (called)", "Bot.enter (called)", "Exchange.place (called)"}) {
		t.Fatalf("flow = %q", got)
	}
	if len(asked) != 1 {
		t.Fatalf("asked %d questions, want the split at process", len(asked))
	}
	roles := map[string]string{}
	for _, option := range asked[0].Options {
		said := ""
		if _, role, found := strings.Cut(option.Meaning, "role: "); found {
			said, _, _ = strings.Cut(role, ";")
		} else if _, typed, found := strings.Cut(option.Meaning, "; type "); found {
			said, _, _ = strings.Cut(typed, ";")
		}
		roles[option.Name] = said
	}
	// _refresh is folded: its call of Strategy is process's own. A member
	// of another type is said with that type's line, as the type's.
	want := map[string]string{"analyze": "Strategy: Decides entry and exit signals.", "enter": "", "exit": ""}
	if !maps.Equal(roles, want) || walk.flow.Steps[1].Explanation != "" {
		t.Fatalf("options %q, want %q; process reads %q", roles, want, walk.flow.Steps[1].Explanation)
	}
}

// An option is what the path enters: a type entered through some of its
// members is said by those members, each with how the step's work reaches
// it, never by the type's own line or signature; a member of another type
// with no line of its own reads its type's line as the type's. etcd's
// startEtcd had offered "Etcd ... serves peers, clients and metrics",
// entered only through Close, handed to the interrupt handler, and Err,
// beside StartEtcd: the walk took the shutdown path 5 of 5, and StartEtcd
// wins 9 of 10 now (one draw parted both ways).
func TestAnOptionSaysWhatThePathEnters(t *testing.T) {
	object := func(id, kind, owner string) groupindex.Subject {
		return groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id[strings.LastIndex(id, ".")+1:], Kind: programindex.ObjectKind(kind),
			OwnerID: owner, Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: "server.go", Line: 1, Column: 1}}}
	}
	lined := func(subject groupindex.Subject, line string) groupindex.Subject {
		subject.Interpretation = &groupindex.Interpretation{Line: line}
		return subject
	}
	calls := func(from, to string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact}
	}
	index := groupindex.Index{
		Target: programindex.Target{ID: "t1", Name: "server", Seeds: []programindex.TargetSeed{{ObjectID: "main", Kind: programindex.SeedCallable}}},
		Subjects: []groupindex.Subject{
			object("main", "function", ""), object("StartServer", "function", ""),
			lined(object("Server", "type", ""), "The server instance: it serves clients and peers."),
			object("Server.Close", "method", "Server"), object("Server.Err", "method", "Server"),
			object("store", "function", ""), object("serve", "function", ""), object("flush", "function", ""),
		},
		Groups: []groupindex.Group{
			{ID: "g1", Title: "Startup", MemberSubjectIDs: []string{"main", "StartServer", "Server", "Server.Close", "Server.Err"}},
			{ID: "g2", Title: "Storage", Core: true, MemberSubjectIDs: []string{"store", "flush"}},
			{ID: "g3", Title: "Serving", Core: true, MemberSubjectIDs: []string{"serve"}},
		},
		StructuralEdges: []groupindex.StructuralEdge{
			calls("main", "StartServer"), calls("main", "Server.Err"), calls("StartServer", "serve"), calls("serve", "store"),
			calls("Server.Close", "flush"),
		},
	}
	registration := facts.Fact{Kind: facts.KindRegistration, TargetID: "t1", OwnerID: "main", ObjectID: "Server.Close", Key: "signal.Notify", Text: "signal.Notify"}
	var asked []llm.Question
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		asked = append(asked, question)
		return llm.Verdict{Choice: "StartServer", Probabilities: map[string]float64{"StartServer": 0.8, "Server": 0.2}}, true
	}}
	if _, err := walkFlow(t.Context(), llm.Executor{}, categorizer, Input{Groups: []groupindex.Index{index}, Facts: facts.Result{Facts: []facts.Fact{registration}}}, "t1"); err != nil {
		t.Fatal(err)
	}
	if len(asked) == 0 {
		t.Fatal("no split asked at main")
	}
	meaning := map[string]string{}
	for _, option := range asked[0].Options {
		meaning[option.Name] = option.Meaning
	}
	server := meaning["Server"]
	if want := "Server; enters Server.Close (handed to signal.Notify), Server.Err (called); in part Startup; reached: handed to signal.Notify"; server != want {
		t.Errorf("the type's option reads %q, want %q", server, want)
	}
	if start := meaning["StartServer"]; start != "StartServer; in part Startup; reached: called" {
		t.Errorf("StartServer's option reads %q", start)
	}
}

// The inputs a candidate handles are a criterion of every option of a split
// or of none: said of one, it reads as "no" on the rest, whose inputs the
// catalogue may not hold (lua's pmain had offered "handles: command W,
// command e l" on runargs alone, the script being a positional argument,
// and the walk took the -l option 5 of 5).
func TestAHandledInputIsSaidOfEveryOptionOrNone(t *testing.T) {
	operation := func(subject, kind, name string) groupindex.Operation {
		return groupindex.Operation{ID: "o" + subject, SubjectID: subject, GroupID: "g1", Kind: kind, Name: name}
	}
	for _, test := range []struct {
		name       string
		operations []groupindex.Operation
		want       map[string]string
	}{
		{"one handles an input", []groupindex.Operation{operation("on_click", "interaction", "click")}, map[string]string{"on_click": "", "on_move": ""}},
		{"each handles an input", []groupindex.Operation{operation("on_click", "interaction", "click"), operation("on_move", "interaction", "move")},
			map[string]string{"on_click": "interaction click", "on_move": "interaction move"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := flowProgram()
			input.Groups[0].Operations = test.operations
			var asked []llm.Question
			categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
				asked = append(asked, question)
				return llm.Verdict{Choice: "on_click", Probabilities: map[string]float64{"on_click": 0.8, "on_move": 0.2}}, true
			}}
			if _, err := walkFlow(t.Context(), llm.Executor{}, categorizer, input, "t1"); err != nil {
				t.Fatal(err)
			}
			if len(asked) != 1 {
				t.Fatalf("asked %d questions, want the split at start", len(asked))
			}
			got := map[string]string{}
			for _, option := range asked[0].Options {
				_, handles, _ := strings.Cut(option.Meaning, "handles: ")
				got[option.Name], _, _ = strings.Cut(handles, ";")
			}
			if !maps.Equal(got, test.want) {
				t.Fatalf("options handle %q, want %q", got, test.want)
			}
		})
	}
}

// A helper is never a step and never a dead end: a helper the step calls or
// hands over serves the step, so what it calls or hands over, other than
// further helpers, is the step's work, said through it, and the step taken
// through it keeps it; a helper's own helpers serve that helper and are not
// looked through. Decided otherwise, it is a candidate of its own and what
// it reaches is its own (Lua's handle_script runs the script through docall,
// a helper: dropped with all it reaches, it had left the path only the
// loader).
func TestAStepsWorkPassesThroughItsHelpers(t *testing.T) {
	program := func(helpers ...string) Input {
		subject := func(id string) groupindex.Subject {
			result := groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "app.c", Line: 1, Column: 1}}}
			if slices.Contains(helpers, id) {
				result.Interpretation = &groupindex.Interpretation{Line: "Wraps a call.", Helper: true}
			}
			return result
		}
		calls := func(from, to string) groupindex.StructuralEdge {
			return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact}
		}
		index := groupindex.Index{
			Target:   programindex.Target{ID: "t1", Name: "app", Seeds: []programindex.TargetSeed{{ObjectID: "S", Kind: programindex.SeedCallable}}},
			Subjects: []groupindex.Subject{subject("S"), subject("A"), subject("B"), subject("C"), subject("H"), subject("H2"), subject("X")},
			Groups: []groupindex.Group{{ID: "g1", Title: "Startup", MemberSubjectIDs: []string{"S", "H", "H2"}},
				{ID: "g2", Title: "Work", Core: true, MemberSubjectIDs: []string{"A", "B", "C", "X"}}},
			StructuralEdges: []groupindex.StructuralEdge{calls("S", "A"), calls("S", "H"), calls("S", "B"), calls("H", "H2"), calls("H", "B"), calls("H2", "X")},
		}
		handed := facts.Fact{Kind: facts.KindRegistration, TargetID: "t1", OwnerID: "H", ObjectID: "C", Text: "X"}
		return Input{Groups: []groupindex.Index{index}, Facts: facts.Result{Facts: []facts.Fact{handed}}}
	}
	reached := func(input Input, step string) map[string]string {
		graph := newFlowGraph(&input.Groups[0], input.Facts.Facts)
		result := map[string]string{}
		for _, candidate := range graph.candidates(step, []string{step}) {
			result[candidate.unit] = candidate.reach.asked()
		}
		return result
	}
	// X is reached only through H's own helper H2: no work of S's.
	if got, want := reached(program("H", "H2"), "S"), map[string]string{"A": "called", "B": "called", "C": "handed to X through H"}; !maps.Equal(got, want) {
		t.Fatalf("S reaches %q, want %q", got, want)
	}
	if got, want := reached(program("H2"), "S"), map[string]string{"A": "called", "H": "called", "B": "called"}; !maps.Equal(got, want) {
		t.Fatalf("with H no helper, S reaches %q, want %q", got, want)
	}
	if got, want := reached(program("H2"), "H"), map[string]string{"B": "called", "C": "handed to X", "X": "called through H2"}; !maps.Equal(got, want) {
		t.Fatalf("with H no helper, H reaches %q, want %q", got, want)
	}
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		return llm.Verdict{Choice: "C", Probabilities: map[string]float64{"C": 0.8, "A": 0.1, "B": 0.1}}, true
	}}
	walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, program("H", "H2"), "t1")
	if err != nil {
		t.Fatal(err)
	}
	if steps := walk.flow.Steps; len(steps) != 2 || steps[1].SubjectID != "C" || steps[1].Via != "handed to X" || !slices.Equal(steps[1].Through, []string{"H"}) {
		t.Fatalf("the flow is %+v, want S then C handed to X through H", steps)
	}
	if _, err := Seal(Result{FactsSHA256: strings.Repeat("a", 64), ClaimsSHA256: strings.Repeat("b", 64), MainFlow: walk.flow}); err != nil {
		t.Fatalf("a flow through helpers does not seal: %v", err)
	}
}

// A step the step before calls as a method of a repository type
// implementing the interface it calls, no observed flow giving the value,
// keeps that basis, as a split's candidate does: the page says it is known
// by method set, never a traced call (etcd's gateway reaching
// electionServer.Campaign).
func TestAStepKnownByItsInterfacesImplementationsKeepsItsBasis(t *testing.T) {
	subject := func(id string) groupindex.Subject {
		return groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "gw.go", Line: 1, Column: 1}}}
	}
	calls := func(from, to string, resolution programindex.Resolution, basis string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationID: from + "-" + basis,
			RelationKind: programindex.RelationCalls, Resolution: resolution, Basis: basis}
	}
	implements, alternatives := programindex.BasisImplements, programindex.ResolutionAlternatives
	index := groupindex.Index{
		Target:   programindex.Target{ID: "t1", Name: "gateway", Seeds: []programindex.TargetSeed{{ObjectID: "handle", Kind: programindex.SeedCallable}}},
		Subjects: []groupindex.Subject{subject("handle"), subject("serverCampaign"), subject("proxyCampaign")},
		Groups:   []groupindex.Group{{ID: "g1", Title: "Election", Core: true, MemberSubjectIDs: []string{"handle", "serverCampaign", "proxyCampaign"}}},
		StructuralEdges: []groupindex.StructuralEdge{
			calls("handle", "serverCampaign", alternatives, implements), calls("handle", "proxyCampaign", alternatives, implements),
		},
	}
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		return llm.Verdict{Choice: "serverCampaign", Probabilities: map[string]float64{"serverCampaign": 0.8, "proxyCampaign": 0.2}}, true
	}}
	walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, Input{Groups: []groupindex.Index{index}}, "t1")
	if err != nil {
		t.Fatal(err)
	}
	steps := walk.flow.Steps
	if len(steps) != 2 || steps[1].SubjectID != "serverCampaign" || steps[1].Basis != implements || len(steps[0].Passed) != 1 || steps[0].Passed[0].Basis != implements {
		t.Fatalf("the flow is %+v, want handle then serverCampaign, both candidates on the implements basis", steps)
	}
	if steps[0].Basis != "" {
		t.Fatalf("the seed took a basis: %+v", steps[0])
	}
	if _, err := Seal(Result{FactsSHA256: strings.Repeat("a", 64), ClaimsSHA256: strings.Repeat("b", 64), MainFlow: walk.flow}); err != nil {
		t.Fatalf("a flow on the implements basis does not seal: %v", err)
	}
	// A traced call of the same method is how the step is reached, in
	// either order of the code's relations.
	traced := calls("handle", "serverCampaign", programindex.ResolutionExact, "")
	for _, edges := range [][]groupindex.StructuralEdge{append([]groupindex.StructuralEdge{traced}, index.StructuralEdges...), append(slices.Clone(index.StructuralEdges), traced)} {
		both := index
		both.StructuralEdges = edges
		walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, Input{Groups: []groupindex.Index{both}}, "t1")
		if err != nil {
			t.Fatal(err)
		}
		if steps := walk.flow.Steps; len(steps) != 2 || steps[1].Basis != "" || steps[1].Via != "called" || len(steps[0].Passed) != 1 || steps[0].Passed[0].Basis != implements {
			t.Fatalf("with a traced call the flow is %+v", steps)
		}
	}
}
