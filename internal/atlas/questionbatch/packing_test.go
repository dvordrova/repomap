package questionbatch

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

func questionKeys(from, to int) []string {
	var keys []string
	for i := from; i <= to; i++ {
		keys = append(keys, "q"+strconv.Itoa(i))
	}
	return keys
}

// Twenty questions over one catalogue go out eight at a time; every group
// reads the same complete rows, so only the decisions per answer are fewer.
func TestQuestionsAreAskedEightAtATimeOverTheCompleteRows(t *testing.T) {
	input := testInput(3, 20)
	provider := &testProvider{}
	result, err := Run(t.Context(), llm.Executor{BatchConcurrency: 4}, provider, input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 3 || len(result.Exchanges) != 3 {
		t.Fatalf("20 questions need three windows of at most %d, got %d/%d", maxQuestionsPerWindow, len(provider.requests), len(result.Exchanges))
	}
	var first modelRequest
	if err := json.Unmarshal(result.Exchanges[0].Input, &first); err != nil {
		t.Fatal(err)
	}
	for i, want := range [][]string{questionKeys(1, 8), questionKeys(9, 16), questionKeys(17, 20)} {
		exchange := result.Exchanges[i]
		if !reflect.DeepEqual(exchange.QuestionRefs, want) || !reflect.DeepEqual(exchange.ChunkIndexes, []int{0, 1, 2}) || exchange.Reask {
			t.Fatalf("window %d = %v over %v", i, exchange.QuestionRefs, exchange.ChunkIndexes)
		}
		var request modelRequest
		if err := json.Unmarshal(exchange.Input, &request); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(request.Evidence, first.Evidence) || len(request.Questions) != len(want) {
			t.Fatalf("window %d changed the shared evidence or its question group", i)
		}
	}
	for q, question := range result.Questions {
		for row, chunk := range question.Chunks {
			if !chunk.Inspected || chunk.Source != atlas.SourceModel {
				t.Fatalf("question group packing lost coverage at %d/%d", q, row)
			}
		}
	}
	// Rows are cut first, questions second, so the windows over one row set
	// are adjacent and their shared prefix is reused in order.
	data, err := prepareCatalogue(input, Options{MaxRows: 2})
	if err != nil {
		t.Fatal(err)
	}
	all := make([]int, 20)
	for i := range all {
		all[i] = i
	}
	planned, err := data.plan(t.Context(), provider, window{rows: []int{0, 1, 2}, questions: all})
	if err != nil {
		t.Fatal(err)
	}
	var got [][2]int
	for _, part := range planned {
		got = append(got, [2]int{len(part.rows), len(part.questions)})
	}
	if !reflect.DeepEqual(got, [][2]int{{2, 8}, {2, 8}, {2, 4}, {1, 8}, {1, 8}, {1, 4}}) ||
		!reflect.DeepEqual(planned[0].rows, []int{0, 1}) || !reflect.DeepEqual(planned[3].rows, []int{2}) || !reflect.DeepEqual(planned[2].questions, []int{16, 17, 18, 19}) {
		t.Fatalf("row and question ceilings composed as %v", got)
	}
}

// sequencingProvider records the order in which windows start and end. The
// lead window of a row set lingers so a sibling started in parallel would be
// seen before it ends; siblings wait for each other to prove they overlap.
type sequencingProvider struct {
	*testProvider
	mu       sync.Mutex
	events   []string
	siblings atomic.Int32
	both     chan struct{}
}

func (p *sequencingProvider) record(event string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func (p *sequencingProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var wire testWire
	if err := json.Unmarshal(prepared.Bytes(), &wire); err != nil {
		return llm.Completion{}, err
	}
	var request modelRequest
	if err := json.Unmarshal([]byte(wire.Prompt.User), &request); err != nil {
		return llm.Completion{}, err
	}
	name := request.Questions[0].Key + "-" + request.Questions[len(request.Questions)-1].Key
	p.record("start " + name)
	if name == "q1-q8" {
		time.Sleep(50 * time.Millisecond)
	} else {
		if p.siblings.Add(1) == 2 {
			close(p.both)
		}
		select {
		case <-p.both:
		case <-time.After(2 * time.Second):
			p.record("timeout " + name)
		}
	}
	p.record("end " + name)
	return p.testProvider.Complete(ctx, prepared)
}

// Windows over the same rows share their request prefix; the provider serves
// it from its prompt cache only once the first of them has completed. The
// first window goes alone, its siblings follow together.
func TestFirstWindowOverARowSetCompletesBeforeItsSiblingsStart(t *testing.T) {
	provider := &sequencingProvider{testProvider: &testProvider{}, both: make(chan struct{})}
	result, err := Run(t.Context(), llm.Executor{BatchConcurrency: 4}, provider, testInput(2, 20), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Exchanges) != 3 {
		t.Fatalf("windows = %d", len(result.Exchanges))
	}
	position := func(event string) int {
		for i, got := range provider.events {
			if got == event {
				return i
			}
		}
		t.Fatalf("missing %q in %v", event, provider.events)
		return -1
	}
	leadDone := position("end q1-q8")
	if position("start q1-q8") != 0 || leadDone > position("start q9-q16") || leadDone > position("start q17-q20") {
		t.Fatalf("siblings did not wait for the lead window: %v", provider.events)
	}
	for _, event := range provider.events {
		if strings.HasPrefix(event, "timeout") {
			t.Fatalf("siblings of a completed lead window did not run in parallel: %v", provider.events)
		}
	}
	for _, exchange := range result.Exchanges {
		if exchange.Err != nil {
			t.Fatalf("ordering changed an outcome: %v", exchange.Err)
		}
	}
	// The ordering itself: one lead per row set the provider has not seen,
	// every window over already-processed rows joins the parallel wave.
	rounds := &execution{warm: map[string]bool{}}
	a, b := []int{0, 1}, []int{2}
	planned := []window{{rows: a, questions: []int{0}}, {rows: a, questions: []int{1}}, {rows: b, questions: []int{0}}, {rows: b, questions: []int{1}}}
	if waves := rounds.waves(planned); !reflect.DeepEqual(waves, [][]window{{planned[0], planned[2]}, {planned[1], planned[3]}}) {
		t.Fatalf("cold waves = %v", waves)
	}
	rounds.warm[rowsKey(a)] = true
	if waves := rounds.waves(planned); !reflect.DeepEqual(waves, [][]window{{planned[2]}, {planned[0], planned[1], planned[3]}}) {
		t.Fatalf("warm waves = %v", waves)
	}
}
