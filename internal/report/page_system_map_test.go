package report

import (
	"encoding/json"
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
	if !strings.Contains(nodes["system-component-worker"].Children, "task") {
		t.Fatal("ungrouped input lost its component")
	}
	if view.Sections[0].Map.Edges[0].To != "remote" {
		t.Fatal("display mutated the original component view")
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
