package entrycall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

func TestPromptAllowsEveryAdvertisedFamilyAndKeepsSurfaceBoundsIndependent(t *testing.T) {
	for _, required := range []string{
		"Every advertised family is already within the per-root response bound",
		"include all useful rooted families; do not stop at an arbitrary count",
		"Family selection and surface proposals have independent bounds",
		"Examine every advertised surface candidate",
		"Do not stop after an arbitrary number of surface proposals",
		"A token method fact must case-insensitively equal CONNECT, DELETE, GET, HEAD, OPTIONS, PATCH, POST, PUT, or TRACE",
		"a string method fact may preserve an exact custom HTTP method",
	} {
		if !strings.Contains(promptSystem, required) {
			t.Fatalf("entry-call prompt lost complete surface classification rule %q", required)
		}
	}
}

func TestArtifactNamesAndBoundsArePinned(t *testing.T) {
	if ResultArtifactFilename != "entry_call_result.v3.json" ||
		StatusArtifactFilename != "entry_call_status.v3.json" || len(ArtifactFilenames) != 6 ||
		ArtifactFilenames[0] != ResultArtifactFilename || ArtifactFilenames[1] != StatusArtifactFilename {
		t.Fatalf("artifact filename drift: %v", ArtifactFilenames)
	}
	if MaxRoots != 4 || MaxDepth != 3 || MaxOutgoingFamiliesPerNode != 12 ||
		MaxNodesPerRoot != 32 || MaxFamiliesPerRoot != 48 || MaxNodes != 128 || MaxFamilies != 192 ||
		MaxSelectedFamiliesPerRoot != MaxFamiliesPerRoot || MaxRepresentativeCallsites != 3 ||
		MaxRequestBytes != 128*1024 || MaxResponseBytes != 64*1024 ||
		MaxSurfaceCandidates != 128 || MaxSurfaceFactsPerCandidate != 8 || MaxSurfaceFacts != 512 ||
		MaxSurfaceCandidateSectionBytes != 32*1024 || MaxSurfaceFactValueRunes != 128 ||
		MaxProviderRequestBytes != 320*1024 {
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
		Entries:          []ResponseEntry{{RootRef: entry.Ref, FamilyRefs: []string{router.Ref, connector.Ref}}},
		SurfaceProposals: []ResponseSurfaceProposal{},
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
	if _, err := Reduce(compilation, []byte(`{"version":3,"request_ref":"`+compilation.Request.RequestRef+`","entries":[{"root_ref":"r1","family_refs":null}],"surface_proposals":[]}`)); err == nil {
		t.Fatal("Reduce accepted null family_refs")
	}
	empty := Response{Version: ResultVersion, RequestRef: compilation.Request.RequestRef, Entries: []ResponseEntry{{RootRef: "r1", FamilyRefs: []string{}}}, SurfaceProposals: []ResponseSurfaceProposal{}}
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
	raw, _ = json.Marshal(Response{Version: ResultVersion, RequestRef: compilation.Request.RequestRef, Entries: []ResponseEntry{{RootRef: "r1", FamilyRefs: []string{disconnected}}}, SurfaceProposals: []ResponseSurfaceProposal{}})
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
		Entries: []ResponseEntry{{RootRef: entry.Ref, FamilyRefs: []string{connector, router, disconnected}}}, SurfaceProposals: []ResponseSurfaceProposal{}})
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
		Entries: []ResponseEntry{{RootRef: entry.Ref, FamilyRefs: []string{router, disconnected}}}, SurfaceProposals: []ResponseSurfaceProposal{}})
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
	raw, _ := json.Marshal(Response{Version: ResultVersion, RequestRef: compilation.Request.RequestRef, Entries: []ResponseEntry{{RootRef: "r1", FamilyRefs: []string{}}}, SurfaceProposals: []ResponseSurfaceProposal{}})
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
	unknown := strings.Replace(string(encoded), `{"version":3`, `{"unknown":true,"version":3`, 1)
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

func TestSurfaceProposalsRestoreExactRouteAndCommandWithoutModelOwnedValues(t *testing.T) {
	compilation, err := Compile(surfaceFixture())
	if err != nil {
		t.Fatal(err)
	}
	if compilation.AdvertisedSurfaceCandidateCount() != 2 {
		t.Fatalf("advertised surface candidates = %d, want 2", compilation.AdvertisedSurfaceCandidateCount())
	}
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"routes.go", "cmd/root.go", "handlers/account.go", "canonical:surface", "github.com/acme/service"} {
		if strings.Contains(string(wire), private) {
			t.Fatalf("provider wire leaked private surface authority %q: %s", private, wire)
		}
	}

	cli := candidateByForm(t, compilation.Request.SurfaceCatalog, SurfaceCandidateKeyedComposite)
	route := candidateByForm(t, compilation.Request.SurfaceCatalog, SurfaceCandidateDirectCall)
	response := emptySurfaceResponse(compilation)
	response.SurfaceProposals = []ResponseSurfaceProposal{
		{
			CandidateRef: route.Ref, KindRef: SurfaceKindRefHTTPRoute,
			Bindings: []ResponseSurfaceBinding{
				{SlotRef: SurfaceSlotRefHandler, FactRef: surfaceFactRefByValue(t, route, "GetAccount")},
				{SlotRef: SurfaceSlotRefPath, FactRef: surfaceFactRefByValue(t, route, "/account/:id")},
				{SlotRef: SurfaceSlotRefMethod, FactRef: surfaceFactRefByValue(t, route, "GET")},
			},
		},
		{
			CandidateRef: cli.Ref, KindRef: SurfaceKindRefCLICommand,
			Bindings: []ResponseSurfaceBinding{
				{SlotRef: SurfaceSlotRefHandler, FactRef: surfaceFactRefByValue(t, cli, "runServe")},
				{SlotRef: SurfaceSlotRefIdentity, FactRef: surfaceFactRefByValue(t, cli, "serve [path]")},
			},
		},
	}
	raw, _ := json.Marshal(response)
	proposalRaw, _ := json.Marshal(response.SurfaceProposals)
	if strings.Contains(string(proposalRaw), "serve [path]") || strings.Contains(string(proposalRaw), "/account/:id") ||
		strings.Contains(string(proposalRaw), "runServe") || strings.Contains(string(proposalRaw), "GetAccount") ||
		strings.Contains(string(proposalRaw), "root_ref") {
		t.Fatalf("surface response contains model-owned exact data: %s", proposalRaw)
	}
	result, err := Reduce(compilation, raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedSurfaceCount() != 2 || result.RejectedSurfaceCount() != 0 {
		t.Fatalf("surface result counts = %d/%d, want 2/0: %+v", result.SelectedSurfaceCount(), result.RejectedSurfaceCount(), result)
	}
	var restoredCLI, restoredRoute ResultSurfaceProposal
	for _, proposal := range result.SurfaceProposals {
		switch proposal.Kind {
		case SurfaceKindCLICommand:
			restoredCLI = proposal
		case SurfaceKindHTTPRoute:
			restoredRoute = proposal
		}
	}
	if restoredCLI.RootRef != cli.RootRef || restoredCLI.Identity == nil || restoredCLI.Identity.Text != "serve [path]" ||
		restoredCLI.Handler == nil || restoredCLI.Handler.Text != "runServe" || restoredCLI.Site.Path != "cmd/root.go" {
		t.Fatalf("restored CLI = %+v", restoredCLI)
	}
	if restoredRoute.RootRef != route.RootRef || restoredRoute.Method == nil || restoredRoute.Method.Text != "GET" ||
		restoredRoute.Path == nil || restoredRoute.Path.Text != "/account/:id" ||
		restoredRoute.Handler == nil || restoredRoute.Handler.Text != "GetAccount" ||
		restoredRoute.Handler.Location == nil || restoredRoute.Handler.Location.Path != "handlers/account.go" {
		t.Fatalf("restored route = %+v", restoredRoute)
	}
}

func TestSurfaceProposalRoundTripsExact128ByteValueAndInteriorSpaces(t *testing.T) {
	substrate := surfaceFixture()
	longPath := "/" + strings.Repeat("segment-", 15) + "abcdefg"
	if len(longPath) != 128 {
		t.Fatalf("test path bytes = %d, want 128", len(longPath))
	}
	commandIdentity := "serve   [path]"
	for candidateIndex := range substrate.SurfaceCandidates {
		for factIndex := range substrate.SurfaceCandidates[candidateIndex].Facts {
			fact := &substrate.SurfaceCandidates[candidateIndex].Facts[factIndex]
			switch fact.Value {
			case "/account/:id":
				fact.Value = longPath
			case "serve [path]":
				fact.Value = commandIdentity
			}
		}
	}
	compilation, err := Compile(substrate)
	if err != nil {
		t.Fatal(err)
	}
	cli := candidateByForm(t, compilation.Request.SurfaceCatalog, SurfaceCandidateKeyedComposite)
	route := candidateByForm(t, compilation.Request.SurfaceCatalog, SurfaceCandidateDirectCall)
	response := emptySurfaceResponse(compilation)
	response.SurfaceProposals = []ResponseSurfaceProposal{
		{
			CandidateRef: cli.Ref, KindRef: SurfaceKindRefCLICommand,
			Bindings: []ResponseSurfaceBinding{
				{SlotRef: SurfaceSlotRefIdentity, FactRef: surfaceFactRefByValue(t, cli, commandIdentity)},
				{SlotRef: SurfaceSlotRefHandler, FactRef: surfaceFactRefByValue(t, cli, "runServe")},
			},
		},
		{
			CandidateRef: route.Ref, KindRef: SurfaceKindRefHTTPRoute,
			Bindings: []ResponseSurfaceBinding{
				{SlotRef: SurfaceSlotRefMethod, FactRef: surfaceFactRefByValue(t, route, "GET")},
				{SlotRef: SurfaceSlotRefPath, FactRef: surfaceFactRefByValue(t, route, longPath)},
				{SlotRef: SurfaceSlotRefHandler, FactRef: surfaceFactRefByValue(t, route, "GetAccount")},
			},
		},
	}
	raw, _ := json.Marshal(response)
	result, err := Reduce(compilation, raw)
	if err != nil {
		t.Fatal(err)
	}
	result.RepositoryStateSHA256 = strings.Repeat("d", 64)
	encoded, err := EncodeResult(result)
	if err != nil {
		t.Fatalf("EncodeResult exact values: %v", err)
	}
	restored, err := DecodeResult(encoded)
	if err != nil {
		t.Fatalf("DecodeResult exact values: %v", err)
	}
	var gotIdentity, gotPath string
	for _, proposal := range restored.SurfaceProposals {
		if proposal.Identity != nil {
			gotIdentity = proposal.Identity.Text
		}
		if proposal.Path != nil {
			gotPath = proposal.Path.Text
		}
	}
	if gotIdentity != commandIdentity || gotPath != longPath {
		t.Fatalf("exact values = %q / %q", gotIdentity, gotPath)
	}
}

func TestSurfaceProposalRejectsInvalidSiblingLocallyButUnknownRefsAtomically(t *testing.T) {
	compilation, err := Compile(surfaceFixture())
	if err != nil {
		t.Fatal(err)
	}
	cli := candidateByForm(t, compilation.Request.SurfaceCatalog, SurfaceCandidateKeyedComposite)
	route := candidateByForm(t, compilation.Request.SurfaceCatalog, SurfaceCandidateDirectCall)
	response := emptySurfaceResponse(compilation)
	response.SurfaceProposals = []ResponseSurfaceProposal{
		{
			CandidateRef: cli.Ref, KindRef: SurfaceKindRefCLICommand,
			Bindings: []ResponseSurfaceBinding{{
				SlotRef: SurfaceSlotRefIdentity, FactRef: surfaceFactRefByValue(t, route, "/account/:id"),
			}},
		},
		{
			CandidateRef: route.Ref, KindRef: SurfaceKindRefHTTPRoute,
			Bindings: []ResponseSurfaceBinding{
				{SlotRef: SurfaceSlotRefMethod, FactRef: surfaceFactRefByValue(t, route, "GET")},
				{SlotRef: SurfaceSlotRefPath, FactRef: surfaceFactRefByValue(t, route, "/account/:id")},
				{SlotRef: SurfaceSlotRefHandler, FactRef: surfaceFactRefByValue(t, route, "GetAccount")},
			},
		},
	}
	raw, _ := json.Marshal(response)
	result, err := Reduce(compilation, raw)
	if err != nil || result.SelectedSurfaceCount() != 1 || result.RejectedSurfaceCount() != 1 ||
		result.RejectedSurfaceProposals[0].CandidateRef != cli.Ref ||
		result.RejectedSurfaceProposals[0].Reason != RejectedSurfaceIncompatibleBinding {
		t.Fatalf("item-local surface salvage = %+v, %v", result, err)
	}

	response.SurfaceProposals[0].Bindings[0].FactRef = "v999"
	raw, _ = json.Marshal(response)
	if _, err := Reduce(compilation, raw); err == nil || !strings.Contains(err.Error(), "unknown surface fact ref") {
		t.Fatalf("unknown fact ref did not reject atomically: %v", err)
	}
}

func TestHTTPTokenMethodValidationRejectsHandleFuncLocally(t *testing.T) {
	substrate := causalFixture()
	location := func(path string, line int) Location { return Location{Path: path, Line: line, Column: 3} }
	substrate.SurfaceCandidates = []ExactSurfaceCandidate{
		{
			ID: "canonical:surface:healthz", RootNodeID: "canonical:main",
			Form: SurfaceCandidateDirectCall, Sketch: "HandleFunc /healthz",
			Site: location("internal/server/server.go", 43),
			Facts: []ExactSurfaceFact{
				{ID: "canonical:surface:healthz:method", Kind: SurfaceFactToken, Position: 0, Label: "method", Value: "HandleFunc", Location: location("internal/server/server.go", 43)},
				{ID: "canonical:surface:healthz:path", Kind: SurfaceFactString, Position: 1, Label: "path", Value: "/healthz", Location: location("internal/server/server.go", 43)},
				{ID: "canonical:surface:healthz:handler", Kind: SurfaceFactCallable, Position: 2, Label: "handler", Value: "func literal", Location: location("internal/server/server.go", 43)},
			},
		},
		{
			ID: "canonical:surface:readyz", RootNodeID: "canonical:main",
			Form: SurfaceCandidateDirectCall, Sketch: "get /readyz",
			Site: location("internal/server/server.go", 44),
			Facts: []ExactSurfaceFact{
				{ID: "canonical:surface:readyz:method", Kind: SurfaceFactToken, Position: 0, Label: "method", Value: "get", Location: location("internal/server/server.go", 44)},
				{ID: "canonical:surface:readyz:path", Kind: SurfaceFactString, Position: 1, Label: "path", Value: "/readyz", Location: location("internal/server/server.go", 44)},
				{ID: "canonical:surface:readyz:handler", Kind: SurfaceFactCallable, Position: 2, Label: "handler", Value: "ready", Location: location("internal/server/server.go", 44)},
			},
		},
		{
			ID: "canonical:surface:webdav", RootNodeID: "canonical:main",
			Form: SurfaceCandidateDirectCall, Sketch: "Add PROPFIND",
			Site: location("internal/server/webdav.go", 21),
			Facts: []ExactSurfaceFact{
				{ID: "canonical:surface:webdav:method", Kind: SurfaceFactString, Position: 0, Label: "method", Value: "PROPFIND", Location: location("internal/server/webdav.go", 21)},
				{ID: "canonical:surface:webdav:path", Kind: SurfaceFactString, Position: 1, Label: "path", Value: "/dav", Location: location("internal/server/webdav.go", 21)},
				{ID: "canonical:surface:webdav:handler", Kind: SurfaceFactCallable, Position: 2, Label: "handler", Value: "webDAV", Location: location("internal/server/webdav.go", 21)},
			},
		},
	}
	substrate.Coverage.SurfaceCandidatesConsidered = len(substrate.SurfaceCandidates)
	substrate.Coverage.SurfaceCandidatesIndexed = len(substrate.SurfaceCandidates)
	for _, candidate := range substrate.SurfaceCandidates {
		substrate.Coverage.SurfaceCandidateFactsConsidered += len(candidate.Facts)
		substrate.Coverage.SurfaceCandidateFactsIndexed += len(candidate.Facts)
	}

	compilation, err := Compile(substrate)
	if err != nil {
		t.Fatal(err)
	}
	healthz := candidateByFactValue(t, compilation.Request.SurfaceCatalog, "/healthz")
	readyz := candidateByFactValue(t, compilation.Request.SurfaceCatalog, "/readyz")
	webdav := candidateByFactValue(t, compilation.Request.SurfaceCatalog, "PROPFIND")
	response := emptySurfaceResponse(compilation)
	entry := compilation.Request.Entries[0]
	for _, family := range entry.Families {
		if family.CallerRef == entry.RootNodeRef {
			response.Entries[0].FamilyRefs = []string{family.Ref}
			break
		}
	}
	if len(response.Entries[0].FamilyRefs) != 1 {
		t.Fatal("causal fixture exposed no rooted family")
	}
	proposal := func(candidate RequestSurfaceCandidate, method, path, handler string) ResponseSurfaceProposal {
		return ResponseSurfaceProposal{
			CandidateRef: candidate.Ref, KindRef: SurfaceKindRefHTTPRoute,
			Bindings: []ResponseSurfaceBinding{
				{SlotRef: SurfaceSlotRefMethod, FactRef: surfaceFactRefByValue(t, candidate, method)},
				{SlotRef: SurfaceSlotRefPath, FactRef: surfaceFactRefByValue(t, candidate, path)},
				{SlotRef: SurfaceSlotRefHandler, FactRef: surfaceFactRefByValue(t, candidate, handler)},
			},
		}
	}
	response.SurfaceProposals = []ResponseSurfaceProposal{
		proposal(healthz, "HandleFunc", "/healthz", "func literal"),
		proposal(readyz, "get", "/readyz", "ready"),
		proposal(webdav, "PROPFIND", "/dav", "webDAV"),
	}
	raw, _ := json.Marshal(response)
	result, err := Reduce(compilation, raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedFamilyCount() != 1 || result.SelectedSurfaceCount() != 2 || result.RejectedSurfaceCount() != 1 ||
		result.RejectedSurfaceProposals[0].CandidateRef != healthz.Ref ||
		result.RejectedSurfaceProposals[0].Reason != RejectedSurfaceIncompatibleBinding {
		t.Fatalf("item-local HTTP token validation = %+v", result)
	}
	methods := make(map[string]SurfaceFactKind, len(result.SurfaceProposals))
	for _, restored := range result.SurfaceProposals {
		methods[restored.Method.Text] = restored.Method.Kind
	}
	if methods["get"] != SurfaceFactToken || methods["PROPFIND"] != SurfaceFactString {
		t.Fatalf("restored exact valid methods = %+v, want token get and string PROPFIND", methods)
	}
	result.RepositoryStateSHA256 = strings.Repeat("e", 64)
	encoded, err := EncodeResult(result)
	if err != nil {
		t.Fatalf("EncodeResult valid exact methods: %v", err)
	}
	if _, err := DecodeResult(encoded); err != nil {
		t.Fatalf("DecodeResult valid exact methods: %v", err)
	}
	for index := range result.SurfaceProposals {
		if result.SurfaceProposals[index].Method.Kind == SurfaceFactToken {
			result.SurfaceProposals[index].Method.Text = "HandleFunc"
			break
		}
	}
	invalid, _ := json.Marshal(result)
	if _, err := DecodeResult(invalid); err == nil {
		t.Fatal("DecodeResult accepted nonstandard token method HandleFunc")
	}
}

func TestStandardHTTPTokenMethodSet(t *testing.T) {
	for _, method := range []string{"CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT", "TRACE", "get"} {
		if !standardHTTPTokenMethod(method) {
			t.Errorf("standardHTTPTokenMethod(%q) = false", method)
		}
	}
	for _, method := range []string{"HandleFunc", "FETCH", "GET "} {
		if standardHTTPTokenMethod(method) {
			t.Errorf("standardHTTPTokenMethod(%q) = true", method)
		}
	}
}

func TestHandlerlessCLIRequiresCompanionDescriptorWhenCallableFactsExist(t *testing.T) {
	substrate := surfaceFixture()
	location := func(path string, line int) Location { return Location{Path: path, Line: line, Column: 3} }
	additional := []ExactSurfaceCandidate{
		{
			ID: "canonical:surface:options", RootNodeID: "canonical:main",
			Form: SurfaceCandidateKeyedComposite, Sketch: "options(gitlabClient,defaultHostname)",
			Site: location("options.go", 41),
			Facts: []ExactSurfaceFact{
				{ID: "canonical:surface:options:client", Kind: SurfaceFactCallable, Position: 1, Label: "gitlabClient", Value: "(Factory).GitLabClient", Location: location("options.go", 42)},
				{ID: "canonical:surface:options:hostname", Kind: SurfaceFactString, Position: 2, Label: "defaultHostname", Value: "gitlab.com", Location: location("options.go", 43)},
			},
		},
		{
			ID: "canonical:surface:parent-command", RootNodeID: "canonical:main",
			Form: SurfaceCandidateKeyedComposite, Sketch: "Command(Use,Short,PersistentPreRunE)",
			Site: location("hook.go", 31),
			Facts: []ExactSurfaceFact{
				{ID: "canonical:surface:parent-command:use", Kind: SurfaceFactString, Position: 1, Label: "Use", Value: "hook", Location: location("hook.go", 32)},
				{ID: "canonical:surface:parent-command:short", Kind: SurfaceFactString, Position: 2, Label: "Short", Value: "Run git server hooks", Location: location("hook.go", 33)},
				{ID: "canonical:surface:parent-command:pre-run", Kind: SurfaceFactCallable, Position: 3, Label: "PersistentPreRunE", Value: "func literal", Location: location("hook.go", 35)},
			},
		},
	}
	for _, candidate := range additional {
		substrate.SurfaceCandidates = append(substrate.SurfaceCandidates, candidate)
		substrate.Coverage.SurfaceCandidatesConsidered++
		substrate.Coverage.SurfaceCandidatesIndexed++
		substrate.Coverage.SurfaceCandidateFactsConsidered += len(candidate.Facts)
		substrate.Coverage.SurfaceCandidateFactsIndexed += len(candidate.Facts)
	}
	compilation, err := Compile(substrate)
	if err != nil {
		t.Fatal(err)
	}
	options := candidateByFactValue(t, compilation.Request.SurfaceCatalog, "gitlab.com")
	parent := candidateByFactValue(t, compilation.Request.SurfaceCatalog, "hook")
	proposals := []ResponseSurfaceProposal{
		{
			CandidateRef: options.Ref, KindRef: SurfaceKindRefCLICommand,
			Bindings: []ResponseSurfaceBinding{{
				SlotRef: SurfaceSlotRefIdentity, FactRef: surfaceFactRefByValue(t, options, "gitlab.com"),
			}},
		},
		{
			CandidateRef: parent.Ref, KindRef: SurfaceKindRefCLICommand,
			Bindings: []ResponseSurfaceBinding{{
				SlotRef: SurfaceSlotRefIdentity, FactRef: surfaceFactRefByValue(t, parent, "hook"),
			}},
		},
	}

	for index, ordered := range [][]ResponseSurfaceProposal{proposals, {proposals[1], proposals[0]}} {
		response := emptySurfaceResponse(compilation)
		response.SurfaceProposals = ordered
		raw, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		result, err := Reduce(compilation, raw)
		if err != nil {
			t.Fatalf("Reduce order %d: %v", index, err)
		}
		if result.SelectedSurfaceCount() != 1 || result.RejectedSurfaceCount() != 1 ||
			result.SurfaceProposals[0].CandidateRef != parent.Ref || result.SurfaceProposals[0].Handler != nil ||
			result.SurfaceProposals[0].Identity == nil || result.SurfaceProposals[0].Identity.Text != "hook" ||
			result.RejectedSurfaceProposals[0].CandidateRef != options.Ref ||
			result.RejectedSurfaceProposals[0].Reason != RejectedSurfaceIncompatibleBinding {
			t.Fatalf("handlerless CLI evidence order %d = %+v", index, result)
		}
	}
}

func TestGotenbergThirteenFamilySelectionIsAcceptedAndPreservesExactCommand(t *testing.T) {
	compilation, err := Compile(surfaceFixture())
	if err != nil {
		t.Fatal(err)
	}
	entry := compilation.Request.Entries[0]
	if len(entry.Families) != 13 {
		t.Fatalf("fixture families = %d, want causal provider response size 13", len(entry.Families))
	}
	command := candidateByForm(t, compilation.Request.SurfaceCatalog, SurfaceCandidateKeyedComposite)
	proposal := ResponseSurfaceProposal{
		CandidateRef: command.Ref, KindRef: SurfaceKindRefCLICommand,
		Bindings: []ResponseSurfaceBinding{
			{SlotRef: SurfaceSlotRefIdentity, FactRef: surfaceFactRefByValue(t, command, "serve [path]")},
			{SlotRef: SurfaceSlotRefHandler, FactRef: surfaceFactRefByValue(t, command, "runServe")},
		},
	}
	familyRefs := make([]string, 0, len(entry.Families))
	for _, family := range entry.Families {
		familyRefs = append(familyRefs, family.Ref)
	}
	reversed := append([]string{}, familyRefs...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}

	var canonical []byte
	for index, refs := range [][]string{familyRefs, reversed} {
		response := emptySurfaceResponse(compilation)
		response.Entries[0].FamilyRefs = refs
		response.SurfaceProposals = []ResponseSurfaceProposal{proposal}
		raw, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		result, err := Reduce(compilation, raw)
		if err != nil {
			t.Fatalf("Reduce order %d: %v", index, err)
		}
		if result.SelectedFamilyCount() != len(familyRefs) || result.RejectedFamilyCount() != 0 ||
			result.SelectedSurfaceCount() != 1 || result.RejectedSurfaceCount() != 0 ||
			result.SurfaceProposals[0].Identity == nil || result.SurfaceProposals[0].Identity.Text != "serve [path]" {
			t.Fatalf("thirteen-family selection order %d = %+v", index, result)
		}
		result.RepositoryStateSHA256 = strings.Repeat("f", 64)
		encoded, err := EncodeResult(result)
		if err != nil {
			t.Fatalf("EncodeResult order %d: %v", index, err)
		}
		if _, err := DecodeResult(encoded); err != nil {
			t.Fatalf("DecodeResult order %d: %v", index, err)
		}
		if canonical == nil {
			canonical = encoded
		} else if string(encoded) != string(canonical) {
			t.Fatalf("thirteen-family restoration depends on provider order:\n%s\n%s", canonical, encoded)
		}
	}

	unknown := emptySurfaceResponse(compilation)
	unknown.Entries[0].FamilyRefs = append(append([]string{}, familyRefs[:len(familyRefs)-1]...), "f999")
	unknown.SurfaceProposals = []ResponseSurfaceProposal{proposal}
	raw, _ := json.Marshal(unknown)
	if _, err := Reduce(compilation, raw); err == nil || !strings.Contains(err.Error(), "unknown family ref") {
		t.Fatalf("unknown family ref did not reject atomically: %v", err)
	}
}

func TestFamilySelectionAcceptsFortyEightAdvertisedRefsAndRejectsFortyNine(t *testing.T) {
	compilation, err := Compile(familyLimitFixture())
	if err != nil {
		t.Fatal(err)
	}
	entry := compilation.Request.Entries[0]
	if len(entry.Families) != MaxFamiliesPerRoot {
		t.Fatalf("advertised families = %d, want exact boundary %d", len(entry.Families), MaxFamiliesPerRoot)
	}
	refs := make([]string, 0, len(entry.Families))
	for _, family := range entry.Families {
		refs = append(refs, family.Ref)
	}
	response := emptySurfaceResponse(compilation)
	response.Entries[0].FamilyRefs = refs
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Reduce(compilation, raw)
	if err != nil {
		t.Fatalf("Reduce 48 advertised refs: %v", err)
	}
	if result.SelectedFamilyCount() != MaxFamiliesPerRoot || result.RejectedFamilyCount() != 0 {
		t.Fatalf("48-family result = %+v", result)
	}
	result.RepositoryStateSHA256 = strings.Repeat("f", 64)
	encoded, err := EncodeResult(result)
	if err != nil {
		t.Fatalf("EncodeResult 48 advertised refs: %v", err)
	}
	if restored, err := DecodeResult(encoded); err != nil || restored.SelectedFamilyCount() != MaxFamiliesPerRoot {
		t.Fatalf("DecodeResult 48 advertised refs = %d, %v", restored.SelectedFamilyCount(), err)
	}
	status := Status{
		Version: StatusVersion, State: StatusAccepted, PromptVersion: PromptVersion,
		RequestRef: compilation.Request.RequestRef, RequestSHA256: compilation.RequestSHA256(),
		SubstrateSHA256: compilation.SubstrateSHA256, RepositoryStateSHA256: strings.Repeat("f", 64),
		ResultSHA256: sha256Hex(encoded), AdvertisedFamilies: MaxFamiliesPerRoot,
		SelectedFamilies: MaxFamiliesPerRoot, SurfaceCandidateCoverage: compilation.SurfaceCoverage(),
	}
	statusRaw, err := EncodeStatus(status)
	if err != nil {
		t.Fatalf("EncodeStatus 48 selected families: %v", err)
	}
	if restored, err := DecodeStatus(statusRaw); err != nil || restored.SelectedFamilies != MaxFamiliesPerRoot {
		t.Fatalf("DecodeStatus 48 selected families = %d, %v", restored.SelectedFamilies, err)
	}

	resourceOverflow := emptySurfaceResponse(compilation)
	resourceOverflow.Entries[0].FamilyRefs = make([]string, MaxFamiliesPerRoot+1)
	raw, _ = json.Marshal(resourceOverflow)
	if _, err := Reduce(compilation, raw); err == nil || !strings.Contains(err.Error(), "resource bound") {
		t.Fatalf("family resource overflow did not reject atomically: %v", err)
	}
}

func TestDuplicateSurfaceProposalsRejectCandidateLocallyAndOrderIndependently(t *testing.T) {
	compilation, err := Compile(surfaceFixture())
	if err != nil {
		t.Fatal(err)
	}
	cli := candidateByForm(t, compilation.Request.SurfaceCatalog, SurfaceCandidateKeyedComposite)
	route := candidateByForm(t, compilation.Request.SurfaceCatalog, SurfaceCandidateDirectCall)
	entry := compilation.Request.Entries[0]
	rootedFamilyRef := ""
	for _, family := range entry.Families {
		if family.CallerRef == entry.RootNodeRef {
			rootedFamilyRef = family.Ref
			break
		}
	}
	if rootedFamilyRef == "" {
		t.Fatal("fixture has no family rooted at the exact entry")
	}
	validCLI := ResponseSurfaceProposal{
		CandidateRef: cli.Ref, KindRef: SurfaceKindRefCLICommand,
		Bindings: []ResponseSurfaceBinding{{
			SlotRef: SurfaceSlotRefIdentity, FactRef: surfaceFactRefByValue(t, cli, "serve [path]"),
		}},
	}
	conflictingCLI := ResponseSurfaceProposal{
		CandidateRef: cli.Ref, KindRef: SurfaceKindRefCLICommand,
		Bindings: []ResponseSurfaceBinding{{
			SlotRef: SurfaceSlotRefIdentity, FactRef: surfaceFactRefByValue(t, route, "/account/:id"),
		}},
	}
	validRoute := ResponseSurfaceProposal{
		CandidateRef: route.Ref, KindRef: SurfaceKindRefHTTPRoute,
		Bindings: []ResponseSurfaceBinding{
			{SlotRef: SurfaceSlotRefMethod, FactRef: surfaceFactRefByValue(t, route, "GET")},
			{SlotRef: SurfaceSlotRefPath, FactRef: surfaceFactRefByValue(t, route, "/account/:id")},
			{SlotRef: SurfaceSlotRefHandler, FactRef: surfaceFactRefByValue(t, route, "GetAccount")},
		},
	}
	orders := [][]ResponseSurfaceProposal{
		{validCLI, validRoute, conflictingCLI},
		{conflictingCLI, validCLI, validRoute},
		{validRoute, conflictingCLI, validCLI},
	}
	var canonical []byte
	for index, proposals := range orders {
		response := emptySurfaceResponse(compilation)
		response.Entries[0].FamilyRefs = []string{rootedFamilyRef}
		response.SurfaceProposals = proposals
		raw, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		result, err := Reduce(compilation, raw)
		if err != nil {
			t.Fatalf("Reduce order %d: %v", index, err)
		}
		if result.SelectedFamilyCount() != 1 || result.SelectedSurfaceCount() != 1 ||
			result.SurfaceProposals[0].CandidateRef != route.Ref || result.RejectedSurfaceCount() != 1 ||
			result.RejectedSurfaceProposals[0].CandidateRef != cli.Ref ||
			result.RejectedSurfaceProposals[0].Reason != RejectedSurfaceDuplicateProposal {
			t.Fatalf("item-local duplicate salvage order %d = %+v", index, result)
		}
		result.RepositoryStateSHA256 = strings.Repeat("e", 64)
		encoded, err := EncodeResult(result)
		if err != nil {
			t.Fatalf("EncodeResult order %d: %v", index, err)
		}
		if _, err := DecodeResult(encoded); err != nil {
			t.Fatalf("DecodeResult order %d: %v", index, err)
		}
		if canonical == nil {
			canonical = encoded
		} else if string(encoded) != string(canonical) {
			t.Fatalf("duplicate salvage depends on response order:\n%s\n%s", canonical, encoded)
		}
	}
}

func TestSurfaceCompilationIsDeterministicBoundedAndOmitsSensitiveFacts(t *testing.T) {
	left := surfaceFixture()
	right := surfaceFixture()
	for start, end := 0, len(right.SurfaceCandidates)-1; start < end; start, end = start+1, end-1 {
		right.SurfaceCandidates[start], right.SurfaceCandidates[end] = right.SurfaceCandidates[end], right.SurfaceCandidates[start]
	}
	for index := range right.SurfaceCandidates {
		for start, end := 0, len(right.SurfaceCandidates[index].Facts)-1; start < end; start, end = start+1, end-1 {
			right.SurfaceCandidates[index].Facts[start], right.SurfaceCandidates[index].Facts[end] =
				right.SurfaceCandidates[index].Facts[end], right.SurfaceCandidates[index].Facts[start]
		}
	}
	leftCompilation, err := Compile(left)
	if err != nil {
		t.Fatal(err)
	}
	rightCompilation, err := Compile(right)
	if err != nil {
		t.Fatal(err)
	}
	leftWire, _ := ProviderVisibleJSON(leftCompilation)
	rightWire, _ := ProviderVisibleJSON(rightCompilation)
	if string(leftWire) != string(rightWire) || leftCompilation.SubstrateSHA256 != rightCompilation.SubstrateSHA256 {
		t.Fatalf("surface compilation is order-sensitive:\n%s\n%s", leftWire, rightWire)
	}

	sensitive := surfaceFixture()
	secret := "sk-abcdefghijklmnopqrstuvwxyz123456"
	sensitive.SurfaceCandidates[1].Facts[0].Value = secret
	compiled, err := Compile(sensitive)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := ProviderVisibleJSON(compiled)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), secret) || compiled.SurfaceCoverage().UnsafeFactsExcluded == 0 ||
		compiled.AdvertisedSurfaceCandidateCount() != 1 {
		t.Fatalf("sensitive fact accounting/wire = %+v / %s", compiled.SurfaceCoverage(), wire)
	}
}

func TestSurfaceCompilationEnforcesIndependentCandidateFactAndByteBounds(t *testing.T) {
	substrate := causalFixture()
	location := func(line int) Location { return Location{Path: "routes.go", Line: line, Column: 1} }
	for candidateIndex := 0; candidateIndex < MaxSurfaceCandidates+12; candidateIndex++ {
		candidate := ExactSurfaceCandidate{
			ID: "candidate-" + string(rune(0x1000+candidateIndex)), RootNodeID: "canonical:main",
			Form: SurfaceCandidateDirectCall, Sketch: "Register", Site: location(candidateIndex + 1),
			Facts: []ExactSurfaceFact{},
		}
		for factIndex := 0; factIndex < MaxRawSurfaceFactsPerCandidate; factIndex++ {
			kind := SurfaceFactString
			value := "/route/" + string(rune(0x2000+candidateIndex)) + "/" + string(rune(0x3000+factIndex))
			if factIndex == 0 {
				kind, value = SurfaceFactToken, "GET"
			}
			if factIndex == 1 {
				kind, value = SurfaceFactCallable, "handler"
			}
			candidate.Facts = append(candidate.Facts, ExactSurfaceFact{
				ID:   "fact-" + string(rune(0x4000+candidateIndex)) + "-" + string(rune(0x5000+factIndex)),
				Kind: kind, Position: factIndex, Label: "argument", Value: value, Location: location(candidateIndex + 1),
			})
		}
		substrate.SurfaceCandidates = append(substrate.SurfaceCandidates, candidate)
	}
	substrate.Coverage.SurfaceCandidatesConsidered = len(substrate.SurfaceCandidates)
	substrate.Coverage.SurfaceCandidatesIndexed = len(substrate.SurfaceCandidates)
	for _, candidate := range substrate.SurfaceCandidates {
		substrate.Coverage.SurfaceCandidateFactsConsidered += len(candidate.Facts)
		substrate.Coverage.SurfaceCandidateFactsIndexed += len(candidate.Facts)
	}
	compilation, err := Compile(substrate)
	if err != nil {
		t.Fatal(err)
	}
	catalog := compilation.Request.SurfaceCatalog
	coverage := compilation.SurfaceCoverage()
	if len(catalog.Candidates) > MaxSurfaceCandidates || coverage.AdvertisedFacts > MaxSurfaceFacts ||
		surfaceCatalogSize(catalog) > MaxSurfaceCandidateSectionBytes || coverage.OmittedCandidates == 0 ||
		coverage.OmittedFacts == 0 ||
		coverage.AdvertisedCandidates+coverage.OmittedCandidates != coverage.ConsideredCandidates ||
		coverage.AdvertisedFacts+coverage.OmittedFacts != coverage.ConsideredFacts {
		t.Fatalf("bounded catalog/coverage = %d candidates, %d bytes, %+v", len(catalog.Candidates), surfaceCatalogSize(catalog), coverage)
	}
	for _, candidate := range catalog.Candidates {
		if len(candidate.Facts) > MaxSurfaceFactsPerCandidate || !candidateHasStringAndCallable(candidate) {
			t.Fatalf("candidate lost per-item structure/bound: %+v", candidate)
		}
	}
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil || len(wire) > MaxRequestBytes {
		t.Fatalf("provider wire bytes/error = %d/%v", len(wire), err)
	}
}

func TestDecodeLegacyV2ResultAndStatus(t *testing.T) {
	compilation, err := Compile(causalFixture())
	if err != nil {
		t.Fatal(err)
	}
	response := emptySurfaceResponse(compilation)
	raw, _ := json.Marshal(response)
	result, err := Reduce(compilation, raw)
	if err != nil {
		t.Fatal(err)
	}
	result.RepositoryStateSHA256 = strings.Repeat("a", 64)
	legacyResultRaw, _ := json.Marshal(legacyV2Result{
		Version: legacyV2ResultVersion, PromptVersion: legacyV2PromptVersion,
		RequestRef: result.RequestRef, RequestSHA256: result.RequestSHA256,
		SubstrateSHA256: result.SubstrateSHA256, RepositoryStateSHA256: result.RepositoryStateSHA256,
		Entries: result.Entries,
	})
	legacyResult, err := DecodeResult(legacyResultRaw)
	if err != nil || legacyResult.Version != legacyV2ResultVersion || legacyResult.Entries == nil {
		t.Fatalf("DecodeResult(v2) = %+v, %v", legacyResult, err)
	}
	legacyStatusRaw, _ := json.Marshal(legacyV2Status{
		Version: legacyV2StatusVersion, State: StatusAccepted, PromptVersion: legacyV2PromptVersion,
		RequestRef: result.RequestRef, RequestSHA256: result.RequestSHA256,
		SubstrateSHA256: result.SubstrateSHA256, RepositoryStateSHA256: result.RepositoryStateSHA256,
		ResultSHA256: strings.Repeat("b", 64), AdvertisedFamilies: compilation.AdvertisedFamilyCount(),
	})
	legacyStatus, err := DecodeStatus(legacyStatusRaw)
	if err != nil || legacyStatus.Version != legacyV2StatusVersion || legacyStatus.State != StatusAccepted {
		t.Fatalf("DecodeStatus(v2) = %+v, %v", legacyStatus, err)
	}
}

func emptySurfaceResponse(compilation Compilation) Response {
	entries := make([]ResponseEntry, 0, len(compilation.Request.Entries))
	for _, entry := range compilation.Request.Entries {
		entries = append(entries, ResponseEntry{RootRef: entry.Ref, FamilyRefs: []string{}})
	}
	return Response{
		Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
		Entries: entries, SurfaceProposals: []ResponseSurfaceProposal{},
	}
}

func candidateByForm(t *testing.T, catalog RequestSurfaceCatalog, form SurfaceCandidateForm) RequestSurfaceCandidate {
	t.Helper()
	for _, candidate := range catalog.Candidates {
		if candidate.Form == form {
			return candidate
		}
	}
	t.Fatalf("surface candidate form %q not found: %+v", form, catalog.Candidates)
	return RequestSurfaceCandidate{}
}

func candidateByFactValue(t *testing.T, catalog RequestSurfaceCatalog, value string) RequestSurfaceCandidate {
	t.Helper()
	for _, candidate := range catalog.Candidates {
		for _, fact := range candidate.Facts {
			if fact.Value == value {
				return candidate
			}
		}
	}
	t.Fatalf("surface candidate fact value %q not found: %+v", value, catalog.Candidates)
	return RequestSurfaceCandidate{}
}

func surfaceFactRefByValue(t *testing.T, candidate RequestSurfaceCandidate, value string) string {
	t.Helper()
	for _, fact := range candidate.Facts {
		if fact.Value == value {
			return fact.Ref
		}
	}
	t.Fatalf("surface fact value %q not found: %+v", value, candidate.Facts)
	return ""
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

func familyLimitFixture() Substrate {
	location := func(line int) Location { return Location{Path: "main.go", Line: line, Column: 2} }
	nodes := []ExactNode{{ID: "root", Label: "main · main", Declaration: location(1)}}
	for index := 0; index < MaxOutgoingFamiliesPerNode; index++ {
		nodes = append(nodes, ExactNode{
			ID: fmt.Sprintf("node-%02d", index), Label: fmt.Sprintf("main · node%02d", index),
			Declaration: location(10 + index),
		})
	}
	families := make([]ExactFamily, 0, MaxFamiliesPerRoot)
	for index := 0; index < MaxOutgoingFamiliesPerNode; index++ {
		families = append(families, ExactFamily{
			ID: fmt.Sprintf("root-node-%02d", index), CallerID: "root", CalleeID: nodes[index+1].ID,
			Invocation: InvocationSynchronous, WitnessCount: 1, Callsites: []Location{location(30 + index)},
		})
	}
	for callerIndex := 0; len(families) < MaxFamiliesPerRoot; callerIndex++ {
		for calleeIndex := 0; calleeIndex < MaxOutgoingFamiliesPerNode && len(families) < MaxFamiliesPerRoot; calleeIndex++ {
			families = append(families, ExactFamily{
				ID:       fmt.Sprintf("node-%02d-node-%02d", callerIndex, calleeIndex),
				CallerID: nodes[callerIndex+1].ID, CalleeID: nodes[calleeIndex+1].ID,
				Invocation: InvocationSynchronous, WitnessCount: 1,
				Callsites: []Location{location(60 + len(families))},
			})
		}
	}
	return Substrate{
		Version: SubstrateVersion, State: StateReady,
		Roots: []ExactRoot{{NodeID: "root"}}, Nodes: nodes, Families: families,
		Frontiers: []ExactFrontier{},
	}
}

func surfaceFixture() Substrate {
	substrate := causalFixture()
	location := func(path string, line int) Location { return Location{Path: path, Line: line, Column: 3} }
	substrate.SurfaceCandidates = []ExactSurfaceCandidate{
		{
			ID: "canonical:surface:route", RootNodeID: "canonical:main",
			Form: SurfaceCandidateDirectCall, Sketch: "echo GET",
			Site: location("routes.go", 18),
			Facts: []ExactSurfaceFact{
				{ID: "canonical:surface:route:method", Kind: SurfaceFactToken, Position: 0, Label: "method", Value: "GET", Location: location("routes.go", 18)},
				{ID: "canonical:surface:route:path", Kind: SurfaceFactString, Position: 1, Label: "path", Value: "/account/:id", Location: location("routes.go", 18)},
				{ID: "canonical:surface:route:handler", Kind: SurfaceFactCallable, Position: 2, Label: "handler", Value: "GetAccount", Location: location("handlers/account.go", 44)},
			},
		},
		{
			ID: "canonical:surface:command", RootNodeID: "canonical:main",
			Form: SurfaceCandidateKeyedComposite, Sketch: "command literal",
			Site: location("cmd/root.go", 21),
			Facts: []ExactSurfaceFact{
				{ID: "canonical:surface:command:identity", Kind: SurfaceFactString, Position: 0, Label: "Use", Value: "serve [path]", Location: location("cmd/root.go", 22)},
				{ID: "canonical:surface:command:handler", Kind: SurfaceFactCallable, Position: 1, Label: "RunE", Value: "runServe", Location: location("cmd/serve.go", 60)},
			},
		},
	}
	substrate.Coverage.SurfaceCandidatesConsidered = len(substrate.SurfaceCandidates)
	substrate.Coverage.SurfaceCandidatesIndexed = len(substrate.SurfaceCandidates)
	for _, candidate := range substrate.SurfaceCandidates {
		substrate.Coverage.SurfaceCandidateFactsConsidered += len(candidate.Facts)
		substrate.Coverage.SurfaceCandidateFactsIndexed += len(candidate.Facts)
	}
	return substrate
}
