package orient

import (
	"testing"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestDeliverExternalCallIndexHandsOffIndependentSnapshot(t *testing.T) {
	want := surfacediscovery.ExternalCallIndex{
		Version: surfacediscovery.ExternalCallIndexVersion,
		Scenario: surfacediscovery.Scenario{
			ID: "scenario", GOOS: "test-os", GOARCH: "test-arch", Tags: []string{"tag"},
		},
		Packages: []surfacediscovery.ExternalCallPackage{{
			ModuleID: "module", PackagePath: "example.com/app/client",
		}},
		Callers: []surfacediscovery.DirectCallNode{{
			ID: "caller", Symbol: surfacediscovery.Symbol{EquivalentIDs: []string{"alias"}},
		}},
		Families: []surfacediscovery.ExternalCallFamily{{
			ID: "family", CallerID: "caller",
			Target:     surfacediscovery.ExternalCallTarget{PackagePath: "example.com/sdk", Receiver: "*Client", Name: "Do"},
			Dispatch:   surfacediscovery.ExternalCallStatic,
			Invocation: surfacediscovery.DirectCallSynchronous,
			Callsites:  []surfacediscovery.Location{{Path: "client/client.go", Line: 12}},
		}},
		Frontiers: []surfacediscovery.ExternalCallFrontier{{
			CallerID: "caller", DynamicInvokesExcluded: 1,
		}},
	}

	called := 0
	deliverExternalCallIndex(Options{ExternalCallIndexSink: func(index surfacediscovery.ExternalCallIndex) {
		called++
		index.Scenario.Tags[0] = "changed"
		index.Packages[0].PackagePath = "changed/package"
		index.Callers[0].Symbol.EquivalentIDs[0] = "changed-alias"
		index.Families[0].Target.Name = "Delete"
		index.Families[0].Callsites[0].Line = 99
		index.Frontiers[0].DynamicInvokesExcluded = 99
	}}, nil)
	if called != 0 {
		t.Fatalf("nil index invoked sink %d time(s)", called)
	}
	deliverExternalCallIndex(Options{ExternalCallIndexSink: func(index surfacediscovery.ExternalCallIndex) {
		called++
		index.Scenario.Tags[0] = "changed"
		index.Packages[0].PackagePath = "changed/package"
		index.Callers[0].Symbol.EquivalentIDs[0] = "changed-alias"
		index.Families[0].Target.Name = "Delete"
		index.Families[0].Callsites[0].Line = 99
		index.Frontiers[0].DynamicInvokesExcluded = 99
	}}, &want)
	if called != 1 {
		t.Fatalf("sink calls = %d, want 1", called)
	}
	if want.Scenario.Tags[0] != "tag" ||
		want.Packages[0].PackagePath != "example.com/app/client" ||
		want.Callers[0].Symbol.EquivalentIDs[0] != "alias" ||
		want.Families[0].Target.Name != "Do" || want.Families[0].Callsites[0].Line != 12 ||
		want.Frontiers[0].DynamicInvokesExcluded != 1 {
		t.Fatalf("sink mutation changed producer index: %+v", want)
	}
	deliverExternalCallIndex(Options{}, &want)
}
