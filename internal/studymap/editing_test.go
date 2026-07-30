package studymap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

func TestEditingContractsDecodeStrictIndependentArtifacts(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	rawDirections := rawDirectionsFromLegacy(legacy)
	directions, err := NormalizeDirectionProposal(rawDirections)
	if err != nil {
		t.Fatal(err)
	}
	reviewBundle, err := BuildReviewBundle(bundle, directions.Directions[0])
	if err != nil {
		t.Fatal(err)
	}
	review := directReview(directions.Directions[0])

	tests := []struct {
		name   string
		value  any
		decode func([]byte) error
	}{
		{
			name: "brief and shape", value: brief,
			decode: func(raw []byte) error {
				_, decodeErr := DecodeBriefShapeProposal(raw)
				return decodeErr
			},
		},
		{
			name: "directions", value: rawDirections,
			decode: func(raw []byte) error {
				_, decodeErr := DecodeDirectionProposal(raw)
				return decodeErr
			},
		},
		{
			name: "review bundle", value: reviewBundle,
			decode: func(raw []byte) error {
				_, decodeErr := DecodeReviewBundle(raw)
				return decodeErr
			},
		},
		{
			name: "review proposal", value: review,
			decode: func(raw []byte) error {
				_, decodeErr := DecodeReviewProposal(raw)
				return decodeErr
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			raw := mustEditingJSON(t, test.value)
			if err := test.decode(raw); err != nil {
				t.Fatalf("decode valid artifact: %v", err)
			}
			withUnknown := append([]byte(`{"unexpected":true,`), raw[1:]...)
			if err := test.decode(withUnknown); err == nil {
				t.Fatal("decoder accepted an unknown field")
			}
			if err := test.decode(append(raw, []byte(` {}`)...)); err == nil {
				t.Fatal("decoder accepted trailing JSON")
			}
		})
	}
	decodedDirections, err := DecodeDirectionProposal(mustEditingJSON(t, rawDirections))
	if err != nil {
		t.Fatal(err)
	}
	if decodedDirections.Directions[0].DirectionID == "" ||
		decodedDirections.Directions[0].DirectionID != directions.Directions[0].DirectionID {
		t.Fatalf("locally derived direction id = %q", decodedDirections.Directions[0].DirectionID)
	}
	normalizedRaw := mustEditingJSON(t, decodedDirections)
	replayedDirections, err := DecodeNormalizedDirectionProposal(normalizedRaw)
	if err != nil {
		t.Fatalf("replay normalized directions: %v", err)
	}
	if !slices.EqualFunc(
		replayedDirections.Directions,
		decodedDirections.Directions,
		func(left, right DirectionCandidate) bool {
			return left.DirectionID == right.DirectionID
		},
	) {
		t.Fatal("normalized direction IDs changed during replay")
	}
	if _, diagnostics, err := DecodeDirectionProposalWithDiagnostics(normalizedRaw); err == nil ||
		len(diagnostics.Issues) == 0 || diagnostics.Issues[0].Code != "model_direction_id" {
		t.Fatalf(
			"raw direction decoder accepted normalized artifact: diagnostics=%#v error=%v",
			diagnostics,
			err,
		)
	}
	missingLocalID := decodedDirections
	missingLocalID.Directions = append([]DirectionCandidate(nil), decodedDirections.Directions...)
	missingLocalID.Directions[0].DirectionID = ""
	if _, err := DecodeNormalizedDirectionProposal(mustEditingJSON(t, missingLocalID)); err == nil {
		t.Fatal("normalized direction decoder accepted an empty local ID")
	}
	suppliedID := rawDirections
	suppliedID.Directions = append([]DirectionCandidate(nil), rawDirections.Directions...)
	suppliedID.Directions[0].DirectionID = "model-direction"
	if _, diagnostics, err := DecodeDirectionProposalWithDiagnostics(
		mustEditingJSON(t, suppliedID),
	); err != nil || diagnostics.Accepted != len(suppliedID.Directions)-1 ||
		len(diagnostics.Issues) != 1 ||
		diagnostics.Issues[0].Code != "model_direction_id" {
		t.Fatalf(
			"DecodeDirectionProposal(model id) diagnostics=%#v error=%v",
			diagnostics,
			err,
		)
	}
	reordered := rawDirections.Directions[0]
	reordered.Question = "  " + strings.ToUpper(reordered.Question) + "  "
	reordered.AnchorIDs = append([]string(nil), reordered.AnchorIDs...)
	slices.Reverse(reordered.AnchorIDs)
	reordered.ReadingAnchors = append([]ReadingAnchor(nil), reordered.ReadingAnchors...)
	slices.Reverse(reordered.ReadingAnchors)
	normalizedReordered, err := NormalizeDirectionProposal(DirectionProposal{
		Version: DirectionProposalVersion, Directions: []DirectionCandidate{reordered},
	})
	if err != nil {
		t.Fatal(err)
	}
	if normalizedReordered.Directions[0].DirectionID != directions.Directions[0].DirectionID {
		t.Fatalf(
			"stable IDs differ after question/anchor normalization: %q / %q",
			normalizedReordered.Directions[0].DirectionID,
			directions.Directions[0].DirectionID,
		)
	}

	invalidFit := review
	invalidFit.Reviews = append([]AnchorReview(nil), review.Reviews...)
	invalidFit.Reviews[0].Fit = AnchorFit("mostly")
	if _, err := DecodeReviewProposal(mustEditingJSON(t, invalidFit)); err == nil {
		t.Fatal("review decoder accepted an open-ended fit")
	}
	invalidRole := review
	invalidRole.Reviews = append([]AnchorReview(nil), review.Reviews...)
	invalidRole.Reviews[0].Role = ReadingRole("helper")
	if _, err := DecodeReviewProposal(mustEditingJSON(t, invalidRole)); err == nil {
		t.Fatal("review decoder accepted an open-ended reading role")
	}
	invalidReason := review
	invalidReason.Reviews = append([]AnchorReview(nil), review.Reviews...)
	invalidReason.Reviews[0].OverclaimReasons = []OverclaimReason{OverclaimNone, OverclaimVagueOrGeneric}
	if _, err := DecodeReviewProposal(mustEditingJSON(t, invalidReason)); err == nil {
		t.Fatal("review decoder accepted none alongside an overclaim")
	}
}

func TestBriefShapeAcceptsOneExactAreaForSmallLibrary(t *testing.T) {
	t.Parallel()

	_, legacy := studyMapFixture(t)
	proposal := briefShapeFromLegacy(legacy)
	proposal.ShapeAreaIDs = proposal.ShapeAreaIDs[:1]
	decoded, err := DecodeBriefShapeProposal(mustEditingJSON(t, proposal))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.ShapeAreaIDs, proposal.ShapeAreaIDs) {
		t.Fatalf("shape areas = %v, want %v", decoded.ShapeAreaIDs, proposal.ShapeAreaIDs)
	}
}

func TestResolveDirectionProposalReferencesExpandsOnlyUniquePrefixes(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	raw := rawDirectionsFromLegacy(legacy)
	direction := raw.Directions[0]
	oldAnchorID := direction.AnchorIDs[0]
	fullAnchorID := "fact-12345678aaaaaaaa"
	for index := range bundle.Anchors {
		if bundle.Anchors[index].ID == oldAnchorID {
			bundle.Anchors[index].ID = fullAnchorID
		}
	}
	direction.AnchorIDs = append([]string(nil), direction.AnchorIDs...)
	for index := range direction.AnchorIDs {
		if direction.AnchorIDs[index] == oldAnchorID {
			direction.AnchorIDs[index] = fullAnchorID
		}
	}
	direction.ReadingAnchors = append([]ReadingAnchor(nil), direction.ReadingAnchors...)
	for index := range direction.ReadingAnchors {
		if direction.ReadingAnchors[index].AnchorID == oldAnchorID {
			direction.ReadingAnchors[index].AnchorID = fullAnchorID
		}
	}
	shortAnchorID := fullAnchorID[:minUniqueBundleReferencePrefixBytes]
	direction.AnchorIDs[0] = shortAnchorID
	for index := range direction.ReadingAnchors {
		if direction.ReadingAnchors[index].AnchorID == fullAnchorID {
			direction.ReadingAnchors[index].AnchorID = shortAnchorID
		}
	}

	resolved, err := ResolveDirectionProposalReferences(bundle, DirectionProposal{
		Version: DirectionProposalVersion, Directions: []DirectionCandidate{direction},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := resolved.Directions[0]
	if got.AnchorIDs[0] != fullAnchorID {
		t.Fatalf("anchor id = %q, want %q", got.AnchorIDs[0], fullAnchorID)
	}
	for _, reading := range got.ReadingAnchors {
		if reading.AnchorID == shortAnchorID {
			t.Fatalf("short reading anchor survived: %#v", reading)
		}
	}
	if got.DirectionID == "" {
		t.Fatal("resolved direction did not receive a local ID")
	}

	if _, err := resolveUniqueBundleReference(
		"frf-12345678",
		[]string{"frf-12345678aaaaaaaa", "frf-12345678bbbbbbbb"},
	); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous prefix error = %v", err)
	}
	if _, err := resolveUniqueBundleReference("frf-short", []string{"frf-short-and-valid"}); err == nil {
		t.Fatal("short prefix was accepted")
	}
}

func TestRecoverStudyProviderJSONBeforeStrictValidation(t *testing.T) {
	t.Parallel()

	_, legacy := studyMapFixture(t)
	briefJSON := mustEditingJSON(t, briefShapeFromLegacy(legacy))
	directionJSON := mustEditingJSON(t, rawDirectionsFromLegacy(legacy))

	leakedBrief := append(
		[]byte("I will now write the result.<｜end▁of▁thinking｜>\n"),
		briefJSON...,
	)
	recoveredBrief, err := RecoverBriefShapeProviderJSON(leakedBrief)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recoveredBrief, briefJSON) {
		t.Fatalf("recovered brief changed:\n%s", recoveredBrief)
	}
	if _, err := DecodeBriefShapeProposal(recoveredBrief); err != nil {
		t.Fatalf("strict brief validation after recovery: %v", err)
	}

	leakedDirections := append(
		[]byte("{\n\"version\":1,\n\"directions\":[\n// direction objects...\n]\n}\n"+
			"Will use proper quotes and commas.\n"+
			"Let's write.<｜end▁of▁thinking｜>"),
		directionJSON...,
	)
	if _, err := DecodeDirectionProposal(leakedDirections); err == nil {
		t.Fatal("strict direction decoder accepted provider prose directly")
	}
	recoveredDirections, err := RecoverDirectionProviderJSON(leakedDirections)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recoveredDirections, directionJSON) {
		t.Fatalf("recovered directions changed:\n%s", recoveredDirections)
	}
	if _, err := DecodeDirectionProposal(recoveredDirections); err != nil {
		t.Fatalf("strict direction validation after recovery: %v", err)
	}

	ambiguous := append(append([]byte(nil), directionJSON...), directionJSON...)
	if _, err := RecoverDirectionProviderJSON(ambiguous); err == nil ||
		!strings.Contains(err.Error(), "ambiguous recoverable direction proposal") {
		t.Fatalf("ambiguous recovery error = %v", err)
	}
}

func TestBuildReviewBundleProjectsExactNonGoSource(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	for index := 0; index < 3; index++ {
		filePath := fmt.Sprintf("src/part%d.py", index+1)
		symbol := fmt.Sprintf("part_%d", index+1)
		line := 20 + index*10
		exactBundle := exactSourceBundleForTest(
			filePath,
			symbol,
			line,
			[]string{fmt.Sprintf("def %s() -> None:", symbol)},
		)
		bundle.Anchors[index].Path = filePath
		bundle.Anchors[index].Symbol = symbol
		bundle.Anchors[index].Line = line
		bundle.Anchors[index].Function = sourcewindowfacts.Function{}
		bundle.Anchors[index].ExactSource = exactBundle.Anchors[0].ExactSource
		bundle.AllowedPaths = append(bundle.AllowedPaths, filePath)
	}
	directions, err := NormalizeDirectionProposal(rawDirectionsFromLegacy(legacy))
	if err != nil {
		t.Fatal(err)
	}
	review, err := BuildReviewBundle(bundle, directions.Directions[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Anchors) != 3 {
		t.Fatalf("review anchors = %#v", review.Anchors)
	}
	for index, anchor := range review.Anchors {
		wantLine := 20 + index*10
		wantText := fmt.Sprintf("def part_%d() -> None:", index+1)
		if anchor.Path != fmt.Sprintf("src/part%d.py", index+1) ||
			anchor.Line != wantLine || anchor.Symbol != fmt.Sprintf("part_%d", index+1) ||
			!reflect.DeepEqual(anchor.SourceFragment, []ReviewSourceLine{{Line: wantLine, Text: wantText}}) {
			t.Fatalf("exact review anchor[%d] = %#v", index, anchor)
		}
	}
}

func TestDecodeBriefShapeProposalNormalizesRetainedChattoResponse(t *testing.T) {
	t.Parallel()

	const retainedChattoResponse = `{"version":1,"repository_type":"service_application","brief":{"what_it_is":{"text":"A self-hostable team chat application with Go backend and TypeScript frontend, using NATS JetStream for event sourcing and real-time delivery.","support_ids":["component-86cdd5ad86c8","component-71c3eb525cb1","frf-9807c3c6d44b41288c7f19a2","doc-ff21b6b9a3a33e9d"]},"problem":{"text":"Need for a free, easy-to-self-host chat application for teams and communities, with optional cloud hosting.","support_ids":["doc-b335630551682c19","component-c6795f39bafe"]},"main_input":{"text":"HTTP/gRPC and WebSocket connections from client applications.","support_ids":["component-73ab7b1c8726","doc-098e7e9855c52e2b"]},"central_responsibility":{"text":"Manage chat rooms, messages, users, and real-time delivery via event-sourced architecture using NATS JetStream.","support_ids":["component-86cdd5ad86c8","component-2f8003d96eb0","doc-2210862128dcaa74"]},"observable_result":{"text":"Real-time message delivery and comprehensive HTTP/gRPC APIs for chat operations.","support_ids":["doc-098e7e9855c52e2b","doc-7e859f8ebb73d9e3","component-73ab7b1c8726"]}},"domain_terms":[{"term":"snapshot","meaning":"Point-in-time backup of a JetStream stream for disaster recovery.","support_ids":["component-c6795f39bafe","doc-ed8c2a13788a337c"]},{"term":"bootstrap","meaning":"Initial setup process to create the first server configuration and admin users.","support_ids":["component-c6795f39bafe","frf-28575782464925918e8fd01d"]},{"term":"projection","meaning":"Event-sourced view built from the event stream, possibly cached for real-time delivery.","support_ids":["doc-2210862128dcaa74","component-86cdd5ad86c8"]},{"term":"realtime","meaning":"WebSocket or SSE-based real-time message delivery to clients.","support_ids":["doc-098e7e9855c52e2b","component-73ab7b1c8726"]},{"term":"connect","meaning":"Buf Connect RPC framework used for HTTP/gRPC API.","support_ids":["doc-7e859f8ebb73d9e3","component-73ab7b1c8726"]}],"shape_area_ids":["component-71c3eb525cb1","component-86cdd5ad86c8","component-73ab7b1c8726","component-2f8003d96eb0","component-f6a477f4b5b6","component-c6795f39bafe"]}`

	normalized, err := DecodeBriefShapeProposal([]byte(retainedChattoResponse))
	if err != nil {
		t.Fatalf("decode retained Chatto response: %v", err)
	}
	if got, want := len(normalized.Brief.DomainTerms), 5; got != want {
		t.Fatalf("domain term count = %d, want %d", got, want)
	}
	if got := normalized.Brief.DomainTerms[0].Term; got != "snapshot" {
		t.Fatalf("first domain term = %q, want snapshot", got)
	}

	canonical := mustEditingJSON(t, normalized)
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &envelope); err != nil {
		t.Fatal(err)
	}
	if _, exists := envelope["domain_terms"]; exists {
		t.Fatal("canonical proposal retained top-level domain_terms")
	}
	var brief map[string]json.RawMessage
	if err := json.Unmarshal(envelope["brief"], &brief); err != nil {
		t.Fatal(err)
	}
	if _, exists := brief["domain_terms"]; !exists {
		t.Fatal("canonical proposal omitted nested brief.domain_terms")
	}

	control, err := DecodeBriefShapeProposal(canonical)
	if err != nil {
		t.Fatalf("decode prompt-conformant nested control: %v", err)
	}
	if got := string(mustEditingJSON(t, control)); got != string(canonical) {
		t.Fatalf("normalized response differs from nested control:\n got: %s\nwant: %s", got, canonical)
	}
}

func TestDecodeBriefShapeProposalCompatibilityFailsClosed(t *testing.T) {
	t.Parallel()

	_, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	brief.Brief.DomainTerms = []BriefDomainTerm{{
		Term:       "fixture term",
		Meaning:    "A bounded fixture meaning.",
		SupportIDs: []string{brief.Brief.WhatItIs.SupportIDs[0]},
	}}
	canonical := mustEditingJSON(t, brief)
	topLevel := briefShapeTopLevelTermsJSON(t, canonical, false)

	var invalidSupport map[string]json.RawMessage
	if err := json.Unmarshal(topLevel, &invalidSupport); err != nil {
		t.Fatal(err)
	}
	var terms []BriefDomainTerm
	if err := json.Unmarshal(invalidSupport["domain_terms"], &terms); err != nil {
		t.Fatal(err)
	}
	terms[0].SupportIDs = nil
	invalidSupport["domain_terms"] = mustEditingJSON(t, terms)

	var nullTerms map[string]json.RawMessage
	if err := json.Unmarshal(topLevel, &nullTerms); err != nil {
		t.Fatal(err)
	}
	nullTerms["domain_terms"] = json.RawMessage(`null`)

	var objectTerms map[string]json.RawMessage
	if err := json.Unmarshal(topLevel, &objectTerms); err != nil {
		t.Fatal(err)
	}
	objectTerms["domain_terms"] = json.RawMessage(`{}`)

	var bothEmptyEnvelope map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &bothEmptyEnvelope); err != nil {
		t.Fatal(err)
	}
	bothEmptyEnvelope["domain_terms"] = json.RawMessage(`[]`)
	var bothEmptyBrief map[string]json.RawMessage
	if err := json.Unmarshal(bothEmptyEnvelope["brief"], &bothEmptyBrief); err != nil {
		t.Fatal(err)
	}
	bothEmptyBrief["domain_terms"] = json.RawMessage(`[]`)
	bothEmptyEnvelope["brief"] = mustEditingJSON(t, bothEmptyBrief)

	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "both populated", raw: briefShapeTopLevelTermsJSON(t, canonical, true)},
		{name: "both empty", raw: mustEditingJSON(t, bothEmptyEnvelope)},
		{name: "null", raw: mustEditingJSON(t, nullTerms)},
		{name: "object", raw: mustEditingJSON(t, objectTerms)},
		{
			name: "unknown term field",
			raw: []byte(strings.Replace(
				string(topLevel),
				`{"term":`,
				`{"unexpected":true,"term":`,
				1,
			)),
		},
		{
			name: "outside byte budget",
			raw:  bytes.Repeat([]byte("x"), maxEditingArtifactBytes+1),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeBriefShapeProposal(test.raw); err == nil {
				t.Fatal("decoder accepted invalid provider response")
			}
		})
	}
}

func TestDecodeBriefShapeProposalDropsInvalidOptionalDomainTerm(t *testing.T) {
	t.Parallel()

	_, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	brief.Brief.DomainTerms = []BriefDomainTerm{
		{
			Term:       "supported",
			Meaning:    "A bounded supported term.",
			SupportIDs: []string{brief.Brief.WhatItIs.SupportIDs[0]},
		},
		{
			Term:    "unsupported",
			Meaning: "A term without evidence.",
		},
	}
	decoded, err := DecodeBriefShapeProposal(mustEditingJSON(t, brief))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Brief.DomainTerms) != 1 ||
		decoded.Brief.DomainTerms[0].Term != "supported" {
		t.Fatalf("domain terms = %#v", decoded.Brief.DomainTerms)
	}
}

func TestBriefShapeValidationDropsUnsupportedOptionalDomainTerm(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	brief.Brief.DomainTerms = []BriefDomainTerm{
		{
			Term:       "supported",
			Meaning:    "A bounded supported term.",
			SupportIDs: []string{brief.Brief.WhatItIs.SupportIDs[0]},
		},
		{
			Term:       "unknown",
			Meaning:    "A term with a structurally valid but unknown support ID.",
			SupportIDs: []string{"unknown-support"},
		},
	}
	normalized, err := validateBriefShapeAgainstBundle(brief, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.Brief.DomainTerms) != 1 ||
		normalized.Brief.DomainTerms[0].Term != "supported" {
		t.Fatalf("domain terms = %#v", normalized.Brief.DomainTerms)
	}
}

func TestDecodeDirectionProposalRetainsValidChattoCandidateSiblings(t *testing.T) {
	t.Parallel()

	_, legacy := studyMapFixture(t)
	base := rawDirectionsFromLegacy(legacy).Directions
	questions := []string{
		"How is the Chatto server initialized and started?",
		"What happens when NewChattoCore is called and how does it set up the event store?",
		"How does Chatto validate its configuration and environment variables?",
		"How does Chatto handle administrative commands like user creation and diagnostics?",
		"How are push notifications set up and filtered in Chatto?",
		"How does Chatto handle image assets, including GIF transformation and caching?",
		"How does Chatto use NATS JetStream for event storage and projections?",
		"How does Chatto manage user encryption keys and data encryption key store?",
		"How does Chatto's Connect API handle account updates and message service clients?",
		"How does Chatto manage background runtime units like search and push?",
		"How does Chatto extract credentials from the request context?",
	}
	raw := DirectionProposal{Version: DirectionProposalVersion}
	for index, question := range questions {
		candidate := base[index%len(base)]
		candidate.Question = question
		candidate.DirectionID = ""
		candidate.AnchorIDs = append([]string(nil), candidate.AnchorIDs...)
		candidate.ReadingAnchors = append([]ReadingAnchor(nil), candidate.ReadingAnchors...)
		if index == 8 || index == 10 {
			candidate.AnchorIDs = candidate.AnchorIDs[:2]
			candidate.ReadingAnchors = candidate.ReadingAnchors[:2]
		}
		raw.Directions = append(raw.Directions, candidate)
	}

	decoded, diagnostics, err := DecodeDirectionProposalWithDiagnostics(
		mustEditingJSON(t, raw),
	)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Received != 11 || diagnostics.Accepted != 9 ||
		diagnostics.Rejected != 2 || len(decoded.Directions) != 9 {
		t.Fatalf(
			"Chatto cardinality = directions %d, diagnostics %#v",
			len(decoded.Directions),
			diagnostics,
		)
	}
	wantIssues := []DirectionProposalIssue{
		{Position: 8, Code: "invalid_anchor_count"},
		{Position: 10, Code: "invalid_anchor_count"},
	}
	if !slices.Equal(diagnostics.Issues, wantIssues) {
		t.Fatalf("Chatto issues = %#v, want %#v", diagnostics.Issues, wantIssues)
	}
	wantQuestions := append([]string(nil), questions[:8]...)
	wantQuestions = append(wantQuestions, questions[9])
	for index, direction := range decoded.Directions {
		if direction.Question != wantQuestions[index] ||
			direction.DirectionID != localDirectionID(direction) {
			t.Fatalf("accepted direction %d changed: %#v", index, direction)
		}
	}
}

func TestDecodeDirectionProposalDistinguishesAnchorSelectionFailures(t *testing.T) {
	t.Parallel()

	_, legacy := studyMapFixture(t)
	base := rawDirectionsFromLegacy(legacy).Directions[0]
	valid := base
	valid.Question = "How does the valid anchor selection work?"
	valid.DirectionID = ""

	count := base
	count.Question = "How does the short anchor selection work?"
	count.DirectionID = ""
	count.AnchorIDs = count.AnchorIDs[:2]
	count.ReadingAnchors = count.ReadingAnchors[:2]

	duplicate := base
	duplicate.Question = "How does the repeated anchor selection work?"
	duplicate.DirectionID = ""
	duplicate.AnchorIDs = append([]string(nil), duplicate.AnchorIDs...)
	duplicate.AnchorIDs[1] = duplicate.AnchorIDs[0]

	malformed := base
	malformed.Question = "How does the malformed anchor selection work?"
	malformed.DirectionID = ""
	malformed.AnchorIDs = append([]string(nil), malformed.AnchorIDs...)
	malformed.AnchorIDs[1] = " " + malformed.AnchorIDs[1] + " "

	raw := DirectionProposal{
		Version:    DirectionProposalVersion,
		Directions: []DirectionCandidate{count, duplicate, malformed, valid},
	}
	decoded, diagnostics, err := DecodeDirectionProposalWithDiagnostics(mustEditingJSON(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Directions) != 1 ||
		!slices.Equal(diagnostics.Issues, []DirectionProposalIssue{
			{Position: 0, Code: "invalid_anchor_count"},
			{Position: 1, Code: "duplicate_anchor_ids"},
			{Position: 2, Code: "invalid_anchor_id"},
		}) {
		t.Fatalf("anchor diagnostics = %#v, directions %#v", diagnostics, decoded.Directions)
	}
}

func TestDecodeDirectionProposalCanonicalizesProviderReadingLabels(t *testing.T) {
	t.Parallel()

	_, legacy := studyMapFixture(t)
	raw := rawDirectionsFromLegacy(legacy)
	raw.Directions = append([]DirectionCandidate(nil), raw.Directions[:1]...)
	raw.Directions[0].ReadingAnchors = append(
		[]ReadingAnchor(nil),
		raw.Directions[0].ReadingAnchors...,
	)
	raw.Directions[0].ReadingAnchors[0].Label = "С чего начать"
	raw.Directions[0].ReadingAnchors[1].Label = "Затем изучите"
	raw.Directions[0].ReadingAnchors[2].Label = "Связанная реализация"

	decoded, diagnostics, err := DecodeDirectionProposalWithDiagnostics(
		mustEditingJSON(t, raw),
	)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Accepted != 1 || diagnostics.Rejected != 0 ||
		len(decoded.Directions) != 1 {
		t.Fatalf("decoded/diagnostics = %#v/%#v", decoded, diagnostics)
	}
	got := decoded.Directions[0].ReadingAnchors
	want := []string{"Start here", "Then inspect", "Related implementation"}
	for index := range want {
		if got[index].Label != want[index] {
			t.Fatalf("reading label %d = %q, want %q", index, got[index].Label, want[index])
		}
	}
	if _, err := DecodeNormalizedDirectionProposal(mustEditingJSON(t, decoded)); err != nil {
		t.Fatalf("canonical normalized proposal: %v", err)
	}

	raw.Directions[0].ReadingAnchors[0].Label = "Platform-specific"
	_, diagnostics, err = DecodeDirectionProposalWithDiagnostics(mustEditingJSON(t, raw))
	if err == nil || diagnostics.Accepted != 0 || diagnostics.Rejected != 1 ||
		diagnostics.Issues[0].Code != "invalid_reading_copy" {
		t.Fatalf("unknown label diagnostics=%#v error=%v", diagnostics, err)
	}
}

func TestNormalizeDirectionProposalPreservesCanonicalEnglishBytes(t *testing.T) {
	t.Parallel()

	_, legacy := studyMapFixture(t)
	canonical := directionsFromLegacy(t, legacy)
	before := mustEditingJSON(t, canonical)
	normalized, err := NormalizeDirectionProposal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	after := mustEditingJSON(t, normalized)
	if !bytes.Equal(after, before) {
		t.Fatalf(
			"ordinary canonical English proposal changed:\nbefore: %s\nafter:  %s",
			before,
			after,
		)
	}
}

func TestDecodeDirectionProposalRejectsItemsIndependentlyAndEnvelopeAtomically(
	t *testing.T,
) {
	t.Parallel()

	_, legacy := studyMapFixture(t)
	raw := rawDirectionsFromLegacy(legacy)
	raw.Directions = append([]DirectionCandidate(nil), raw.Directions[:3]...)
	raw.Directions[0].Question = "How does the first valid direction work?"
	raw.Directions[1].Question = "How does the malformed direction work?"
	raw.Directions[2].Question = "How does the final valid direction work?"

	var envelope struct {
		Version    int               `json:"version"`
		Directions []json.RawMessage `json:"directions"`
	}
	if err := json.Unmarshal(mustEditingJSON(t, raw), &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Directions[1] = append(
		[]byte(`{"unexpected":true,`),
		envelope.Directions[1][1:]...,
	)
	decoded, diagnostics, err := DecodeDirectionProposalWithDiagnostics(
		mustEditingJSON(t, envelope),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Directions) != 2 ||
		decoded.Directions[0].Question != raw.Directions[0].Question ||
		decoded.Directions[1].Question != raw.Directions[2].Question ||
		!slices.Equal(diagnostics.Issues, []DirectionProposalIssue{{
			Position: 1,
			Code:     "decode_candidate",
		}}) {
		t.Fatalf("item-local decode = %#v, diagnostics %#v", decoded, diagnostics)
	}

	duplicate := raw.Directions[0]
	raw.Directions = []DirectionCandidate{raw.Directions[0], duplicate, raw.Directions[2]}
	decoded, diagnostics, err = DecodeDirectionProposalWithDiagnostics(mustEditingJSON(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Directions) != 2 ||
		!slices.Equal(diagnostics.Issues, []DirectionProposalIssue{{
			Position: 1,
			Code:     "duplicate_direction_id",
		}}) {
		t.Fatalf("duplicate reduction = %#v, diagnostics %#v", decoded, diagnostics)
	}

	whitespaceID := raw.Directions[0]
	whitespaceID.DirectionID = " \t\n "
	raw.Directions = []DirectionCandidate{whitespaceID}
	decoded, diagnostics, err = DecodeDirectionProposalWithDiagnostics(mustEditingJSON(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	withoutModelID := whitespaceID
	withoutModelID.DirectionID = ""
	normalizedWhitespaceID, err := normalizeDirectionCandidate(withoutModelID)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Directions) != 1 ||
		!reflect.DeepEqual(decoded.Directions[0], normalizedWhitespaceID) ||
		decoded.Directions[0].DirectionID != localDirectionID(decoded.Directions[0]) ||
		diagnostics.Accepted != 1 || diagnostics.Rejected != 0 {
		t.Fatalf(
			"whitespace direction id = %#v, diagnostics %#v",
			decoded,
			diagnostics,
		)
	}

	raw.Directions = []DirectionCandidate{raw.Directions[0]}
	raw.Directions[0].AnchorIDs = raw.Directions[0].AnchorIDs[:2]
	raw.Directions[0].ReadingAnchors = raw.Directions[0].ReadingAnchors[:2]
	if _, diagnostics, err = DecodeDirectionProposalWithDiagnostics(
		mustEditingJSON(t, raw),
	); err == nil || diagnostics.Accepted != 0 || diagnostics.Rejected != 1 ||
		diagnostics.Issues[0].Code != "invalid_anchor_count" {
		t.Fatalf("zero survivors diagnostics=%#v error=%v", diagnostics, err)
	}

	tooMany := rawDirectionsFromLegacy(legacy)
	for len(tooMany.Directions) <= MaxCandidates {
		candidate := tooMany.Directions[len(tooMany.Directions)%len(baseDirections(legacy))]
		candidate.Question = fmt.Sprintf(
			"How does excessive direction %d work?",
			len(tooMany.Directions),
		)
		tooMany.Directions = append(tooMany.Directions, candidate)
	}
	if _, diagnostics, err = DecodeDirectionProposalWithDiagnostics(
		mustEditingJSON(t, tooMany),
	); err == nil || diagnostics.Received != 0 {
		t.Fatalf("excessive envelope diagnostics=%#v error=%v", diagnostics, err)
	}
}

func TestDecodeIncompleteDirectionsRetainsResolvedStartsInProviderOrder(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	base := rawDirectionsFromLegacy(legacy).Directions[0]
	raw := DirectionProposal{Version: DirectionProposalVersion}
	for index := 0; index < MaxCandidates; index++ {
		candidate := base
		candidate.DirectionID = ""
		candidate.Question = fmt.Sprintf(
			"How does retained incomplete direction %d help a new contributor?",
			index+1,
		)
		candidate.AnchorIDs = []string{"not-used-by-incomplete-projection"}
		candidate.ReadingAnchors = []ReadingAnchor{{
			AnchorID:      bundle.Anchors[index%len(bundle.Anchors)].ID,
			Label:         "Start here",
			WhatToLookFor: "Inspect the exact saved declaration and its local responsibility.",
		}}
		raw.Directions = append(raw.Directions, candidate)
	}

	got, diagnostics, err := DecodeIncompleteDirections(mustEditingJSON(t, raw), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxCandidates ||
		diagnostics.Received != MaxCandidates ||
		diagnostics.Accepted != MaxCandidates ||
		diagnostics.Rejected != 0 {
		t.Fatalf("incomplete directions = %d, diagnostics %#v", len(got), diagnostics)
	}
	for index, direction := range got {
		if direction.Question != raw.Directions[index].Question ||
			len(direction.ReadingAnchors) != 1 ||
			direction.ReadingAnchors[0].AnchorID !=
				bundle.Anchors[index%len(bundle.Anchors)].ID {
			t.Fatalf("direction %d changed provider order or exact start: %#v", index, direction)
		}
	}
}

func TestDecodeIncompleteDirectionsCanonicalizesLocalizedReadingLabel(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	candidate := rawDirectionsFromLegacy(legacy).Directions[0]
	candidate.DirectionID = ""
	candidate.ReadingAnchors = []ReadingAnchor{{
		AnchorID:      bundle.Anchors[0].ID,
		Label:         "С чего начать",
		WhatToLookFor: "Изучите точное сохранённое объявление.",
	}}
	raw := DirectionProposal{
		Version:    DirectionProposalVersion,
		Directions: []DirectionCandidate{candidate},
	}

	got, diagnostics, err := DecodeIncompleteDirections(mustEditingJSON(t, raw), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Accepted != 1 || diagnostics.Rejected != 0 ||
		len(got) != 1 || got[0].ReadingAnchors[0].Label != "Start here" {
		t.Fatalf("incomplete directions/diagnostics = %#v/%#v", got, diagnostics)
	}

	raw.Directions[0].ReadingAnchors[0].Label = "Platform-specific"
	got, diagnostics, err = DecodeIncompleteDirections(mustEditingJSON(t, raw), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 || diagnostics.Accepted != 0 || diagnostics.Rejected != 1 ||
		!slices.Equal(diagnostics.Issues, []DirectionProposalIssue{{
			Position: 0,
			Code:     "invalid_reading_anchors",
		}}) {
		t.Fatalf("unknown incomplete label was not rejected: %#v/%#v", got, diagnostics)
	}
}

func TestDecodeIncompleteDirectionsRejectsUnsafeCandidatesIndependently(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	base := rawDirectionsFromLegacy(legacy).Directions[0]
	valid := func(question, anchorID string) DirectionCandidate {
		candidate := base
		candidate.DirectionID = ""
		candidate.Question = question
		candidate.ReadingAnchors = []ReadingAnchor{{
			AnchorID:      anchorID,
			Label:         "Start here",
			WhatToLookFor: "Inspect the exact saved declaration.",
		}}
		return candidate
	}
	raw := DirectionProposal{
		Version: DirectionProposalVersion,
		Directions: []DirectionCandidate{
			valid("How does the first incomplete direction help contributors?", bundle.Anchors[0].ID),
			valid("How does "+strings.Repeat("oversized ", 128)+"metadata help?", bundle.Anchors[0].ID),
			valid("How does an unresolved incomplete direction help contributors?", "unknown-anchor"),
			valid("How does the final incomplete direction help contributors?", bundle.Anchors[1].ID),
		},
	}
	raw.Directions[2].ReadingAnchors[0].WhatToLookFor = "Then the system executes an unsupported runtime step."

	got, diagnostics, err := DecodeIncompleteDirections(mustEditingJSON(t, raw), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 ||
		got[0].Question != raw.Directions[0].Question ||
		got[1].Question != raw.Directions[3].Question ||
		!slices.Equal(diagnostics.Issues, []DirectionProposalIssue{
			{Position: 1, Code: "invalid_candidate"},
			{Position: 2, Code: "invalid_reading_anchors"},
		}) {
		t.Fatalf("item-local incomplete projection = %#v, diagnostics %#v", got, diagnostics)
	}
}

func baseDirections(proposal Proposal) []DirectionCandidate {
	return rawDirectionsFromLegacy(proposal).Directions
}

func briefShapeTopLevelTermsJSON(
	t *testing.T,
	canonical []byte,
	keepNested bool,
) []byte {
	t.Helper()

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &envelope); err != nil {
		t.Fatal(err)
	}
	var brief map[string]json.RawMessage
	if err := json.Unmarshal(envelope["brief"], &brief); err != nil {
		t.Fatal(err)
	}
	terms, exists := brief["domain_terms"]
	if !exists {
		t.Fatal("canonical fixture has no nested domain terms")
	}
	envelope["domain_terms"] = terms
	if !keepNested {
		delete(brief, "domain_terms")
	}
	envelope["brief"] = mustEditingJSON(t, brief)
	return mustEditingJSON(t, envelope)
}

func TestBuildReviewBundleBoundsExactLineNumberedSource(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	direction := directionsFromLegacy(t, legacy).Directions[0]
	for index, anchorID := range direction.AnchorIDs {
		anchorIndex := anchorIndexByID(t, bundle, anchorID)
		anchor := &bundle.Anchors[anchorIndex]
		function := largeReviewFunction(t, anchor.Path, anchor.Symbol, 100+index*200, 96, 240)
		anchor.Function = function
		anchor.Line = function.StartLine + 48
	}

	reviewBundle, err := BuildReviewBundle(bundle, direction)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviewBundle.Anchors) != len(direction.AnchorIDs) {
		t.Fatalf("review anchors = %d, want %d", len(reviewBundle.Anchors), len(direction.AnchorIDs))
	}
	totalBytes := 0
	shrunk := false
	for index, anchor := range reviewBundle.Anchors {
		if len(anchor.SourceFragment) == 0 || len(anchor.SourceFragment) > maxReviewSourceLines {
			t.Fatalf("anchor %q source lines = %d", anchor.AnchorID, len(anchor.SourceFragment))
		}
		shrunk = shrunk || len(anchor.SourceFragment) < maxReviewSourceLines
		if !slices.ContainsFunc(anchor.SourceFragment, func(line ReviewSourceLine) bool {
			return line.Line == anchor.Line
		}) {
			t.Fatalf("anchor %q exact line %d is absent", anchor.AnchorID, anchor.Line)
		}
		for _, line := range anchor.SourceFragment {
			totalBytes += reviewSourceLineBytes(line)
		}
		original := bundle.Anchors[anchorIndexByID(t, bundle, anchor.AnchorID)]
		if anchor.RepositoryRole != original.Role || anchor.Path != original.Path ||
			anchor.Symbol != original.Symbol || anchor.CurrentSentence != direction.ReadingAnchors[index].WhatToLookFor {
			t.Fatalf("review anchor lost local identity: %#v", anchor)
		}
		if len(anchor.Areas) == 0 || anchor.Areas[0].ID != original.AreaIDs[0] {
			t.Fatalf("review anchor areas = %#v", anchor.Areas)
		}
	}
	if totalBytes > maxReviewSourceBytes {
		t.Fatalf("review source bytes = %d, want <= %d", totalBytes, maxReviewSourceBytes)
	}
	if !shrunk {
		t.Fatal("aggregate source budget did not shrink any 60-line fragment")
	}
	raw := mustEditingJSON(t, reviewBundle)
	decoded, err := DecodeReviewBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.DirectionID != direction.DirectionID {
		t.Fatalf("decoded direction = %q", decoded.DirectionID)
	}
}

func TestApplyReviewsNarrowsCopyAndKeepsWeakAnchor(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	direction := directionsFromLegacy(t, legacy).Directions[0]
	direction.AnchorIDs = []string{"fact-1", "fact-2", "fact-3", "fact-4", "fact-5"}
	direction.ReadingAnchors = readingAnchors(direction.AnchorIDs)
	direction = normalizeDirectionForTest(t, direction)
	proposal := DirectionProposal{Version: DirectionProposalVersion, Directions: []DirectionCandidate{direction}}
	review := ReviewProposal{
		Version: ReviewProposalVersion, DirectionID: direction.DirectionID,
		Reviews: []AnchorReview{
			reviewAnchor("fact-1", AnchorFitDirect, ReadingRolePublicOrCLIEntry, OverclaimNone),
			reviewAnchor("fact-2", AnchorFitSupporting, ReadingRoleCoreOrchestration, OverclaimWrongResponsibility),
			reviewAnchor("fact-3", AnchorFitSupporting, ReadingRoleStateOrDataModel, OverclaimNone),
			reviewAnchor("fact-4", AnchorFitWeak, ReadingRoleRepresentativeImplementation, OverclaimBehaviorOutsideWindow),
			reviewAnchor("fact-5", AnchorFitIrrelevant, ReadingRoleExampleOrUsage, OverclaimNone),
		},
	}
	review.Reviews[0].NarrowerDisplaySentence = "Inspect the exact public entry shown here."
	review.Reviews[1].NarrowerDisplaySentence = "Inspect the local value returned by this function."
	review.Reviews[3].SupportedObservation = "The visible function assigns a local value."

	reduction, err := ApplyReviews(bundle, proposal, []ReviewProposal{review})
	if err != nil {
		t.Fatal(err)
	}
	if reduction.Reviewed != 1 || len(reduction.Directions) != 1 || len(reduction.Issues) != 0 {
		t.Fatalf("reduction = %#v", reduction)
	}
	reviewed := reduction.Directions[0]
	if slices.Contains(reviewed.Candidate.AnchorIDs, "fact-5") || len(reviewed.Candidate.AnchorIDs) != 4 {
		t.Fatalf("retained anchors = %#v", reviewed.Candidate.AnchorIDs)
	}
	reading := readingByID(reviewed.Candidate.ReadingAnchors)
	if got := reading["fact-1"].WhatToLookFor; got != review.Reviews[0].NarrowerDisplaySentence {
		t.Fatalf("bounded direct copy = %q", got)
	}
	if got := reading["fact-2"].WhatToLookFor; got != review.Reviews[1].NarrowerDisplaySentence {
		t.Fatalf("narrowed direct copy = %q", got)
	}
	if got := reading["fact-4"].WhatToLookFor; got != review.Reviews[3].SupportedObservation {
		t.Fatalf("weak fallback copy = %q", got)
	}
	if reviewed.RoleDiversity != 4 || reviewed.QualityScore != ReviewQualityScore(reviewed.Reviews) {
		t.Fatalf("quality/diversity = %d/%d", reviewed.QualityScore, reviewed.RoleDiversity)
	}
}

func TestApplyReviewsIsolatesMalformedAndMissingResponses(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	directions := directionsFromLegacy(t, legacy)
	directions.Directions = directions.Directions[:3]
	malformed := directReview(directions.Directions[0])
	malformed.Reviews[0].Fit = AnchorFit("unknown")
	accepted := directReview(directions.Directions[1])

	reduction, err := ApplyReviews(bundle, directions, []ReviewProposal{malformed, accepted})
	if err != nil {
		t.Fatal(err)
	}
	if reduction.Proposed != 3 || reduction.Reviewed != 1 ||
		len(reduction.Directions) != 1 || reduction.Directions[0].DirectionID != directions.Directions[1].DirectionID {
		t.Fatalf("reduction = %#v", reduction)
	}
	if !hasReviewIssue(reduction, directions.Directions[0].DirectionID, "review_malformed") ||
		!hasReviewIssue(reduction, directions.Directions[2].DirectionID, "review_missing") {
		t.Fatalf("issues = %#v", reduction.Issues)
	}
}

func TestApplyReviewsRejectsDirectionLevelQualityFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prepare  func(*Bundle, *DirectionCandidate, *ReviewProposal)
		wantCode string
	}{
		{
			name: "fewer than three strong fits",
			prepare: func(_ *Bundle, _ *DirectionCandidate, review *ReviewProposal) {
				review.Reviews[1].Fit = AnchorFitWeak
			},
			wantCode: "fewer_than_three_direct_or_supporting_anchors",
		},
		{
			name: "question broader in majority",
			prepare: func(_ *Bundle, _ *DirectionCandidate, review *ReviewProposal) {
				review.Reviews[0].OverclaimReasons = []OverclaimReason{OverclaimQuestionScopeBroader}
				review.Reviews[1].OverclaimReasons = []OverclaimReason{OverclaimQuestionScopeBroader}
			},
			wantCode: "question_scope_broader",
		},
		{
			name: "learning outcome broader in majority",
			prepare: func(_ *Bundle, _ *DirectionCandidate, review *ReviewProposal) {
				review.Reviews[0].OverclaimReasons = []OverclaimReason{OverclaimLearningOutcomeScopeBroader}
				review.Reviews[1].OverclaimReasons = []OverclaimReason{OverclaimLearningOutcomeScopeBroader}
			},
			wantCode: "learning_outcome_scope_broader",
		},
		{
			name: "unsupported order remains in the question",
			prepare: func(_ *Bundle, direction *DirectionCandidate, review *ReviewProposal) {
				direction.Question = "How does this code preserve the execution order?"
				review.Reviews[0].OverclaimReasons = []OverclaimReason{OverclaimUnsupportedRuntimeOrder}
			},
			wantCode: "unsupported_runtime_order",
		},
		{
			name: "unsupported Russian order remains in the question",
			prepare: func(_ *Bundle, direction *DirectionCandidate, review *ReviewProposal) {
				direction.Question = "В каком порядке выполняются вызовы этого кода?"
				review.Reviews[0].OverclaimReasons = []OverclaimReason{OverclaimUnsupportedRuntimeOrder}
			},
			wantCode: "unsupported_runtime_order",
		},
		{
			name: "unknown anchor",
			prepare: func(_ *Bundle, _ *DirectionCandidate, review *ReviewProposal) {
				review.Reviews[2].AnchorID = "fact-8"
			},
			wantCode: "review_anchor_unknown",
		},
		{
			name: "production anchor removed",
			prepare: func(bundle *Bundle, direction *DirectionCandidate, review *ReviewProposal) {
				direction.AnchorIDs = []string{"fact-1", "fact-2", "fact-3", "fact-4"}
				direction.ReadingAnchors = readingAnchors(direction.AnchorIDs)
				for _, anchorID := range []string{"fact-2", "fact-3", "fact-4"} {
					bundle.Anchors[anchorIndexByID(t, *bundle, anchorID)].Role = artifactrole.RoleTest
				}
				*review = directReview(*direction)
				review.Reviews[0].Fit = AnchorFitIrrelevant
			},
			wantCode: "production_or_operational_anchor_missing",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			bundle, legacy := studyMapFixture(t)
			direction := directionsFromLegacy(t, legacy).Directions[0]
			review := directReview(direction)
			test.prepare(&bundle, &direction, &review)
			direction = normalizeDirectionForTest(t, direction)
			review.DirectionID = direction.DirectionID
			proposal := DirectionProposal{
				Version: DirectionProposalVersion, Directions: []DirectionCandidate{direction},
			}
			reduction, err := ApplyReviews(bundle, proposal, []ReviewProposal{review})
			if err != nil {
				t.Fatal(err)
			}
			if reduction.Reviewed != 0 || !hasReviewIssue(reduction, direction.DirectionID, test.wantCode) {
				t.Fatalf("reduction = %#v, want %q", reduction, test.wantCode)
			}
		})
	}
}

func TestApplyReviewsPreservesEditorialReadingOrder(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	direction := directionsFromLegacy(t, legacy).Directions[0]
	direction.AnchorIDs = []string{"fact-3", "fact-1", "fact-2"}
	direction.ReadingAnchors = []ReadingAnchor{
		{AnchorID: "fact-2", Label: "Start here", WhatToLookFor: "Inspect the bounded declaration."},
		{AnchorID: "fact-3", Label: "Then inspect", WhatToLookFor: "Inspect the bounded declaration."},
		{AnchorID: "fact-1", Label: "Related implementation", WhatToLookFor: "Inspect the bounded declaration."},
	}
	direction = normalizeDirectionForTest(t, direction)
	review := directReview(direction)
	reduction, err := ApplyReviews(
		bundle,
		DirectionProposal{Version: DirectionProposalVersion, Directions: []DirectionCandidate{direction}},
		[]ReviewProposal{review},
	)
	if err != nil {
		t.Fatal(err)
	}
	if reduction.Reviewed != 1 {
		t.Fatalf("reduction = %#v", reduction)
	}
	got := reduction.Directions[0].Candidate.ReadingAnchors
	if len(got) != 3 || got[0].Label != "Start here" || got[0].AnchorID != "fact-2" {
		t.Fatalf("reading order = %#v", got)
	}
}

func TestApplyReviewsScoresQuestionFitFromLocalSources(t *testing.T) {
	t.Parallel()

	bundle, directions := questionFitFixture(t)
	directions, err := NormalizeDirectionProposal(directions)
	if err != nil {
		t.Fatal(err)
	}
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}
	reduction, err := ApplyReviews(bundle, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Directions) != 2 {
		t.Fatalf("reviewed directions = %d", len(reduction.Directions))
	}
	relevant := reduction.Directions[0]
	irrelevant := reduction.Directions[1]
	if relevant.QuestionFitScore <= 0 || relevant.QuestionFitScore <= irrelevant.QuestionFitScore {
		t.Fatalf(
			"question fit did not prefer local storage/backend/interface evidence: relevant=%d irrelevant=%d",
			relevant.QuestionFitScore,
			irrelevant.QuestionFitScore,
		)
	}
}

func TestQuestionFitIgnoresLocalizedProseAndKeepsTechnicalTerms(t *testing.T) {
	t.Parallel()

	english := questionFitTerms(
		"How do storage backends expose a common interface?",
		"fixture",
	)
	if !slices.Equal(english, []string{
		"storage",
		"backend",
		"expose",
		"common",
		"interface",
	}) {
		t.Fatalf("English question-fit terms changed: %#v", english)
	}
	if terms := questionFitTerms(
		"Как система обрабатывает входящий запрос?",
		"fixture",
	); len(terms) != 0 {
		t.Fatalf("localized prose became negative-fit terms: %#v", terms)
	}
	if terms := questionFitTerms(
		"Как storage backend предоставляет общий interface?",
		"fixture",
	); !slices.Equal(terms, []string{"storage", "backend", "interface"}) {
		t.Fatalf("technical terms in localized question = %#v", terms)
	}

	bundle, directions := questionFitFixture(t)
	for index := range directions.Directions {
		directions.Directions[index].Question =
			"Как storage backend предоставляет общий interface?"
	}
	directions, err := NormalizeDirectionProposal(directions)
	if err != nil {
		t.Fatal(err)
	}
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}
	reduction, err := ApplyReviews(bundle, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Directions) != 2 ||
		reduction.Directions[0].QuestionFitScore <= 0 ||
		reduction.Directions[0].QuestionFitScore <= reduction.Directions[1].QuestionFitScore {
		t.Fatalf("localized technical question fit = %#v", reduction.Directions)
	}
}

func TestCompressReviewedDirectionsPrefersQuestionFitOnTies(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	directions := directionsFromLegacy(t, legacy)
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}
	reduction, err := ApplyReviews(bundle, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Directions) <= MaxReviewedDirections {
		t.Fatalf("fixture has %d reviewed directions, need more than %d", len(reduction.Directions), MaxReviewedDirections)
	}
	for index := range reduction.Directions {
		reduction.Directions[index].Candidate.TargetJob = JobContribute
		reduction.Directions[index].Candidate.LearningStage = StageOperations
		reduction.Directions[index].QualityScore = 10
		reduction.Directions[index].RoleDiversity = 1
		reduction.Directions[index].QuestionFitScore = 0
	}
	lateFitID := reduction.Directions[len(reduction.Directions)-1].DirectionID
	reduction.Directions[len(reduction.Directions)-1].QuestionFitScore = 50

	selected := CompressReviewedDirections(bundle, reduction.Directions)
	if !slices.ContainsFunc(selected, func(direction ReviewedDirection) bool {
		return direction.DirectionID == lateFitID
	}) {
		t.Fatalf("late high-fit direction %q was displaced: %#v", lateFitID, selected)
	}
}

func TestComposeReviewedRecordCompressesDuplicatesAndKeepsV1Contract(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	directions := directionsFromLegacy(t, legacy)
	duplicateID := directions.Directions[len(directions.Directions)-1].DirectionID
	directions.Directions[len(directions.Directions)-1].Question = directions.Directions[0].Question
	directions.Directions[len(directions.Directions)-1].LearningOutcome = directions.Directions[0].LearningOutcome
	directions.Directions[len(directions.Directions)-1].DirectionID = ""
	var err error
	directions, err = NormalizeDirectionProposal(directions)
	if err != nil {
		t.Fatal(err)
	}
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}
	for index := range reviews[len(reviews)-1].Reviews {
		reviews[len(reviews)-1].Reviews[index].OverclaimReasons = []OverclaimReason{OverclaimVagueOrGeneric}
	}

	proposal, reduction, err := ComposeReviewedProposal(bundle, brief, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Candidates) < MinReviewedDirections || len(proposal.Candidates) > MaxReviewedDirections {
		t.Fatalf("composed candidates = %d", len(proposal.Candidates))
	}
	if reduction.Proposed != len(directions.Directions) ||
		reduction.Reviewed != len(directions.Directions) || reduction.Selected != len(proposal.Candidates) {
		t.Fatalf("reduction = %#v", reduction)
	}
	for _, candidate := range proposal.Candidates {
		if candidate.Confidence != "" {
			t.Fatalf("composed proposal retained model confidence %q", candidate.Confidence)
		}
	}
	questionCount := 0
	for _, candidate := range proposal.Candidates {
		if candidate.Question == directions.Directions[0].Question {
			questionCount++
		}
	}
	if questionCount != 1 {
		t.Fatalf("semantic duplicate count = %d; lower-quality %q survived", questionCount, duplicateID)
	}

	record, recordReduction, err := BuildReviewedRecord(bundle, brief, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if record.Version != RecordVersion || RecordVersion != 1 {
		t.Fatalf("record version = %d", record.Version)
	}
	if len(record.Directions) < MinReviewedDirections || len(record.Directions) > MaxReviewedDirections ||
		recordReduction.Selected != len(record.Directions) {
		t.Fatalf("record/reduction = %d/%#v", len(record.Directions), recordReduction)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestComposeReviewedRecordPublishesOneAcceptedDirection(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	directions := directionsFromLegacy(t, legacy)
	directions.Directions = directions.Directions[:1]
	reviews := []ReviewProposal{directReview(directions.Directions[0])}

	proposal, reduction, err := ComposeReviewedProposal(bundle, brief, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Candidates) != 1 || reduction.Selected != 1 {
		t.Fatalf("proposal/reduction = %d/%#v", len(proposal.Candidates), reduction)
	}

	record, recordReduction, err := BuildReviewedRecord(bundle, brief, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Directions) != 1 || recordReduction.Selected != 1 {
		t.Fatalf("record/reduction = %d/%#v", len(record.Directions), recordReduction)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestComposeReviewedProposalRejectsZeroAcceptedDirections(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	directions := directionsFromLegacy(t, legacy)
	directions.Directions = directions.Directions[:1]

	_, reduction, err := ComposeReviewedProposal(bundle, brief, directions, nil)
	if err == nil || !strings.Contains(err.Error(), "reviewed selection has 0 directions; need at least 1") {
		t.Fatalf("ComposeReviewedProposal() error = %v", err)
	}
	if reduction.Selected != 0 || reduction.Reviewed != 0 {
		t.Fatalf("reduction = %#v", reduction)
	}
}

func TestCompressReviewedDirectionsReservesFirstContact(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	directions := directionsFromLegacy(t, legacy)
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}
	reduction, err := ApplyReviews(bundle, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if len(reduction.Directions) <= MaxReviewedDirections {
		t.Fatalf("fixture has %d reviewed directions, need more than %d", len(reduction.Directions), MaxReviewedDirections)
	}
	for index := range reduction.Directions {
		reduction.Directions[index].Candidate.TargetJob = JobContribute
		reduction.Directions[index].QualityScore = 100 - index
	}
	firstContactID := reduction.Directions[len(reduction.Directions)-1].DirectionID
	reduction.Directions[len(reduction.Directions)-1].Candidate.TargetJob = JobFirstContact
	reduction.Directions[len(reduction.Directions)-1].QualityScore = 1

	selected := CompressReviewedDirections(bundle, reduction.Directions)
	if !slices.ContainsFunc(selected, func(direction ReviewedDirection) bool {
		return direction.DirectionID == firstContactID
	}) {
		t.Fatalf("first-contact direction %q was displaced: %#v", firstContactID, selected)
	}
}

func TestBuildReviewedRecordTreatsBriefSupportIDsAsUnordered(t *testing.T) {
	t.Parallel()

	bundle, legacy := studyMapFixture(t)
	brief := briefShapeFromLegacy(legacy)
	brief.Brief.WhatItIs.SupportIDs = []string{bundle.Anchors[0].ID, bundle.Areas[0].ID}
	if brief.Brief.WhatItIs.SupportIDs[0] < brief.Brief.WhatItIs.SupportIDs[1] {
		brief.Brief.WhatItIs.SupportIDs[0], brief.Brief.WhatItIs.SupportIDs[1] =
			brief.Brief.WhatItIs.SupportIDs[1], brief.Brief.WhatItIs.SupportIDs[0]
	}
	directions := directionsFromLegacy(t, legacy)
	reviews := make([]ReviewProposal, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		reviews = append(reviews, directReview(direction))
	}

	record, _, err := BuildReviewedRecord(bundle, brief, directions, reviews)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func briefShapeFromLegacy(proposal Proposal) BriefShapeProposal {
	return BriefShapeProposal{
		Version: BriefShapeProposalVersion, RepositoryType: proposal.RepositoryType,
		Brief: proposal.Brief, ShapeAreaIDs: append([]string(nil), proposal.ShapeAreaIDs...),
	}
}

func rawDirectionsFromLegacy(proposal Proposal) DirectionProposal {
	result := DirectionProposal{Version: DirectionProposalVersion}
	for _, candidate := range proposal.Candidates {
		result.Directions = append(result.Directions, DirectionCandidate{
			Question: candidate.Question, WhyItMatters: candidate.WhyItMatters,
			LearningOutcome: candidate.LearningOutcome, TargetJob: candidate.TargetJob,
			LearningStage: candidate.LearningStage,
			AnchorIDs:     append([]string(nil), candidate.AnchorIDs...),
			DocumentIDs:   append([]string(nil), candidate.DocumentIDs...),
			AreaIDs:       append([]string(nil), candidate.AreaIDs...), MechanismID: candidate.MechanismID,
			ReadingAnchors: append([]ReadingAnchor(nil), candidate.ReadingAnchors...),
			SearchQueries:  append([]string(nil), candidate.SearchQueries...),
		})
	}
	return result
}

func directionsFromLegacy(t *testing.T, proposal Proposal) DirectionProposal {
	t.Helper()
	result, err := NormalizeDirectionProposal(rawDirectionsFromLegacy(proposal))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func normalizeDirectionForTest(t *testing.T, direction DirectionCandidate) DirectionCandidate {
	t.Helper()
	direction.DirectionID = ""
	proposal, err := NormalizeDirectionProposal(DirectionProposal{
		Version: DirectionProposalVersion, Directions: []DirectionCandidate{direction},
	})
	if err != nil {
		t.Fatal(err)
	}
	return proposal.Directions[0]
}

func directReview(direction DirectionCandidate) ReviewProposal {
	roles := []ReadingRole{
		ReadingRolePublicOrCLIEntry,
		ReadingRoleCoreOrchestration,
		ReadingRoleStateOrDataModel,
		ReadingRoleEffectOrIntegrationBoundary,
		ReadingRoleRepresentativeImplementation,
	}
	proposal := ReviewProposal{Version: ReviewProposalVersion, DirectionID: direction.DirectionID}
	for index, anchorID := range direction.AnchorIDs {
		proposal.Reviews = append(proposal.Reviews, reviewAnchor(
			anchorID, AnchorFitDirect, roles[index%len(roles)], OverclaimNone,
		))
	}
	return proposal
}

func reviewAnchor(
	anchorID string,
	fit AnchorFit,
	role ReadingRole,
	reason OverclaimReason,
) AnchorReview {
	return AnchorReview{
		AnchorID: anchorID, Fit: fit,
		SupportedObservation: "The visible function contains the selected local implementation.",
		Role:                 role, OverclaimReasons: []OverclaimReason{reason},
	}
}

func readingAnchors(anchorIDs []string) []ReadingAnchor {
	labels := []string{"Start here", "Then inspect", "Related implementation", "Public boundary", "Core data type"}
	result := make([]ReadingAnchor, 0, len(anchorIDs))
	for index, anchorID := range anchorIDs {
		result = append(result, ReadingAnchor{
			AnchorID: anchorID, Label: labels[index],
			WhatToLookFor: "Inspect the bounded declaration and its local implementation.",
		})
	}
	return result
}

func readingByID(reading []ReadingAnchor) map[string]ReadingAnchor {
	result := make(map[string]ReadingAnchor, len(reading))
	for _, anchor := range reading {
		result[anchor.AnchorID] = anchor
	}
	return result
}

func questionFitFixture(t *testing.T) (Bundle, DirectionProposal) {
	t.Helper()

	areas := []Area{
		{
			ID:             "area-storage",
			Name:           "Storage backend interface",
			Responsibility: "Defines the common storage backend interface.",
		},
		{
			ID:             "area-routing",
			Name:           "HTTP routing",
			Responsibility: "Routes HTTP requests to handlers.",
		},
	}
	anchorSpecs := []struct {
		id        string
		path      string
		symbol    string
		statement string
		areaID    string
	}{
		{"fact-storage", "internal/backend/storage.go", "storageBackend", "Storage backend accepts blob writes.", "area-storage"},
		{"fact-backend", "internal/backend/backend.go", "backendFactory", "Backend factory creates storage backends.", "area-storage"},
		{"fact-interface", "internal/backend/interface.go", "commonInterface", "Common interface methods hide backend details.", "area-storage"},
		{"fact-router", "internal/http/router.go", "routeRequest", "Router dispatches requests.", "area-routing"},
		{"fact-cache", "internal/http/cache.go", "responseCache", "Cache stores HTTP responses.", "area-routing"},
		{"fact-log", "internal/http/log.go", "requestLogger", "Logger records request metadata.", "area-routing"},
	}
	allowed := []string{"docs/storage.md", "docs/routing.md"}
	anchors := make([]Anchor, 0, len(anchorSpecs))
	for index, spec := range anchorSpecs {
		function := testFunction(t, spec.path, spec.symbol, 10+index*20)
		anchors = append(anchors, Anchor{
			ID: spec.id, Path: spec.path, Symbol: spec.symbol,
			Line: function.StartLine + 1, Role: artifactrole.RoleProductionCore,
			Statement: spec.statement, AreaIDs: []string{spec.areaID}, Function: function,
		})
		allowed = append(allowed, spec.path)
	}
	bundle := Bundle{
		Version: BundleVersion, RepoName: "fixture",
		DocumentedPurpose:  "Fixture demonstrates local question fit.",
		RepositoryTypeHint: RepositoryLibrary,
		Areas:              areas,
		Anchors:            anchors,
		Documents: []Document{
			{
				ID:      "doc-storage",
				Path:    "docs/storage.md",
				Label:   "Storage backends",
				Excerpt: "Storage backends expose a common interface.",
			},
			{
				ID:      "doc-routing",
				Path:    "docs/routing.md",
				Label:   "Routing",
				Excerpt: "HTTP routes are dispatched to request handlers.",
			},
		},
		AllowedPaths: allowed,
	}
	question := "How do storage backends expose a common interface?"
	proposal := DirectionProposal{
		Version: DirectionProposalVersion,
		Directions: []DirectionCandidate{
			{
				Question: question, WhyItMatters: "This explains a useful repository responsibility.",
				LearningOutcome: "The reader can name the relevant code and responsibility.",
				TargetJob:       JobMaintain, LearningStage: StageCoreModel,
				AnchorIDs:      []string{"fact-storage", "fact-backend", "fact-interface"},
				DocumentIDs:    []string{"doc-storage"},
				AreaIDs:        []string{"area-storage"},
				ReadingAnchors: readingAnchors([]string{"fact-storage", "fact-backend", "fact-interface"}),
			},
			{
				Question: question, WhyItMatters: "This explains a useful repository responsibility.",
				LearningOutcome: "The reader can name the relevant code and responsibility.",
				TargetJob:       JobMaintain, LearningStage: StageCoreModel,
				AnchorIDs:      []string{"fact-router", "fact-cache", "fact-log"},
				DocumentIDs:    []string{"doc-routing"},
				AreaIDs:        []string{"area-routing"},
				ReadingAnchors: readingAnchors([]string{"fact-router", "fact-cache", "fact-log"}),
			},
		},
	}
	return bundle, proposal
}

func largeReviewFunction(
	t *testing.T,
	filePath string,
	symbol string,
	startLine int,
	bodyLines int,
	width int,
) sourcewindowfacts.Function {
	t.Helper()
	lines := []string{"func " + symbol + "() int {", "\tvalue := 1"}
	for index := 0; index < bodyLines; index++ {
		lines = append(lines, fmt.Sprintf("\t_ = \"%03d-%s\"", index, strings.Repeat("x", width)))
	}
	lines = append(lines, "\treturn value", "}")
	window, err := sourcewindowfacts.NewWindow("review-"+symbol, filePath, startLine, lines)
	if err != nil {
		t.Fatal(err)
	}
	function, err := sourcewindowfacts.ExtractGoFunction(window, symbol)
	if err != nil {
		t.Fatal(err)
	}
	return function
}

func anchorIndexByID(t *testing.T, bundle Bundle, anchorID string) int {
	t.Helper()
	for index, anchor := range bundle.Anchors {
		if anchor.ID == anchorID {
			return index
		}
	}
	t.Fatalf("missing fixture anchor %q", anchorID)
	return -1
}

func hasReviewIssue(reduction ReviewReduction, directionID, code string) bool {
	for _, issue := range reduction.Issues {
		if issue.DirectionID == directionID && issue.Code == code {
			return true
		}
	}
	return false
}

func mustEditingJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
