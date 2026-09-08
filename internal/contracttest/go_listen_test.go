package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

func assertGoListenAddresses(t *testing.T, index programindex.Index) {
	t.Helper()
	const source = "internal/storefixture/listen_addresses.go"
	result, err := facts.Build(facts.Input{Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		value string
		line  int
	}{
		"ListenIPv6Loopback": {"[::1]:8080", 6},
		"ListenIPv6Zone":     {"[fe80::1%eth0]:8080", 7},
		"ListenIPv4Control":  {"127.0.0.1:8080", 8},
		"ListenUnixControl":  {"/tmp/fixture.sock", 9},
	}
	objects := make(map[string]programindex.Object)
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	for _, row := range result.OfKind(facts.KindListenAddress) {
		if row.Anchor == nil || row.Anchor.Path != source {
			continue
		}
		expected, exists := want[row.Symbol]
		object := objects[row.ObjectID]
		if !exists || row.Value != expected.value || row.Anchor.Line != expected.line || row.Anchor.Column <= 0 || row.Resolution != facts.ResolutionExact ||
			object.Name != row.Symbol || object.Location == nil || object.Location.Path != source || row.Key != "Listen" {
			t.Fatalf("native listen fact has incorrect literal/source: %+v", row)
		}
		delete(want, row.Symbol)
	}
	if len(want) > 0 {
		t.Fatalf("native IPv6/control bind sites lost from facts: %v", want)
	}
}
