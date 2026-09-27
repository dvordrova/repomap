package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A client request is any protocol's: a socket connect or a call whose verb
// is not written states no method, and the report must not make it an HTTP
// GET. It is listed without a method, joins another target's route by its
// path alone as a possible link, and the system map counts it as a request.
func TestClientRequestsStateOnlyTheMethodTheirCodeWrites(t *testing.T) {
	call := facts.Fact{ID: "call", Kind: facts.KindRegistration, TargetID: "caller", Path: "/api/items", Values: []string{"/api/items"}, Resolution: facts.ResolutionExact, Anchor: &facts.Anchor{Path: "client.c", Line: 4}}
	route := facts.Fact{ID: "route", Kind: facts.KindRegistration, TargetID: "server", Method: "POST", Path: "/api/items", Resolution: facts.ResolutionExact, Anchor: &facts.Anchor{Path: "server.go", Line: 9}}
	builder := pageBuilder{
		data: &ReportData{Facts: &facts.Result{Facts: []facts.Fact{call, route}}}, testPaths: map[string]bool{},
		factsByID: map[string]facts.Fact{"call": call, "route": route},
		sections:  []*pageSection{{programTargetID: "caller"}, {programTargetID: "server"}},
		indexes: []groupindex.Index{
			{Target: programindex.Target{ID: "caller"}, Outbound: []groupindex.OutboundCall{{ID: "x", FactID: "call", Kind: "client_request"}}},
			{Target: programindex.Target{ID: "server"}, Operations: []groupindex.Operation{{ID: "o1", FactID: "route", Kind: "request"}}},
		},
	}
	rows := builder.outboundRequests("caller", "")
	if len(rows) != 1 || rows[0].Method != "" || rows[0].Path != "/api/items" {
		t.Fatalf("a request that states no method is listed as %+v", rows)
	}
	// Nothing written says GET, so a POST route on the same path is still the
	// one it may reach; the link stays possible and shows the route's method.
	portals := builder.portalLinks()
	if len(portals) != 1 || !portals[0].possible || portals[0].method != "POST" {
		t.Fatalf("portals = %+v, want one possible link to the POST route", portals)
	}
	// A stated method still decides: GET does not reach a POST route.
	call.Method = "GET"
	builder.factsByID["call"] = call
	if portals := builder.portalLinks(); len(portals) != 0 {
		t.Fatalf("a stated GET reached a POST route: %+v", portals)
	}
	label := repoEdgeLabel(2, 0)
	if strings.Contains(label, "HTTP") {
		t.Fatalf("system map edge label %q names a protocol", label)
	}
	if translated := localizedUIValue(Russian, label); translated == label {
		t.Fatalf("system map edge label %q has no Russian", label)
	}
}
