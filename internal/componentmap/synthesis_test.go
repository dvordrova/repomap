package componentmap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

func TestSavedCaddyArchitectureProposalReplaysWithoutFallback(t *testing.T) {
	t.Parallel()

	bundle, canonicalResponse := caddyArchitectureReplayFixture(t)
	var canonicalProposal Proposal
	if err := json.Unmarshal(canonicalResponse, &canonicalProposal); err != nil {
		t.Fatal(err)
	}
	response := synthesisWireProposalJSON(t, bundle, canonicalProposal)
	result, err := RecordSynthesisResponse(
		bundle, "caddy-saved-run", "openai-compatible/bearer", "deepseek-v4-flash", 0, response,
	)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse() error = %v", err)
	}
	if result.Landscape.Fallback || result.Landscape.ValidationOutcome == ValidationRejected {
		t.Fatalf("saved Caddy response selected fallback: %#v", result.Landscape)
	}
	if result.Landscape.Source != SourceValidatedModel || len(result.Landscape.Subsystems) != 6 {
		t.Fatalf("source/subsystems = %q/%d", result.Landscape.Source, len(result.Landscape.Subsystems))
	}
	componentCount := 0
	for _, subsystem := range result.Landscape.Subsystems {
		componentCount += len(subsystem.Components)
		for _, component := range subsystem.Components {
			if !component.Hypothesis && len(component.AnchorIDs) == 0 {
				t.Fatalf("component %q has no supplied anchor", component.Name)
			}
		}
	}
	if componentCount != 12 {
		t.Fatalf("nested components = %d, want 12", componentCount)
	}
	wantSubsystems := []string{"Core", "Config", "Admin", "HTTP", "Security", "Entry"}
	for index, want := range wantSubsystems {
		if result.Landscape.Subsystems[index].Name != want {
			t.Fatalf("subsystem[%d] = %q, want %q", index, result.Landscape.Subsystems[index].Name, want)
		}
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplaySynthesis(bundle, "caddy-saved-run", saved)
	if err != nil {
		t.Fatalf("ReplaySynthesis() error = %v", err)
	}
	if !reflect.DeepEqual(replayed, result.Landscape) {
		t.Fatal("saved Caddy response did not replay deterministically")
	}
}

func TestSavedEtcdSizedArchitectureStructuralBridgeGate(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/etcd_architecture_parity_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version     int             `json:"version"`
		Bundle      CandidateBundle `json:"bundle"`
		SavedRecord json.RawMessage `json:"saved_record"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 {
		t.Fatalf("fixture version = %d", fixture.Version)
	}
	var saved SynthesisRecord
	if err := json.Unmarshal(fixture.SavedRecord, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Call == nil || saved.Call.ResponseState != ResponseCaptured ||
		saved.Call.Metadata.OutputTokens <= 0 || saved.Call.ResponseBytes != len(saved.Call.Response) {
		t.Fatalf("saved legacy etcd source capture is incomplete: %#v", saved.Call)
	}
	var canonicalProposal Proposal
	if err := json.Unmarshal(saved.Call.Response, &canonicalProposal); err != nil {
		t.Fatalf("decode saved etcd canonical proposal: %v", err)
	}
	response := synthesisWireProposalJSON(t, fixture.Bundle, canonicalProposal)
	request, requestJSON, err := BuildSynthesisRequest(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildSynthesisPrompt(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponseForLanguage(
		fixture.Bundle, "etcd-parity-v1", saved.Call.Metadata.Profile,
		saved.Call.Metadata.Model, "en", 0, response,
	)
	if err != nil {
		t.Fatalf("saved etcd response did not complete against its bounded shape: %v", err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected ||
		!hasLandscapeDiagnostic(result.Landscape.Diagnostics, "proposal.incomplete_member_coverage") {
		t.Fatalf("incomplete saved etcd response was not rejected closed: %#v", result.Landscape)
	}
	if !result.Membership.Counted || result.Membership.DistinctMembers >= len(fixture.Bundle.Candidates) {
		t.Fatalf("saved etcd response coverage = %#v over %d candidates", result.Membership, len(fixture.Bundle.Candidates))
	}
	requestText := string(requestJSON)
	for _, forbidden := range []string{
		`"allowed_paths"`, `"location"`, `"provenance"`, `"scenario"`,
		`"provider"`, "go.etcd.io/", "member-", "anchor-",
	} {
		if strings.Contains(requestText, forbidden) {
			t.Fatalf("etcd-sized short-ref request leaked private field/value %q", forbidden)
		}
	}
	wantMembers := make(map[string]Candidate, len(fixture.Bundle.Candidates))
	for _, candidate := range fixture.Bundle.Candidates {
		wantMembers[candidate.ID.key()] = candidate
	}
	gotMembers := make(map[string][]Candidate, len(wantMembers))
	for _, subsystem := range result.Landscape.Subsystems {
		for _, component := range subsystem.Components {
			for _, member := range component.Members {
				gotMembers[member.ID.key()] = append(gotMembers[member.ID.key()], member)
			}
		}
	}
	if len(gotMembers) != len(wantMembers) {
		t.Fatalf("accepted member cardinality = %d, want %d", len(gotMembers), len(wantMembers))
	}
	for member, want := range wantMembers {
		if len(gotMembers[member]) != 1 {
			t.Fatalf("local D177 membership %q count = %d, want exactly one", member, len(gotMembers[member]))
		}
		if !reflect.DeepEqual(gotMembers[member][0], want) {
			t.Fatalf("rejected model grouping changed exact local candidate %q", member)
		}
	}
	if !reflect.DeepEqual(result.Landscape.Relations, fixture.Bundle.Relations) {
		t.Fatal("model grouping changed exact local relations")
	}
	if !reflect.DeepEqual(result.Landscape.AnchorBindings, fixture.Bundle.AnchorBindings) {
		t.Fatal("model grouping changed exact local anchor bindings")
	}
	if result.Record.PrivateCatalogSHA256 == "" || len(result.Record.PrivateCatalogSHA256) != 64 ||
		result.Record.Call == nil || result.Record.Call.ResponseState != ResponseCaptured {
		t.Fatalf("rejected etcd structural record lacks private catalog/captured response state: %#v", result.Record)
	}
	if result.Record.Call.Metadata.OutputTokens != 0 || result.Record.Call.Metadata.ResponseComplete {
		t.Fatalf("mechanically converted response claimed live short-ref completion: %#v", result.Record.Call.Metadata)
	}
	t.Logf(
		"etcd incomplete short-ref bridge: candidates=%d local_members=%d response_distinct_members=%d anchors=%d request_json=%d prompt_bytes=%d response_bytes=%d legacy_reference_output_tokens=%d live_completion_proven=false",
		len(fixture.Bundle.Candidates), len(gotMembers), result.Membership.DistinctMembers,
		len(fixture.Bundle.BehaviorAnchors), len(requestJSON), synthesisPromptSize(prompt), len(response), saved.Call.Metadata.OutputTokens,
	)
	if len(request.Candidates) != len(fixture.Bundle.Candidates) {
		t.Fatalf("request candidates = %d, want %d", len(request.Candidates), len(fixture.Bundle.Candidates))
	}
}

func TestRejectedCaddyProposalUsesAnchorFirstFallback(t *testing.T) {
	t.Parallel()

	bundle, canonicalResponse := caddyArchitectureReplayFixture(t)
	var proposal Proposal
	if err := json.Unmarshal(canonicalResponse, &proposal); err != nil {
		t.Fatal(err)
	}
	wire := synthesisWireProposalFromCanonical(t, bundle, proposal)
	wire.Records[1].AnchorRefs = []SynthesisAnchorRef{{
		Kind: AnchorRegistryLookup, Ref: "a999",
	}}
	response, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "caddy-saved-run", "test", "test", 0, response)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.Source != SourceLocalAnchors || result.Landscape.Level != 3 ||
		result.Landscape.FallbackReason != FallbackRejectedUnknownAnchor {
		t.Fatalf("fallback ladder result = %#v", result.Landscape)
	}
	for _, subsystem := range result.Landscape.Subsystems {
		if subsystem.Name == "Packages" || subsystem.Name == "Files" || subsystem.Name == "Symbols" {
			t.Fatalf("anchor-first fallback exposed raw kind group %q", subsystem.Name)
		}
	}
}

func TestBuildSynthesisRequestIsBoundedAndPresentationNeutral(t *testing.T) {
	t.Parallel()

	firstBundle := landscapeTestBundle()
	secondBundle := landscapeTestBundle()
	for left, right := 0, len(secondBundle.Candidates)-1; left < right; left, right = left+1, right-1 {
		secondBundle.Candidates[left], secondBundle.Candidates[right] = secondBundle.Candidates[right], secondBundle.Candidates[left]
	}

	request, firstJSON, err := BuildSynthesisRequest(firstBundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest(first) error = %v", err)
	}
	_, secondJSON, err := BuildSynthesisRequest(secondBundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest(second) error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("synthesis request depends on top-level candidate order")
	}
	if len(request.Candidates) != len(firstBundle.Candidates) {
		t.Fatalf("request candidates = %d, want %d", len(request.Candidates), len(firstBundle.Candidates))
	}
	if len(request.RequiredMemberRefs) != len(request.Candidates) {
		t.Fatalf("required checklist = %d, want %d", len(request.RequiredMemberRefs), len(request.Candidates))
	}
	for index, candidate := range request.Candidates {
		if request.RequiredMemberRefs[index] != candidate.Ref {
			t.Fatalf(
				"required checklist[%d] = %#v, want candidate ref %#v",
				index, request.RequiredMemberRefs[index], candidate.Ref,
			)
		}
	}
	if len(firstJSON) > maxSynthesisRequestBytes {
		t.Fatalf("request bytes = %d, limit %d", len(firstJSON), maxSynthesisRequestBytes)
	}
	encoded := string(firstJSON)
	for _, forbidden := range []string{
		"file_tree", "coordinates", "styles", "allowed_paths", "location", "provenance", "scenario",
		"cmd-imports-repo", "cmd/root.go", "internal/repo/repo.go",
	} {
		if strings.Contains(encoded, forbidden) {
			t.Errorf("provider request leaked forbidden presentation/raw field %q: %s", forbidden, encoded)
		}
	}
	for _, candidate := range firstBundle.Candidates {
		if strings.HasPrefix(candidate.ID.Value, "member-") && strings.Contains(encoded, candidate.ID.Value) {
			t.Errorf("provider request leaked canonical member id %q", candidate.ID.Value)
		}
	}
	for _, anchor := range firstBundle.BehaviorAnchors {
		if strings.Contains(encoded, anchor.ID) {
			t.Errorf("provider request leaked canonical anchor id %q", anchor.ID)
		}
	}
	if !strings.Contains(encoded, "supporting_relations") || !strings.Contains(encoded, "flow_anchor_bindings") ||
		!strings.Contains(encoded, `"required_member_refs":[`) ||
		!strings.Contains(encoded, `"ref":{"kind":"package","ref":"p1"}`) {
		t.Fatalf("request omitted compact typed relations/bindings: %s", encoded)
	}
	prompt, err := BuildSynthesisPrompt(firstBundle)
	if err != nil {
		t.Fatalf("BuildSynthesisPrompt() error = %v", err)
	}
	if prompt.Version != SynthesisPromptVersion || prompt.OutputLanguage != "en" || !strings.Contains(prompt.User, encoded) {
		t.Fatalf("prompt is not bound to the versioned request: %#v", prompt)
	}
	for _, required := range []string{"request-local typed", "member_refs", "anchor_refs", "Do not return versions", "coordinates", "provenance"} {
		if !strings.Contains(prompt.System, required) {
			t.Errorf("synthesis instruction misses %q", required)
		}
	}
	for _, required := range []string{
		`{"records":[{"kind":"subsystem","ref":"g1"`,
		`{"kind":"component","subsystem_ref":"g1"`,
		`{"kind":"subsystem","ref":"g2"`,
		"one ordered records array",
		"prefer four to seven distinct subsystem records when the supplied evidence supports that many",
		"Tiny, library, and package-landscape requests may honestly use one to three",
		"exactly one complete JSON object",
		"Its only root field is records",
		"A subsystem record contains exactly kind, ref, name, and description",
		"A component record contains exactly kind, subsystem_ref, name, description, member_refs, anchor_refs, and hypothesis",
		"Do not nest records or emit a second root object",
		"silently validate the complete JSON syntax",
		"Every supplied candidate member ref is present in that checklist and must appear in at least one component",
		"Treat required_member_refs as the exhaustive flat coverage checklist",
		"A candidate parent_ref is grouping context only and never satisfies coverage",
		"self-check them separately by kind",
		"their exact typed set equals required_member_refs",
		"an incomplete proposal is rejected rather than repaired or supplemented locally",
	} {
		if !strings.Contains(prompt.System, required) {
			t.Errorf("synthesis nesting contract misses %q", required)
		}
	}
	if strings.Contains(prompt.System, "Omit an uncertain member") {
		t.Fatal("synthesis prompt still permits incomplete candidate coverage")
	}

	firstKey, err := SynthesisCacheKey("revision-a", firstBundle)
	if err != nil {
		t.Fatalf("SynthesisCacheKey(first) error = %v", err)
	}
	secondKey, err := SynthesisCacheKey("revision-a", secondBundle)
	if err != nil {
		t.Fatalf("SynthesisCacheKey(second) error = %v", err)
	}
	changedKey, err := SynthesisCacheKey("revision-b", firstBundle)
	if err != nil {
		t.Fatalf("SynthesisCacheKey(changed) error = %v", err)
	}
	if firstKey != secondKey || firstKey == changedKey {
		t.Fatalf("cache keys = %q, %q, changed %q", firstKey, secondKey, changedKey)
	}
	changedBundle := landscapeTestBundle()
	changedBundle.Candidates[0].Facts[0].Value += " changed"
	changedRequestKey, err := SynthesisCacheKey("revision-a", changedBundle)
	if err != nil {
		t.Fatalf("SynthesisCacheKey(changed request) error = %v", err)
	}
	if changedRequestKey == firstKey {
		t.Fatalf("cache key %q did not bind the exact synthesis request", firstKey)
	}

	defaultLanguageKey, err := SynthesisCacheKeyForProvider(
		"revision-a", firstBundle, "deepseek-compatible", "deepseek-v4-flash",
	)
	if err != nil {
		t.Fatal(err)
	}
	englishKey, err := SynthesisCacheKeyForProviderAndLanguage(
		"revision-a", firstBundle, "deepseek-compatible", "deepseek-v4-flash", "en",
	)
	if err != nil {
		t.Fatal(err)
	}
	russianKey, err := SynthesisCacheKeyForProviderAndLanguage(
		"revision-a", firstBundle, "deepseek-compatible", "deepseek-v4-flash", "ru",
	)
	if err != nil {
		t.Fatal(err)
	}
	if defaultLanguageKey != englishKey {
		t.Fatalf("default/English cache keys = %q/%q, want path compatibility", defaultLanguageKey, englishKey)
	}
	if russianKey == englishKey {
		t.Fatalf("Russian cache key %q reused English identity", russianKey)
	}
}

func TestSynthesisRequestRejectsRequiredChecklistMismatch(t *testing.T) {
	t.Parallel()

	request, _, err := BuildSynthesisRequest(landscapeTestBundle())
	if err != nil {
		t.Fatal(err)
	}
	if len(request.RequiredMemberRefs) < 2 {
		t.Fatalf("required checklist = %#v", request.RequiredMemberRefs)
	}

	tests := map[string]func(*SynthesisRequest){
		"missing": func(request *SynthesisRequest) {
			request.RequiredMemberRefs = request.RequiredMemberRefs[:len(request.RequiredMemberRefs)-1]
		},
		"duplicate": func(request *SynthesisRequest) {
			request.RequiredMemberRefs[0] = request.RequiredMemberRefs[1]
		},
		"wrong kind": func(request *SynthesisRequest) {
			if request.RequiredMemberRefs[0].Kind == MemberPackage {
				request.RequiredMemberRefs[0].Kind = MemberFile
			} else {
				request.RequiredMemberRefs[0].Kind = MemberPackage
			}
		},
		"wrong ref": func(request *SynthesisRequest) {
			request.RequiredMemberRefs[0].Ref += "-unknown"
		},
		"reordered": func(request *SynthesisRequest) {
			request.RequiredMemberRefs[0], request.RequiredMemberRefs[1] =
				request.RequiredMemberRefs[1], request.RequiredMemberRefs[0]
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			mutated := request
			mutated.RequiredMemberRefs = append([]SynthesisMemberRef(nil), request.RequiredMemberRefs...)
			mutate(&mutated)
			if err := validateSynthesisRequestCoverage(mutated); err == nil {
				t.Fatal("mismatched required checklist was accepted")
			}
		})
	}
}

func TestSynthesisResponseRequiresCompleteDistinctCandidateCoverage(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	fullRaw := validSynthesisProposalJSON(t, bundle)
	full, err := RecordSynthesisResponse(
		bundle, "complete-coverage", "test", "test", time.Millisecond, fullRaw,
	)
	if err != nil {
		t.Fatal(err)
	}
	if full.Landscape.Fallback || !full.Membership.Counted ||
		full.Membership.DistinctMembers != len(bundle.Candidates) {
		t.Fatalf("complete synthesis result = %#v", full)
	}

	wire, err := decodeSynthesisWireProposalJSON(fullRaw)
	if err != nil {
		t.Fatal(err)
	}
	component := &wire.Records[1]
	component.MemberRefs = component.MemberRefs[:len(component.MemberRefs)-1]
	partialRaw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	partial, err := RecordSynthesisResponse(
		bundle, "incomplete-coverage", "test", "test", time.Millisecond, partialRaw,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !partial.Landscape.Fallback || partial.Landscape.ValidationOutcome != ValidationRejected ||
		!hasLandscapeDiagnostic(partial.Landscape.Diagnostics, "proposal.incomplete_member_coverage") {
		t.Fatalf("incomplete synthesis was not rejected atomically: %#v", partial.Landscape)
	}
	if !partial.Membership.Counted || partial.Membership.DistinctMembers != len(bundle.Candidates)-1 ||
		partial.Membership.MemberOccurrences != len(bundle.Candidates)-1 {
		t.Fatalf("incomplete response diagnostics = %#v", partial.Membership)
	}
	if got := landscapeMemberCount(partial.Landscape); got != len(bundle.Candidates) {
		t.Fatalf("rejected synthesis changed local coverage to %d of %d", got, len(bundle.Candidates))
	}
	if !reflect.DeepEqual(partial.Landscape.Relations, bundle.Relations) ||
		!reflect.DeepEqual(partial.Landscape.AnchorBindings, bundle.AnchorBindings) {
		t.Fatal("rejected incomplete synthesis changed local facts or relations")
	}

	reorderedWire, err := decodeSynthesisWireProposalJSON(fullRaw)
	if err != nil {
		t.Fatal(err)
	}
	refs := reorderedWire.Records[1].MemberRefs
	for left, right := 0, len(refs)-1; left < right; left, right = left+1, right-1 {
		refs[left], refs[right] = refs[right], refs[left]
	}
	reorderedRaw, err := json.Marshal(reorderedWire)
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := RecordSynthesisResponse(
		bundle, "complete-coverage-reordered", "test", "test", time.Millisecond, reorderedRaw,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reordered.Landscape.Fallback ||
		!reflect.DeepEqual(full.Landscape.ConceptualMemberships, reordered.Landscape.ConceptualMemberships) {
		t.Fatal("complete conceptual membership depends on member-ref order")
	}
}

func TestSynthesisV11CacheIdentityDoesNotReuseV10Record(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	const (
		revision = "cache-contract-version"
		profile  = "deepseek-compatible"
		model    = "deepseek-v4-flash"
	)
	currentKey, err := SynthesisCacheKeyForProvider(revision, bundle, profile, model)
	if err != nil {
		t.Fatal(err)
	}
	request, requestJSON, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	// Mirror the exact v10 request struct rather than round-tripping through a
	// map, whose key order could manufacture a false cache miss.
	legacyRequestJSON, err := json.Marshal(struct {
		RepositoryArchetype RepositoryArchetype       `json:"repository_archetype"`
		GroundingMode       GroundingMode             `json:"grounding_mode"`
		BehaviorAnchors     []SynthesisBehaviorAnchor `json:"behavior_anchors,omitempty"`
		Flows               []SynthesisFlow           `json:"flows,omitempty"`
		Candidates          []SynthesisCandidate      `json:"candidates"`
		Relations           []SynthesisRelation       `json:"supporting_relations,omitempty"`
		AnchorBindings      []SynthesisAnchorBinding  `json:"flow_anchor_bindings,omitempty"`
	}{
		RepositoryArchetype: request.RepositoryArchetype,
		GroundingMode:       request.GroundingMode,
		BehaviorAnchors:     request.BehaviorAnchors,
		Flows:               request.Flows,
		Candidates:          request.Candidates,
		Relations:           request.Relations,
		AnchorBindings:      request.AnchorBindings,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(legacyRequestJSON, []byte(`"required_member_refs"`)) ||
		!bytes.Contains(requestJSON, []byte(`"required_member_refs"`)) {
		t.Fatal("v10/v11 request fixtures do not isolate the explicit coverage checklist")
	}

	// With every version field held at v11, replacing only the current wire by
	// the exact v10 wire must still change the key. This proves that the new
	// checklist bytes themselves participate in identity, independently of the
	// explicit contract and prompt version bumps below.
	v11LegacyWireHash := sha256.New()
	fmt.Fprintf(
		v11LegacyWireHash,
		"componentmap-synthesis\nrevision=%s\nrequest_contract=%d\nproposal_contract=%d\nprompt=%s\nprofile=%s\nmodel=%s\n",
		revision, SynthesisRequestVersion, ProposalVersion, SynthesisPromptVersion, profile, model,
	)
	fmt.Fprintf(
		v11LegacyWireHash, "request=%s\nprivate_catalog=%s\n",
		sha256String(legacyRequestJSON), catalog.identitySHA256,
	)
	v11LegacyWireKey := "component-synthesis-" + hex.EncodeToString(v11LegacyWireHash.Sum(nil))
	if currentKey == v11LegacyWireKey {
		t.Fatalf("v11 cache key %q did not bind required_member_refs bytes", currentKey)
	}

	legacyHash := sha256.New()
	fmt.Fprintf(
		legacyHash,
		"componentmap-synthesis\nrevision=%s\nrequest_contract=7\nproposal_contract=7\nprompt=architecture-grounding-v10\nprofile=%s\nmodel=%s\n",
		revision, profile, model,
	)
	fmt.Fprintf(
		legacyHash, "request=%s\nprivate_catalog=%s\n",
		sha256String(legacyRequestJSON), catalog.identitySHA256,
	)
	legacyKey := "component-synthesis-" + hex.EncodeToString(legacyHash.Sum(nil))
	if currentKey == legacyKey {
		t.Fatalf("v11 cache key reused v10 identity %q", currentKey)
	}

	result, err := RecordSynthesisResponse(
		bundle, revision, profile, model, time.Millisecond, validSynthesisProposalJSON(t, bundle),
	)
	if err != nil {
		t.Fatal(err)
	}
	oldRecord := result.Record
	oldRecord.Version = SynthesisRecordVersion - 1
	saved, err := json.Marshal(oldRecord)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaySynthesisResult(bundle, revision, saved); err == nil ||
		!strings.Contains(err.Error(), "unsupported synthesis record version") {
		t.Fatalf("old synthesis record replay error = %v", err)
	}
}

func TestBuildSynthesisRequestKeepsCanonicalFlowIDPrivate(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	oldFlowID := bundle.Flows[0].ID
	privateFlowID := FlowID("flow-canonical-private-7f8fce")
	bundle.Flows[0].ID = privateFlowID
	for candidateIndex := range bundle.Candidates {
		for participationIndex := range bundle.Candidates[candidateIndex].Participations {
			participation := &bundle.Candidates[candidateIndex].Participations[participationIndex]
			if participation.FlowID == oldFlowID {
				participation.FlowID = privateFlowID
				participation.Evidence.Value = string(privateFlowID)
			}
		}
		for factIndex := range bundle.Candidates[candidateIndex].Facts {
			fact := &bundle.Candidates[candidateIndex].Facts[factIndex]
			if fact.Kind == FactFlowParticipation && fact.Value == string(oldFlowID) {
				fact.Value = string(privateFlowID)
			}
		}
	}
	for bindingIndex := range bundle.AnchorBindings {
		if bundle.AnchorBindings[bindingIndex].FlowID == oldFlowID {
			bundle.AnchorBindings[bindingIndex].FlowID = privateFlowID
		}
	}

	request, encoded, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(privateFlowID)) {
		t.Fatalf("provider request leaked canonical flow id %q: %s", privateFlowID, encoded)
	}
	if len(request.Flows) != 1 || request.Flows[0].Ref != "q1" || request.Flows[0].Label != "Backup" {
		t.Fatalf("request-local flow projection = %#v", request.Flows)
	}
}

func TestBuildSynthesisRequestRejectsUnknownBindingAnchorBeforeProjection(t *testing.T) {
	t.Parallel()

	bundle := adversarialPrivateIdentitySynthesisBundle()
	baseline, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatalf("valid exact-anchor fixture: %v", err)
	}
	if len(baseline.AnchorBindings) != 1 || baseline.AnchorBindings[0].AnchorRef.Ref == "" {
		t.Fatalf("valid binding projection = %#v", baseline.AnchorBindings)
	}

	bundle.AnchorBindings[0].AnchorID = "cobalt-unregistered-anchor-91f2"
	request, encoded, err := BuildSynthesisRequest(bundle)
	if err == nil || !strings.Contains(err.Error(), "unknown behavior anchor") {
		t.Fatalf("unknown binding anchor error = %v", err)
	}
	if !reflect.DeepEqual(request, SynthesisRequest{}) || encoded != nil {
		t.Fatalf("unknown anchor reached model wire: request=%#v encoded=%s", request, encoded)
	}
	key, err := SynthesisCacheKey("revision-private-anchor", bundle)
	if err == nil || key != "" {
		t.Fatalf("unknown anchor reached cache identity: key=%q error=%v", key, err)
	}
}

func TestBuildSynthesisRequestHidesArbitraryCanonicalIDsWithoutRewritingOrdinaryProse(t *testing.T) {
	t.Parallel()

	bundle := adversarialPrivateIdentitySynthesisBundle()
	request, encoded, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	privateValues := []string{
		bundle.Candidates[0].ID.Value,
		bundle.Candidates[1].ID.Value,
		bundle.BehaviorAnchors[0].ID,
		string(bundle.Flows[0].ID),
	}
	for _, privateValue := range privateValues {
		if bytes.Contains(encoded, []byte(privateValue)) {
			t.Fatalf("provider wire leaked arbitrary canonical identity %q: %s", privateValue, encoded)
		}
	}

	ordinaryLabelFound := false
	embeddedCandidateLabelFound := false
	embeddedFactLabelFound := false
	for _, candidate := range request.Candidates {
		if candidate.Label == "member-facing helper" {
			ordinaryLabelFound = true
		}
		if candidate.Label == "runtime" {
			embeddedCandidateLabelFound = true
		}
		for _, fact := range candidate.Facts {
			if fact.Label == "handles safely" {
				embeddedFactLabelFound = true
			}
			for _, privateValue := range privateValues {
				if slicesContainExactOpaqueToken(fact.Label, privateValue) {
					t.Fatalf("candidate fact label leaked canonical identity %q", privateValue)
				}
			}
		}
	}
	if !ordinaryLabelFound {
		t.Fatalf("identity-aware projection rewrote ordinary member-prefixed prose: %#v", request.Candidates)
	}
	if !embeddedCandidateLabelFound || !embeddedFactLabelFound {
		t.Fatalf("embedded canonical tokens were not removed exactly: %#v", request.Candidates)
	}
	if len(request.Flows) != 1 || request.Flows[0].Label != "startup" {
		t.Fatalf("embedded canonical flow label projection = %#v", request.Flows)
	}

	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	nearMatch := bundle.Candidates[0].ID.Value + "-suffix"
	if got := synthesisSemanticLabel(catalog, MemberSymbol, nearMatch); got != nearMatch {
		t.Fatalf("exact token sanitizer rewrote non-identical prose %q as %q", nearMatch, got)
	}
}

func TestBuildSynthesisRequestAllocatesCollisionFreeDeterministicRefs(t *testing.T) {
	t.Parallel()

	bundle := requestLocalRefCollisionSynthesisBundle()
	request, encoded, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	privateValues := map[string]struct{}{
		bundle.Candidates[0].ID.Value: {},
		bundle.Candidates[1].ID.Value: {},
		bundle.BehaviorAnchors[0].ID:  {},
		string(bundle.Flows[0].ID):    {},
	}
	for _, field := range synthesisRequestIdentityFields(request) {
		if _, collision := privateValues[field.ref]; collision {
			t.Fatalf("wire identity %s reused private canonical value %q: %s", field.name, field.ref, encoded)
		}
	}
	if got := []string{request.Candidates[0].Ref.Ref, request.Candidates[1].Ref.Ref}; !reflect.DeepEqual(got, []string{"p3", "p4"}) {
		t.Fatalf("member refs = %v, want collision-free [p3 p4]", got)
	}
	if len(request.BehaviorAnchors) != 1 || request.BehaviorAnchors[0].Ref.Ref != "a2" ||
		len(request.Flows) != 1 || request.Flows[0].Ref != "q2" {
		t.Fatalf("anchor/flow refs did not skip private identities: anchors=%#v flows=%#v", request.BehaviorAnchors, request.Flows)
	}
	for _, forbidden := range []string{`"ref":"p1"`, `"ref":"p2"`, `"ref":"a1"`, `"flow_ref":"q1"`} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("wire reused private canonical identity field %q: %s", forbidden, encoded)
		}
	}

	reordered := bundle
	reordered.Candidates = append([]Candidate(nil), bundle.Candidates...)
	reordered.Candidates[0], reordered.Candidates[1] = reordered.Candidates[1], reordered.Candidates[0]
	_, reorderedJSON, err := BuildSynthesisRequest(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, reorderedJSON) {
		t.Fatalf("collision-free refs depend on candidate input order:\nfirst=%s\nsecond=%s", encoded, reorderedJSON)
	}
	firstKey, err := SynthesisCacheKey("collision-revision", bundle)
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := SynthesisCacheKey("collision-revision", reordered)
	if err != nil {
		t.Fatal(err)
	}
	if firstKey != secondKey {
		t.Fatalf("deterministic private catalog/cache identity changed across order: %q != %q", firstKey, secondKey)
	}
}

func TestSynthesisRequestIdentityScanRejectsExactCanonicalWireRefWithoutEcho(t *testing.T) {
	t.Parallel()

	catalog := synthesisPrivateCatalog{canonicalOpaqueIDs: map[string]struct{}{"p1": {}}}
	request := SynthesisRequest{Candidates: []SynthesisCandidate{{
		Ref: SynthesisMemberRef{Kind: MemberPackage, Ref: "p1"}, Label: "runtime",
	}}}
	err := validateSynthesisRequestIdentityFields(catalog, request)
	if err == nil || !strings.Contains(err.Error(), "collides with a private canonical identity") {
		t.Fatalf("exact canonical wire ref error = %v", err)
	}
	if strings.Contains(err.Error(), "p1") {
		t.Fatalf("identity collision error echoed private identity: %v", err)
	}
}

func TestBuildSynthesisPromptLanguageKeepsEnglishBytesAndScopesRussianProse(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	canonicalEnglish, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	explicitEnglish, err := BuildSynthesisPromptForLanguage(bundle, "en")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonicalEnglish, explicitEnglish) {
		t.Fatalf("explicit English prompt changed canonical bytes:\ncanonical=%#v\nexplicit=%#v", canonicalEnglish, explicitEnglish)
	}

	russian, err := BuildSynthesisPromptForLanguage(bundle, "ru")
	if err != nil {
		t.Fatal(err)
	}
	if russian.Version != canonicalEnglish.Version || russian.User != canonicalEnglish.User ||
		russian.OutputLanguage != "ru" || canonicalEnglish.OutputLanguage != "en" {
		t.Fatalf("Russian prompt changed versioned facts request: %#v", russian)
	}
	if russian.System == canonicalEnglish.System ||
		!strings.Contains(russian.System, "name and description prose in Russian") ||
		!strings.Contains(russian.System, "Preserve technical identifiers") {
		t.Fatalf("Russian prompt has no narrow prose instruction: %q", russian.System)
	}
	if !strings.Contains(canonicalEnglish.System, "name and description prose in English") ||
		strings.Contains(canonicalEnglish.System, "prose in Russian") {
		t.Fatal("canonical English prompt does not own an explicit English prose contract")
	}
	if _, err := BuildSynthesisPromptForLanguage(bundle, "fr"); err == nil {
		t.Fatal("unsupported synthesis language was accepted")
	}
}

func adversarialPrivateIdentitySynthesisBundle() CandidateBundle {
	primaryID := MemberID{Kind: MemberPackage, Value: "quartz-private-member-91f2"}
	ordinaryID := MemberID{Kind: MemberPackage, Value: "cobalt-private-member-84d7"}
	flowID := FlowID("violet-private-flow-62ac")
	anchorID := "amber-private-anchor-07bd"
	location := evidence.Location{Path: "main.go", Line: 12, Column: 1}
	return CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeApplication, GroundingMode: GroundingMixed,
		BehaviorAnchors: []BehaviorAnchor{{
			ID: anchorID, Kind: AnchorProcessEntry, Label: "application entry",
			Location: location, Scenario: ScenarioContext{ID: "go:test", Name: "test build"},
			Producer: evidence.Provenance{
				Provider: "fixture", Version: "v1", Operation: "classify_process_entry",
			},
			Certainty: evidence.CertaintyStatic, MemberIDs: []MemberID{primaryID},
			Limitations: []string{"Static fixture evidence; runtime execution is not observed."},
		}},
		Candidates: []Candidate{
			{
				ID: primaryID, Name: "runtime " + primaryID.Value,
				Participations: []FlowParticipation{testFlowParticipation(flowID, "main.go", 12)},
				Facts: []LocalFact{
					testLocalFact(FactDeclaration, "handles "+anchorID+" safely", "main.go", 12),
					testLocalFact(FactExecutableRole, ordinaryID.Value, "main.go", 12),
					testLocalFact(FactContainment, string(flowID), "main.go", 12),
				},
			},
			{
				ID: ordinaryID, Name: "member-facing helper",
				Facts: []LocalFact{testLocalFact(FactDeclaration, "ordinary helper declaration", "helper.go", 4)},
			},
		},
		Flows: []Flow{{
			ID: flowID, Name: "startup " + primaryID.Value,
			Facts: []LocalFact{testLocalFact(FactDeclaration, anchorID, "main.go", 12)},
		}},
		AnchorBindings: []FlowAnchorBinding{{
			FlowID: flowID, AnchorID: anchorID, MemberID: primaryID,
			Location: &location, Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider: "fixture", Version: "v1", Operation: "bind_flow_anchor",
			}},
		}},
	}
}

func requestLocalRefCollisionSynthesisBundle() CandidateBundle {
	primaryID := MemberID{Kind: MemberPackage, Value: "p1"}
	secondaryID := MemberID{Kind: MemberPackage, Value: "p2"}
	flowID := FlowID("q1")
	anchorID := "a1"
	location := evidence.Location{Path: "main.go", Line: 12, Column: 1}
	return CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeApplication, GroundingMode: GroundingMixed,
		BehaviorAnchors: []BehaviorAnchor{{
			ID: anchorID, Kind: AnchorProcessEntry, Label: "application entry",
			Location: location, Scenario: ScenarioContext{ID: "go:test", Name: "test build"},
			Producer: evidence.Provenance{
				Provider: "fixture", Version: "v1", Operation: "classify_process_entry",
			},
			Certainty: evidence.CertaintyStatic, MemberIDs: []MemberID{primaryID},
			Limitations: []string{"Static fixture evidence; runtime execution is not observed."},
		}},
		Candidates: []Candidate{
			{
				ID: primaryID, Name: "runtime",
				Participations: []FlowParticipation{testFlowParticipation(flowID, "main.go", 12)},
				Facts:          []LocalFact{testLocalFact(FactDeclaration, "runtime", "main.go", 12)},
			},
			{
				ID: secondaryID, Name: "storage", ParentID: &primaryID,
				Facts: []LocalFact{testLocalFact(FactDeclaration, "storage", "storage.go", 4)},
			},
		},
		Flows: []Flow{{
			ID: flowID, Name: "startup",
			Facts: []LocalFact{testLocalFact(FactDeclaration, "startup", "main.go", 12)},
		}},
		Relations: []LocalRelation{{
			ID: "runtime-owns-storage", From: primaryID, To: secondaryID,
			Kind: StructuralRelationPackageImport, Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider: "fixture", Version: "v1", Operation: "relate_members",
			}},
			Scenarios: []ScenarioContext{{ID: "go:test", Name: "test build"}},
		}},
		AnchorBindings: []FlowAnchorBinding{{
			FlowID: flowID, AnchorID: anchorID, MemberID: primaryID,
			Location: &location, Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider: "fixture", Version: "v1", Operation: "bind_flow_anchor",
			}},
		}},
	}
}

func slicesContainExactOpaqueToken(value, opaque string) bool {
	for _, field := range strings.Fields(value) {
		if field == opaque {
			return true
		}
	}
	return false
}

func TestPrivateCatalogSeparatesEqualShortWiresAndReplay(t *testing.T) {
	t.Parallel()

	first := minimalPackageSynthesisBundle("member-package-first-private-id")
	second := minimalPackageSynthesisBundle("member-package-second-private-id")
	_, firstWire, err := BuildSynthesisRequest(first)
	if err != nil {
		t.Fatal(err)
	}
	_, secondWire, err := BuildSynthesisRequest(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstWire, secondWire) {
		t.Fatalf("semantic short wire changed with private canonical mapping:\nfirst=%s\nsecond=%s", firstWire, secondWire)
	}
	firstKey, err := SynthesisCacheKeyForProviderAndLanguage("same-revision", first, "test", "test", "en")
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := SynthesisCacheKeyForProviderAndLanguage("same-revision", second, "test", "test", "en")
	if err != nil {
		t.Fatal(err)
	}
	if firstKey == secondKey {
		t.Fatal("equal provider wire reused a cache key across different private canonical mappings")
	}
	response := validSynthesisProposalJSON(t, first)
	result, err := RecordSynthesisResponse(first, "same-revision", "test", "test", time.Millisecond, response)
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.PrivateCatalogSHA256 == "" {
		t.Fatal("saved synthesis record omitted private catalog identity")
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaySynthesis(second, "same-revision", saved); err == nil ||
		!strings.Contains(err.Error(), "private catalog") {
		t.Fatalf("replay with substituted private mapping error = %v", err)
	}
}

func TestSynthesisResponseRejectsNonExactMemberRefs(t *testing.T) {
	t.Parallel()

	bundle := minimalPackageSynthesisBundle("member-package-private-canonical")
	base := synthesisWireProposalFromCanonical(t, bundle, validSynthesisProposal(bundle))
	tests := []struct {
		name string
		ref  SynthesisMemberRef
	}{
		{name: "unknown", ref: SynthesisMemberRef{Kind: MemberPackage, Ref: "p9"}},
		{name: "prefix", ref: SynthesisMemberRef{Kind: MemberPackage, Ref: "p"}},
		{name: "raw canonical", ref: SynthesisMemberRef{Kind: MemberPackage, Ref: bundle.Candidates[0].ID.Value}},
		{name: "wrong kind", ref: SynthesisMemberRef{Kind: MemberSymbol, Ref: "p1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			wire := base
			wire.Records = append([]synthesisWireRecord(nil), base.Records...)
			wire.Records[1].MemberRefs = []SynthesisMemberRef{test.ref}
			response, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			result, err := RecordSynthesisResponse(bundle, "revision-ref", "test", "test", time.Millisecond, response)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Landscape.Fallback || result.Landscape.FallbackReason != FallbackRejectedUnknownMember ||
				!hasLandscapeDiagnostic(result.Landscape.Diagnostics, "proposal.unknown_member_id") {
				t.Fatalf("non-exact ref was not rejected closed: %#v", result.Landscape)
			}
		})
	}

	rawCanonical := []byte(`{"records":[{"kind":"subsystem","ref":"g1","name":"Repository","description":""},{"kind":"component","subsystem_ref":"g1","name":"Object","description":"","member_ids":[{"kind":"package","value":"member-package-private-canonical"}],"anchor_refs":[],"hypothesis":true}]}`)
	result, err := RecordSynthesisResponse(bundle, "revision-raw", "test", "test", time.Millisecond, rawCanonical)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected {
		t.Fatalf("legacy canonical provider shape entered active synthesis: %#v", result.Landscape)
	}
}

func TestSynthesisResponseRejectsForbiddenBackendIdentityFields(t *testing.T) {
	t.Parallel()

	bundle := minimalPackageSynthesisBundle("member-package-private-canonical")
	valid := synthesisWireProposalFromCanonical(t, bundle, validSynthesisProposal(bundle))
	validJSON, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(validJSON, &root); err != nil {
		t.Fatal(err)
	}
	root["version"] = ProposalVersion
	records := root["records"].([]any)
	records[1].(map[string]any)["member_ids"] = []any{map[string]any{
		"kind": "package", "value": bundle.Candidates[0].ID.Value,
	}}
	mixed, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "revision-mixed", "test", "test", time.Millisecond, mixed)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected ||
		!hasLandscapeDiagnostic(result.Landscape.Diagnostics, "response.invalid_proposal") {
		t.Fatalf("mixed short-ref/backend response was not rejected closed: %#v", result.Landscape)
	}
}

func TestFreshCasdoorSharedMemberRefIsAcceptedManyToMany(t *testing.T) {
	t.Parallel()

	bundle := CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeApplication, GroundingMode: GroundingPackages,
		Candidates: []Candidate{
			{ID: MemberID{Kind: MemberPackage, Value: "member-package-3c4e406309b6c4ce0e8eb848"}, Name: "object", Facts: []LocalFact{testLocalFact(FactDeclaration, "object", "object/object.go", 1)}},
			{ID: MemberID{Kind: MemberPackage, Value: "member-package-2c3c1568bf99db9806ef2f8e"}, Name: "certificate", Facts: []LocalFact{testLocalFact(FactDeclaration, "certificate", "certificate/certificate.go", 1)}},
		},
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	objectRef := catalog.membersByID[bundle.Candidates[0].ID]
	certificateRef := catalog.membersByID[bundle.Candidates[1].ID]
	wire := synthesisWireProposal{Records: []synthesisWireRecord{
		{Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Security and Identity"},
		{Kind: synthesisWireComponentRecord, SubsystemRef: "g1", Name: "Certificate Management", MemberRefs: []SynthesisMemberRef{objectRef, certificateRef}, Hypothesis: true},
		{Kind: synthesisWireSubsystemRecord, Ref: "g2", Name: "Domain Objects"},
		{Kind: synthesisWireComponentRecord, SubsystemRef: "g2", Name: "Object Model", MemberRefs: []SynthesisMemberRef{objectRef}, Hypothesis: true},
	}}
	response, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "casdoor-20260802-133147", "test", "test", time.Millisecond, response)
	if err != nil {
		t.Fatal(err)
	}
	if result.Landscape.Fallback || result.Landscape.ValidationOutcome == ValidationRejected {
		t.Fatalf("fresh Casdoor shared membership was rejected: %#v", result.Landscape)
	}
	if !result.Membership.Counted || result.Membership.MemberOccurrences != 3 ||
		result.Membership.DistinctMembers != 2 {
		t.Fatalf("resolved membership counts = %#v", result.Membership)
	}
	shared := 0
	for _, membership := range result.Landscape.ConceptualMemberships {
		if membership.MemberID == bundle.Candidates[0].ID {
			shared++
		}
	}
	if shared != 2 {
		t.Fatalf("shared conceptual relations = %d, want 2", shared)
	}
	if !reflect.DeepEqual(result.Landscape.Relations, bundle.Relations) {
		t.Fatal("rejected duplicate changed local relations")
	}
}

func TestSynthesisWireRejectsOverBoundComponentsBeforeNormalization(t *testing.T) {
	t.Parallel()

	bundle := CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeApplication, GroundingMode: GroundingPackages,
	}
	for index := 0; index < 10; index++ {
		path := fmt.Sprintf("package-%02d/package.go", index)
		bundle.Candidates = append(bundle.Candidates, Candidate{
			ID:   MemberID{Kind: MemberPackage, Value: fmt.Sprintf("package-%02d", index)},
			Name: fmt.Sprintf("package-%02d", index),
			Facts: []LocalFact{
				testLocalFact(FactDeclaration, fmt.Sprintf("example/package-%02d", index), path, 1),
			},
		})
	}
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	components := make([]synthesisWireRecord, MaxComponentsPerSubsystem+1)
	for index := 0; index < MaxComponentsPerSubsystem-1; index++ {
		components[index] = synthesisWireRecord{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g1",
			Name:       fmt.Sprintf("Responsibility %02d", index),
			MemberRefs: []SynthesisMemberRef{catalog.membersByID[bundle.Candidates[index].ID]},
		}
	}
	sharedRef := catalog.membersByID[bundle.Candidates[9].ID]
	components[7] = synthesisWireRecord{
		Kind: synthesisWireComponentRecord, SubsystemRef: "g1", Name: "Cross-cut A",
		MemberRefs: []SynthesisMemberRef{
			catalog.membersByID[bundle.Candidates[7].ID], sharedRef,
		},
	}
	components[8] = synthesisWireRecord{
		Kind: synthesisWireComponentRecord, SubsystemRef: "g1", Name: "Cross-cut B",
		MemberRefs: []SynthesisMemberRef{
			catalog.membersByID[bundle.Candidates[8].ID], sharedRef,
		},
	}
	records := append([]synthesisWireRecord{{
		Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Repository",
	}}, components...)
	response, err := json.Marshal(synthesisWireProposal{Records: records})
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(
		bundle, "normalized-membership-counts", "test", "test", time.Millisecond, response,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected ||
		!hasLandscapeDiagnostic(result.Landscape.Diagnostics, "response.invalid_proposal") {
		t.Fatalf("over-bound synthesis result = %#v", result.Landscape)
	}
	if result.Membership.Counted || result.Membership.MemberOccurrences != 0 || result.Membership.DistinctMembers != 0 {
		t.Fatalf("over-bound response was partially counted: %#v", result.Membership)
	}
	if result.Record.Call == nil || result.Record.Call.Metadata.MembershipCounted ||
		result.Record.Call.Metadata.MemberOccurrences != 0 || result.Record.Call.Metadata.DistinctMembers != 0 {
		t.Fatalf("saved over-bound membership counts = %#v", result.Record.Call)
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplaySynthesisResult(bundle, "normalized-membership-counts", saved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replayed.Membership, result.Membership) {
		t.Fatalf("replayed membership counts = %#v, want %#v", replayed.Membership, result.Membership)
	}
}

func TestSavedCasdoorP21ManyToManyResponseIsRejectedWhenCoverageIsIncomplete(t *testing.T) {
	t.Parallel()

	legacyRaw, err := os.ReadFile("testdata/casdoor_architecture_many_to_many_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	bundle := savedCasdoorManyToManyBundle()
	legacy, err := RecordSynthesisResponseForLanguage(
		bundle, "casdoor-many-to-many-legacy-v1", "openai-compatible/bearer",
		"deepseek-v4-flash", "ru", time.Millisecond, legacyRaw,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.Landscape.Fallback || legacy.Landscape.ValidationOutcome != ValidationRejected ||
		!hasLandscapeDiagnostic(legacy.Landscape.Diagnostics, "response.invalid_proposal") || legacy.Membership.Counted {
		t.Fatalf("old nested response was reinterpreted under the records contract: %#v", legacy)
	}

	raw, err := os.ReadFile("testdata/casdoor_architecture_many_to_many_records_v2.json")
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponseForLanguage(
		bundle, "casdoor-many-to-many-records-v2", "openai-compatible/bearer",
		"deepseek-v4-flash", "ru", time.Millisecond, raw,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected ||
		!hasLandscapeDiagnostic(result.Landscape.Diagnostics, "proposal.incomplete_member_coverage") {
		t.Fatalf("incomplete saved Casdoor response was not rejected: %#v", result.Landscape)
	}
	if !result.Membership.Counted || result.Membership.MemberOccurrences != 29 ||
		result.Membership.DistinctMembers != 28 {
		t.Fatalf("saved Casdoor membership counts = %#v", result.Membership)
	}
	if result.Record.Call == nil || !result.Record.Call.Metadata.MembershipCounted ||
		result.Record.Call.Metadata.MemberOccurrences != 29 ||
		result.Record.Call.Metadata.DistinctMembers != 28 {
		t.Fatalf("saved Casdoor record counts = %#v", result.Record.Call)
	}
	if !reflect.DeepEqual(result.Landscape.Relations, bundle.Relations) {
		t.Fatal("rejected saved Casdoor grouping changed exact local relations")
	}

	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplaySynthesisResult(bundle, "casdoor-many-to-many-records-v2", saved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replayed.Membership, result.Membership) ||
		!reflect.DeepEqual(replayed.Landscape, result.Landscape) {
		t.Fatal("saved Casdoor many-to-many result did not replay exactly")
	}
	tampered := result.Record
	tamperedCall := *tampered.Call
	tamperedCall.Metadata.MemberOccurrences++
	tampered.Call = &tamperedCall
	tamperedSaved, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaySynthesisResult(bundle, "casdoor-many-to-many-records-v2", tamperedSaved); err == nil ||
		!strings.Contains(err.Error(), "membership counts do not replay") {
		t.Fatalf("tampered membership count replay error = %v", err)
	}

}

func TestSavedCasdoorD202ResponseKeepsFileOmissionRejectedWithExplicitChecklist(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/casdoor_architecture_20260802_215721_incomplete_response.json")
	if err != nil {
		t.Fatal(err)
	}
	raw := bytes.TrimSpace(fixture)
	if len(raw) != 5334 {
		t.Fatalf("saved D202 response bytes = %d, want 5334", len(raw))
	}
	digest := sha256.Sum256(raw)
	if got := hex.EncodeToString(digest[:]); got != "c8d9e96c02d2d45d0402d31b6b883c130535e6171d8d186f1c5c4b538c39f63b" {
		t.Fatalf("saved D202 response sha256 = %s", got)
	}

	bundle := savedCasdoorManyToManyBundle()
	request, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.RequiredMemberRefs) != 50 {
		t.Fatalf("required checklist = %d, want 50", len(request.RequiredMemberRefs))
	}
	checklistKinds := map[MemberKind]int{}
	for index, ref := range request.RequiredMemberRefs {
		checklistKinds[ref.Kind]++
		if ref != request.Candidates[index].Ref {
			t.Fatalf("required checklist[%d] = %#v, candidate ref = %#v", index, ref, request.Candidates[index].Ref)
		}
	}
	if checklistKinds[MemberPackage] != 34 || checklistKinds[MemberSymbol] != 8 ||
		checklistKinds[MemberFile] != 8 {
		t.Fatalf("required checklist kinds = %#v", checklistKinds)
	}

	result, err := RecordSynthesisResponseForLanguage(
		bundle, "casdoor-d202-incomplete", "openai-compatible/bearer",
		"deepseek-v4-flash", "ru", time.Millisecond, raw,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected ||
		!hasLandscapeDiagnostic(result.Landscape.Diagnostics, "proposal.incomplete_member_coverage") {
		t.Fatalf("saved D202 file omission was not rejected closed: %#v", result.Landscape)
	}
	if !result.Membership.Counted || result.Membership.MemberOccurrences != 42 ||
		result.Membership.DistinctMembers != 42 {
		t.Fatalf("saved D202 membership counts = %#v", result.Membership)
	}
	savedRejected, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	replayedRejected, err := ReplaySynthesisResult(bundle, "casdoor-d202-incomplete", savedRejected)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(replayedRejected.Record.Call.Response, raw) ||
		!reflect.DeepEqual(replayedRejected.Membership, result.Membership) ||
		!reflect.DeepEqual(replayedRejected.Landscape, result.Landscape) {
		t.Fatal("saved D202 42/50 response did not replay byte-exactly")
	}

	completeProposal := validSynthesisProposal(bundle)
	completeProposal.Subsystems[0].Components[0].Hypothesis = true
	completeRaw := synthesisWireProposalJSON(t, bundle, completeProposal)
	complete, err := RecordSynthesisResponseForLanguage(
		bundle, "casdoor-d203-complete", "openai-compatible/bearer",
		"deepseek-v4-flash", "ru", time.Millisecond, completeRaw,
	)
	if err != nil {
		t.Fatal(err)
	}
	if complete.Landscape.Fallback || !complete.Membership.Counted ||
		complete.Membership.DistinctMembers != len(request.RequiredMemberRefs) {
		t.Fatalf(
			"synthetic full-checklist response: fallback=%t outcome=%s membership=%#v diagnostics=%#v",
			complete.Landscape.Fallback, complete.Landscape.ValidationOutcome,
			complete.Membership, complete.Landscape.Diagnostics,
		)
	}
	savedComplete, err := json.Marshal(complete.Record)
	if err != nil {
		t.Fatal(err)
	}
	replayedComplete, err := ReplaySynthesisResult(bundle, "casdoor-d203-complete", savedComplete)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(replayedComplete.Record.Call.Response, completeRaw) ||
		!reflect.DeepEqual(replayedComplete.Membership, complete.Membership) ||
		!reflect.DeepEqual(replayedComplete.Landscape, complete.Landscape) {
		t.Fatal("synthetic D203 50/50 response did not replay byte-exactly")
	}
}

func savedCasdoorManyToManyBundle() CandidateBundle {
	bundle := CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeApplication, GroundingMode: GroundingMixed,
	}
	for index := 1; index <= 8; index++ {
		id := MemberID{Kind: MemberFile, Value: fmt.Sprintf("casdoor-file-%02d", index)}
		bundle.Candidates = append(bundle.Candidates, Candidate{
			ID: id, Name: fmt.Sprintf("file-%02d.go", index),
			Facts: []LocalFact{testLocalFact(FactRepositoryPath, fmt.Sprintf("file-%02d.go", index), fmt.Sprintf("fixture/file-%02d.go", index), 1)},
		})
	}
	for index := 1; index <= 34; index++ {
		id := MemberID{Kind: MemberPackage, Value: fmt.Sprintf("casdoor-package-%02d", index)}
		bundle.Candidates = append(bundle.Candidates, Candidate{
			ID: id, Name: fmt.Sprintf("package-%02d", index),
			Facts: []LocalFact{testLocalFact(FactDeclaration, fmt.Sprintf("package-%02d", index), fmt.Sprintf("fixture/package-%02d.go", index), 1)},
		})
	}
	for index := 1; index <= 8; index++ {
		id := MemberID{Kind: MemberSymbol, Value: fmt.Sprintf("casdoor-symbol-%02d", index)}
		bundle.Candidates = append(bundle.Candidates, Candidate{
			ID: id, Name: fmt.Sprintf("symbol-%02d", index),
			Facts: []LocalFact{testLocalFact(FactDeclaration, fmt.Sprintf("symbol-%02d", index), fmt.Sprintf("fixture/symbol-%02d.go", index), 1)},
		})
	}
	anchorKinds := []BehaviorAnchorKind{
		AnchorProcessEntry, AnchorSecurityBoundary, AnchorLifecycleInterface,
		AnchorLifecycleStart, AnchorLifecycleStart,
	}
	anchorMembers := [][]int{{1}, {4}, {3}, {5}, {2, 5, 6, 7, 8}}
	for index, kind := range anchorKinds {
		location := evidence.Location{Path: "fixture/anchors.go", Line: index + 1, Column: 1}
		members := make([]MemberID, 0, len(anchorMembers[index]))
		for _, ordinal := range anchorMembers[index] {
			members = append(members, MemberID{Kind: MemberSymbol, Value: fmt.Sprintf("casdoor-symbol-%02d", ordinal)})
		}
		bundle.BehaviorAnchors = append(bundle.BehaviorAnchors, BehaviorAnchor{
			ID: fmt.Sprintf("casdoor-anchor-%02d", index+1), Kind: kind, Label: string(kind),
			Location: location, Scenario: ScenarioContext{ID: "go:fixture", Name: "saved Casdoor fixture"},
			Producer: evidence.Provenance{
				Provider: "saved_casdoor_run", Version: "20260802-155159",
				Operation: "replay_architecture_anchor", Location: &location,
			},
			Certainty: evidence.CertaintyStatic, MemberIDs: members,
			Limitations: []string{"Saved deterministic fixture evidence; runtime execution is not implied."},
		})
	}
	relationProvenance := []evidence.Provenance{{
		Provider: "saved_casdoor_run", Version: "20260802-155159", Operation: "replay_architecture_relation",
	}}
	scenario := []ScenarioContext{{ID: "go:fixture", Name: "saved Casdoor fixture"}}
	bundle.Relations = []LocalRelation{
		{
			ID: "casdoor-relation-1", From: MemberID{Kind: MemberSymbol, Value: "casdoor-symbol-01"},
			To: MemberID{Kind: MemberSymbol, Value: "casdoor-symbol-04"}, Kind: StructuralRelationBehaviorHandoff,
			Certainty: evidence.CertaintyStatic, Provenance: relationProvenance, Scenarios: scenario,
		},
		{
			ID: "casdoor-relation-2", From: MemberID{Kind: MemberSymbol, Value: "casdoor-symbol-01"},
			To: MemberID{Kind: MemberSymbol, Value: "casdoor-symbol-05"}, Kind: StructuralRelationBehaviorHandoff,
			Certainty: evidence.CertaintyStatic, Provenance: relationProvenance, Scenarios: scenario,
		},
	}
	return bundle
}

func TestAnchorRefsRejectWithinComponentDuplicateButAllowSharedContext(t *testing.T) {
	t.Parallel()

	bundle := groundedTwoMemberSynthesisBundle()
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	anchorRef := catalog.anchorsByID[bundle.BehaviorAnchors[0].ID]
	firstRef := catalog.membersByID[bundle.Candidates[0].ID]
	secondRef := catalog.membersByID[bundle.Candidates[1].ID]
	duplicate := synthesisWireProposal{Records: []synthesisWireRecord{
		{Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Runtime"},
		{Kind: synthesisWireComponentRecord, SubsystemRef: "g1", Name: "Duplicate anchor", MemberRefs: []SynthesisMemberRef{firstRef}, AnchorRefs: []SynthesisAnchorRef{anchorRef, anchorRef}},
	}}
	response, err := json.Marshal(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "duplicate-anchor", "test", "test", time.Millisecond, response)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected {
		t.Fatalf("duplicate anchor ref within one component was accepted: %#v", result.Landscape)
	}
	for _, test := range []struct {
		name string
		ref  SynthesisAnchorRef
	}{
		{name: "unknown", ref: SynthesisAnchorRef{Kind: AnchorProcessEntry, Ref: "a9"}},
		{name: "prefix", ref: SynthesisAnchorRef{Kind: AnchorProcessEntry, Ref: "a"}},
		{name: "raw canonical", ref: SynthesisAnchorRef{Kind: AnchorProcessEntry, Ref: bundle.BehaviorAnchors[0].ID}},
		{name: "wrong kind", ref: SynthesisAnchorRef{Kind: AnchorLifecycleStart, Ref: "a1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := synthesisWireProposal{Records: []synthesisWireRecord{
				{Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Runtime"},
				{Kind: synthesisWireComponentRecord, SubsystemRef: "g1", Name: "Invalid anchor", MemberRefs: []SynthesisMemberRef{firstRef}, AnchorRefs: []SynthesisAnchorRef{test.ref}},
			}}
			response, err := json.Marshal(invalid)
			if err != nil {
				t.Fatal(err)
			}
			result, err := RecordSynthesisResponse(bundle, "invalid-anchor", "test", "test", time.Millisecond, response)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Landscape.Fallback || result.Landscape.FallbackReason != FallbackRejectedUnknownAnchor ||
				!hasLandscapeDiagnostic(result.Landscape.Diagnostics, "proposal.unknown_anchor_id") {
				t.Fatalf("non-exact anchor ref was not rejected closed: %#v", result.Landscape)
			}
		})
	}

	shared := synthesisWireProposal{Records: []synthesisWireRecord{
		{Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Runtime"},
		{Kind: synthesisWireComponentRecord, SubsystemRef: "g1", Name: "First", MemberRefs: []SynthesisMemberRef{firstRef}, AnchorRefs: []SynthesisAnchorRef{anchorRef}},
		{Kind: synthesisWireComponentRecord, SubsystemRef: "g1", Name: "Second", MemberRefs: []SynthesisMemberRef{secondRef}, AnchorRefs: []SynthesisAnchorRef{anchorRef}},
	}}
	response, err = json.Marshal(shared)
	if err != nil {
		t.Fatal(err)
	}
	result, err = RecordSynthesisResponse(bundle, "shared-anchor", "test", "test", time.Millisecond, response)
	if err != nil {
		t.Fatal(err)
	}
	if result.Landscape.Fallback || result.Landscape.ValidationOutcome == ValidationRejected {
		t.Fatalf("same exact anchor used as context by distinct components was rejected: %#v", result.Landscape)
	}
}

func TestBuildSynthesisRequestRejectsObviousCredentialWithoutEcho(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	secret := "company-secret-value-12345"
	bundle.Candidates[0].Facts[0].Value = `api_key="` + secret + `"`
	_, _, err := BuildSynthesisRequest(bundle)
	if err == nil {
		t.Fatal("BuildSynthesisRequest() accepted an obvious credential")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("BuildSynthesisRequest() error echoed the credential-like value")
	}
}

func TestRecordSynthesisResponseReplaysDeterministically(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	response := validSynthesisProposalJSON(t, bundle)
	for _, forbidden := range []string{`"version"`, `"catalog"`, `"hash"`, `"member_ids"`, `"anchor_ids"`} {
		if strings.Contains(string(response), forbidden) {
			t.Fatalf("active provider response shape contains private/backend field %q: %s", forbidden, response)
		}
	}
	result, err := RecordSynthesisResponse(
		bundle,
		"revision-a",
		"deepseek-compatible",
		"deepseek-v4-flash",
		1450*time.Millisecond,
		response,
	)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse() error = %v", err)
	}
	if result.Landscape.Fallback || result.Record.Call == nil {
		t.Fatalf("result = %#v, want one successful represented call", result)
	}
	metadata := result.Record.Call.Metadata
	if metadata.PromptVersion != SynthesisPromptVersion ||
		metadata.Profile != "deepseek-compatible" ||
		metadata.Model != "deepseek-v4-flash" ||
		metadata.OutputLanguage != "en" ||
		metadata.InputBytes <= 0 || metadata.LatencyMillis != 1450 ||
		len(metadata.ValidationWarnings) != 0 || metadata.FallbackReason != "" {
		t.Fatalf("metadata = %#v", metadata)
	}
	result.Record.Call.Metadata.UsageReported = true
	result.Record.Call.Metadata.InputTokens = 120
	result.Record.Call.Metadata.OutputTokens = 40
	result.Record.Call.Metadata.FinishReason = "stop"
	result.Record.Call.Metadata.TransportAttempts = 2
	result.Record.Call.Metadata.ResponseComplete = true

	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatalf("json.Marshal(record) error = %v", err)
	}
	if !bytes.Contains(saved, []byte(`"call"`)) || bytes.Contains(saved, []byte(`"calls"`)) {
		t.Fatalf("record does not represent a singular call: %s", saved)
	}
	replayed, err := ReplaySynthesis(bundle, "revision-a", saved)
	if err != nil {
		t.Fatalf("ReplaySynthesis() error = %v", err)
	}
	if !reflect.DeepEqual(replayed, result.Landscape) {
		t.Fatalf("replayed landscape differs:\nrecorded: %#v\nreplayed: %#v", result.Landscape, replayed)
	}

	legacyUnknownLanguage := bytes.Replace(saved, []byte(`,"output_language":"en"`), nil, 1)
	if bytes.Equal(legacyUnknownLanguage, saved) {
		t.Fatalf("saved record did not contain explicit output language: %s", saved)
	}
	legacyReplayed, err := ReplaySynthesis(bundle, "revision-a", legacyUnknownLanguage)
	if err != nil {
		t.Fatalf("ReplaySynthesis() rejected historical language-unknown record: %v", err)
	}
	if !reflect.DeepEqual(legacyReplayed, result.Landscape) {
		t.Fatalf("historical language-unknown replay differs:\nrecorded: %#v\nreplayed: %#v", result.Landscape, legacyReplayed)
	}
}

func TestRussianSynthesisRecordBindsLanguagePromptAndReplays(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	response := validSynthesisProposalJSON(t, bundle)
	result, err := RecordSynthesisResponseForLanguage(
		bundle,
		"revision-ru",
		"deepseek-compatible",
		"deepseek-v4-flash",
		"ru",
		2*time.Second,
		response,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.Call == nil || result.Record.Call.Metadata.OutputLanguage != "ru" {
		t.Fatalf("Russian record metadata = %#v", result.Record.Call)
	}
	russianPrompt, err := BuildSynthesisPromptForLanguage(bundle, "ru")
	if err != nil {
		t.Fatal(err)
	}
	englishPrompt, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.Call.Metadata.InputBytes != synthesisPromptSize(russianPrompt) ||
		russianPrompt.OutputLanguage == englishPrompt.OutputLanguage || russianPrompt.System == englishPrompt.System {
		t.Fatalf("Russian prompt identity was not recorded: %#v", result.Record.Call.Metadata)
	}

	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplaySynthesis(bundle, "revision-ru", saved)
	if err != nil {
		t.Fatalf("ReplaySynthesis(Russian) error = %v", err)
	}
	if !reflect.DeepEqual(replayed, result.Landscape) {
		t.Fatal("Russian synthesis record did not replay deterministically")
	}

	tampered := result.Record
	call := *tampered.Call
	call.Metadata.OutputLanguage = "en"
	tampered.Call = &call
	tamperedJSON, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaySynthesis(bundle, "revision-ru", tamperedJSON); err == nil {
		t.Fatal("replay accepted a Russian record relabeled as English")
	}
}

func TestSynthesisResponseRejectsWrappersWithoutRepair(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	proposal := validSynthesisProposalJSON(t, bundle)
	tests := []struct {
		name     string
		response []byte
	}{
		{
			name:     "markdown fence",
			response: []byte("Here is the map:\n```json\n" + string(proposal) + "\n```\n"),
		},
		{
			name:     "surrounding prose",
			response: []byte("The bounded proposal follows.\n" + string(proposal) + "\nEnd of proposal."),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := RecordSynthesisResponse(bundle, "revision-a", "local-openai", "weak-model", time.Second, test.response)
			if err != nil {
				t.Fatalf("RecordSynthesisResponse() error = %v", err)
			}
			if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected {
				t.Fatalf("wrapped response was repaired: %#v", result.Landscape)
			}
			if !hasLandscapeDiagnostic(result.Landscape.Diagnostics, "response.invalid_proposal") {
				t.Fatalf("diagnostics = %#v, want strict invalid proposal", result.Landscape.Diagnostics)
			}
			if !reflect.DeepEqual(result.Record.Call.Metadata.ValidationWarnings, result.Landscape.Diagnostics) {
				t.Fatalf("saved warnings do not match local validation: %#v", result.Record.Call.Metadata)
			}
			saved, err := json.Marshal(result.Record)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := ReplaySynthesis(bundle, "revision-a", saved)
			if err != nil {
				t.Fatalf("ReplaySynthesis() error = %v", err)
			}
			if !reflect.DeepEqual(replayed, result.Landscape) {
				t.Fatal("formatted response did not replay deterministically")
			}
		})
	}
}

func TestSynthesisWireRejectsDuplicateNullAndInvalidUnicodeFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "duplicate root field",
			raw:  `{"records":[],"records":[]}`,
		},
		{
			name: "null description",
			raw:  `{"records":[{"kind":"subsystem","ref":"g1","name":"n","description":null}]}`,
		},
		{
			name: "null component values",
			raw:  `{"records":[{"kind":"subsystem","ref":"g1","name":"n","description":""},{"kind":"component","subsystem_ref":"g1","name":"c","description":"","member_refs":null,"anchor_refs":null,"hypothesis":null}]}`,
		},
		{
			name: "invalid unicode surrogate",
			raw:  `{"records":[{"kind":"subsystem","ref":"g1","name":"\ud800","description":""}]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := decodeSynthesisWireProposalJSON([]byte(test.raw)); err == nil {
				t.Fatal("strict wire decoder accepted malformed field encoding")
			}
		})
	}
}

func TestSynthesisResponseMembershipCountsUsesOnlyExactCurrentFlatWire(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		raw         string
		counted     bool
		occurrences int
		distinct    int
	}{
		{
			name: "flat many to many membership",
			raw: `{"records":[` +
				`{"kind":"subsystem","ref":"g1","name":"Application","description":""},` +
				`{"kind":"component","subsystem_ref":"g1","name":"Runtime","description":"","member_refs":[{"kind":"package","ref":"p1"}],"anchor_refs":[],"hypothesis":true},` +
				`{"kind":"component","subsystem_ref":"g1","name":"Storage","description":"","member_refs":[{"kind":"package","ref":"p1"},{"kind":"file","ref":"f1"}],"anchor_refs":[],"hypothesis":true}` +
				`]}`,
			counted: true, occurrences: 3, distinct: 2,
		},
		{
			name: "retired nested response",
			raw:  `{"subsystems":[{"components":[{"member_ids":[{"kind":"package","value":"a"}]}]}]}`,
		},
		{
			name: "unknown component field",
			raw:  `{"records":[{"kind":"component","subsystem_ref":"g1","name":"Runtime","description":"","member_refs":[{"kind":"package","ref":"p1"}],"anchor_refs":[],"hypothesis":true,"owner_ref":"p1"}]}`,
		},
		{
			name: "duplicate ref field",
			raw:  `{"records":[{"kind":"component","subsystem_ref":"g1","name":"Runtime","description":"","member_refs":[{"kind":"package","ref":"p1","ref":"p2"}],"anchor_refs":[],"hypothesis":true}]}`,
		},
		{name: "fenced response", raw: "```json\n{}\n```"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			counted, occurrences, distinct := SynthesisResponseMembershipCounts([]byte(test.raw))
			if counted != test.counted || occurrences != test.occurrences || distinct != test.distinct {
				t.Fatalf("counts = %t/%d/%d, want %t/%d/%d", counted, occurrences, distinct, test.counted, test.occurrences, test.distinct)
			}
		})
	}
}

func TestInvalidSynthesisOutputFallsBackAndReplays(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	proposal := validSynthesisProposalJSON(t, bundle)
	tests := []struct {
		name       string
		response   []byte
		diagnostic string
		state      ResponseState
	}{
		{name: "junk", response: []byte("not json at all"), diagnostic: "response.no_json", state: ResponseCaptured},
		{name: "ambiguous objects", response: append(append(append([]byte(nil), proposal...), '\n'), proposal...), diagnostic: "response.ambiguous_json", state: ResponseCaptured},
		{name: "invalid proposal type", response: []byte(`{"version":2,"subsystems":"not-an-array"}`), diagnostic: "response.invalid_proposal", state: ResponseCaptured},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := RecordSynthesisResponse(bundle, "revision-a", "local-openai", "weak-model", time.Second, test.response)
			if err != nil {
				t.Fatalf("RecordSynthesisResponse() error = %v", err)
			}
			if !result.Landscape.Fallback || result.Landscape.FallbackReason != FallbackRejectedMalformed {
				t.Fatalf("fallback = %v (%q), want invalid proposal fallback", result.Landscape.Fallback, result.Landscape.FallbackReason)
			}
			if !hasLandscapeDiagnostic(result.Landscape.Diagnostics, test.diagnostic) {
				t.Fatalf("diagnostics = %#v, want %q", result.Landscape.Diagnostics, test.diagnostic)
			}
			if hasLandscapeDiagnostic(result.Landscape.Diagnostics, "proposal.unsupported_version") {
				t.Fatalf("response failure gained an unrelated empty-proposal diagnostic: %#v", result.Landscape.Diagnostics)
			}
			if result.Record.Call.ResponseState != test.state || result.Record.Call.Metadata.FallbackReason != FallbackRejectedMalformed {
				t.Fatalf("saved call = %#v", result.Record.Call)
			}
			saved, err := json.Marshal(result.Record)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := ReplaySynthesis(bundle, "revision-a", saved)
			if err != nil {
				t.Fatalf("ReplaySynthesis() error = %v", err)
			}
			if !reflect.DeepEqual(replayed, result.Landscape) {
				t.Fatal("invalid response fallback did not replay deterministically")
			}
		})
	}
}

func TestMinimizedCasdoorMalformedFragmentsRejectWithoutFirstObjectAcceptance(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/casdoor_architecture_malformed_fragments_minimized_v1.txt")
	if err != nil {
		t.Fatal(err)
	}
	bundle := landscapeTestBundle()
	result, err := RecordSynthesisResponseForLanguage(
		bundle, "casdoor-malformed-fragments-v1", "openai-compatible/bearer",
		"deepseek-v4-flash", "ru", time.Millisecond, raw,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected ||
		result.Landscape.FallbackReason != FallbackRejectedMalformed {
		t.Fatalf("malformed live response result = %#v", result.Landscape)
	}
	if len(result.Landscape.Diagnostics) != 1 ||
		result.Landscape.Diagnostics[0].Code != "response.ambiguous_json" {
		t.Fatalf("malformed live response diagnostics = %#v", result.Landscape.Diagnostics)
	}
	if result.Membership.Counted || result.Membership.MemberOccurrences != 0 || result.Membership.DistinctMembers != 0 {
		t.Fatalf("first recoverable object was partially accepted: %#v", result.Membership)
	}
	if result.Record.Call == nil || result.Record.Call.ResponseState != ResponseCaptured ||
		!bytes.Equal(result.Record.Call.Response, raw) {
		t.Fatalf("malformed live response was not saved exactly: %#v", result.Record.Call)
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplaySynthesisResult(bundle, "casdoor-malformed-fragments-v1", saved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replayed.Landscape, result.Landscape) ||
		!reflect.DeepEqual(replayed.Membership, result.Membership) {
		t.Fatal("malformed live response did not replay exactly")
	}
}

func TestSavedCasdoor1750ResponseIsByteExactAndRejectsWithoutFirstObjectAcceptance(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/casdoor_architecture_20260802_175017_response.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 2004 {
		t.Fatalf("saved live response bytes = %d, want 2004", len(raw))
	}
	digest := sha256.Sum256(raw)
	if got := hex.EncodeToString(digest[:]); got != "a3f8aea4320cab5c65bde693d3898a1b6f0322c56eba2cd21e97907631888895" {
		t.Fatalf("saved live response sha256 = %s", got)
	}
	bundle := landscapeTestBundle()
	result, err := RecordSynthesisResponseForLanguage(
		bundle, "casdoor-20260802-175017", "openai-compatible/bearer",
		"deepseek-v4-flash", "ru", time.Millisecond, raw,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected ||
		result.Landscape.FallbackReason != FallbackRejectedMalformed {
		t.Fatalf("saved live response result = %#v", result.Landscape)
	}
	if len(result.Landscape.Diagnostics) != 1 ||
		result.Landscape.Diagnostics[0].Code != "response.ambiguous_json" {
		t.Fatalf("saved live response diagnostics = %#v", result.Landscape.Diagnostics)
	}
	if result.Membership.Counted || result.Membership.MemberOccurrences != 0 || result.Membership.DistinctMembers != 0 {
		t.Fatalf("saved live response first object was partially accepted: %#v", result.Membership)
	}
	if result.Record.Call == nil || !bytes.Equal(result.Record.Call.Response, raw) {
		t.Fatalf("saved live response was not retained exactly: %#v", result.Record.Call)
	}
}

func TestSynthesisResponseUsesSharedEnvelopeAndResourceLimitIsTerminal(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	proposal := validSynthesisProposalJSON(t, bundle)
	response := append(bytes.Repeat([]byte(" "), (256<<10)+1), proposal...)
	result, err := RecordSynthesisResponse(
		bundle, "revision-a", "test", "test", time.Millisecond, response,
	)
	if err != nil {
		t.Fatalf("response above former stage cap rejected: %v", err)
	}
	if result.Landscape.Fallback || result.Record.Call.ResponseState != ResponseCaptured {
		t.Fatalf("response above former stage cap used fallback/omission: %#v", result)
	}

	oversize := bytes.Repeat([]byte("x"), maxSynthesisResponseBytes+1)
	limited, err := RecordSynthesisResponse(
		bundle, "revision-a", "test", "test", time.Millisecond, oversize,
	)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) ||
		limitErr.Kind != modelresearch.ResourceLimitResponseBytes ||
		limitErr.Limit != maxSynthesisResponseBytes ||
		limitErr.Observed != len(oversize) ||
		limited.Landscape.Fallback || limited.Record.Call != nil {
		t.Fatalf("terminal response limit = result %#v, error %#v", limited, limitErr)
	}

	_, err = ReplaySynthesis(
		bundle,
		"revision-a",
		bytes.Repeat([]byte("x"), maxSynthesisRecordBytes+1),
	)
	limitErr = nil
	if !errors.As(err, &limitErr) || limitErr.Kind != modelresearch.ResourceLimitRecordBytes {
		t.Fatalf("record limit error = %#v, want typed record resource limit", err)
	}
}

func TestUnknownSynthesisResponseFieldsRejectWholeProposal(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	proposal := validSynthesisProposalJSON(t, bundle)
	response := append([]byte(nil), proposal[:len(proposal)-1]...)
	response = append(response, []byte(`,"commentary":{"note":"harmless"}}`)...)
	result, err := RecordSynthesisResponse(bundle, "revision-a", "local-openai", "weak-model", time.Second, response)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse() error = %v", err)
	}
	if !result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationRejected ||
		!hasLandscapeDiagnostic(result.Landscape.Diagnostics, "response.invalid_proposal") {
		t.Fatalf("unknown root field did not reject the whole proposal: %#v", result.Landscape)
	}
}

func TestGroundedSynthesisNormalizesPackageOnlyComponentButRequiresOtherGrounding(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	bundle.GroundingMode = GroundingMixed
	bundle.BehaviorAnchors = []BehaviorAnchor{{
		ID: "process", Kind: AnchorProcessEntry, Label: "process entry",
		Location: evidence.Location{Path: "cmd/main.go", Line: 10, Column: 1},
		Scenario: ScenarioContext{ID: "go:test", Name: "test build"},
		Producer: evidence.Provenance{
			Provider: "test", Version: "v1", Operation: "fixture",
			Location: &evidence.Location{Path: "cmd/main.go", Line: 10, Column: 1},
		},
		Certainty:   evidence.CertaintyStatic,
		MemberIDs:   []MemberID{bundle.Candidates[0].ID},
		Limitations: []string{"Static test evidence; execution is not observed."},
	}}
	bundle.AnchorBindings = nil

	proposal := Proposal{
		Version: ProposalVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Runtime",
			Components: []ProposedComponent{
				{Name: "Process", MemberIDs: []MemberID{bundle.Candidates[0].ID}},
				{Name: "Other exact members", MemberIDs: candidateIDsExcept(bundle, bundle.Candidates[0].ID), Hypothesis: true},
			},
		}},
	}
	raw := synthesisWireProposalJSON(t, bundle, proposal)
	result, err := RecordSynthesisResponse(bundle, "revision-grounded", "test", "test", time.Millisecond, raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationAcceptedNormalized ||
		!hasLandscapeDiagnostic(result.Landscape.Diagnostics, "proposal.normalized_package_only_hypothesis") {
		t.Fatalf("package-only proposal = %#v", result.Landscape)
	}

	proposal.Subsystems[0].Components[0] = ProposedComponent{
		Name: "Source file", MemberIDs: []MemberID{bundle.Candidates[2].ID},
	}
	proposal.Subsystems[0].Components[1].MemberIDs = candidateIDsExcept(bundle, bundle.Candidates[2].ID)
	raw = synthesisWireProposalJSON(t, bundle, proposal)
	result, err = RecordSynthesisResponse(bundle, "revision-grounded", "test", "test", time.Millisecond, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || !hasLandscapeDiagnostic(result.Landscape.Diagnostics, "proposal.ungrounded_primary_component") {
		t.Fatalf("ungrounded non-package proposal = %#v", result.Landscape)
	}

	proposal.Subsystems[0].Components[0] = ProposedComponent{
		Name: "Process", MemberIDs: []MemberID{bundle.Candidates[0].ID},
	}
	proposal.Subsystems[0].Components[0].AnchorIDs = []string{"process"}
	proposal.Subsystems[0].Components[1].MemberIDs = candidateIDsExcept(bundle, bundle.Candidates[0].ID)
	raw = synthesisWireProposalJSON(t, bundle, proposal)
	result, err = RecordSynthesisResponse(bundle, "revision-grounded", "test", "test", time.Millisecond, raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Landscape.Fallback || !reflect.DeepEqual(result.Landscape.Subsystems[0].Components[0].AnchorIDs, []string{"process"}) {
		t.Fatalf("grounded proposal = %#v", result.Landscape)
	}
}

func TestSensitiveSynthesisResponseIsNotSavedOrEchoed(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	secret := "company-secret-value-12345"
	proposal := validSynthesisProposal(bundle)
	proposal.Subsystems[0].Description = `api_key="` + secret + `"`
	response := synthesisWireProposalJSON(t, bundle, proposal)
	result, err := RecordSynthesisResponse(bundle, "revision-a", "local-openai", "weak-model", time.Second, response)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse() error = %v", err)
	}
	if result.Record.Call.ResponseState != ResponseSensitiveOmitted || len(result.Record.Call.Response) != 0 {
		t.Fatalf("sensitive response was retained: %#v", result.Record.Call)
	}
	if !hasLandscapeDiagnostic(result.Landscape.Diagnostics, "response.sensitive_omitted") {
		t.Fatalf("diagnostics = %#v, want sensitive response warning", result.Landscape.Diagnostics)
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(saved, []byte(secret)) {
		t.Fatal("saved synthesis record exposed the credential-like value")
	}
	replayed, err := ReplaySynthesis(bundle, "revision-a", saved)
	if err != nil {
		if strings.Contains(err.Error(), secret) {
			t.Fatal("replay error echoed the credential-like value")
		}
		t.Fatalf("ReplaySynthesis() error = %v", err)
	}
	if !reflect.DeepEqual(replayed, result.Landscape) {
		t.Fatal("sensitive response fallback did not replay deterministically")
	}
}

func TestReplaySynthesisRejectsPluralCallHistory(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	result, err := RecordSynthesisResponse(
		bundle, "revision-a", "deepseek-compatible", "deepseek-v4-flash", time.Second,
		validSynthesisProposalJSON(t, bundle),
	)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(saved, &object); err != nil {
		t.Fatal(err)
	}
	object["calls"] = []any{object["call"], object["call"]}
	saved, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaySynthesis(bundle, "revision-a", saved); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("ReplaySynthesis() error = %v, want plural call history rejection", err)
	}
}

func validSynthesisProposalJSON(t *testing.T, bundle CandidateBundle) []byte {
	t.Helper()

	return synthesisWireProposalJSON(t, bundle, validSynthesisProposal(bundle))
}

func validSynthesisProposal(bundle CandidateBundle) Proposal {
	memberIDs := make([]MemberID, len(bundle.Candidates))
	for index, candidate := range bundle.Candidates {
		memberIDs[index] = candidate.ID
	}
	return Proposal{
		Version: ProposalVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Repository",
			Components: []ProposedComponent{{
				Name: "Local architecture", Description: "Conceptual grouping over exact local candidates.", MemberIDs: memberIDs,
			}},
		}},
	}
}

func synthesisWireProposalJSON(t *testing.T, bundle CandidateBundle, proposal Proposal) []byte {
	t.Helper()

	wire := synthesisWireProposalFromCanonical(t, bundle, proposal)
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("json.Marshal(wire proposal) error = %v", err)
	}
	return encoded
}

func synthesisWireProposalFromCanonical(
	t *testing.T,
	bundle CandidateBundle,
	proposal Proposal,
) synthesisWireProposal {
	t.Helper()

	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatalf("buildSynthesisPrivateCatalog() error = %v", err)
	}
	wire := synthesisWireProposal{Records: make([]synthesisWireRecord, 0, len(proposal.Subsystems)+proposalComponentCount(proposal))}
	for subsystemIndex, subsystem := range proposal.Subsystems {
		subsystemRef := fmt.Sprintf("g%d", subsystemIndex+1)
		wire.Records = append(wire.Records, synthesisWireRecord{
			Kind: synthesisWireSubsystemRecord, Ref: subsystemRef,
			Name: subsystem.Name, Description: subsystem.Description,
		})
		for _, component := range subsystem.Components {
			wireComponent := synthesisWireRecord{
				Kind: synthesisWireComponentRecord, SubsystemRef: subsystemRef,
				Name: component.Name, Description: component.Description, Hypothesis: component.Hypothesis,
				MemberRefs: make([]SynthesisMemberRef, 0, len(component.MemberIDs)),
				AnchorRefs: make([]SynthesisAnchorRef, 0, len(component.AnchorIDs)),
			}
			for _, memberID := range component.MemberIDs {
				ref, exists := catalog.membersByID[memberID]
				if !exists {
					t.Fatalf("canonical fixture references unknown member %q", memberID.key())
				}
				wireComponent.MemberRefs = append(wireComponent.MemberRefs, ref)
			}
			for _, anchorID := range component.AnchorIDs {
				ref, exists := catalog.anchorsByID[anchorID]
				if !exists {
					t.Fatalf("canonical fixture references unknown anchor %q", anchorID)
				}
				wireComponent.AnchorRefs = append(wireComponent.AnchorRefs, ref)
			}
			wire.Records = append(wire.Records, wireComponent)
		}
	}
	return wire
}

func minimalPackageSynthesisBundle(canonicalID string) CandidateBundle {
	return CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeApplication, GroundingMode: GroundingPackages,
		Candidates: []Candidate{{
			ID: MemberID{Kind: MemberPackage, Value: canonicalID}, Name: "object",
			Facts: []LocalFact{testLocalFact(FactDeclaration, "object", "object/object.go", 1)},
		}},
	}
}

func groundedTwoMemberSynthesisBundle() CandidateBundle {
	first := Candidate{
		ID: MemberID{Kind: MemberPackage, Value: "member-package-first"}, Name: "first",
		Facts: []LocalFact{testLocalFact(FactDeclaration, "first", "first/first.go", 1)},
	}
	second := Candidate{
		ID: MemberID{Kind: MemberPackage, Value: "member-package-second"}, Name: "second",
		Facts: []LocalFact{testLocalFact(FactDeclaration, "second", "second/second.go", 1)},
	}
	location := evidence.Location{Path: "main.go", Line: 10, Column: 1}
	return CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeApplication, GroundingMode: GroundingMixed,
		Candidates: []Candidate{first, second},
		BehaviorAnchors: []BehaviorAnchor{{
			ID: "anchor-process", Kind: AnchorProcessEntry, Label: "process entry", Location: location,
			Scenario: ScenarioContext{ID: "go:test", Name: "test build"},
			Producer: evidence.Provenance{
				Provider: "test", Version: "v1", Operation: "fixture", Location: &location,
			},
			Certainty: evidence.CertaintyStatic, MemberIDs: []MemberID{first.ID, second.ID},
			Limitations: []string{"Static fixture evidence; runtime execution is not observed."},
		}},
	}
}

func caddyArchitectureReplayFixture(t *testing.T) (CandidateBundle, []byte) {
	t.Helper()

	response, err := os.ReadFile("testdata/caddy_architecture_proposal.json")
	if err != nil {
		t.Fatal(err)
	}
	var proposal Proposal
	if err := json.Unmarshal(response, &proposal); err != nil {
		t.Fatal(err)
	}
	anchorKinds := map[string]BehaviorAnchorKind{
		"anchor-13712c6ead2b942f11dcffac": AnchorRegistryLookup,
		"anchor-15a7795600d479a868920444": AnchorAdminControlPlane,
		"anchor-19ecb1abd8774376a314e8c5": AnchorProcessEntry,
		"anchor-1cb683cf6bdf9da982a3a2cb": AnchorCommandDispatch,
		"anchor-1cbc8e5d96c6a3d4ee8cfc55": AnchorConfigApply,
		"anchor-472447da3f3b81394c5d1b51": AnchorRequestDispatchRoot,
		"anchor-5b79058ae0786d830a6cd1d5": AnchorExtensionFamily,
		"anchor-a020c474f76db85ae85d9357": AnchorConfigIngress,
		"anchor-b95e592bce5b179ca6607eca": AnchorLifecycleInterface,
		"anchor-c068a8bd8b4edb1b113b3b4a": AnchorLifecycleStart,
		"anchor-e55d283d2f5bd2fd480082ec": AnchorConfigAdapter,
		"anchor-e97c73cefeeab8629977e8e2": AnchorRegistryWrite,
		"anchor-fa09229b7ce874a630de6544": AnchorSecurityBoundary,
	}
	candidates := make(map[MemberID]Candidate)
	anchorMembers := make(map[string]map[MemberID]struct{})
	for _, subsystem := range proposal.Subsystems {
		for _, component := range subsystem.Components {
			for index, memberID := range component.MemberIDs {
				if _, exists := candidates[memberID]; !exists {
					path := fmt.Sprintf("fixture/%s/%02d.go", subsystem.Name, index+1)
					candidates[memberID] = Candidate{
						ID: memberID, Name: memberID.Value,
						Facts: []LocalFact{testLocalFact(FactDeclaration, memberID.Value, path, 1)},
					}
				}
				for _, anchorID := range component.AnchorIDs {
					if anchorMembers[anchorID] == nil {
						anchorMembers[anchorID] = make(map[MemberID]struct{})
					}
					anchorMembers[anchorID][memberID] = struct{}{}
				}
			}
		}
	}
	bundle := CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeModularPlatformServer, GroundingMode: GroundingMixed,
	}
	for _, candidate := range candidates {
		bundle.Candidates = append(bundle.Candidates, candidate)
	}
	sortCandidates(bundle.Candidates)
	for anchorID, kind := range anchorKinds {
		memberIDs := make([]MemberID, 0, len(anchorMembers[anchorID]))
		for memberID := range anchorMembers[anchorID] {
			memberIDs = append(memberIDs, memberID)
		}
		sort.Slice(memberIDs, func(i, j int) bool { return memberIDs[i].key() < memberIDs[j].key() })
		location := evidence.Location{Path: "fixture/anchors.go", Line: len(bundle.BehaviorAnchors) + 1, Column: 1}
		bundle.BehaviorAnchors = append(bundle.BehaviorAnchors, BehaviorAnchor{
			ID: anchorID, Kind: kind, Label: string(kind), Location: location,
			Scenario: ScenarioContext{ID: "go:fixture", Name: "saved Caddy replay"},
			Producer: evidence.Provenance{
				Provider: "saved_caddy_run", Version: "20260712-184001", Operation: "replay_architecture_anchor", Location: &location,
			},
			Certainty: evidence.CertaintyStatic, MemberIDs: memberIDs,
			Limitations: []string{"Saved deterministic fixture evidence; runtime execution is not implied."},
		})
	}
	sort.Slice(bundle.BehaviorAnchors, func(i, j int) bool { return bundle.BehaviorAnchors[i].ID < bundle.BehaviorAnchors[j].ID })
	return bundle, response
}
