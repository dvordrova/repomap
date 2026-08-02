package surfacebridge

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestFlowSeedDoesNotInventDownstreamProof(t *testing.T) {
	trigger := surfacediscovery.TriggerRecord{
		ID: "trigger-grounded", Kind: "http_route",
		Identity: surfacediscovery.Identity{
			Method: "POST",
			Path:   surfacediscovery.Value{Kind: "constant", Text: "/runs", Known: true, Candidates: []string{}},
		},
		ProcessEntrypoint: surfacediscovery.Symbol{
			ID: "example.com/app.main", Name: "main",
			Location: surfacediscovery.Location{Path: "main.go", Line: 10},
		},
		RegistrationSite: surfacediscovery.Location{Path: "routes.go", Line: 22},
		Handler:          surfacediscovery.Value{Kind: "function", Text: "example.com/app.create", Known: true, Candidates: []string{}},
		Dispatcher:       surfacediscovery.Value{Kind: "dispatcher", Text: "mux@main.go:11", Known: true, Candidates: []string{}},
		FinalSeed:        "net-http-servemux-handlefunc",
		Certainty:        "static",
		Resolution:       "exact",
		WrapperChain:     []surfacediscovery.Wrapper{},
	}
	seed, err := FlowSeed(trigger)
	if err != nil {
		t.Fatal(err)
	}
	if seed.ID != trigger.ID || seed.LikelyEntrypoint != "main.go" || len(seed.ValidSeedFiles) != 2 {
		t.Fatalf("seed = %#v", seed)
	}
	joined := strings.ToLower(strings.Join(seed.Evidence, " "))
	for _, invented := range []string{"core operation", "i/o boundary", "concurrency", "termination", "confidence"} {
		if strings.Contains(joined, invented) {
			t.Fatalf("adapter invented %q in %#v", invented, seed)
		}
	}
}
