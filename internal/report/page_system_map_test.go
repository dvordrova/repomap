package report

import (
	"bytes"
	"encoding/json"
	"html/template"
	"slices"
	"strings"
	"testing"
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
		Outbound:    []pageOutbound{{ID: "send", Destination: "Queue", KindLabel: "HTTP"}},
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
	group := nodes["system-get-destination"]
	if group.Branch != "communication" || group.FullTitle != "backend API" || group.Children != "system-get system-post system-unmatched" {
		t.Fatalf("external catalogue grouping lost: %+v", group)
	}
	if nodes["system-other-get-destination"].Owner != "other" {
		t.Fatal("equal destination in another component lost its scope")
	}
	if nodes["system-post"].FullTitle != "POST /items" || nodes["system-post"].Source.Text != "http.ts:20" {
		t.Fatal("record lost its own method or source")
	}
	if len(got.Edges) != 1 || got.Edges[0].From != "system-post" || got.Edges[0].To != "post" || !got.Edges[0].Possible || got.Edges[0].Operations != "click" {
		t.Fatalf("communication gained a peer by name or lost its exact connection: %+v", got.Edges)
	}
	if view.Sections[0].Map.Edges[0].From != "n-http" {
		t.Fatal("display changed the saved component map")
	}
}
