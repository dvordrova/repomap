package reading

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// boxesReader is a reader of target t1 with count drawn parts p1..pN, each
// holding its own file fN.
func boxesReader(t *testing.T, count int) *reader {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, atlas.TablesDir), 0700); err != nil {
		t.Fatal(err)
	}
	r := &reader{opts: Options{Targets: []TargetMeta{{ID: "t1"}}, Provider: &tableProvider{}, OwnerRunDir: root, Stage: func(string, ...string) {}, State: func(string, string, ...string) {}},
		boxes: map[string]*boxState{}, places: map[string]atlas.Place{}, uses: map[string]*atlas.StageUse{}, started: map[string]time.Time{}}
	for i := 1; i <= count; i++ {
		id := fmt.Sprintf("p%d", i)
		file := fmt.Sprintf("f%d", i)
		r.places[file] = atlas.Place{ID: file, Path: "src/" + id + ".c", TargetIDs: []string{"t1"}}
		r.boxes[id] = &boxState{id: id, targetID: "t1", title: id, line: "Original " + id, files: []string{file}, sources: []string{file}, units: i}
	}
	return r
}

func TestNativeCallObserversOwnDesignRolesAndArrows(t *testing.T) {
	r := boxesReader(t, 2)
	r.opts.Targets = []TargetMeta{{ID: "t1"}, {ID: "t2"}}
	r.designSubjects = map[string]string{}
	r.symbolsBySource = map[string]string{}
	r.designBoxOf = map[string]map[string]string{"t1": {"s1": "p1", "s2": "p2"}, "t2": {"s1": "p1", "s2": "p2"}}
	for i, id := range []string{"s1", "s2"} {
		file := []string{"f1", "f2"}[i]
		decl := atlas.Decl{Name: id, Kind: "function", LineNo: 1, ObjectID: "t1.n" + id}
		fp := r.places[file]
		fp.TargetIDs = []string{"t1", "t2"}
		fp.Kind = atlas.PlaceFile
		fp.File = &atlas.FileFacts{Decls: []atlas.Decl{decl}}
		r.places[file] = fp
		sp := atlas.Place{ID: id, Kind: atlas.PlaceSymbol, Parent: file, Path: fp.Path, LineNo: 1, TargetIDs: []string{"t1", "t2"}, Symbol: &atlas.SymbolFacts{Decl: decl}}
		if i == 0 {
			sp.Symbol.Calls = []atlas.SymbolCall{{TargetIDs: []string{"t1"}, Kind: "calls", Resolution: "exact", Line: 2, Column: 3, CalleeIDs: []string{"s2"}}}
		}
		r.places[id] = sp
		r.opts.Graph.Places = append(r.opts.Graph.Places, fp, sp)
		r.symbolsBySource[symbolSourceKey(sp.Path, sp.LineNo, decl.Name)] = id
		r.boxes[[]string{"p1", "p2"}[i]].symbols = map[string]bool{id: true}
	}
	// Shared file edges must never reintroduce the other view's call.
	r.opts.Graph.Edges = []atlas.Edge{{From: "f1", To: "f2", Kind: "calls", Count: 1, Static: true}}
	for _, target := range []string{"t1", "t2"} {
		want := 0
		if target == "t1" {
			want = 1
		}
		view := r.designView(target)
		if len(view.sites) != want {
			t.Fatalf("%s design borrowed a call: %+v", target, view.sites)
		}
		facts := r.unitFacts(view)
		if len(facts.units["s1"].uses) != want {
			t.Fatalf("%s roles borrowed a call: %+v", target, facts.units["s1"].uses)
		}
	}
	r.foldArrows()
	if len(r.arrows["t1"]) != 1 || len(r.arrows["t2"]) != 0 {
		t.Fatalf("arrows borrowed file observation: %+v", r.arrows)
	}
	caller := r.places["s1"]
	callee := r.places["s2"]
	callee.Symbol.CalledBy = []atlas.SymbolCaller{{ObjectID: "t1.n1", PlaceID: caller.ID, Kind: "calls", Resolution: "exact"}}
	if len(r.runningCallers([]string{"t1"}, callee, "exact")) != 1 || len(r.runningCallers([]string{"t2"}, callee, "exact")) != 0 {
		t.Fatal("reverse call borrowed shared caller ownership")
	}
}

func TestDestinationCallResultsKeepTheirNativeObservers(t *testing.T) {
	api := &atlas.CallAPI{Package: "example/sdk", Name: "Open"}
	at := &sourcevalue.Anchor{Path: "shared.c", Line: 4, Column: 8}
	place := atlas.Place{ID: "s1", Path: "shared.c", LineNo: 1, TargetIDs: []string{"t1", "t2"}, Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "Shared"}}}
	for _, target := range []string{"t1", "t2"} {
		place.Symbol.Calls = append(place.Symbol.Calls, atlas.SymbolCall{TargetIDs: []string{target}, API: api, Name: "Open", Line: 4, Column: 8, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "literal", Text: target}}}})
	}
	d := NewDestinationReader([]atlas.Place{place}, DestinationChoices{Arguments: map[string]ArgumentChoice{"example/sdk.Open": {Position: 1}}, Talks: map[string]string{}})
	for _, target := range []string{"t1", "t2"} {
		send := atlas.SymbolCall{TargetIDs: []string{target}, API: api, Name: "Open", Line: 9, Column: 2, SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "call_result", Anchor: at}}}}
		uses := d.Read(place, send)
		if len(uses) != 1 || uses[0].Address != target || !slices.Equal(uses[0].TargetIDs, []string{target}) || len(uses[0].Steps) != 2 {
			t.Fatalf("%s borrowed same-anchor result: %+v", target, uses)
		}
		send.ReceiverValue = &sourcevalue.Value{Kind: "call_result", Anchor: at}
		uses = d.Exchange(place, send, "client_request")
		if len(uses) != 1 || uses[0].Address != target || !slices.Equal(uses[0].TargetIDs, []string{target}) {
			t.Fatalf("%s exchange borrowed same-anchor result: %+v", target, uses)
		}
	}
}
