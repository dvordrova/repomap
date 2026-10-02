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
		_, role, _ := strings.Cut(option.Meaning, "role: ")
		role, _, _ = strings.Cut(role, ";")
		roles[option.Name] = role
	}
	// _refresh is folded: its call of Strategy is process's own.
	want := map[string]string{"analyze": "Decides entry and exit signals.", "enter": "", "exit": ""}
	if !maps.Equal(roles, want) || walk.flow.Steps[1].Explanation != "" {
		t.Fatalf("options %q, want %q; process reads %q", roles, want, walk.flow.Steps[1].Explanation)
	}
}
