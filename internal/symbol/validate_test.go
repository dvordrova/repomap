package symbol

import "testing"

func TestBundleValidateAcceptsBuiltBundle(t *testing.T) {
	t.Parallel()

	bundle, err := Build(testGraph(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestBundleValidateRejectsContractDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Bundle)
	}{
		{name: "query mismatch", mutate: func(bundle *Bundle) { bundle.Query = "other" }},
		{name: "invented allowed path", mutate: func(bundle *Bundle) { bundle.AllowedPaths = append(bundle.AllowedPaths, "invented.go") }},
		{name: "absolute path", mutate: func(bundle *Bundle) { bundle.Target.Entity.Location.Path = "/tmp/escape.go" }},
		{name: "duplicate evidence", mutate: func(bundle *Bundle) { bundle.Candidates[0].EvidenceID = bundle.Target.EvidenceID }},
		{name: "unknown scenario", mutate: func(bundle *Bundle) { bundle.Target.Scenarios = []string{"other-build"} }},
		{name: "possible call", mutate: func(bundle *Bundle) { bundle.OutgoingCalls[0].Certainty = "possible" }},
		{name: "missing callee location", mutate: func(bundle *Bundle) { bundle.OutgoingCalls[0].Callee.Location = nil }},
		{name: "contradictory target endpoint", mutate: func(bundle *Bundle) { bundle.OutgoingCalls[0].Caller.Name = "other" }},
		{name: "absolute provenance path", mutate: func(bundle *Bundle) { bundle.Target.Provenance[0].Location.Path = "/tmp/escape.go" }},
		{name: "free provenance detail", mutate: func(bundle *Bundle) { bundle.Target.Provenance[0].Detail = "token=must-not-leak" }},
		{name: "raw analyzer warning", mutate: func(bundle *Bundle) { bundle.Warnings = append(bundle.Warnings, "gopls failed at /private/repo") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, err := Build(testGraph(), Options{})
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&bundle)
			if err := bundle.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}
