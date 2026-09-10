package reading

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestJointMemoSharesExactRowsKeepsRefusedNeighboursAndRevalidatesReplay(t *testing.T) {
	p := &independentResponseProvider{response: []byte(`{"rows":[{"key":"r1","same":"yes","label":"reads orders"},{"key":"r2","same":"invalid","label":"refused"}]}`)}
	r := answerTestReader(t, nil, p)
	def := lines.Joints()
	shared := []table.Field{{Name: "question", Value: "joints"}}
	side := lines.BoundarySide{Target: "service", Path: "api.py", Caller: "orders", Method: "GET", Values: []string{"/orders"}}
	rows := []table.Row{lines.JointRow("first-current-id", "/orders", side, side), lines.JointRow("duplicate-current-id", "/orders", side, side), lines.JointRow("different-current-id", "/other", side, side)}
	got, err := r.runJointTable(t.Context(), def, 1, shared, rows)
	if err != nil || p.calls != 1 || got[0].answer == nil || got[1].answer == nil || got[2].answer != nil {
		t.Fatalf("bad independent result: %+v / %v calls%d", got, err, p.calls)
	}
	if got[0].requestKey != got[1].requestKey || got[0].rowKey != got[1].rowKey || len(r.knowledge) != 0 {
		t.Fatal("duplicate input copied semantics or entity knowledge")
	}
	old := got[0]
	current := rows[0]
	current.ID = "entirely-new-graph-id"
	warm, err := r.runJointTable(t.Context(), def, 2, shared, []table.Row{current})
	if err != nil || p.calls != 1 || warm[0].source != atlas.SourceCache {
		t.Fatalf("exact row not reused across batch/graph: %+v %v", warm, err)
	}
	exchange, found, err := llm.CachedExchange(r.opts.Executor.RootDir, old.requestKey)
	if err != nil || !found {
		t.Fatal("missing exact original exchange", err)
	}
	p.response = []byte(`{"rows":[{"key":"r1","same":"no","label":"-"},{"key":"r2","same":"yes","label":"reads other"}]}`)
	prepared, err := llm.NewPrepared(exchange.Request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = llm.ReplayJSON(t.Context(), r.opts.Executor, p, prepared); err != nil {
		t.Fatal(err)
	}
	// A new reading reopens original response bytes after replay.
	r.responseTables = make(map[string]rememberedTable)
	replayed, err := r.runJointTable(t.Context(), def, 3, shared, []table.Row{current})
	if err != nil || p.calls != 2 || replayed[0].answer["same"] != "no" || replayed[0].responseSHA == old.responseSHA {
		t.Fatalf("stale memo after replay: %+v %v", replayed, err)
	}
	changed := side
	changed.Target = "different service"
	p.response = []byte(`{"rows":[{"key":"r1","same":"yes","label":"new context"}]}`)
	_, err = r.runJointTable(t.Context(), def, 4, shared, []table.Row{lines.JointRow("same-id", "/orders", changed, side)})
	if err != nil || p.calls != 3 {
		t.Fatalf("semantic target context ignored: %v calls%d", err, p.calls)
	}
	def.System += "\nNew protocol requirement."
	_, err = r.runJointTable(t.Context(), def, 5, shared, []table.Row{current})
	if err != nil || p.calls != 4 {
		t.Fatalf("changed prompt reused: %v calls%d", err, p.calls)
	}
}

func TestPeerMemoCanonicalizesFullCatalogueAndRestoresCurrentEndpoints(t *testing.T) {
	p := &independentResponseProvider{response: []byte(`{"rows":[{"key":"r1","peer":"p1","label":"reads orders"}]}`)}
	r := answerTestReader(t, nil, p)
	r.targets = map[string]*targetState{"client": {role: atlas.RoleProduct}, "server": {role: atlas.RoleProduct}}
	targets := map[string]TargetMeta{"client": {ID: "client", Name: "client"}, "server": {ID: "server", Name: "server"}}
	makeBoundary := func(id, path, caller, direction, target string) *boundaryState {
		return &boundaryState{place: atlas.Place{ID: id, Path: path, TargetIDs: []string{target}, Boundary: &atlas.BoundaryFacts{Direction: direction, Caller: caller, External: "http", Method: "GET", Source: "fact"}}}
	}
	out := makeBoundary("old-out", "client.py", "send", atlas.DirectionOut, "client")
	a := makeBoundary("old-z", "a.py", "orders", atlas.DirectionIn, "server")
	b := makeBoundary("old-a", "b.py", "positions", atlas.DirectionIn, "server")
	round := 1
	first, err := r.chooseBlindPeers(t.Context(), []peerBatch{{outs: []*boundaryState{out}, ins: []*boundaryState{b, a}}}, targets, &round)
	if err != nil || first[out.place.ID].in != a || p.calls != 1 {
		t.Fatalf("wrong initial endpoint: %+v %v", first, err)
	}
	currentOut := makeBoundary("new-out", "client.py", "send", atlas.DirectionOut, "client")
	currentA := makeBoundary("new-a", "a.py", "orders", atlas.DirectionIn, "server")
	currentB := makeBoundary("new-z", "b.py", "positions", atlas.DirectionIn, "server")
	second, err := r.chooseBlindPeers(t.Context(), []peerBatch{{outs: []*boundaryState{currentOut}, ins: []*boundaryState{currentA, currentB}}}, targets, &round)
	if err != nil || second[currentOut.place.ID].in != currentA || p.calls != 1 {
		t.Fatalf("memo kept old endpoint or reordered source: %+v %v calls%d", second, err, p.calls)
	}
	currentB.place.Boundary.CallerDoc = "Different protocol behavior."
	_, err = r.chooseBlindPeers(t.Context(), []peerBatch{{outs: []*boundaryState{currentOut}, ins: []*boundaryState{currentA, currentB}}}, targets, &round)
	if err != nil || p.calls != 2 {
		t.Fatalf("changed unselected peer failed to invalidate: %v calls%d", err, p.calls)
	}
	r.opts.Executor.Enabled = false
	_, err = r.chooseBlindPeers(t.Context(), []peerBatch{{outs: []*boundaryState{currentOut}, ins: []*boundaryState{currentA, currentB}}}, targets, &round)
	if err != nil || p.calls != 3 {
		t.Fatalf("no-cache reused persistent memo: %v calls%d", err, p.calls)
	}
}

type jointProviderState struct{ llm.Provider }

func (jointProviderState) State() []byte {
	return []byte(`{"endpoint":"https://provider.test","model":"changed"}`)
}

func TestJointMemoDoesNotCrossProviderConfiguration(t *testing.T) {
	p := &independentResponseProvider{response: []byte(`{"rows":[{"key":"r1","same":"no","label":"-"}]}`)}
	r := answerTestReader(t, nil, p)
	rows := []table.Row{lines.JointRow("joint", "x", lines.BoundarySide{Path: "a.py"}, lines.BoundarySide{Path: "b.py"})}
	if _, err := r.runJointTable(t.Context(), lines.Joints(), 1, nil, rows); err != nil {
		t.Fatal(err)
	}
	r.opts.Provider = jointProviderState{Provider: p}
	if _, err := r.runJointTable(t.Context(), lines.Joints(), 2, nil, rows); err != nil {
		t.Fatal(err)
	}
	if p.calls != 2 {
		t.Fatalf("provider change reused previous model answer: %d calls", p.calls)
	}
}
