package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

// Captures actual completed prepared bytes, not a reconstructed prompt or
// a provider-independent mirror. Arrival order is deliberately not authority.
type queueCaptureProvider struct {
	*tableProvider
	lock     sync.Mutex
	requests []string
}

func (p *queueCaptureProvider) Complete(ctx context.Context, req llm.Prepared) (llm.Completion, error) {
	p.lock.Lock()
	p.requests = append(p.requests, string(req.Bytes()))
	p.lock.Unlock()
	return p.tableProvider.Complete(ctx, req)
}

func queueGraph(t *testing.T, invalidLast bool) (atlas.Graph, []TargetMeta) {
	t.Helper()
	g := twoTargetGraph(t)
	ids := []string{"svc"}
	for i := 2; i <= 13; i++ {
		ids = append(ids, fmt.Sprintf("svc%d", i))
	}
	for i := range g.Places {
		if slices.Contains(g.Places[i].TargetIDs, "svc") {
			g.Places[i].TargetIDs = append(slices.Clone(g.Places[i].TargetIDs), ids[1:]...)
		}
	}
	targets := []TargetMeta{{ID: "web", Language: "typescript", Kind: "package", Name: "web", Root: "web"}}
	for _, id := range ids {
		targets = append(targets, TargetMeta{ID: id, Language: "go", Kind: "executable", Name: "example.com/" + id, Root: "svc"})
	}
	// The relevant race is ObjectID-only registration -> shared designSubjects.
	// Repeated complete boundary rows enlarge that read corridor without new
	// model evidence: appendUnique retains one actual command observation.
	for i := 0; i < 1800; i++ {
		g.Places = append(g.Places, atlas.Place{ID: fmt.Sprintf("bnd:queue:%d", i), Kind: atlas.PlaceBoundary, Path: "svc/api/h.go", Parent: atlas.FileID("svc/api/h.go"), LineNo: 5, TargetIDs: slices.Clone(ids), Boundary: &atlas.BoundaryFacts{Source: "fact", Direction: atlas.DirectionIn, ObjectID: "n1", Words: []string{"queue_command"}}})
	}
	if invalidLast {
		// Intentionally malformed supplied native owner relation, isolated to
		// target14: it must fail the same source preflight, never become a model
		// question. This is not a claim that a supported adapter emits a cycle.
		id := targets[len(targets)-1].ID
		file := atlas.FileID("late/cycle.go")
		a := atlas.Decl{ObjectID: "lateA", Name: "A", Kind: "type", LineNo: 1}
		b := atlas.Decl{ObjectID: "lateB", Name: "B", Kind: "type", LineNo: 2}
		aid := atlas.SymbolID("late/cycle.go", 1, "A")
		bid := atlas.SymbolID("late/cycle.go", 2, "B")
		ma, mb := a, b
		ma.Kind = "method"
		mb.Kind = "method"
		g.Places = append(g.Places,
			atlas.Place{ID: file, Kind: atlas.PlaceFile, Path: "late/cycle.go", TargetIDs: []string{id}, File: &atlas.FileFacts{Decls: []atlas.Decl{a, b}}},
			atlas.Place{ID: aid, Kind: atlas.PlaceSymbol, Parent: file, Path: "late/cycle.go", LineNo: 1, TargetIDs: []string{id}, Symbol: &atlas.SymbolFacts{Decl: a, Members: []atlas.TypeMember{{Path: "late/cycle.go", Decl: mb}}}},
			atlas.Place{ID: bid, Kind: atlas.PlaceSymbol, Parent: file, Path: "late/cycle.go", LineNo: 2, TargetIDs: []string{id}, Symbol: &atlas.SymbolFacts{Decl: b, Members: []atlas.TypeMember{{Path: "late/cycle.go", Decl: ma}}}},
		)
	}
	atlas.SortPlaces(g.Places)
	return g, targets
}

type queueRun struct {
	requests []string
	tables   string
	payloads map[string]string
	atlas    []byte
	subjects map[string]string
}

func runQueueFixture(t *testing.T, cap int, eager, invalid bool) (queueRun, error) {
	t.Helper()
	graph, targets := queueGraph(t, invalid)
	graphBefore, e := json.Marshal(graph)
	if e != nil {
		t.Fatal(e)
	}
	p := &queueCaptureProvider{tableProvider: &tableProvider{}}
	opts := readOptions(t, graph, p, "")
	opts.Targets = targets
	opts.Executor.BatchConcurrency = cap
	opts.Stage = func(string, ...string) {}
	opts.State = func(string, string, ...string) {}
	if e = os.MkdirAll(filepath.Join(opts.OwnerRunDir, atlas.TablesDir), 0700); e != nil {
		t.Fatal(e)
	}
	r, files := newReader(opts)
	if eager {
		e = r.readDesignEagerQueueOracle(t.Context())
	} else {
		e = r.readDesign(t.Context())
	}
	after, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	if string(graphBefore) != string(after) {
		t.Fatal("queue mutated original graph authority")
	}
	p.lock.Lock()
	requests := slices.Clone(p.requests)
	p.lock.Unlock()
	slices.Sort(requests)
	got := queueRun{requests: requests, tables: r.tables.String(), payloads: map[string]string{}, subjects: r.designSubjects}
	if e != nil {
		return got, e
	}
	got.atlas, err = json.Marshal(r.atlas(files))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(opts.OwnerRunDir, atlas.TablesDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatal("unexpected directory in table window inventory")
		}
		name := entry.Name()
		f := filepath.Join(opts.OwnerRunDir, atlas.TablesDir, name)
		var raw []byte
		if strings.HasSuffix(name, ".ref.json") {
			raw, err = readWindowPayload(f)
		} else {
			raw, err = os.ReadFile(f)
		}
		if err != nil {
			t.Fatal(err)
		}
		got.payloads[name] = string(raw)
	}
	// Explicit positive assertion: registration remains attributed through
	// the final native ObjectID->place map before every target worker starts.
	for _, target := range targets[1:] {
		view := r.designView(target.ID)
		facts := r.unitFacts(view)
		found := false
		for _, unit := range facts.units {
			if slices.Contains(unit.registered, "queue_command") {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s lost ObjectID-only registration", target.ID)
		}
	}
	return got, nil
}

func TestPreparationQueuePreservesEagerCompleteBytesAndOrderedArtifacts(t *testing.T) {
	baseline, err := runQueueFixture(t, 12, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline.requests) == 0 || len(baseline.subjects) == 0 || len(baseline.payloads) == 0 {
		t.Fatal("vacuous eager baseline")
	}
	for _, cap := range []int{1, 2, 12} {
		t.Run(fmt.Sprint(cap), func(t *testing.T) {
			got, err := runQueueFixture(t, cap, false, false)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.requests, baseline.requests) {
				t.Fatalf("cap%d changed actual complete prepared requests: %d vs%d", cap, len(got.requests), len(baseline.requests))
			}
			if got.tables != baseline.tables {
				t.Fatalf("cap%d changed ordered tables", cap)
			}
			if !reflect.DeepEqual(got.payloads, baseline.payloads) {
				t.Fatalf("cap%d changed exact window bytes/inventory", cap)
			}
			if string(got.atlas) != string(baseline.atlas) {
				t.Fatalf("cap%d changed exact target order/membership/compact IDs/captions", cap)
			}
			if !reflect.DeepEqual(got.subjects, baseline.subjects) {
				t.Fatalf("cap%d changed complete subject map", cap)
			}
		})
	}
}
func TestPreparationQueueLateInvalidHasEagerErrorAndZeroTransport(t *testing.T) {
	baseline, err := runQueueFixture(t, 12, true, true)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("late source fixture did not fail eager preflight: %v", err)
	}
	if len(baseline.requests) != 0 {
		t.Fatal("eager preflight transported incomplete target scope")
	}
	for _, cap := range []int{1, 2, 12} {
		t.Run(fmt.Sprint(cap), func(t *testing.T) {
			got, e := runQueueFixture(t, cap, false, true)
			if e == nil || e.Error() != err.Error() {
				t.Fatalf("cap%d changed exact first source error: %v vs%v", cap, e, err)
			}
			if len(got.requests) != 0 {
				t.Fatalf("cap%d transported %d requests before late native preflight refusal", cap, len(got.requests))
			}
			if len(got.payloads) != 0 || got.tables != "" {
				t.Fatalf("cap%d installed partial target results", cap)
			}
		})
	}
}
