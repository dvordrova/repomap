package entrycall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/secretscan"
)

func TestPromptVersionIsPinnedToExactPrompt(t *testing.T) {
	digest := sha256.Sum256([]byte(promptSystem + promptUserShape))
	want := "entry-call-compression-prompt-" + hex.EncodeToString(digest[:6])
	if PromptVersion != want {
		t.Fatalf("PromptVersion = %q, want %q", PromptVersion, want)
	}
}

func TestArtifactNamesAndBoundsArePinned(t *testing.T) {
	if ResultArtifactFilename != "entry_call_result.v2.json" ||
		StatusArtifactFilename != "entry_call_status.v2.json" || len(ArtifactFilenames) != 4 ||
		ArtifactFilenames[0] != ResultArtifactFilename || ArtifactFilenames[1] != StatusArtifactFilename {
		t.Fatalf("artifact filename drift: %v", ArtifactFilenames)
	}
	if MaxRoots != 4 || MaxDepth != 3 || MaxOutgoingFamiliesPerNode != 12 ||
		MaxNodesPerRoot != 32 || MaxFamiliesPerRoot != 48 || MaxNodes != 128 || MaxFamilies != 192 ||
		MaxSelectedFamiliesPerRoot != 12 || MaxRepresentativeCallsites != 3 ||
		MaxRequestBytes != 64*1024 || MaxResponseBytes != 64*1024 {
		t.Fatal("entry-call safety bounds drifted")
	}
}

func TestCompileKeepsEarlyMainConnectorAndGroupsHighWitnessExternalFamily(t *testing.T) {
	substrate := causalFixture()
	accidental, err := json.Marshal(substrate)
	if err != nil || string(accidental) != "{}" {
		t.Fatalf("exact substrate became JSON-visible: %s, %v", accidental, err)
	}
	compilation, err := Compile(substrate)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := compilation.AdvertisedFamilyCount(); got != MaxOutgoingFamiliesPerNode+1 {
		t.Fatalf("advertised families = %d, want %d", got, MaxOutgoingFamiliesPerNode+1)
	}
	entry := compilation.Request.Entries[0]
	nodes := make(map[string]RequestNode, len(entry.Nodes))
	for _, node := range entry.Nodes {
		nodes[node.Ref] = node
	}
	var connector, router RequestFamily
	for _, family := range entry.Families {
		caller, callee := nodes[family.CallerRef], nodes[family.CalleeRef]
		if caller.Label == "main · main" && callee.Label == "routers · InitAPI" {
			connector = family
		}
		if caller.Label == "routers · InitAPI" && callee.Label == "web · Router" {
			router = family
		}
	}
	if connector.Ref == "" {
		t.Fatal("early main -> routers.InitAPI connector was lost behind equal-witness outgoing bound")
	}
	if router.Ref == "" || router.WitnessCount != 322 {
		t.Fatalf("grouped InitAPI -> web.Router = %+v, want one family with 322 witnesses", router)
	}
	foundOmission := false
	for _, frontier := range entry.Frontier {
		if frontier.NodeRef == entry.RootNodeRef && frontier.Reason == "outgoing_limit" &&
			frontier.FamilyCount == 2 && frontier.WitnessCount == 2 {
			foundOmission = true
		}
	}
	if !foundOmission || entry.Omitted.Nodes != 2 || entry.Omitted.Families != 2 || entry.Omitted.Witnesses != 2 {
		t.Fatalf("frontier/omitted = %+v/%+v, want exact two late witnesses", entry.Frontier, entry.Omitted)
	}

	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		t.Fatalf("ProviderVisibleJSON: %v", err)
	}
	for _, private := range []string{"main.go", "router.go", "canonical:", "github.com/acme/casdoor"} {
		if strings.Contains(string(wire), private) {
			t.Fatalf("provider wire leaked private authority %q: %s", private, wire)
		}
	}
	if !strings.Contains(string(wire), `"label":"web · Router"`) ||
		!strings.Contains(string(wire), `"witness_count":322`) {
		t.Fatalf("provider wire omitted compact family signal: %s", wire)
	}

	response, _ := json.Marshal(Response{
		Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
		Entries: []ResponseEntry{{RootRef: entry.Ref, FamilyRefs: []string{router.Ref, connector.Ref}}},
	})
	result, err := Reduce(compilation, response)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if result.SelectedFamilyCount() != 2 || result.Entries[0].Families[1].WitnessCount != 322 {
		t.Fatalf("restored result = %+v", result)
	}
	if got := result.Entries[0].Families[1].Callsites; len(got) != 3 || got[0].Path != "routers/router.go" {
		t.Fatalf("restored representative callsites = %+v", got)
	}
}

func TestReduceRequiresArrayAndRootedConnectivity(t *testing.T) {
	compilation, err := Compile(causalFixture())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	entry := compilation.Request.Entries[0]
	if _, err := Reduce(compilation, []byte(`{"version":2,"request_ref":"`+compilation.Request.RequestRef+`","entries":[{"root_ref":"r1","family_refs":null}]}`)); err == nil {
		t.Fatal("Reduce accepted null family_refs")
	}
	empty := Response{Version: ResultVersion, RequestRef: compilation.Request.RequestRef, Entries: []ResponseEntry{{RootRef: "r1", FamilyRefs: []string{}}}}
	raw, _ := json.Marshal(empty)
	result, err := Reduce(compilation, raw)
	if err != nil {
		t.Fatalf("Reduce empty array: %v", err)
	}
	if result.Entries[0].Families == nil || result.Entries[0].Frontier == nil {
		t.Fatalf("Reduce emitted nil canonical arrays: %+v", result.Entries[0])
	}

	var disconnected string
	nodeByRef := map[string]RequestNode{}
	for _, node := range entry.Nodes {
		nodeByRef[node.Ref] = node
	}
	for _, family := range entry.Families {
		if nodeByRef[family.CalleeRef].Label == "web · Router" {
			disconnected = family.Ref
		}
	}
	raw, _ = json.Marshal(Response{Version: ResultVersion, RequestRef: compilation.Request.RequestRef, Entries: []ResponseEntry{{RootRef: "r1", FamilyRefs: []string{disconnected}}}})
	result, err = Reduce(compilation, raw)
	if err != nil || result.SelectedFamilyCount() != 0 || result.RejectedFamilyCount() != 1 ||
		result.Entries[0].RejectedFamilies[0].Ref != disconnected {
		t.Fatalf("disconnected Reduce result = %+v, %v", result, err)
	}
}

func TestReduceRetainsRootedFamiliesAndRejectsOnlyUnreachableSibling(t *testing.T) {
	compilation, err := Compile(causalFixture())
	if err != nil {
		t.Fatal(err)
	}
	entry := compilation.Request.Entries[0]
	nodeByRef := make(map[string]RequestNode, len(entry.Nodes))
	for _, node := range entry.Nodes {
		nodeByRef[node.Ref] = node
	}
	var connector, router, disconnected string
	for _, family := range entry.Families {
		caller, callee := nodeByRef[family.CallerRef], nodeByRef[family.CalleeRef]
		switch {
		case caller.Label == "main · main" && callee.Label == "routers · InitAPI":
			connector = family.Ref
		case caller.Label == "routers · InitAPI" && callee.Label == "web · Router":
			router = family.Ref
		case caller.Label == "main · main" && disconnected == "":
			disconnected = family.Ref
		}
	}
	// Select a valid rooted chain plus the deep router family without its
	// connector in a second disconnected branch. Only that unreachable ref is
	// rejected; the expensive response is not discarded wholesale.
	raw, _ := json.Marshal(Response{Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
		Entries: []ResponseEntry{{RootRef: entry.Ref, FamilyRefs: []string{connector, router, disconnected}}}})
	result, err := Reduce(compilation, raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedFamilyCount() != 3 || result.RejectedFamilyCount() != 0 {
		t.Fatalf("rooted selection = %+v", result)
	}
	// Removing the connector makes only router unreachable; the direct family
	// remains useful and accepted.
	raw, _ = json.Marshal(Response{Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
		Entries: []ResponseEntry{{RootRef: entry.Ref, FamilyRefs: []string{router, disconnected}}}})
	result, err = Reduce(compilation, raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedFamilyCount() != 1 || result.RejectedFamilyCount() != 1 ||
		result.Entries[0].RejectedFamilies[0].Ref != router {
		t.Fatalf("partial rooted selection = %+v", result)
	}
}

func TestArtifactsAreBoundedStrictAndPreserveEmptyArrays(t *testing.T) {
	compilation, err := Compile(causalFixture())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(Response{Version: ResultVersion, RequestRef: compilation.Request.RequestRef, Entries: []ResponseEntry{{RootRef: "r1", FamilyRefs: []string{}}}})
	result, err := Reduce(compilation, raw)
	if err != nil {
		t.Fatal(err)
	}
	result.RepositoryStateSHA256 = strings.Repeat("a", 64)
	encoded, err := EncodeResult(result)
	if err != nil {
		t.Fatalf("EncodeResult: %v", err)
	}
	if !strings.Contains(string(encoded), `"families":[]`) || !strings.Contains(string(encoded), `"rejected_families":[]`) || !strings.Contains(string(encoded), `"frontier":[`) {
		t.Fatalf("result array identity drift: %s", encoded)
	}
	if _, err := DecodeResult(encoded); err != nil {
		t.Fatalf("DecodeResult: %v", err)
	}
	nullFamilies := strings.Replace(string(encoded), `"families":[]`, `"families":null`, 1)
	if _, err := DecodeResult([]byte(nullFamilies)); err == nil {
		t.Fatal("DecodeResult accepted null families")
	}
	unknown := strings.Replace(string(encoded), `{"version":2`, `{"unknown":true,"version":2`, 1)
	if _, err := DecodeResult([]byte(unknown)); err == nil {
		t.Fatal("DecodeResult accepted unknown field")
	}

	for _, reason := range []StatusReason{ReasonConfigurationFailed, ReasonOutputLimit} {
		status := Status{
			Version: StatusVersion, State: StatusRejected, Reason: reason, PromptVersion: PromptVersion,
			RequestRef: compilation.Request.RequestRef, RequestSHA256: compilation.RequestSHA256(),
			SubstrateSHA256:       compilation.SubstrateSHA256,
			RepositoryStateSHA256: strings.Repeat("b", 64),
			AdvertisedFamilies:    compilation.AdvertisedFamilyCount(),
		}
		statusRaw, err := EncodeStatus(status)
		if err != nil {
			t.Fatalf("EncodeStatus(%s): %v", reason, err)
		}
		if restored, err := DecodeStatus(statusRaw); err != nil || restored.Reason != reason {
			t.Fatalf("DecodeStatus(%s) = %+v, %v", reason, restored, err)
		}
	}
}

func TestProviderBoundaryRejectsCredentialShapedLabel(t *testing.T) {
	substrate := causalFixture()
	for index := range substrate.Nodes {
		if substrate.Nodes[index].ID == "canonical:main" {
			substrate.Nodes[index].Label = "sk-abcdefghijklmnopqrstuvwxyz123456"
		}
	}
	compilation, err := Compile(substrate)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	restore := secretscan.SetDisabled(true)
	defer restore()
	if _, err := ProviderVisibleJSON(compilation); err == nil {
		t.Fatal("ProviderVisibleJSON accepted credential-shaped label")
	}
}

func causalFixture() Substrate {
	location := func(path string, line int) Location { return Location{Path: path, Line: line, Column: 2} }
	nodes := []ExactNode{
		{ID: "canonical:main", Label: "main · main", Declaration: location("main.go", 50)},
		{ID: "canonical:init-api", Label: "routers · InitAPI", Declaration: location("routers/router.go", 40)},
		{ID: "external:web-router", Label: "web · Router", External: true},
	}
	families := []ExactFamily{{
		ID: "canonical:main-init-api", CallerID: "canonical:main", CalleeID: "canonical:init-api",
		Invocation: InvocationSynchronous, WitnessCount: 1, Callsites: []Location{location("main.go", 54)},
	}, {
		ID: "canonical:init-api-router", CallerID: "canonical:init-api", CalleeID: "external:web-router",
		Invocation: InvocationSynchronous, WitnessCount: 322,
		Callsites: []Location{location("routers/router.go", 47), location("routers/router.go", 48), location("routers/router.go", 49)},
	}}
	for index := 0; index < 13; index++ {
		id := "canonical:helper-" + string(rune('a'+index))
		nodes = append(nodes, ExactNode{ID: id, Label: "main · helper" + string(rune('A'+index)), Declaration: location("main.go", 100+index)})
		families = append(families, ExactFamily{
			ID: "canonical:main-helper-" + string(rune('a'+index)), CallerID: "canonical:main", CalleeID: id,
			Invocation: InvocationSynchronous, WitnessCount: 1, Callsites: []Location{location("main.go", 100+index)},
		})
	}
	return Substrate{
		Version: SubstrateVersion, State: StateReady,
		Roots: []ExactRoot{{NodeID: "canonical:main"}}, Nodes: nodes, Families: families,
		Frontiers: []ExactFrontier{},
	}
}
