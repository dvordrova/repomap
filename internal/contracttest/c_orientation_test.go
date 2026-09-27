package contracttest

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
)

// The orientation reads kvd's main by its own calls, in the order main writes
// them, from the graph an ordinary run builds: its places name each
// declaration by the object its program indexed (t1.n1), and a member of the
// group index (n1 of t1) finds its place there. Looked up by the bare member
// ID, no member found one and every request went without member evidence;
// listed in the graph's order, main's external calls came first. Redis's main
// flow then put loadServerConfig before the initServerConfig main calls first.
func TestCFixtureOrientationReadsMainsCallsInWrittenOrder(t *testing.T) {
	fixture := loadCFixture(t)
	set := buildCSet(t, fixture, "c:kvd", "c:kvcli")
	server, client := set["c:kvd"], set["c:kvcli"]
	layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: []facts.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	preset := &kvdPreset{roles: map[string]map[string]string{"sys/socket.h.connect": {"talks": "client_request"}}}
	var metas []reading.TargetMeta
	for _, index := range []programindex.Index{server, client} {
		metas = append(metas, reading.TargetMeta{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."})
	}
	read, err := reading.Read(t.Context(), reading.Options{
		Graph: graph, Repository: "kvd", Revision: "test", NoCaptions: true, Targets: metas,
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		Provider: preset, Categorizer: preset.categorizer(), OwnerRunDir: t.TempDir(),
		ReadSource: func(path string) ([]byte, error) { return fixture.source(t, path), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{server.Target.ID: server, client.Target.ID: client}, read.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	noClaims, err := claims.Seal(claims.Result{Revision: "test", Claims: []claims.Claim{}})
	if err != nil {
		t.Fatal(err)
	}
	asked := &capturedOrientation{}
	if _, _, err := orientation.Run(t.Context(), llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}}, asked,
		orientation.Input{RepositoryName: "kvd", Facts: layer, Claims: noClaims, Groups: indexes, Graph: graph}); err != nil {
		t.Fatal(err)
	}
	var request struct {
		Groups []struct {
			Members []struct {
				Ref, Name, Anchor string
			}
		}
		MemberEvidence []struct {
			Ref      string
			Evidence struct {
				Calls []struct {
					Name string
					Line int
				}
				CallsOmitted int `json:"calls_omitted"`
			}
		} `json:"member_evidence"`
	}
	if err := json.Unmarshal(asked.request(t), &request); err != nil {
		t.Fatal(err)
	}
	main := ""
	for _, group := range request.Groups {
		for _, member := range group.Members {
			if member.Name == "main" && member.Anchor == "kvd.c:302" {
				main = member.Ref
			}
		}
	}
	for _, row := range request.MemberEvidence {
		if row.Ref != main {
			continue
		}
		var names []string
		lines := make([]int, 0, len(row.Evidence.Calls))
		for _, call := range row.Evidence.Calls {
			names = append(names, call.Name)
			lines = append(lines, call.Line)
		}
		// The first six of main's fifteen calls, as written: its two
		// settings, the --symbols branch, then setupSignals.
		if want := []string{"stdlib.h.getenv", "stdlib.h.getenv", "string.h.strcmp", "printSymbols", "stdlib.h.atoi", "setupSignals"}; !slices.Equal(names, want) || !slices.IsSorted(lines) || row.Evidence.CallsOmitted != 9 {
			t.Fatalf("main's calls read %v at %v (%d more), want %v first as written", names, lines, row.Evidence.CallsOmitted, want)
		}
		return
	}
	t.Fatalf("kvd's main (%q) is advertised without its calls: %d members have evidence", main, len(request.MemberEvidence))
}

// capturedOrientation keeps the orientation request and answers with the
// legitimate empty orientation.
type capturedOrientation struct {
	mu   sync.Mutex
	user []byte
}

func (*capturedOrientation) State() []byte { return []byte(`{"provider":"captured-orientation"}`) }

func (*capturedOrientation) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (asked *capturedOrientation) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	asked.mu.Lock()
	defer asked.mu.Unlock()
	if asked.user != nil {
		return llm.Completion{}, fmt.Errorf("the orientation was asked twice")
	}
	asked.user = slices.Clone(prepared.Bytes())
	return llm.Completion{Response: []byte(`{}`), FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func (asked *capturedOrientation) request(t *testing.T) []byte {
	t.Helper()
	asked.mu.Lock()
	defer asked.mu.Unlock()
	if asked.user == nil {
		t.Fatal("the orientation was not asked")
	}
	return asked.user
}
