package orientation

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// A registered callable on the Main flow is registered where the path
// reaches, over the edges the walk walks, the fewest hops first, every run
// of that length kept: redis's readQueryFromClient, reached as one of the
// callables aeProcessEvents may call, is registered by createClient, which
// acceptHandler (another of them) calls, never through serverCron →
// syncWithMaster, the replication path exact calls alone had found; its
// registration in beforeSleep, which the path reaches only from aeMain, is
// not named. A registration no step reaches is its registering function
// alone, and the site the step is reached from runs it without being said
// again.
func TestARegisteredStepIsRegisteredWhereThePathReaches(t *testing.T) {
	subject := func(id string) groupindex.Subject {
		return groupindex.Subject{ID: id, Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "redis.c", Line: 1, Column: 1}}}
	}
	calls := func(from, to string) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionExact}
	}
	ids := []string{"main", "aeMain", "aeProcessEvents", "processTimeEvents", "serverCron", "syncWithMaster", "acceptHandler", "acceptUnix", "createClient", "beforeSleep", "readQueryFromClient", "processCommand", "lonely", "sleeper", "loadAppendOnlyFile"}
	var subjects []groupindex.Subject
	for _, id := range ids {
		subjects = append(subjects, subject(id))
	}
	index := groupindex.Index{
		Target:   programindex.Target{ID: "t1", Name: "redis-server", Seeds: []programindex.TargetSeed{{ObjectID: "main", Kind: programindex.SeedCallable}}},
		Subjects: subjects,
		Groups:   []groupindex.Group{{ID: "g1", Title: "Commands", Core: true, MemberSubjectIDs: []string{"processCommand", "createClient"}}},
		StructuralEdges: []groupindex.StructuralEdge{
			calls("main", "aeMain"), calls("aeMain", "aeProcessEvents"), calls("aeMain", "beforeSleep"),
			calls("aeProcessEvents", "processTimeEvents"), calls("processTimeEvents", "serverCron"), calls("serverCron", "syncWithMaster"), calls("syncWithMaster", "createClient"),
			calls("acceptHandler", "createClient"), calls("acceptUnix", "createClient"), calls("readQueryFromClient", "processCommand"),
			calls("main", "loadAppendOnlyFile"),
			{FromSubjectID: "loadAppendOnlyFile", ToSubjectID: "readQueryFromClient", Role: groupindex.EdgeRelationTarget, RelationID: "proc", RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionAlternatives},
			{FromSubjectID: "loadAppendOnlyFile", ToSubjectID: "processCommand", Role: groupindex.EdgeRelationTarget, RelationID: "proc", RelationKind: programindex.RelationCalls, Resolution: programindex.ResolutionAlternatives},
		},
		Unresolved: []groupindex.UnresolvedCall{{FromSubjectID: "aeProcessEvents", Possible: []string{"acceptHandler", "acceptUnix", "readQueryFromClient"}, Location: &programindex.Location{Path: "ae.c", Line: 335}}},
	}
	registration := func(id, owner, object string) facts.Fact {
		return facts.Fact{ID: id, Kind: facts.KindRegistration, TargetID: "t1", OwnerID: owner, ObjectID: object, Key: "aeCreateFileEvent", Text: "aeCreateFileEvent"}
	}
	input := Input{Groups: []groupindex.Index{index}, Facts: facts.Result{Facts: []facts.Fact{
		registration("a1", "beforeSleep", "readQueryFromClient"), registration("a2", "createClient", "readQueryFromClient"),
		registration("a3", "lonely", "sleeper"),
	}}}
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		for _, choice := range []string{"aeMain", "aeProcessEvents", "readQueryFromClient"} {
			if slices.ContainsFunc(question.Options, func(option llm.Option) bool { return option.Name == choice }) {
				return llm.Verdict{Choice: choice, Probabilities: map[string]float64{choice: 0.9}}, true
			}
		}
		return llm.Verdict{}, false
	}}
	walk, err := walkFlow(t.Context(), llm.Executor{}, categorizer, input, "t1")
	if err != nil {
		t.Fatal(err)
	}
	var read *FlowStep
	for position := range walk.flow.Steps {
		if walk.flow.Steps[position].SubjectID == "readQueryFromClient" {
			read = &walk.flow.Steps[position]
		}
	}
	if read == nil {
		t.Fatalf("the flow does not reach readQueryFromClient: %+v", walk.flow.Steps)
	}
	said := func(chain []FlowHop) string {
		var out []string
		for position, hop := range chain {
			switch {
			case position == 0:
			case hop.Possible:
				out = append(out, "may call")
			case hop.Handed:
				out = append(out, "hands over")
			default:
				out = append(out, "→")
			}
			out = append(out, hop.SubjectID)
		}
		return strings.Join(out, " ")
	}
	var got []string
	for _, registration := range read.Registered {
		got = append(got, said(registration.Chain)+" @"+registration.FactID)
	}
	// Two callables aeProcessEvents may call reach createClient in two
	// hops: both runs are kept.
	if want := []string{"aeProcessEvents may call acceptHandler → createClient @a2", "aeProcessEvents may call acceptUnix → createClient @a2"}; !slices.Equal(got, want) {
		t.Fatalf("readQueryFromClient is registered by %q, want %q", got, want)
	}
	// The site the step is reached from is not said again; another
	// function calling it through a value runs it, read from the flow's
	// first step.
	if len(read.RunBy) != 1 || said(read.RunBy[0].Chain) != "main → loadAppendOnlyFile" {
		t.Fatalf("readQueryFromClient is run by %+v, want main → loadAppendOnlyFile alone", read.RunBy)
	}
	// Unreached, a registration is its registering function alone.
	graph := newFlowGraph(&index, input.Facts.OfKind(facts.KindRegistration))
	if alone := graph.registrationsOf([]facts.Fact{registration("a3", "lonely", "sleeper")}, []string{"main"}); len(alone) != 1 || said(alone[0].Chain) != "lonely" {
		t.Fatalf("an unreached registration reads %+v", alone)
	}
}
