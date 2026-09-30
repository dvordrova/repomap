package groupindex

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A file two targets index lists the declarations of both views: a part
// of one target may name a declaration only the other target's view
// declares (othello's cljs view kept (declare negamax), which its clj view
// no longer reads, and the run failed with "part p4 names an unknown
// declaration t2.n229"). Such a member is none of this program's and is
// left out of its part; an unknown declaration of the program's own still
// fails its projection.
func TestAPartMemberOnlyAnotherTargetsViewDeclaresIsNotThisProgramsOwn(t *testing.T) {
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "src/search.cljc", Line: line, Column: 1}
	}
	program, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{Language: "clojure", Kind: "package", Name: "app", Selector: "app", Sources: []programindex.TargetSource{{FileRef: "search", Path: "src/search.cljc"}}, AnchorFileRef: "search"},
		Objects: []programindex.ObjectInput{
			{SourceRef: "negamax", Name: "search/negamax", Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityPublic, Location: location(59)},
		},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	own := program.Objects[0].ID
	box := atlas.Box{ID: "p4", Dir: "src", Title: "AI search", Line: "Searches moves.", Side: atlas.SideMid,
		MemberIDs: []string{"t9.n229", own}, Files: []atlas.File{{Path: "src/search.cljc", Line: "Search.", Source: atlas.SourceModel, Symbols: []atlas.Symbol{}}}}
	value := atlas.Atlas{Version: atlas.Version, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}, Targets: []atlas.Target{{
		ID: program.Target.ID, Name: "app", Root: "src", Zones: []atlas.Zone{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{}, Boxes: []atlas.Box{box}}}}
	indexes, err := ProjectAtlas(map[string]programindex.Index{program.Target.ID: program}, value)
	if err != nil {
		t.Fatalf("another view's declaration failed the projection: %v", err)
	}
	if groups := indexes[0].Groups; len(groups) != 1 || len(groups[0].MemberSubjectIDs) != 1 || groups[0].MemberSubjectIDs[0] != own {
		t.Fatalf("the part's members: %+v", groups)
	}
	value.Targets[0].Boxes[0].MemberIDs = []string{"n229", own}
	if _, err := ProjectAtlas(map[string]programindex.Index{program.Target.ID: program}, value); err == nil {
		t.Fatal("an unknown declaration of the program's own was accepted")
	}
}
