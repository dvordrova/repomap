package orient

import (
	"testing"

	"github.com/dvordrova/repomap/internal/entrycall"
)

func TestDeliverEntryCallSubstrateHandsOffIndependentSnapshot(t *testing.T) {
	want := entrycall.Substrate{
		Version: entrycall.SubstrateVersion, State: entrycall.StateReady,
		Roots: []entrycall.ExactRoot{{NodeID: "root"}},
		Nodes: []entrycall.ExactNode{{ID: "root", Label: "main · main", Declaration: entrycall.Location{Path: "main.go", Line: 1}}},
		Families: []entrycall.ExactFamily{{
			ID: "family", CallerID: "root", CalleeID: "root",
			Invocation: entrycall.InvocationSynchronous, WitnessCount: 1,
			Callsites: []entrycall.Location{{Path: "main.go", Line: 1}},
		}},
	}
	called := 0
	deliverEntryCallSubstrate(Options{EntryCallSubstrateSink: func(substrate entrycall.Substrate) {
		called++
		substrate.Roots[0].NodeID = "changed"
		substrate.Nodes[0].Label = "changed"
		substrate.Families[0].Callsites[0].Line = 99
	}}, nil)
	if called != 0 {
		t.Fatalf("nil substrate invoked sink %d time(s)", called)
	}
	deliverEntryCallSubstrate(Options{EntryCallSubstrateSink: func(substrate entrycall.Substrate) {
		called++
		substrate.Roots[0].NodeID = "changed"
		substrate.Nodes[0].Label = "changed"
		substrate.Families[0].Callsites[0].Line = 99
	}}, &want)
	if called != 1 {
		t.Fatalf("sink calls = %d, want 1", called)
	}
	if want.Roots[0].NodeID != "root" || want.Nodes[0].Label != "main · main" ||
		want.Families[0].Callsites[0].Line != 1 {
		t.Fatalf("sink mutation changed producer substrate: %+v", want)
	}
	deliverEntryCallSubstrate(Options{}, &want)
}
