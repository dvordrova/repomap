package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestSystemMapKeepsInventoryAndExactCrossComponentDestinations(t *testing.T) {
	native := pageMapNode{ID: "backend-part", Href: "#backend-code", FullTitle: "Validation"}
	view := pageView{Sections: []*pageSection{
		{ID: "front", ShortLabel: "front", Map: &pageMap{Nodes: []pageMapNode{{ID: "front-part", FullTitle: "Form"}, {ID: "remote", Remote: true, Href: "#backend-code", FullTitle: "Validation"}}, Edges: []pageMapEdge{{From: "front-part", To: "remote", Scope: "structure", Label: "POST /run", FromSource: pageAnchor{Text: "client.ts:9"}, ToSource: pageAnchor{Text: "server.py:18"}}}}},
		{ID: "backend", ShortLabel: "backend", Map: &pageMap{Nodes: []pageMapNode{native}}, Outbound: []pageOutbound{{ID: "out-1", Destination: "Queue", MapGroup: "absent"}, {ID: "out-2", Destination: "Queue"}}},
		{ID: "worker", ShortLabel: "worker", Activities: []pageGroupOperation{{Name: "Scheduled task", Kind: "scheduled", Href: "#task"}}},
	}, RepoMap: &pageRepoMap{Nodes: []pageRepoNode{{FullName: "failed", Note: "Could not analyze"}, {FullName: "failed2", Note: "No compiler"}}}}
	got := view.SystemMap()
	nodes := map[string]pageMapNode{}
	for _, n := range got.Nodes {
		if _, exists := nodes[n.ID]; exists {
			t.Fatalf("duplicate %s", n.ID)
		}
		nodes[n.ID] = n
	}
	if _, ok := nodes["remote"]; ok {
		t.Fatal("remote copy survived exact canonical join")
	}
	if !slices.Contains(nodes["backend-part"].Aliases, "remote") {
		t.Fatal("old deep link lost")
	}
	if len(got.Edges) != 1 || got.Edges[0].To != "backend-part" || got.Edges[0].FromSource.Text != "client.ts:9" || got.Edges[0].ToSource.Text != "server.py:18" {
		t.Fatalf("original connection lost: %+v", got.Edges)
	}
	if nodes["task"].Activation != "scheduled" {
		t.Fatal("ungrouped operation disappeared")
	}
	if nodes["system-out-1"].FullTitle != "Queue" || nodes["system-out-2"].FullTitle != "Queue" {
		t.Fatal("equal destination names merged unrelated records")
	}
	for _, id := range []string{"system-component-front", "system-component-backend", "system-component-worker", "system-unread-0", "system-unread-1"} {
		if _, ok := nodes[id]; !ok {
			t.Fatalf("component %s absent", id)
		}
	}
	if nodes["system-inputs-worker"].Children != "task" || nodes["system-inputs-worker"].Owner != "worker" || strings.Contains(nodes["system-component-worker"].Children, "task") {
		t.Fatal("input lost its owning catalogue or remained inside the component")
	}
	if view.Sections[0].Map.Edges[0].To != "remote" {
		t.Fatal("display mutated the original component view")
	}
}

func TestSystemMapKeepsEqualTargetLocalGroupIDsInTheirOwnComponents(t *testing.T) {
	api := &pageMap{Nodes: []pageMapNode{{ID: mapNodeID("g1"), FullTitle: "API wiring"}}}
	cli := &pageMap{Nodes: []pageMapNode{{ID: mapNodeID("g1"), FullTitle: "CLI wiring"}}}
	scopeTargetMapIDs(api, "t1")
	scopeTargetMapIDs(cli, "t2")

	view := pageView{Sections: []*pageSection{
		{ID: "api", programTargetID: "t1", ShortLabel: "cmd/api", Map: api},
		{ID: "cli", programTargetID: "t2", ShortLabel: "cmd/users", Map: cli},
	}}
	got := view.SystemMap()
	nodes := make(map[string]pageMapNode, len(got.Nodes))
	for _, node := range got.Nodes {
		if _, duplicate := nodes[node.ID]; duplicate {
			t.Fatalf("duplicate system-map node %q", node.ID)
		}
		nodes[node.ID] = node
	}
	apiGroup := targetMapNodeID("t1", mapNodeID("g1"))
	cliGroup := targetMapNodeID("t2", mapNodeID("g1"))
	if nodes[apiGroup].FullTitle != "API wiring" || nodes[cliGroup].FullTitle != "CLI wiring" {
		t.Fatalf("target-local groups collapsed: api=%+v cli=%+v", nodes[apiGroup], nodes[cliGroup])
	}
	if nodes["system-component-api"].Children != apiGroup || nodes["system-component-cli"].Children != cliGroup {
		t.Fatalf("component membership crossed targets: api=%q cli=%q", nodes["system-component-api"].Children, nodes["system-component-cli"].Children)
	}
}

func TestSystemMapDrawsOneArrowPerDirectedNodePair(t *testing.T) {
	view := pageView{Sections: []*pageSection{{
		ID: "api", ShortLabel: "api",
		Map: &pageMap{
			Nodes: []pageMapNode{{ID: "handler"}, {ID: "service"}, {ID: "o1", Activation: "request"}, {ID: "o2", Activation: "command"}},
			Edges: []pageMapEdge{
				{ConnectionID: "x1", From: "handler", To: "service", Scope: "operation", Operations: "o1", Label: "calls", Possible: true, FromSource: pageAnchor{Text: "handler.go:10"}},
				{ConnectionID: "x2", From: "handler", To: "service", Scope: "structure", Operations: "o2 o1", Label: "passes callback", Summary: "Handler supplies the service method.", Possible: false, FromSource: pageAnchor{Text: "main.go:8"}},
			},
		},
	}}}
	got := view.SystemMap()
	if len(got.Edges) != 1 {
		t.Fatalf("same directed pair produced %d physical arrows: %+v", len(got.Edges), got.Edges)
	}
	edge := got.Edges[0]
	if edge.From != "handler" || edge.To != "service" || edge.Scope != "operation" || edge.Operations != "o1 o2" ||
		edge.Label != "calls · passes callback" || edge.Summary != "Handler supplies the service method." || edge.Possible ||
		edge.ConnectionID != "" || edge.FromSource != (pageAnchor{}) {
		t.Fatalf("collapsed arrow lost its combined meaning: %+v", edge)
	}
}

func TestPageHydratesEqualTargetLocalSubjectIDsFromTheirOwner(t *testing.T) {
	apiProgram, api := reportCategorizedGroupFixtureAt(t, "APIHandler", "go:./cmd/api", "cmd/api/main.go", []programindex.Category{programindex.CategoryCore}, groupindex.LaneCore, "t1")
	cliProgram, cli := reportCategorizedGroupFixtureAt(t, "CLICommand", "go:./cmd/users", "cmd/users/main.go", []programindex.Category{programindex.CategoryCore}, groupindex.LaneCore, "t2")
	if api.Subjects[0].ID != cli.Subjects[0].ID {
		t.Fatalf("fixture did not exercise target-local collision: %q != %q", api.Subjects[0].ID, cli.Subjects[0].ID)
	}
	graph, err := NewGroupGraphView([]groupindex.Index{api, cli}, api.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := &ReportData{
		FormatVersion: CurrentFormatVersion, RepoName: "two-targets", CapturedRevision: strings.Repeat("a", 40), GroupGraph: graph,
		TargetOutcomePortfolio: reportTargetOutcomeViewFixture(t, []TargetNavigationPage{
			{RunID: "api-run", ProgramTarget: apiProgram.Target.Snapshot(), ArtifactFilename: programindex.ArtifactFilename},
			{RunID: "cli-run", ProgramTarget: cliProgram.Target.Snapshot(), ArtifactFilename: programindex.ArtifactFilename},
		}, api.Target.ID),
	}
	view, err := buildPageView(data, strings.Repeat("b", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Sections) != 2 || len(view.Sections[0].Core) != 1 || len(view.Sections[1].Core) != 1 {
		t.Fatalf("unexpected sections: %#v", view.Sections)
	}
	if view.Sections[0].ID != "t1" || view.Sections[1].ID != "t2" {
		t.Fatalf("sections received a second identity namespace: %q, %q", view.Sections[0].ID, view.Sections[1].ID)
	}
	for position, want := range []struct{ name, path string }{{"APIHandler", "cmd/api/main.go"}, {"CLICommand", "cmd/users/main.go"}} {
		rows := view.Sections[position].Core[0].Inventory
		if len(rows) != 1 || rows[0].Path != want.path || len(rows[0].Members) != 1 || rows[0].Members[0].Name != want.name {
			t.Fatalf("section %d borrowed another target's subject: %#v", position, rows)
		}
	}
}

func TestSystemInputCataloguesKeepEveryKindOutsideItsActualComponent(t *testing.T) {
	command := pageMapNode{ID: "command", Activation: "command", FullTitle: "Run", InputOwner: "part", SourceKind: "model", Source: pageAnchor{Text: "main.go:12"}, SummaryRef: "run-summary"}
	section := &pageSection{ID: "first", ShortLabel: "Same name", InputsCount: 6, InboundCount: 1,
		Map: &pageMap{Nodes: []pageMapNode{
			{ID: "area", Branch: "area", Children: "part command"}, {ID: "part", FullTitle: "Implementation"}, command,
			{ID: "click", Activation: "interaction"}, {ID: "scheduled", Activation: "scheduled"},
			{ID: "worker", Activation: "continuous"}, {ID: "handler", Activation: "request"},
		}, Edges: []pageMapEdge{{From: "command", To: "part", Label: "implemented in", Operations: "command", Scope: "operation", FromSource: command.Source}}},
		RouteGroups: []pageRouteGroup{{Method: "GET", Rows: []pageRouteRow{{Paths: []pageRoutePath{{Path: "/unmatched", Anchor: &pageAnchor{Href: "server.py#L3", Text: "server.py:3"}}}}}}},
		Activities:  []pageGroupOperation{{Href: "#command", Name: "Run", Kind: "command"}},
		Outbound:    []pageOutbound{{ID: "send", Destination: "Queue", KindLabel: "Request"}},
	}
	view := pageView{Sections: []*pageSection{section,
		{ID: "second", ShortLabel: "Same name", Map: &pageMap{Nodes: []pageMapNode{{ID: "other-command", Activation: "command", FullTitle: "Run"}}}},
		{ID: "empty", ShortLabel: "No inputs", Outbound: []pageOutbound{{ID: "only-outbound", Destination: "Queue"}}},
	}}
	before, err := json.Marshal(view.Sections)
	if err != nil {
		t.Fatal(err)
	}
	got := view.SystemMap()
	nodes, parents := map[string]pageMapNode{}, map[string]string{}
	for _, n := range got.Nodes {
		nodes[n.ID] = n
		for _, child := range strings.Fields(n.Children) {
			if parent := parents[child]; parent != "" {
				t.Fatalf("%s duplicated inside %s and %s", child, parent, n.ID)
			}
			parents[child] = n.ID
		}
	}
	for _, owner := range []string{"first", "second"} {
		id := "system-inputs-" + owner
		group := nodes[id]
		if group.Branch != "inputs" || group.ItemKind != "Inputs" || group.FullTitle != "Same name" || group.Owner != owner || parents[id] != "" {
			t.Fatalf("input catalogue is not one root for its actual component: %+v", group)
		}
		if group.Href != "#"+owner+"-inbound" || group.DetailsID != owner+"-inbound" {
			t.Fatalf("catalogue does not target its existing input section: %+v", group)
		}
	}
	var native pageMapNode
	for _, n := range got.Nodes {
		if n.Activation != "" && parents[n.ID] != "system-inputs-"+n.Owner {
			t.Fatalf("input %s left outside its owning catalogue: %+v", n.ID, n)
		}
		if n.FullTitle == "GET /unmatched" {
			native = n
		}
	}
	if native.ID == "" || native.InputOwner != "" || len(strings.Fields(nodes["system-inputs-first"].Children)) != 6 {
		t.Fatal("unmatched native route disappeared or gained an invented implementation")
	}
	if _, ok := nodes["system-inputs-empty"]; ok {
		t.Fatal("outbound communication manufactured an input catalogue")
	}
	if parents["system-send"] == "system-inputs-first" || nodes["area"].Children != "part" {
		t.Fatal("outbound entered the input catalogue or an input stayed in its old frame")
	}
	if len(got.Edges) != 1 || got.Edges[0].From != "command" || got.Edges[0].To != "part" || got.Edges[0].FromSource.Text != "main.go:12" {
		t.Fatalf("grouping changed or invented runtime connections: %+v", got.Edges)
	}
	if nodes["command"].InputOwner != command.InputOwner || nodes["command"].SummaryRef != command.SummaryRef || nodes["command"].Source != command.Source {
		t.Fatal("grouping changed the original input's implementation or source")
	}
	after, _ := json.Marshal(view.Sections)
	if !bytes.Equal(before, after) {
		t.Fatal("display grouping mutated the original component views")
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := parsed.ExecuteTemplate(&rendered, "input-catalog", section); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), `id="`+nodes["system-inputs-first"].DetailsID+`"`) {
		t.Fatal("input catalogue details point to a nonexistent HTML anchor")
	}
}

func TestSystemMapKeepsConnectionsToUnreadComponents(t *testing.T) {
	view := pageView{Sections: []*pageSection{{ID: "front", ShortLabel: "front"}}, RepoMap: &pageRepoMap{
		Nodes: []pageRepoNode{{ID: "source", Href: "#front", Analyzed: true}, {ID: "failed", FullName: "worker", Note: "No compiler"}},
		Edges: []pageRepoEdge{{From: "source", To: "failed", Label: "uses", Possible: true}},
	}}
	got := view.SystemMap()
	if len(got.Edges) != 1 || got.Edges[0].From != "system-component-front" || got.Edges[0].To != "system-unread-1" || !got.Edges[0].Possible {
		t.Fatalf("unread component lost its original connection: %+v", got.Edges)
	}
}

func TestSystemPathsContinueThroughExactInputsWithoutBorrowingSiblingPaths(t *testing.T) {
	view := &pageMap{Nodes: []pageMapNode{{ID: "click", Activation: "interaction"}, {ID: "post", Activation: "request"}, {ID: "get", Activation: "request"}, {ID: "timer", Activation: "scheduled"}}, Edges: []pageMapEdge{
		{From: "ui", To: "post", Operations: "click", Possible: true},
		{From: "post", To: "api", Operations: "post"},
		{From: "api", To: "validation", Operations: "post"},
		{From: "get", To: "api", Operations: "get"},
		{From: "api", To: "read-only", Operations: "get"},
		{From: "validation", To: "timer"}, // Neighbour only: never a reached input.
		{From: "validation", To: "queue", Operations: "post"},
		{From: "queue", To: "click", Operations: "post", Possible: true}, // Cycle.
	}}
	completeSystemPaths(view)
	for _, at := range []int{0, 1, 2, 6, 7} {
		if !slices.Contains(strings.Fields(view.Edges[at].Operations), "click") {
			t.Fatalf("click path stopped before edge %d", at)
		}
	}
	for _, at := range []int{3, 4, 5} {
		if slices.Contains(strings.Fields(view.Edges[at].Operations), "click") {
			t.Fatalf("click borrowed unrelated sibling edge %d", at)
		}
	}
	if !view.Edges[0].Possible || view.Edges[2].Possible {
		t.Fatal("joining paths changed the authority of the original observations")
	}
	if !slices.Contains(strings.Fields(view.Nodes[0].Neighbours), "queue") {
		t.Fatal("continued path absent from the input's participant inventory")
	}
}

func TestSystemPathWitnessRetainsBothSidesAndTheIntegrationUncertainty(t *testing.T) {
	view := &pageMap{Nodes: []pageMapNode{
		{ID: "click", Activation: "interaction", CallPaths: `{"post":[{"name":"handleClick","source":"ui.ts:10","href":"ui.ts#L10"},{"name":"send","source":"http.ts:20","href":"http.ts#L20"}]}`},
		{ID: "post", Activation: "request", CallPaths: `{"validation":[{"name":"handler","source":"api.py:30","href":"api.py#L30"},{"name":"validate","source":"check.py:40","href":"check.py#L40","possible":true}]}`},
	}, Edges: []pageMapEdge{{From: "ui", To: "post", Operations: "click", Possible: true}, {From: "api", To: "validation", Operations: "post"}}}
	completeSystemPaths(view)
	var paths map[string][]pageCallStep
	if err := json.Unmarshal([]byte(view.Nodes[0].CallPaths), &paths); err != nil {
		t.Fatal(err)
	}
	witness := paths["validation"]
	if len(witness) != 4 || witness[0].Name != "handleClick" || witness[3].Name != "validate" || !witness[2].Possible || !witness[3].Possible || witness[0].Possible || witness[1].Possible {
		t.Fatalf("incomplete or misattributed cross-component proof: %+v", witness)
	}
	for _, step := range witness {
		if step.Href == "" {
			t.Fatalf("source link lost: %+v", step)
		}
	}
}

func TestSystemOutboundGroupingRetainsRecordsAndTheirExactPeerInputs(t *testing.T) {
	view := pageView{Sections: []*pageSection{
		{ID: "front", Map: &pageMap{Nodes: []pageMapNode{
			{ID: "click", Activation: "interaction"},
			{ID: "n-http", FullTitle: "Client"}, {ID: "remote-post", Remote: true, Href: "#post", Activation: "request"},
		}, Edges: []pageMapEdge{{ConnectionID: "post-match", From: "n-http", To: "remote-post", Scope: "operation", Operations: "click", Possible: true}}}, Outbound: []pageOutbound{
			{ID: "get", Destination: "backend API", Method: "GET", Address: "/items", NativeLabel: "GET /items", Source: "fact", Anchor: pageAnchor{Text: "http.ts:10"}},
			{ID: "post", Destination: "backend API", Method: "POST", Address: "/items", NativeLabel: "POST /items", Connections: []string{"post-match"}, Source: "fact", Anchor: pageAnchor{Text: "http.ts:20"}},
			{ID: "unmatched", Destination: "backend API", Method: "GET", Address: "/unknown", NativeLabel: "GET /unknown", Source: "fact"},
		}},
		{ID: "backend", Map: &pageMap{Nodes: []pageMapNode{{ID: "post", Activation: "request", FullTitle: "POST /items"}}}},
		{ID: "other", Outbound: []pageOutbound{{ID: "other-get", Destination: "backend API", NativeLabel: "GET /items"}}},
	}}
	got := view.SystemMap()
	nodes := map[string]pageMapNode{}
	for _, n := range got.Nodes {
		nodes[n.ID] = n
	}
	// One frame per destination a component's records name; each record
	// keeps its own tile and component. Another component naming the same
	// destination keeps its own frame, beside this one in a display group.
	group := nodes["system-get-destination"]
	if group.Branch != "communication" || group.FullTitle != "backend API" || group.Children != "system-get system-unmatched" || group.Owner != "front" {
		t.Fatalf("external catalogue grouping lost: %+v", group)
	}
	other := nodes["system-other-get-destination"]
	if other.Owner != "other" || other.Children != "system-other-get" || nodes["system-other-get"].Owner != "other" {
		t.Fatalf("another component's records joined this one's destination: %+v", other)
	}
	if group.DisplayGroup == "" || group.DisplayGroup != other.DisplayGroup {
		t.Fatalf("frames naming one destination do not stand together: %q %q", group.DisplayGroup, other.DisplayGroup)
	}
	if _, exists := nodes["system-post"]; exists {
		t.Fatal("known backend input was drawn as another external participant")
	}
	if len(got.Edges) != 1 || got.Edges[0].From != "n-http" || got.Edges[0].To != "post" || !got.Edges[0].Possible || got.Edges[0].Operations != "click" || got.Edges[0].FromSource.Text != "http.ts:20" {
		t.Fatalf("communication gained a peer by name or lost its exact connection: %+v", got.Edges)
	}
	if view.Sections[0].Map.Edges[0].From != "n-http" {
		t.Fatal("display changed the saved component map")
	}
}

func TestSystemMatchedOutboundKeepsReadingWithoutAnotherParticipant(t *testing.T) {
	for _, endpoint := range []string{"input", "target"} {
		t.Run(endpoint, func(t *testing.T) {
			peer := pageMapNode{ID: "post", Activation: "request", FullTitle: "Run", Source: pageAnchor{Text: "server.py:30", Href: "server.py#L30"}}
			peerHref, peerID := "#post", peer.ID
			if endpoint == "target" {
				peerHref, peerID = "#backend", "system-component-backend"
			}
			row := pageOutbound{ID: "send", Destination: "Run service", Summary: "Submits a run to the backend.", KindLabel: outboundKindLabel("client_request"),
				Connections: []string{"run-match"}, Method: "POST", Address: "/run", Source: "fact",
				Anchor: pageAnchor{Text: "http.ts:20", Href: "http.ts#L20"}, MapGroup: "http"}
			view := pageView{Sections: []*pageSection{
				{ID: "front", Outbound: []pageOutbound{row}, Map: &pageMap{Nodes: []pageMapNode{
					{ID: "click", Activation: "interaction"}, {ID: "n-http", FullTitle: "Client"}, {ID: "remote", Remote: true, Href: peerHref},
				}, Edges: []pageMapEdge{{ConnectionID: "run-match", From: "n-http", To: "remote", Scope: "operation", Operations: "click", Possible: true, ToSource: peer.Source}}}},
				{ID: "backend", Map: &pageMap{Nodes: []pageMapNode{peer}}},
			}}
			before, _ := json.Marshal(view.Sections)
			got := view.SystemMap()
			for _, node := range got.Nodes {
				if node.ItemKind == "External communication" {
					t.Fatalf("matched participant still has an external copy: %+v", node)
				}
			}
			if len(got.Edges) != 1 || got.Edges[0].From != "n-http" || got.Edges[0].To != peerID ||
				got.Edges[0].ConnectionID != "run-match" || got.Edges[0].Operations != "click" || !got.Edges[0].Possible ||
				got.Edges[0].FromSource != row.Anchor || got.Edges[0].ToSource != peer.Source {
				t.Fatalf("direct integration lost its exact endpoint, input path or sources: %+v", got.Edges)
			}
			after, _ := json.Marshal(view.Sections)
			if !bytes.Equal(before, after) {
				t.Fatal("canvas folding changed the saved outbound reading or component map")
			}
			parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
			if err != nil {
				t.Fatal(err)
			}
			var html bytes.Buffer
			if err := parsed.ExecuteTemplate(&html, "outbound-row", view.Sections[0].Outbound[0]); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(html.String(), `id="send"`) || !strings.Contains(html.String(), row.Summary) || !strings.Contains(html.String(), row.Anchor.Href) {
				t.Fatal("matched outbound record lost its original reading and source link")
			}
		})
	}
}

func TestSystemOutboundWithoutOneExactPeerRemainsExternal(t *testing.T) {
	for _, mismatch := range []string{"missing peer", "same owner", "two inputs", "two owners", "two records", "incomplete matches", "two matches"} {
		t.Run(mismatch, func(t *testing.T) {
			row := pageOutbound{ID: "send", Destination: "Run service", Connections: []string{"match"}}
			front := &pageSection{ID: "front", Map: &pageMap{Nodes: []pageMapNode{{ID: "caller"}}, Edges: []pageMapEdge{{ConnectionID: "match", From: "caller", To: "peer", Scope: "operation"}}}, Outbound: []pageOutbound{row}}
			backend := &pageSection{ID: "backend", Map: &pageMap{Nodes: []pageMapNode{{ID: "peer", Activation: "request"}}}}
			view := pageView{Sections: []*pageSection{front, backend}}
			switch mismatch {
			case "missing peer":
				backend.Map.Nodes = nil
			case "same owner":
				front.Map.Nodes = append(front.Map.Nodes, backend.Map.Nodes...)
				backend.Map.Nodes = nil
			case "two inputs", "two matches":
				backend.Map.Nodes = append(backend.Map.Nodes, pageMapNode{ID: "other", Activation: "request"})
				edge := pageMapEdge{ConnectionID: "match", From: "caller", To: "other", Scope: "operation"}
				if mismatch == "two matches" {
					edge.ConnectionID = "another-match"
					front.Outbound[0].Connections = append(front.Outbound[0].Connections, edge.ConnectionID)
				}
				front.Map.Edges = append(front.Map.Edges, edge)
			case "two owners":
				view.Sections = append(view.Sections, &pageSection{ID: "worker", Map: &pageMap{Nodes: []pageMapNode{{ID: "other"}}}})
				front.Map.Edges = append(front.Map.Edges, pageMapEdge{ConnectionID: "match", From: "caller", To: "other", Scope: "operation"})
			case "two records":
				other := row
				other.ID = "other-send"
				front.Outbound = append(front.Outbound, other)
			case "incomplete matches":
				front.Outbound[0].Connections = append(front.Outbound[0].Connections, "missing")
			}
			got := view.SystemMap()
			if !slices.ContainsFunc(got.Nodes, func(node pageMapNode) bool {
				return node.ID == "system-send" && node.ItemKind == "External communication"
			}) {
				t.Fatalf("%s incorrectly removed the independent communication record", mismatch)
			}
		})
	}
}

// The areas answer lists areas in its own order, usually along the pipeline.
// Nothing asks the model for an order and nothing checks one; code only keeps
// it. The atlas zones arrive in that order and the page lists them in it
// inside their component, with the part that is in no area after them. The
// legend says what the numbers on a frame's border are wherever a frame holds
// numbered parts.
func TestAreasKeepTheOrderTheModelListedThemOnThePage(t *testing.T) {
	type part struct{ id, title, path string }
	parts := []part{
		{"p1", "Routing", "api/routes.go"}, {"p2", "Handlers", "api/handlers.go"},
		{"p3", "Database", "store/db.go"}, {"p4", "Cache", "store/cache.go"},
		{"p5", "Rendering", "report/render.go"}, {"p6", "Export", "report/export.go"},
		{"p7", "Logging", "logging/log.go"},
	}
	objects := make([]programindex.ObjectInput, 0, len(parts))
	sources := make([]programindex.TargetSource, 0, len(parts))
	for i, p := range parts {
		objects = append(objects, programindex.ObjectInput{SourceRef: p.id, Kind: programindex.ObjectFunction, Name: p.title,
			Visibility: programindex.VisibilityPublic, Location: &programindex.Location{Path: p.path, Line: 3, Column: 1}})
		sources = append(sources, programindex.TargetSource{FileRef: fmt.Sprintf("f%d", i+1), Path: p.path})
	}
	program, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target:  programindex.TargetInput{ID: "t1", Language: "go", Kind: "executable", Name: "pipeline", Selector: "pipeline", Sources: sources, AnchorFileRef: "f1"},
		Objects: objects, Relations: []programindex.RelationInput{},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects)},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Neither alphabetical (Accessing, Reporting, Serving) nor its reverse.
	zones := []atlas.Zone{
		{ID: "z1", Title: "Serving requests", Line: "Answers requests.", BoxIDs: []string{"p1", "p2"}},
		{ID: "z2", Title: "Accessing storage", Line: "Keeps data.", BoxIDs: []string{"p3", "p4"}},
		{ID: "z3", Title: "Reporting", Line: "Writes reports.", BoxIDs: []string{"p5", "p6"}},
	}
	zoneOf := map[string]string{}
	for _, zone := range zones {
		for _, id := range zone.BoxIDs {
			zoneOf[id] = zone.ID
		}
	}
	target := atlas.Target{ID: program.Target.ID, Language: "go", Kind: "executable", Name: "pipeline", Root: ".",
		Zones: zones, Boxes: []atlas.Box{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{}}
	for i, p := range parts {
		target.Boxes = append(target.Boxes, atlas.Box{ID: p.id, Dir: filepath.Dir(p.path), Title: p.title, Line: "Does " + p.title + ".",
			ZoneID: zoneOf[p.id], Side: atlas.SideMid, Open: true, MemberIDs: []string{program.Objects[i].ID},
			Files: []atlas.File{{Path: p.path, Line: p.title + ".", Source: atlas.SourceModel, Open: true, Asked: true, Symbols: []atlas.Symbol{}}}})
	}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{program.Target.ID: program},
		atlas.Atlas{Version: atlas.Version, Repository: "pipeline", Revision: "abc", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := NewProgramPortfolio(program.Target.ID, []programindex.Index{program})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewGroupGraphView(indexes, program.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := ReportData{FormatVersion: CurrentFormatVersion, RepoName: "pipeline", CapturedRevision: strings.Repeat("a", 40),
		ProgramPortfolio: portfolio, GroupGraph: graph,
		TargetOutcomePortfolio: reportTargetOutcomeViewFixture(t, []TargetNavigationPage{{
			RunID: "20260926-120000-page-a1b2c3", ProgramTarget: program.Target.Snapshot(), ArtifactFilename: programindex.ArtifactFilename,
		}}, program.Target.ID)}
	if err := collectOpenablePaths(&data); err != nil {
		t.Fatal(err)
	}
	options := reportSingleTargetRenderOptionsFixture(t, &data)
	english, err := RenderHTMLWithOptions(&data, options)
	if err != nil {
		t.Fatal(err)
	}

	node := regexp.MustCompile(`<a [^>]*\bdata-node="[^"]*"[^>]*>`)
	attribute := func(tag, name string) string {
		match := regexp.MustCompile(`\s` + name + `="([^"]*)"`).FindStringSubmatch(tag)
		if match == nil {
			return ""
		}
		return html.UnescapeString(match[1])
	}
	titles, component := map[string]string{}, ""
	for _, tag := range node.FindAllString(string(english), -1) {
		titles[attribute(tag, "data-node")] = attribute(tag, "data-title")
		if attribute(tag, "data-branch") == "component" {
			component = attribute(tag, "data-children")
		}
	}
	var listed []string
	for _, id := range strings.Fields(component) {
		listed = append(listed, titles[id])
	}
	want := []string{"Serving requests", "Accessing storage", "Reporting", "Logging"}
	if !slices.Equal(listed, want) {
		t.Fatalf("the component lists %q, want the model's areas in its order and then the loose part: %q", listed, want)
	}
	// The parts entrance below the map lists the same areas the same way.
	links := regexp.MustCompile(`(?s)<ul class="plain learn-area-links">(.*?)</ul>`).FindStringSubmatch(string(english))
	if links == nil {
		t.Fatal("the parts entrance lists no areas")
	}
	var entrance []string
	for _, link := range regexp.MustCompile(`<a href="[^"]*">([^<]*) →</a>`).FindAllStringSubmatch(links[1], -1) {
		entrance = append(entrance, html.UnescapeString(link[1]))
	}
	if !slices.Equal(entrance, want) {
		t.Fatalf("the parts entrance lists %q, want %q", entrance, want)
	}

	line := template.HTMLEscapeString("Numbers on an area's or component's border are the numbered parts inside that the arrow connects, not an execution order.")
	if !strings.Contains(string(english), line) {
		t.Fatal("the legend does not explain the numbers on a frame's border")
	}
	options.Language, options.NoModel = Russian, true
	russian, err := RenderHTMLWithOptions(&data, options)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(russian), "Номера на границе области или компонента — это пронумерованные части внутри, которые соединяет стрелка, а не порядок выполнения.") {
		t.Fatal("the Russian legend does not explain the numbers on a frame's border")
	}
	// With no areas an open component still numbers its loose parts on its
	// own border, so the line stays.
	plain := reportProgramShellDataFixture(t, "fixture")
	loose, err := RenderHTMLWithOptions(&plain, reportSingleTargetRenderOptionsFixture(t, &plain))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loose), line) {
		t.Fatal("a component of loose parts does not explain their numbers")
	}
	// A component with no parts numbers nothing.
	empty, err := groupindex.Empty(program)
	if err != nil {
		t.Fatal(err)
	}
	if data.GroupGraph, err = NewGroupGraphView([]groupindex.Index{empty}, program.Target.ID); err != nil {
		t.Fatal(err)
	}
	options.Language, options.NoModel = English, false
	without, err := RenderHTMLWithOptions(&data, options)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(without), `class="map-legend"`) || strings.Contains(string(without), line) {
		t.Fatal("a map with nothing numbered explains numbers")
	}
}

// A connection's x* ID is its source target's ordinal. redis-benchmark's own
// x17 (listRelease calls zfree) shared its ID with the cli's link into
// redis-server, and the system map drew it solid into redis-server's
// Networking as if the benchmark's linked list were the client link.
func TestSystemMapKeepsEqualConnectionIDsOfDifferentTargetsApart(t *testing.T) {
	server := groupindex.Index{Target: programindex.Target{ID: "t1"}, Groups: []groupindex.Group{{ID: "g15", Title: "Networking", Lane: groupindex.LaneCore}}}
	bench := groupindex.Index{Target: programindex.Target{ID: "t2"}, Groups: []groupindex.Group{
		{ID: "g2", Title: "Linked list", Lane: groupindex.LaneCore}, {ID: "g3", Title: "Memory allocation", Lane: groupindex.LaneCore},
	}, Connections: []groupindex.Connection{{ID: "x17", SourceKind: "native_calls", SupportResolution: programindex.PatternValueExact,
		From: groupindex.Endpoint{TargetID: "t2", GroupID: "g2"}, To: groupindex.Endpoint{TargetID: "t2", GroupID: "g3"}, Label: "listRelease calls zfree"}}}
	cli := groupindex.Index{Target: programindex.Target{ID: "t4"}, Groups: []groupindex.Group{{ID: "g4", Title: "Network sockets", Lane: groupindex.LaneCore}},
		Connections: []groupindex.Connection{{ID: "x17", SourceKind: "integration", SupportResolution: programindex.PatternValuePossible,
			From: groupindex.Endpoint{TargetID: "t4", GroupID: "g4"}, To: groupindex.Endpoint{TargetID: "t1", GroupID: "g15"}, Label: "integrates with"}}}
	sections := map[string]*pageSection{
		"t1": {ID: "t1", programTargetID: "t1", ShortLabel: "redis-server"},
		"t2": {ID: "t2", programTargetID: "t2", ShortLabel: "redis-benchmark"},
		"t4": {ID: "t4", programTargetID: "t4", ShortLabel: "redis-cli"},
	}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{server, bench, cli}, byProgram: sections}
	view := pageView{}
	for _, id := range []string{"t1", "t2", "t4"} {
		sections[id].Map = builder.buildMap(sections[id])
		view.Sections = append(view.Sections, sections[id])
	}
	got := view.SystemMap()
	ends := map[string][2]string{}
	for _, edge := range got.Edges {
		ends[edge.Label] = [2]string{edge.From, edge.To}
	}
	memory, networking := targetMapNodeID("t2", mapNodeID("g3")), targetMapNodeID("t1", mapNodeID("g15"))
	if ends["listRelease calls zfree"][1] != memory {
		t.Fatalf("the benchmark's own call left its memory part: %v", ends)
	}
	if ends["integrates with"] != [2]string{targetMapNodeID("t4", mapNodeID("g4")), networking} {
		t.Fatalf("the cli's link lost its ends: %v", ends)
	}
}
