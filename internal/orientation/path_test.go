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
	if walk.flow.Title != "" || walk.flow.Steps[1].Explanation != "Opens the window and registers its handlers." {
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
	if walk.flow.Title != "" || validFlowSteps(walk.flow.Steps) != nil {
		t.Fatalf("title %q", walk.flow.Title)
	}
	// play calls on_click: the way from on_move goes on into the other
	// way's start, joining it, never walking it twice; the way from
	// on_click comes back to its own start.
	ways := walk.flow.Steps[len(walk.flow.Steps)-1].Paths
	first, second := ways[0].Steps[len(ways[0].Steps)-1], ways[1].Steps[len(ways[1].Steps)-1]
	if first.Stop != StopRevisits || second.Stop != StopJoins || len(second.Joins) != 1 || second.Joins[0].SubjectID != "on_click" || second.Joins[0].Via != "called" {
		t.Fatalf("the ways end %q and %q (%+v)", first.Stop, second.Stop, second.Joins)
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

// A helper that only passes the call on to one further helper (a conduit:
// the walk follows exactly one edge from it, to a helper, and it makes no
// call the index leaves open) serves whoever it serves, so the walk passes
// through it to the first helper doing more, whose non-helper calls are the
// step's work and whose own helpers serve it. "./lua script.lua" runs in the
// VM: lua_pcallk calls luaV_execute through luaD_call, ccall. A helper
// calling two helpers, or with a hand-over or an open call besides, does
// more than pass on.
func TestAStepsWorkPassesThroughConduits(t *testing.T) {
	type link struct{ from, to string }
	program := func(edges []link, handed []link, open []string) Input {
		subject := func(id string) groupindex.Subject {
			result := groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "vm.c", Line: 1, Column: 1}}}
			if strings.HasPrefix(id, "H") {
				result.Interpretation = &groupindex.Interpretation{Line: "Passes a call on.", Helper: true}
			}
			return result
		}
		index := groupindex.Index{
			Target:   programindex.Target{ID: "t1", Name: "vm", Seeds: []programindex.TargetSeed{{ObjectID: "S", Kind: programindex.SeedCallable}}},
			Subjects: []groupindex.Subject{subject("S"), subject("H1"), subject("H2"), subject("H3"), subject("H4"), subject("X"), subject("Y"), subject("Z")},
			Groups: []groupindex.Group{{ID: "g1", Title: "API", MemberSubjectIDs: []string{"S", "H1", "H2", "H3", "H4"}},
				{ID: "g2", Title: "Virtual machine", Core: true, MemberSubjectIDs: []string{"X", "Y", "Z"}}},
		}
		for _, edge := range edges {
			index.StructuralEdges = append(index.StructuralEdges, groupindex.StructuralEdge{FromSubjectID: edge.from, ToSubjectID: edge.to, Role: groupindex.EdgeRelationTarget,
				RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact})
		}
		for _, id := range open {
			index.Unresolved = append(index.Unresolved, groupindex.UnresolvedCall{FromSubjectID: id})
		}
		var registrations []facts.Fact
		for _, edge := range handed {
			registrations = append(registrations, facts.Fact{Kind: facts.KindRegistration, TargetID: "t1", OwnerID: edge.from, ObjectID: edge.to, Text: "loop.on"})
		}
		return Input{Groups: []groupindex.Index{index}, Facts: facts.Result{Facts: registrations}}
	}
	reached := func(input Input) map[string]string {
		graph := newFlowGraph(&input.Groups[0], input.Facts.Facts)
		result := map[string]string{}
		for _, candidate := range graph.candidates("S", []string{"S"}) {
			result[candidate.unit] = candidate.reach.asked()
		}
		return result
	}
	chain := []link{{"S", "H1"}, {"H1", "H2"}, {"H2", "H3"}, {"H3", "X"}, {"H3", "H4"}, {"H4", "Y"}}
	for _, test := range []struct {
		name   string
		edges  []link
		handed []link
		open   []string
		want   map[string]string
	}{
		{"conduits pass on", chain, nil, nil, map[string]string{"X": "called through H1, H2, H3"}},
		{"a hand-over is more than passing on", chain, []link{{"H1", "Z"}}, nil, map[string]string{"Z": "handed to loop.on through H1"}},
		{"an open call is more than passing on", chain, nil, []string{"H1"}, map[string]string{}},
		{"two helpers are more than passing on", []link{{"S", "H1"}, {"H1", "H2"}, {"H1", "H4"}, {"H2", "X"}, {"H4", "Y"}}, nil, nil, map[string]string{}},
		{"a cycle of conduits ends", []link{{"S", "H1"}, {"H1", "H2"}, {"H2", "H1"}}, nil, nil, map[string]string{}},
	} {
		if got := reached(program(test.edges, test.handed, test.open)); !maps.Equal(got, test.want) {
			t.Errorf("%s: S reaches %q, want %q", test.name, got, test.want)
		}
	}
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		return llm.Verdict{Choice: "X", Probabilities: map[string]float64{"X": 1}}, true
	}}
	walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, program(chain, nil, nil), "t1")
	if err != nil {
		t.Fatal(err)
	}
	if steps := walk.flow.Steps; len(steps) != 2 || steps[1].SubjectID != "X" || !slices.Equal(steps[1].Through, []string{"H1", "H2", "H3"}) {
		t.Fatalf("the flow is %+v, want S then X through H1, H2, H3", steps)
	}
	if _, err := Seal(Result{FactsSHA256: strings.Repeat("a", 64), ClaimsSHA256: strings.Repeat("b", 64), MainFlow: walk.flow}); err != nil {
		t.Fatalf("a flow through conduits does not seal: %v", err)
	}
}

// A unit the step's work reaches only on failing paths (a call that never
// returns, an error path, or a helper reached so) is no way on: it stays
// beside the step with its guard, never asked or followed, and a step left
// with only such units ends the path, saying so. Lua's forprep reaches the
// collector only through luaG_runerror, which never returns.
func TestAFailingPathIsNoWayOnAndThePathSaysWhyItStops(t *testing.T) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "lvm.c", Line: line, Column: 3}
	}
	guard := func(kind string, line int) *programindex.Guard {
		return &programindex.Guard{Kind: kind, Location: at(line)}
	}
	subject := func(id string, helper bool) groupindex.Subject {
		result := groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectFunction, Location: at(1)}}
		if helper {
			result.Interpretation = &groupindex.Interpretation{Line: "Raises an error.", Helper: true}
		}
		return result
	}
	calls := func(from, to string, guarded *programindex.Guard, loop *programindex.Location) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls,
			Resolution: programindex.ResolutionExact, Guard: guarded, Loop: loop}
	}
	index := groupindex.Index{
		Target:   programindex.Target{ID: "t1", Name: "vm", Seeds: []programindex.TargetSeed{{ObjectID: "execute", Kind: programindex.SeedCallable}}},
		Subjects: []groupindex.Subject{subject("execute", false), subject("forprep", false), subject("runerror", true), subject("collect", false), subject("panic", false)},
		Groups:   []groupindex.Group{{ID: "g1", Title: "Virtual machine", Core: true, MemberSubjectIDs: []string{"execute", "forprep", "runerror", "collect", "panic"}}},
		StructuralEdges: []groupindex.StructuralEdge{
			calls("execute", "forprep", guard(programindex.GuardBranch, 10), at(5)),
			calls("execute", "panic", guard(programindex.GuardNoReturn, 12), nil),
			calls("forprep", "runerror", guard(programindex.GuardNoReturn, 223), nil),
			calls("runerror", "collect", nil, nil),
		},
		Unresolved: []groupindex.UnresolvedCall{{FromSubjectID: "forprep", Location: at(462)}},
	}
	walk, err := walkFlow(t.Context(), llm.Executor{}, nil, Input{Groups: []groupindex.Index{index}}, "t1")
	if err != nil {
		t.Fatal(err)
	}
	steps := walk.flow.Steps
	if len(steps) != 2 || steps[1].SubjectID != "forprep" || steps[1].Guard == nil || steps[1].Guard.Kind != programindex.GuardBranch || steps[1].Loop == nil || steps[1].Loop.Line != 5 {
		t.Fatalf("the flow is %+v, want execute then forprep, under its branch, in its loop", steps)
	}
	if passed := steps[0].Passed; len(passed) != 1 || passed[0].SubjectID != "panic" || !passed[0].Guard.Fails() || passed[0].Guard.Location.Line != 12 {
		t.Fatalf("execute passed %+v, want panic, never returning at line 12", passed)
	}
	if passed := steps[1].Passed; len(passed) != 1 || passed[0].SubjectID != "collect" || passed[0].Guard.Kind != programindex.GuardNoReturn ||
		passed[0].Guard.Location.Line != 223 || !slices.Equal(passed[0].Through, []string{"runerror"}) {
		t.Fatalf("forprep passed %+v, want collect through runerror, never returning at line 223", passed)
	}
	// The route ends while forprep calls through a value whose target is
	// not established: it says where.
	if steps[1].OpenAt == nil || steps[1].OpenAt.Line != 462 || steps[0].OpenAt != nil {
		t.Fatalf("open calls: %+v, %+v", steps[0].OpenAt, steps[1].OpenAt)
	}
	if steps[1].Stop != StopFailureOnly || steps[0].Stop != "" || walk.flow.Title != "" {
		t.Fatalf("stops %q, %q; title %q", steps[0].Stop, steps[1].Stop, walk.flow.Title)
	}
	if _, err := Seal(Result{FactsSHA256: strings.Repeat("a", 64), ClaimsSHA256: strings.Repeat("b", 64), MainFlow: walk.flow}); err != nil {
		t.Fatalf("a flow with failing paths beside it does not seal: %v", err)
	}
}

// A unit reached through a helper two routes share keeps the weakest route,
// whichever the walk meets first: a normal route through H2 is not lost
// behind a failing route through H1 (control review, 2026-10-03).
func TestAWeakerRouteThroughASharedHelperGoesOn(t *testing.T) {
	failing := &programindex.Guard{Kind: programindex.GuardNoReturn, Location: &programindex.Location{Path: "s.c", Line: 3, Column: 1}}
	for _, test := range []struct {
		name    string
		failing string
		through []string
	}{{"the failing route first", "H1", []string{"H2", "H3"}}, {"the failing route second", "H2", []string{"H1", "H3"}}} {
		subject := func(id string) groupindex.Subject {
			result := groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "s.c", Line: 1, Column: 1}}}
			if strings.HasPrefix(id, "H") {
				result.Interpretation = &groupindex.Interpretation{Line: "Passes a call on.", Helper: true}
			}
			return result
		}
		calls := func(from, to string) groupindex.StructuralEdge {
			edge := groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact}
			if from == "S" && to == test.failing {
				edge.Guard = failing
			}
			return edge
		}
		index := groupindex.Index{
			Target:          programindex.Target{ID: "t1", Name: "s", Seeds: []programindex.TargetSeed{{ObjectID: "S", Kind: programindex.SeedCallable}}},
			Subjects:        []groupindex.Subject{subject("S"), subject("H1"), subject("H2"), subject("H3"), subject("X")},
			Groups:          []groupindex.Group{{ID: "g1", Title: "Work", Core: true, MemberSubjectIDs: []string{"S", "H1", "H2", "H3", "X"}}},
			StructuralEdges: []groupindex.StructuralEdge{calls("S", "H1"), calls("S", "H2"), calls("H1", "H3"), calls("H2", "H3"), calls("H3", "X")},
		}
		graph := newFlowGraph(&index, nil)
		candidates := graph.candidates("S", []string{"S"})
		if len(candidates) != 1 || candidates[0].unit != "X" || candidates[0].guard.Fails() || !slices.Equal(candidates[0].reach.through, test.through) {
			t.Fatalf("%s: S reaches %+v, want X on its normal route through %v", test.name, candidates, test.through)
		}
	}
}

// joinProgram is a program of bare functions, P its seed, each call an
// exact call written in its own file at its own line, every function in a
// core part.
func joinProgram(calls [][2]string) Input {
	var subjects []groupindex.Subject
	var members []string
	seen := map[string]bool{}
	for _, call := range calls {
		for _, id := range call {
			if !seen[id] {
				seen[id] = true
				members = append(members, id)
				subjects = append(subjects, groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectFunction,
					Location: &programindex.Location{Path: "join.c", Line: len(members), Column: 1}}})
			}
		}
	}
	var edges []groupindex.StructuralEdge
	for _, call := range calls {
		edges = append(edges, groupindex.StructuralEdge{FromSubjectID: call[0], ToSubjectID: call[1], Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact})
	}
	index := groupindex.Index{Target: programindex.Target{ID: "t1", Name: "join", Seeds: []programindex.TargetSeed{{ObjectID: "P", Kind: programindex.SeedCallable}}},
		Subjects: subjects, Groups: []groupindex.Group{{ID: "g1", Title: "Core", Core: true, MemberSubjectIDs: members}}, StructuralEdges: edges}
	return Input{Groups: []groupindex.Index{index}}
}

// Where a way goes on into where other ways start, every such choice is a
// way it goes on as, kept with how it is reached, and none is walked twice
// (control review, 2026-10-03: H torn between A and B kept only A). A join
// alone ends the way; two end it as either; a join beside a way of its own
// leaves the split torn, that way walked and the join beside it; a split
// nested in a way may join a start of the split around it.
func TestAWayGoesOnAsEveryWayItJoins(t *testing.T) {
	walkOf := func(t *testing.T, calls [][2]string, decide map[string]map[string]float64) MainFlow {
		t.Helper()
		categorizer := &typesafetest.Categorizer{Decide: func(_ string, question llm.Question) (llm.Verdict, bool) {
			step, _ := question.Item["step"].(string)
			if odds, known := decide[step]; known {
				lead := ""
				for name, odd := range odds {
					if lead == "" || odd > odds[lead] || odd == odds[lead] && name < lead {
						lead = name
					}
				}
				return llm.Verdict{Choice: lead, Probabilities: odds}, true
			}
			return llm.Verdict{}, false
		}}
		walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, joinProgram(calls), "t1")
		if err != nil {
			t.Fatal(err)
		}
		if err := validFlowSteps(walk.flow.Steps); err != nil {
			t.Fatal(err)
		}
		if _, err := Seal(Result{FactsSHA256: strings.Repeat("a", 64), ClaimsSHA256: strings.Repeat("b", 64), MainFlow: walk.flow}); err != nil {
			t.Fatalf("a flow with joins does not seal: %v", err)
		}
		return walk.flow
	}
	way := func(t *testing.T, step FlowStep, start string) FlowStep {
		t.Helper()
		for _, path := range step.Paths {
			if path.Steps[0].SubjectID == start {
				return path.Steps[len(path.Steps)-1]
			}
		}
		t.Fatalf("no way from %s: %+v", start, step.Paths)
		return FlowStep{}
	}
	subjects := func(branches []FlowBranch) []string {
		var ids []string
		for _, branch := range branches {
			ids = append(ids, branch.SubjectID)
		}
		slices.Sort(ids)
		return ids
	}
	names := func(steps []FlowStep) []string {
		var ids []string
		for _, step := range steps {
			ids = append(ids, step.SubjectID)
		}
		return ids
	}
	t.Run("one join", func(t *testing.T) {
		flow := walkOf(t, [][2]string{{"P", "A"}, {"P", "H"}, {"H", "A"}, {"H", "X"}, {"A", "VA"}},
			map[string]map[string]float64{"P": {"A": 0.52, "H": 0.48}, "H": {"A": 0.9, "X": 0.1}})
		h := way(t, flow.Steps[0], "H")
		if h.Stop != StopJoins || !slices.Equal(subjects(h.Joins), []string{"A"}) || h.Joins[0].Via != "called" || !slices.Equal(subjects(h.Passed), []string{"X"}) {
			t.Fatalf("H ends %q joining %+v, passing %+v", h.Stop, h.Joins, h.Passed)
		}
	})
	for _, order := range [][2]string{{"A", "B"}, {"B", "A"}} {
		t.Run("two joins "+order[0]+order[1], func(t *testing.T) {
			flow := walkOf(t, [][2]string{{"P", "A"}, {"P", "B"}, {"P", "H"}, {"H", order[0]}, {"H", order[1]}, {"A", "VA"}, {"B", "VB"}},
				map[string]map[string]float64{"P": {"A": 0.34, "B": 0.33, "H": 0.33}, "H": {"A": 0.52, "B": 0.48}})
			h := way(t, flow.Steps[0], "H")
			if h.Stop != StopJoins || !slices.Equal(subjects(h.Joins), []string{"A", "B"}) || len(h.Paths) != 0 || len(h.Passed) != 0 {
				t.Fatalf("H ends %q joining %+v (ways %+v, passed %+v)", h.Stop, h.Joins, h.Paths, h.Passed)
			}
			if a, b := way(t, flow.Steps[0], "A"), way(t, flow.Steps[0], "B"); a.SubjectID != "VA" || b.SubjectID != "VB" {
				t.Fatalf("the joined ways end at %s and %s", a.SubjectID, b.SubjectID)
			}
		})
	}
	t.Run("a join beside a way", func(t *testing.T) {
		flow := walkOf(t, [][2]string{{"P", "A"}, {"P", "H"}, {"H", "A"}, {"H", "C"}, {"C", "VC"}, {"A", "VA"}},
			map[string]map[string]float64{"P": {"A": 0.52, "H": 0.48}, "H": {"A": 0.52, "C": 0.48}})
		ways := flow.Steps[0].Paths
		if len(ways) != 2 || ways[1].Steps[0].SubjectID != "H" {
			t.Fatalf("P's ways %+v", ways)
		}
		h := ways[1].Steps[len(ways[1].Steps)-1]
		if h.SubjectID != "H" || h.Stop != StopTorn || !slices.Equal(subjects(h.Joins), []string{"A"}) || len(h.Paths) != 1 || !slices.Equal(names(h.Paths[0].Steps), []string{"C", "VC"}) {
			t.Fatalf("H ends %q joining %+v, its ways %+v", h.Stop, h.Joins, h.Paths)
		}
	})
	t.Run("a nested split joins the split around it", func(t *testing.T) {
		flow := walkOf(t, [][2]string{{"P", "A"}, {"P", "H"}, {"H", "C"}, {"H", "D"}, {"D", "A"}, {"D", "E"}, {"A", "VA"}, {"C", "VC"}},
			map[string]map[string]float64{"P": {"A": 0.52, "H": 0.48}, "H": {"C": 0.52, "D": 0.48}, "D": {"A": 0.8, "E": 0.2}})
		h := way(t, flow.Steps[0], "H")
		if h.SubjectID != "H" || h.Stop != StopTorn || len(h.Joins) != 0 {
			t.Fatalf("H ends %q joining %+v", h.Stop, h.Joins)
		}
		if d := way(t, h, "D"); d.SubjectID != "D" || d.Stop != StopJoins || !slices.Equal(subjects(d.Joins), []string{"A"}) || !slices.Equal(subjects(d.Passed), []string{"E"}) {
			t.Fatalf("D ends %q joining %+v, passing %+v", d.Stop, d.Joins, d.Passed)
		}
	})
}

// An orientation saved before every join was kept (version 3, one
// stop_subject) is refused by its version, never field by field: render on
// an older run says which version it holds (control review, 2026-10-03).
func TestAnOlderOrientationIsRefusedByItsVersion(t *testing.T) {
	old := []byte(`{"version":3,"main_flow":{"steps":[{"target_id":"t1","subject_id":"H","stop":"joins","stop_subject":"A"}]}}`)
	for _, err := range []error{CheckVersion(old), func() error { _, err := Decode(old); return err }()} {
		if err == nil || !strings.Contains(err.Error(), "unsupported version 3") || strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("an older orientation is refused by %v", err)
		}
	}
}

// A step parts at a path's end, or where the path is next back in a step
// before it: whichever way runs returns there (Lua 5.1.5's luaD_precall
// parts, then the path is back in luaD_call at luaV_execute).
func TestAFlowPartsMidPathOnlyWhereThePathIsBackInAStepBeforeIt(t *testing.T) {
	way := []FlowPath{{Steps: []FlowStep{{TargetID: "t1", SubjectID: "gc", Via: "called"}}}, {Steps: []FlowStep{{TargetID: "t1", SubjectID: "read", Via: "called"}}}}
	steps := func(resumes string) []FlowStep {
		return []FlowStep{{TargetID: "t1", SubjectID: "call"}, {TargetID: "t1", SubjectID: "precall", Via: "called", Stop: StopTorn, Paths: way},
			{TargetID: "t1", SubjectID: "execute", Via: "called", Resumes: resumes}}
	}
	if err := validFlowSteps(steps("call")); err != nil {
		t.Fatalf("parts, then back in call: %v", err)
	}
	if validFlowSteps(steps("")) == nil {
		t.Fatal("a step parting before a step that is not back in a step before it was accepted")
	}
	if validFlowSteps([]FlowStep{{TargetID: "t1", SubjectID: "call", Resumes: "main"}}) == nil {
		t.Fatal("a flow's first step back in a step before it was accepted")
	}
}

// Where a route ends the path is back in the step before it at the calls
// it writes after the chosen one, each under its own guard (luaD_call:
// first luaD_precall, then, only if its result says so, luaV_execute). A
// callee written twice, guarded first, hands back from its unguarded site,
// so a call between the two sites is no later call; a language whose
// adapter records no guards hands nothing back (skeptic, 2026-10-04).
func TestAStepIsReadBackInItsCallerAfterTheChosenCallsRoute(t *testing.T) {
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "ldo.c", Line: line, Column: 3}
	}
	subject := func(id string) groupindex.Subject {
		return groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectFunction, Location: at(1)}}
	}
	calls := func(from, to string, line int, guarded *programindex.Guard) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls,
			Resolution: programindex.ResolutionExact, Location: at(line), Guard: guarded}
	}
	branch := &programindex.Guard{Kind: programindex.GuardBranch, Location: at(10)}
	walkOf := func(language string, edges ...groupindex.StructuralEdge) MainFlow {
		t.Helper()
		index := groupindex.Index{
			Target:          programindex.Target{ID: "t1", Name: "lua", Language: language, Seeds: []programindex.TargetSeed{{ObjectID: "call", Kind: programindex.SeedCallable}}},
			Subjects:        []groupindex.Subject{subject("call"), subject("precall"), subject("execute"), subject("collect")},
			Groups:          []groupindex.Group{{ID: "g1", Title: "Virtual machine", Core: true, MemberSubjectIDs: []string{"call", "precall", "execute", "collect"}}},
			StructuralEdges: edges,
		}
		categorizer := &typesafetest.Categorizer{Decide: func(_ string, question llm.Question) (llm.Verdict, bool) {
			if step, _ := question.Item["step"].(string); step == "call" && slices.ContainsFunc(question.Options, func(option llm.Option) bool { return option.Name == "precall" }) {
				return typesafetest.Choose("precall"), true
			}
			return llm.Verdict{}, false
		}}
		walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, Input{Groups: []groupindex.Index{index}}, "t1")
		if err != nil {
			t.Fatal(err)
		}
		if err := validFlowSteps(walk.flow.Steps); err != nil {
			t.Fatal(err)
		}
		return walk.flow
	}
	flow := walkOf("c", calls("call", "precall", 10, nil), calls("call", "execute", 11, branch), calls("precall", "collect", 30, nil))
	if got := flowNames(flow); !slices.Equal(got, []string{"call ()", "precall (called)", "collect (called)", "execute (called)"}) ||
		flow.Steps[3].Resumes != "call" || flow.Steps[3].Guard == nil || flow.Steps[3].Guard.Location.Line != 10 || len(flow.Steps[0].Passed) != 0 {
		t.Fatalf("the flow is %q, %+v", got, flow.Steps)
	}
	flow = walkOf("c", calls("call", "precall", 5, branch), calls("call", "execute", 8, nil), calls("call", "precall", 12, nil), calls("precall", "collect", 30, nil))
	if got := flowNames(flow); !slices.Equal(got, []string{"call ()", "precall (called)", "collect (called)"}) || len(flow.Steps[0].Passed) != 1 {
		t.Fatalf("a callee written twice, guarded first: the flow is %q, %+v", got, flow.Steps)
	}
	flow = walkOf("typescript", calls("call", "precall", 10, nil), calls("call", "execute", 11, branch), calls("precall", "collect", 30, nil))
	if got := flowNames(flow); !slices.Equal(got, []string{"call ()", "precall (called)", "collect (called)"}) {
		t.Fatalf("without recorded guards: the flow is %q", got)
	}
}
