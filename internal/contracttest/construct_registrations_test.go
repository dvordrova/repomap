package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A command handed to a framework as a value of the framework's type: the
// literal beside the callable names it, the type is the symbol the reading
// stage gives a role.
func assertGoConstructRegistrations(t *testing.T, index programindex.Index) {
	t.Helper()
	result, err := facts.Build(facts.Input{Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, fact := range result.OfKind(facts.KindRegistration) {
		if fact.Text != "testing.InternalTest" {
			continue
		}
		found++
		if fact.Key != "InternalTest" || fact.Path != "" || len(fact.Values) != 1 || fact.Values[0] != "restore-state" || fact.Symbol != "restoreStateCommand" || fact.Resolution != facts.ResolutionExact || fact.Anchor == nil || fact.Anchor.Path != "internal/storefixture/listen_addresses.go" {
			t.Fatalf("constructed command registered differently: %+v", fact)
		}
	}
	if found != 1 {
		t.Fatalf("constructed command registrations = %d", found)
	}
}
