package reading

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// slowProvider answers like tableProvider, holding back the requests of the
// tables slow names: so a chain of stages can be made to finish last.
type slowProvider struct {
	*tableProvider
	slow  map[string]bool
	delay time.Duration
}

func (p slowProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Table string `json:"table"`
		Task  string `json:"task"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	if p.slow[request.Table] || p.slow[request.Task] {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return llm.Completion{}, ctx.Err()
		}
	}
	return p.tableProvider.Complete(ctx, prepared)
}

// withEntryCallingOutside binds the route of svc/api/h.go to its F, which
// calls Op01 of svc/core/c.go, which calls net/http.Get: an outside symbol
// for the api table, an outgoing boundary and a way for the layers table.
func withEntryCallingOutside(t *testing.T, graph atlas.Graph) atlas.Graph {
	t.Helper()
	symbol := func(path, name string) int {
		for i, place := range graph.Places {
			if place.Symbol != nil && place.Path == path && place.Symbol.Decl.Name == name {
				return i
			}
		}
		t.Fatalf("missing %s %s", path, name)
		return -1
	}
	entry, op := symbol("svc/api/h.go", "F"), symbol("svc/core/c.go", "Op01")
	graph.Places[entry].Symbol.Calls = []atlas.SymbolCall{{Name: "Op01", Kind: "calls", Line: 4, Column: 2, CalleeIDs: []string{graph.Places[op].ID}}}
	graph.Places[op].Symbol.Calls = []atlas.SymbolCall{{Name: "http.Get", Kind: "invokes_external", Line: 12, Column: 5, API: &atlas.CallAPI{Package: "net/http", Name: "Get"}}}
	for i, place := range graph.Places {
		if place.Boundary != nil && place.Path == "svc/api/h.go" {
			boundary := *place.Boundary
			boundary.ObjectID = graph.Places[entry].Symbol.Decl.ObjectID
			graph.Places[i].Boundary = &boundary
		}
	}
	encoded, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := atlas.DecodeGraph(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

// forkedReading reads two targets with symbol candidates, an outside symbol,
// boundaries, a way from an entry to an outgoing call and drawn parts in
// both targets, refusing rows of the symbols and the boundaries.
func forkedReading(t *testing.T, slow map[string]bool, delay time.Duration) (Options, *tableProvider) {
	t.Helper()
	provider := &tableProvider{
		refuse:  map[string]bool{"svc/core/c.go": true, "svc/db/d.go": true},
		areaFor: func(map[string]any) string { return "Everything" },
	}
	opts := twoTargetOptions(t, withEntryCallingOutside(t, withSymbols(t, twoTargetGraph(t))), provider)
	opts.Provider = slowProvider{tableProvider: provider, slow: slow, delay: delay}
	// The outside symbol talks to another system without serving anything.
	opts.Categorizer = closedDecisionsWith(map[string]llm.Verdict{"talks": typesafetest.Choose(atlas.BoundaryClientRequest)})
	return opts, provider
}

var tableLatency = regexp.MustCompile(`(?m) · [0-9.]+[a-zµ]+$`)

// The symbols, the boundaries and the zones run at once, yet whichever
// finishes last, the reading prints, remembers, rejects and folds exactly
// the same, with tables.md in step order.
func TestConcurrentStagesKeepStepOrder(t *testing.T) {
	type outcome struct {
		tables, knowledge, atlas, rejected string
	}
	read := func(slow ...string) outcome {
		set := map[string]bool{}
		for _, name := range slow {
			set[name] = true
		}
		opts, _ := forkedReading(t, set, 150*time.Millisecond)
		result, err := Read(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
		tables, err := os.ReadFile(result.TablesPath)
		if err != nil {
			t.Fatal(err)
		}
		knowledge, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, KnowledgeFilename))
		if err != nil {
			t.Fatal(err)
		}
		folded, _ := json.Marshal(result.Atlas)
		rejected, _ := json.Marshal(result.Rejected)
		return outcome{tableLatency.ReplaceAllString(string(tables), ""), string(knowledge), string(folded), string(rejected)}
	}
	symbolsLast := read(lines.StageSymbols)
	zonesLast := read(designPartsTask, designDescribeTask, lines.StagePlacement)
	boundariesLast := read(lines.StageBoundaries)
	for name, other := range map[string]outcome{"zones last": zonesLast, "boundaries last": boundariesLast} {
		if other.tables != symbolsLast.tables {
			t.Errorf("%s: tables.md differs", name)
		}
		if other.knowledge != symbolsLast.knowledge {
			t.Errorf("%s: knowledge.json differs", name)
		}
		if other.atlas != symbolsLast.atlas {
			t.Errorf("%s: atlas differs", name)
		}
		if other.rejected != symbolsLast.rejected {
			t.Errorf("%s: rejected rows differ:\n%s\n%s", name, other.rejected, symbolsLast.rejected)
		}
	}
	var order []string
	for _, phase := range readingPhases {
		for _, chain := range phase {
			order = append(order, chain...)
		}
	}
	position := func(stage string) int {
		switch stage {
		case lines.StagePublish:
			stage = lines.StageBoundaries
		case lines.StagePlacement, lines.StageDescribe, lines.StageRoleGate, lines.StageRoleBoxes, lines.StageRoleAssign, lines.StageRoleNeighbours:
			stage = lines.StageZones
		}
		return slices.Index(order, stage)
	}
	last, seen := -1, map[string]bool{}
	for _, line := range strings.Split(symbolsLast.tables, "\n") {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		stage := strings.Fields(line)[1]
		seen[stage] = true
		if at := position(stage); at < last {
			t.Fatalf("tables.md prints %s after a later stage", stage)
		} else {
			last = at
		}
	}
	for _, stage := range []string{lines.StageSymbols, lines.StageAPI, lines.StageBoundaries, lines.StageLayers, lines.StageZones, lines.StageDescribe, lines.StageAreas, lines.StageCore} {
		if !seen[stage] {
			t.Fatalf("the reading printed no %s window; seen %v", stage, seen)
		}
	}
	for _, stage := range []string{lines.StageSymbols, lines.StageBoundaries} {
		if !strings.Contains(symbolsLast.rejected, `"stage":"`+stage+`"`) {
			t.Fatalf("no %s row was refused: %s", stage, symbolsLast.rejected)
		}
	}
	// What each concurrent stage decided reaches the folded atlas.
	var folded atlas.Atlas
	if err := json.Unmarshal([]byte(symbolsLast.atlas), &folded); err != nil {
		t.Fatal(err)
	}
	roles, outgoing, zones := 0, 0, 0
	for _, target := range folded.Targets {
		zones += len(target.Zones)
		for _, box := range target.Boxes {
			for _, file := range box.Files {
				for _, symbol := range file.Symbols {
					if symbol.Role != "" {
						roles++
					}
				}
			}
		}
		for _, boundary := range target.Boundaries {
			if boundary.Kind == atlas.BoundaryClientRequest && boundary.Path == "svc/core/c.go" {
				outgoing++
			}
		}
	}
	if len(folded.API) == 0 || roles == 0 || outgoing == 0 || zones == 0 {
		t.Fatalf("a concurrent stage's decisions are missing: api %d, layer roles %d, interpreted outgoing %d, zones %d", len(folded.API), roles, outgoing, zones)
	}
}

// The selection runs beside the other stages, yet a key it chose is still
// Learn evidence carrying its selection record.
func TestSelectedKeysReachLearn(t *testing.T) {
	opts, provider := forkedReading(t, nil, 0)
	opts.Learn = true
	provider.learningFor = func(pool learningRequest) learningResponse {
		reply := learningReply()
		for i := range reply.Reviews {
			reply.Reviews[i].State, reply.Reviews[i].Questions = "unknown", nil
		}
		var refs []string
		for _, item := range pool.Evidence {
			refs = append(refs, item.Ref)
		}
		reply.Reviews[0].State = "questions"
		reply.Reviews[0].Questions = []learningProposal{{Question: "Which operations does the service run?", Why: "They are its work.", Sources: refs}}
		return reply
	}
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.Learning == nil {
		t.Fatal("no learning plan")
	}
	for _, question := range result.Learning.Questions {
		for _, origin := range question.Origins {
			for _, source := range origin.Sources {
				if strings.HasPrefix(source.Name, "Op") && len(source.KnowledgeIDs) > 0 {
					return
				}
			}
		}
	}
	t.Fatalf("no selected key reached Learn with its selection: %+v", result.Learning.Questions)
}

// A stage that fails stops the stages running beside it, and the reading
// reports that failure, not the cancellations it caused.
func TestFailingStageStopsItsNeighboursAndIsReported(t *testing.T) {
	opts, _ := forkedReading(t, map[string]bool{lines.StageSymbols: true, designPartsTask: true}, time.Minute)
	blocked := filepath.Join(opts.OwnerRunDir, atlas.TablesDir, lines.StageBoundaries+"-r1-w0.prompt.ref.json")
	if err := os.MkdirAll(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err := Read(t.Context(), opts)
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || !strings.Contains(err.Error(), filepath.Base(blocked)) {
		t.Fatalf("reported %v, want the failed boundaries window", err)
	}
	if waited := time.Since(started); waited > 30*time.Second {
		t.Fatalf("the reading waited %s for stages it should have stopped", waited)
	}
}

// Through stops the walk at its stage: stages beside it in the same phase
// that come after it in step order are not run.
func TestThroughStopsInsideConcurrentStages(t *testing.T) {
	for through, want := range map[string][]string{
		lines.StageSymbols:    {lines.StageSymbols},
		lines.StageBoundaries: {lines.StageSymbols, lines.StageAPI, lines.StageBoundaries},
		lines.StageZones:      {lines.StageSymbols, lines.StageAPI, lines.StageBoundaries, lines.StageLayers, lines.StageZones},
	} {
		opts, _ := forkedReading(t, nil, 0)
		opts.Through = through
		result, err := Read(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
		if result.Complete || result.Through != through {
			t.Fatalf("through %s: complete=%v through=%s", through, result.Complete, result.Through)
		}
		ran := map[string]bool{}
		for _, use := range result.Uses {
			ran[use.Stage] = true
		}
		for _, stage := range []string{lines.StageSymbols, lines.StageAPI, lines.StageBoundaries, lines.StageLayers, lines.StageZones, lines.StageArrows} {
			if ran[stage] != slices.Contains(want, stage) {
				t.Fatalf("through %s: ran %v", through, ran)
			}
		}
	}
}
