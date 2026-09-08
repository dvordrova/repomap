package facts

import (
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestBuildListenFactsKeepIPv6AndExactSources(t *testing.T) {
	s := newSynthetic(t, "go", "server", "main.go")
	s.object("main", programindex.ObjectFunction, "main", "main.go", 1, "")
	s.external("listen", "net", "Listen", programindex.ExternalAuthorityPlatform)
	want := map[int]string{}
	for i, test := range []struct {
		network, address string
		valid            bool
	}{
		{"tcp4", "127.0.0.1:8080", true},
		{"tcp6", "[::1]:8080", true},
		{"tcp6", "[fe80::1%eth0]:8080", true},
		{"tcp6", "[::1]:8080", true}, // The same value at another source is another fact.
		{"unix", "/tmp/app.sock", true},
		{"unix", "./app.sock", true},
		{"tcp6", "[::1]:0", false},
		{"tcp6", "[::1]:65536", false},
		{"tcp6", "[::1]:http", false},
		{"tcp6", "::1:8080", false},
		{"tcp6", "[::1]8080", false},
		{"tcp6", "[::1:8080", false},
	} {
		line := i + 2
		s.relate(itoa(line), programindex.RelationInvokesExternal, "main", []string{"listen"}, loc("main.go", line),
			pattern("p", programindex.PatternCall, "Listen", loc("main.go", line), nil, literal(1, test.network), literal(2, test.address)))
		if test.valid {
			want[line] = test.address
		}
	}
	s.relate("computed", programindex.RelationInvokesExternal, "main", []string{"listen"}, loc("main.go", 20),
		pattern("p", programindex.PatternCall, "Listen", loc("main.go", 20), nil, literal(1, "tcp6"), dynamic(2)))
	s.relate("template", programindex.RelationInvokesExternal, "main", []string{"listen"}, loc("main.go", 21),
		pattern("p", programindex.PatternCall, "Listen", loc("main.go", 21), nil, literal(1, "tcp6"), template(2, "[::1]:", "")))
	index := s.index()
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: index}}})
	rows := result.OfKind(KindListenAddress)
	if len(rows) != len(want) {
		t.Fatalf("listen facts = %+v, want %v", rows, want)
	}
	for _, row := range rows {
		if row.Anchor == nil || row.Anchor.Path != "main.go" || row.Anchor.Column != 1 || row.Value != want[row.Anchor.Line] ||
			row.Resolution != ResolutionExact || row.Symbol != "main" || row.ObjectID == "" || row.Key != "Listen" || row.TargetID != result.Targets[0].ID {
			t.Fatalf("listen fact lost its literal or owning source: %+v", row)
		}
		delete(want, row.Anchor.Line)
	}
	if len(want) != 0 {
		t.Fatalf("missing source-distinct listen facts: %v", want)
	}
}
