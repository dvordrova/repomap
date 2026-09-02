package groupmatching

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

type matchingPresetProvider struct {
	mu                 sync.Mutex
	rejectPreparedPair bool
	attempts           int
	requests           []Request
	userWires          []string
	respond            func(Request) []byte
}

func (*matchingPresetProvider) State() []byte {
	return []byte(`{"provider":"group-matching-preset-v6"}`)
}

func (provider *matchingPresetProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var request Request
	if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
		return llm.Prepared{}, err
	}
	if provider.rejectPreparedPair && request.Pair.Ref != "" {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitRequestBytes, Limit: 1,
			Observed: 2, ObservedKnown: true,
		})
	}
	wire, err := json.Marshal(struct {
		System string `json:"system"`
		User   string `json:"user"`
	}{System: prompt.System, User: prompt.User})
	if err != nil {
		return llm.Prepared{}, err
	}
	if len(wire) > limits.MaxRequestBytes {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitRequestBytes, Limit: limits.MaxRequestBytes,
			Observed: len(wire), ObservedKnown: true,
		})
	}
	return llm.NewPrepared(wire)
}

func (provider *matchingPresetProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var envelope struct {
		System string `json:"system"`
		User   string `json:"user"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &envelope); err != nil {
		return llm.Completion{}, err
	}
	var request Request
	if err := json.Unmarshal([]byte(envelope.User), &request); err != nil {
		return llm.Completion{}, err
	}
	if err := validateMatchingPresetRequest(request); err != nil {
		return llm.Completion{}, err
	}
	provider.mu.Lock()
	provider.attempts++
	provider.mu.Unlock()
	provider.mu.Lock()
	provider.requests = append(provider.requests, request)
	provider.userWires = append(provider.userWires, envelope.User)
	provider.mu.Unlock()
	response := []byte(`{"connections":[]}`)
	if provider.respond != nil {
		response = provider.respond(request)
	}
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, Latency: time.Millisecond},
	}, nil
}

func validateMatchingPresetRequest(request Request) error {
	if request.Version != requestVersion || !strings.HasPrefix(request.Pair.Ref, "p") ||
		request.Targets == nil || request.Subjects == nil || request.StructuralEdges == nil ||
		request.LocalConnections == nil || request.WitnessCandidates == nil {
		return fmt.Errorf("preset: incomplete request")
	}
	targets := make(map[string]struct{}, len(request.Targets))
	for _, target := range request.Targets {
		if !strings.HasPrefix(target.Ref, "t") {
			return fmt.Errorf("preset: invalid target %#v", target)
		}
		targets[target.Ref] = struct{}{}
	}
	groups := make(map[string]groupWire, 2)
	for _, group := range []groupWire{request.Pair.LeftGroup, request.Pair.RightGroup} {
		if !strings.HasPrefix(group.Ref, "g") || group.MemberRefs == nil || group.EvidenceRefs == nil ||
			group.BoundaryEdgeRefs == nil {
			return fmt.Errorf("preset: invalid group %#v", group)
		}
		if _, known := targets[group.TargetRef]; !known {
			return fmt.Errorf("preset: group has unknown target %q", group.TargetRef)
		}
		groups[group.Ref] = group
	}
	subjects := make(map[string]subjectWire, len(request.Subjects))
	for _, subject := range request.Subjects {
		if !strings.HasPrefix(subject.Ref, "s") || (subject.Object == nil) == (subject.Pattern == nil) {
			return fmt.Errorf("preset: invalid subject %#v", subject)
		}
		if _, known := targets[subject.TargetRef]; !known {
			return fmt.Errorf("preset: subject has unknown target %q", subject.TargetRef)
		}
		subjects[subject.Ref] = subject
	}
	arguments, candidates, err := validateClosedSubjectWires(request.Subjects, subjects)
	if err != nil {
		return err
	}
	for _, group := range []groupWire{request.Pair.LeftGroup, request.Pair.RightGroup} {
		refs := append(append([]string(nil), group.MemberRefs...), group.EvidenceRefs...)
		for _, ref := range refs {
			if _, known := subjects[ref]; !known {
				return fmt.Errorf("preset: group has unknown subject %q", ref)
			}
		}
	}
	left, leftOK := groups[request.Pair.LeftGroup.Ref]
	right, rightOK := groups[request.Pair.RightGroup.Ref]
	if !leftOK || !rightOK || left.TargetRef == right.TargetRef {
		return fmt.Errorf("preset: invalid cross-target pair %#v", request.Pair)
	}
	edges := make(map[string]structuralEdgeWire, len(request.StructuralEdges))
	for _, edge := range request.StructuralEdges {
		if !strings.HasPrefix(edge.Ref, "e") {
			return fmt.Errorf("preset: invalid edge ref %q", edge.Ref)
		}
		if _, known := subjects[edge.FromRef]; !known {
			return fmt.Errorf("preset: edge has unknown source %q", edge.FromRef)
		}
		if _, known := subjects[edge.ToRef]; !known {
			return fmt.Errorf("preset: edge has unknown destination %q", edge.ToRef)
		}
		if edge.ArgumentRef != "" {
			if _, known := arguments[edge.ArgumentRef]; !known {
				return fmt.Errorf("preset: edge has unknown argument %q", edge.ArgumentRef)
			}
		}
		if edge.ValueCandidateRef != "" {
			if _, known := candidates[edge.ValueCandidateRef]; !known {
				return fmt.Errorf("preset: edge has unknown value candidate %q", edge.ValueCandidateRef)
			}
		}
		if edge.SourceArgumentRef != "" {
			if _, known := arguments[edge.SourceArgumentRef]; !known {
				return fmt.Errorf("preset: edge has unknown source argument %q", edge.SourceArgumentRef)
			}
		}
		edges[edge.Ref] = edge
	}
	for _, group := range []groupWire{request.Pair.LeftGroup, request.Pair.RightGroup} {
		for _, ref := range group.BoundaryEdgeRefs {
			edge, known := edges[ref]
			if !known || subjects[edge.FromRef].Pattern == nil || subjects[edge.ToRef].Object == nil ||
				!programindex.IsExternalPackageAuthority(subjects[edge.ToRef].Object.External) {
				return fmt.Errorf("preset: group has invalid or open boundary edge %q", ref)
			}
		}
	}
	argumentOwners := make(map[string]string)
	for _, subject := range request.Subjects {
		if subject.Pattern == nil {
			continue
		}
		for _, argument := range subject.Pattern.Arguments {
			argumentOwners[argument.Ref] = subject.Ref
		}
	}
	leftBoundaryEdges := make(map[string]struct{}, len(request.Pair.LeftGroup.BoundaryEdgeRefs))
	for _, ref := range request.Pair.LeftGroup.BoundaryEdgeRefs {
		leftBoundaryEdges[ref] = struct{}{}
	}
	rightBoundaryEdges := make(map[string]struct{}, len(request.Pair.RightGroup.BoundaryEdgeRefs))
	for _, ref := range request.Pair.RightGroup.BoundaryEdgeRefs {
		rightBoundaryEdges[ref] = struct{}{}
	}
	jointRefs := make(map[string]struct{}, len(request.WitnessCandidates))
	for _, candidate := range request.WitnessCandidates {
		if !strings.HasPrefix(candidate.Ref, "j") || candidate.Kind != witnessJointArgumentValue ||
			!candidate.SupportResolution.Valid() {
			return fmt.Errorf("preset: invalid witness candidate %#v", candidate)
		}
		hasRequiredFrom := candidate.RequiredFromGroupRef != ""
		hasRequiredTo := candidate.RequiredToGroupRef != ""
		if hasRequiredFrom != hasRequiredTo {
			return fmt.Errorf("preset: witness candidate has an incomplete required direction %#v", candidate)
		}
		if hasRequiredFrom {
			forward := candidate.RequiredFromGroupRef == request.Pair.LeftGroup.Ref &&
				candidate.RequiredToGroupRef == request.Pair.RightGroup.Ref
			reverse := candidate.RequiredFromGroupRef == request.Pair.RightGroup.Ref &&
				candidate.RequiredToGroupRef == request.Pair.LeftGroup.Ref
			if !forward && !reverse {
				return fmt.Errorf("preset: witness candidate has a direction outside its pair %#v", candidate)
			}
		}
		if _, duplicate := jointRefs[candidate.Ref]; duplicate {
			return fmt.Errorf("preset: duplicate witness candidate %q", candidate.Ref)
		}
		jointRefs[candidate.Ref] = struct{}{}
		leftEdge, leftEdgeKnown := edges[candidate.LeftBoundaryEdgeRef]
		rightEdge, rightEdgeKnown := edges[candidate.RightBoundaryEdgeRef]
		_, leftAdvertised := leftBoundaryEdges[candidate.LeftBoundaryEdgeRef]
		_, rightAdvertised := rightBoundaryEdges[candidate.RightBoundaryEdgeRef]
		if !leftEdgeKnown || !rightEdgeKnown || !leftAdvertised || !rightAdvertised ||
			leftEdge.FromRef != candidate.LeftPatternRef || rightEdge.FromRef != candidate.RightPatternRef ||
			argumentOwners[candidate.LeftArgumentRef] != candidate.LeftPatternRef ||
			argumentOwners[candidate.RightArgumentRef] != candidate.RightPatternRef {
			return fmt.Errorf("preset: witness candidate has open or inconsistent refs %#v", candidate)
		}
	}
	for _, connection := range request.LocalConnections {
		if !strings.HasPrefix(connection.Ref, "c") {
			return fmt.Errorf("preset: invalid local connection ref %q", connection.Ref)
		}
		if !strings.HasPrefix(connection.FromGroup.Ref, "g") || !strings.HasPrefix(connection.ToGroup.Ref, "g") ||
			connection.FromGroup.TargetRef == "" || connection.FromGroup.TargetRef != connection.ToGroup.TargetRef ||
			(connection.FromGroup.Ref != request.Pair.LeftGroup.Ref && connection.FromGroup.Ref != request.Pair.RightGroup.Ref &&
				connection.ToGroup.Ref != request.Pair.LeftGroup.Ref && connection.ToGroup.Ref != request.Pair.RightGroup.Ref) {
			return fmt.Errorf("preset: invalid local connection %#v", connection)
		}
		for _, ref := range connection.EvidenceRefs {
			if _, known := subjects[ref]; !known {
				return fmt.Errorf("preset: local connection has unknown evidence %q", ref)
			}
		}
	}
	return nil
}

func validateClosedSubjectWires(
	rows []subjectWire,
	subjects map[string]subjectWire,
) (map[string]struct{}, map[string]struct{}, error) {
	arguments := make(map[string]struct{})
	candidates := make(map[string]struct{})
	for _, subject := range rows {
		if subject.Pattern == nil {
			continue
		}
		for _, argument := range subject.Pattern.Arguments {
			if !strings.HasPrefix(argument.Ref, "a") {
				return nil, nil, fmt.Errorf("preset: invalid argument ref %q", argument.Ref)
			}
			arguments[argument.Ref] = struct{}{}
			for _, candidate := range argument.ValueCandidates {
				if !strings.HasPrefix(candidate.Ref, "v") {
					return nil, nil, fmt.Errorf("preset: invalid value candidate ref %q", candidate.Ref)
				}
				candidates[candidate.Ref] = struct{}{}
			}
		}
	}
	knownSubject := func(ref string) bool {
		if ref == "" {
			return true
		}
		_, known := subjects[ref]
		return known
	}
	for _, subject := range rows {
		if subject.Object != nil {
			if !knownSubject(subject.Object.OwnerRef) || !knownSubject(subject.Object.ContainerRef) {
				return nil, nil, fmt.Errorf("preset: object has an open owner/container ref %#v", subject)
			}
			continue
		}
		pattern := subject.Pattern
		refs := []string{pattern.FromRef, pattern.ResultRef, pattern.ReceiverRef}
		refs = append(refs, pattern.ToRefs...)
		refs = append(refs, pattern.ReceiverOriginRefs...)
		for _, ref := range refs {
			if !knownSubject(ref) {
				return nil, nil, fmt.Errorf("preset: pattern has an open subject ref %q", ref)
			}
		}
		for _, argument := range pattern.Arguments {
			for _, ref := range argument.ObjectRefs {
				if !knownSubject(ref) {
					return nil, nil, fmt.Errorf("preset: argument has an open object ref %q", ref)
				}
			}
			for _, candidate := range argument.ValueCandidates {
				for _, ref := range candidate.SourceObjectRefs {
					if !knownSubject(ref) {
						return nil, nil, fmt.Errorf("preset: value candidate has an open source object ref %q", ref)
					}
				}
				for _, ref := range candidate.SourceArgumentRefs {
					if _, known := arguments[ref]; !known {
						return nil, nil, fmt.Errorf("preset: value candidate has an open source argument ref %q", ref)
					}
				}
			}
		}
	}
	return arguments, candidates, nil
}

func TestRunExhaustivelyMatchesThreeTargetsWithSparseOpenEdges(t *testing.T) {
	indexes := []groupindex.Index{
		matchingTestIndex(t, "python", "py"),
		matchingTestIndex(t, "go", "go"),
		matchingTestIndex(t, "typescript", "ts"),
	}
	before := snapshotIndexes(indexes)
	provider := &matchingPresetProvider{respond: matchingResponse}
	matched, diagnostics, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 4, BatchController: &llm.BatchController{},
	}, provider, indexes)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !reflect.DeepEqual(indexes, before) {
		t.Fatal("Run mutated its complete GroupsIndex input")
	}
	if err := groupindex.ValidateSet(matched); err != nil {
		t.Fatalf("ValidateSet: %v", err)
	}
	assertMatchingDiagnostic(t, diagnostics, diagnosticMalformedConnection)
	assertMatchingDiagnostic(t, diagnostics, diagnosticUnknownPairRef)
	assertMatchingDiagnostic(t, diagnostics, diagnosticInvalidPairEndpoint)
	assertMatchingDiagnostic(t, diagnostics, diagnosticInvalidWitnessJoint)

	var cross *groupindex.Connection
	for indexPosition := range matched {
		for connectionPosition := range matched[indexPosition].Connections {
			connection := &matched[indexPosition].Connections[connectionPosition]
			if connection.From.TargetID != connection.To.TargetID {
				if cross != nil {
					t.Fatalf("more than one sparse cross-target edge: %#v and %#v", *cross, *connection)
				}
				cross = connection
			}
		}
	}
	if cross == nil || cross.SemanticKind != "translates_domain_commands_for" || len(cross.Evidence) != 2 ||
		!cross.SupportResolution.Valid() {
		t.Fatalf("custom cross-target connection = %#v", cross)
	}
	if targetLanguage(indexes, cross.From.TargetID) != "python" || targetLanguage(indexes, cross.To.TargetID) != "go" {
		t.Fatalf("model-selected reverse direction = %#v", *cross)
	}
	for _, index := range matched {
		want := 1
		if index.Target.ID == cross.From.TargetID {
			want = 2
		}
		if len(index.Connections) != want {
			t.Fatalf("target %s stores %d connections, want %d (cross edge only in From target)", index.Target.Language, len(index.Connections), want)
		}
	}

	provider.mu.Lock()
	requests := append([]Request(nil), provider.requests...)
	userWires := append([]string(nil), provider.userWires...)
	provider.mu.Unlock()
	if len(requests) != 12 {
		t.Fatalf("provider requests = %d, want 3 target pairs x 2 x 2 groups", len(requests))
	}
	var pairRefs []string
	for _, request := range requests {
		if request.Pair.Ref == "" || len(request.Targets) != 2 ||
			len(request.LocalConnections) != 2 || len(request.StructuralEdges) == 0 {
			t.Fatalf("pair lost its closed dossier context: %#v", request)
		}
		for _, subject := range request.Subjects {
			if subject.Object != nil && strings.Contains(subject.Object.Name, "Unrelated") {
				t.Fatalf("pair dossier pulled unrelated subject %#v", subject)
			}
		}
		assertValueCandidateProjection(t, request)
		if len(request.WitnessCandidates) == 0 {
			t.Fatalf("pair request has no prevalidated witness candidates: %#v", request.Pair)
		}
		pairRefs = append(pairRefs, request.Pair.Ref)
	}
	sort.Strings(pairRefs)
	wantPairRefs := make([]string, 12)
	for position := range wantPairRefs {
		wantPairRefs[position] = fmt.Sprintf("p%d", position+1)
	}
	sort.Strings(wantPairRefs)
	if !reflect.DeepEqual(pairRefs, wantPairRefs) {
		t.Fatalf("exhaustive pair cover = %#v, want %#v", pairRefs, wantPairRefs)
	}
	assertProviderWiresHideCanonicalIdentities(t, indexes, userWires)
}

func TestPromptMakesSemanticDirectionIndependentOfPairOrder(t *testing.T) {
	for _, required := range []string{
		"Request order is never direction",
		"FROM semantic_kind TO",
		"frontend group is `from_group_ref`",
		"backend API group is `to_group_ref`",
		"required_from_group_ref",
		"Conflicting dual-role subjects do not",
		"receive this constraint",
	} {
		if !strings.Contains(promptText, required) {
			t.Fatalf("matching prompt lost direction rule %q", required)
		}
	}
}

func TestNormalizeResponseEnforcesLocallyProvenOutboundToInboundDirection(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingDirectionTestIndex(t, "python", "direction-inbound", true, false, false),
		matchingDirectionTestIndex(t, "typescript", "direction-outbound", false, true, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(compilation.pairs) != 1 {
		t.Fatalf("direction fixture pairs = %d, want 1", len(compilation.pairs))
	}
	pair := compilation.pairs[0]
	request, err := compilation.request(pair.ref)
	if err != nil {
		t.Fatal(err)
	}
	var candidate witnessCandidateWire
	for _, row := range request.WitnessCandidates {
		if row.RequiredFromGroupRef != "" {
			candidate = row
			break
		}
	}
	if candidate.Ref == "" || candidate.RequiredToGroupRef == "" ||
		candidate.RequiredFromGroupRef == candidate.RequiredToGroupRef {
		t.Fatalf("direction fixture exposed no constrained witness: %#v", request.WitnessCandidates)
	}

	correct := matchingDirectionResponse(t, request.Pair.Ref,
		candidate.RequiredFromGroupRef, candidate.RequiredToGroupRef, []string{candidate.Ref})
	normalized, err := normalizeResponse(correct, compilation, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.connections) != 1 ||
		normalized.connections[0].fromGroupRef != candidate.RequiredFromGroupRef ||
		normalized.connections[0].toGroupRef != candidate.RequiredToGroupRef {
		t.Fatalf("correct constrained direction = %#v", normalized.connections)
	}

	reversed := matchingDirectionResponse(t, request.Pair.Ref,
		candidate.RequiredToGroupRef, candidate.RequiredFromGroupRef, []string{candidate.Ref})
	normalized, err = normalizeResponse(reversed, compilation, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.connections) != 0 {
		t.Fatalf("reverse constrained direction was accepted or flipped: %#v", normalized.connections)
	}
	assertMatchingDiagnostic(t, normalized.diagnostics, diagnosticInvalidDirection)

	tampered := request
	tampered.WitnessCandidates = append([]witnessCandidateWire(nil), request.WitnessCandidates...)
	for position := range tampered.WitnessCandidates {
		if tampered.WitnessCandidates[position].Ref != candidate.Ref {
			continue
		}
		tampered.WitnessCandidates[position].RequiredFromGroupRef = candidate.RequiredToGroupRef
		tampered.WitnessCandidates[position].RequiredToGroupRef = candidate.RequiredFromGroupRef
	}
	normalized, err = normalizeResponse(correct, compilation, tampered)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.connections) != 0 {
		t.Fatalf("tampered request direction created a connection: %#v", normalized.connections)
	}
	assertMatchingDiagnostic(t, normalized.diagnostics, diagnosticInvalidWitnessJoint)
	assertMatchingDiagnostic(t, normalized.diagnostics, diagnosticMissingWitnessJoint)
}

func TestRequiredDirectionLeavesDualRoleInboundPatternModelSelected(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingDirectionTestIndex(t, "python", "direction-dual", true, false, true),
		matchingDirectionTestIndex(t, "typescript", "direction-exact", false, true, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(compilation.pairs) != 1 {
		t.Fatalf("dual-role fixture pairs = %d, want 1", len(compilation.pairs))
	}
	request, err := compilation.request(compilation.pairs[0].ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.WitnessCandidates) == 0 {
		t.Fatal("dual-role fixture exposed no witness candidate")
	}
	for _, candidate := range request.WitnessCandidates {
		if candidate.RequiredFromGroupRef != "" || candidate.RequiredToGroupRef != "" {
			t.Fatalf("dual-role inbound pattern gained direction authority: %#v", candidate)
		}
	}
}

func TestNormalizeResponseRejectsConflictingSelectedRequiredDirections(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingDirectionTestIndex(t, "python", "direction-both-python", true, true, false),
		matchingDirectionTestIndex(t, "typescript", "direction-both-typescript", true, true, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(compilation.pairs) != 1 {
		t.Fatalf("conflicting-direction fixture pairs = %d, want 1", len(compilation.pairs))
	}
	pair := compilation.pairs[0]
	request, err := compilation.request(pair.ref)
	if err != nil {
		t.Fatal(err)
	}
	byDirection := make(map[string]string)
	for _, candidate := range request.WitnessCandidates {
		if candidate.RequiredFromGroupRef == "" {
			continue
		}
		key := candidate.RequiredFromGroupRef + "->" + candidate.RequiredToGroupRef
		if _, exists := byDirection[key]; !exists {
			byDirection[key] = candidate.Ref
		}
	}
	forwardKey := pair.leftGroupRef + "->" + pair.rightGroupRef
	reverseKey := pair.rightGroupRef + "->" + pair.leftGroupRef
	if byDirection[forwardKey] == "" || byDirection[reverseKey] == "" {
		t.Fatalf("fixture required directions = %#v, candidates=%#v", byDirection, request.WitnessCandidates)
	}
	raw := matchingDirectionResponse(t, request.Pair.Ref, pair.leftGroupRef, pair.rightGroupRef,
		[]string{byDirection[forwardKey], byDirection[reverseKey]})
	normalized, err := normalizeResponse(raw, compilation, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.connections) != 0 {
		t.Fatalf("conflicting selected directions created a connection: %#v", normalized.connections)
	}
	assertMatchingDiagnostic(t, normalized.diagnostics, diagnosticInvalidDirection)
}

func TestNormalizeResponseSelectsOnlyClosedWitnessRefsAndRestoresPatternEvidence(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndex(t, "go", "filter-go"),
		matchingTestIndex(t, "python", "filter-python"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(compilation.pairs) == 0 {
		t.Fatal("fixture compiled no cross-target group pairs")
	}
	pair := compilation.pairs[0]
	request, err := compilation.request(pair.ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Subjects) == 0 {
		t.Fatal("pair request has no evidence subjects")
	}
	if len(request.WitnessCandidates) == 0 {
		t.Fatal("pair request has no valid fixture witness candidate")
	}
	candidate := request.WitnessCandidates[0]
	raw, err := json.Marshal(struct {
		Connections []responseConnection `json:"connections"`
	}{Connections: []responseConnection{
		{
			PairRef: pair.ref, FromGroupRef: pair.leftGroupRef, ToGroupRef: pair.rightGroupRef,
			SemanticKind: "closed_witness", Label: "closed witness", Summary: "Keeps valid witness support",
			WitnessJointRefs: []string{candidate.Ref, "j9999", candidate.Ref},
		},
		{
			PairRef: pair.ref, FromGroupRef: pair.rightGroupRef, ToGroupRef: pair.leftGroupRef,
			SemanticKind: "missing_witness", Label: "missing witness", Summary: "Must be discarded",
			WitnessJointRefs: []string{},
		},
		{
			PairRef: pair.ref, FromGroupRef: pair.rightGroupRef, ToGroupRef: pair.leftGroupRef,
			SemanticKind: "unknown_witness", Label: "unknown witness", Summary: "Must be discarded",
			WitnessJointRefs: []string{"j9999"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	normalized, err := normalizeResponse(raw, compilation, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.connections) != 1 {
		t.Fatalf("filtered connections = %#v", normalized.connections)
	}
	byKind := make(map[string]connectionInput, len(normalized.connections))
	for _, connection := range normalized.connections {
		byKind[connection.semanticKind] = connection
	}
	wantEvidence := canonicalStrings([]string{candidate.LeftPatternRef, candidate.RightPatternRef})
	if got := byKind["closed_witness"].evidenceRefs; !reflect.DeepEqual(got, wantEvidence) {
		t.Fatalf("restored witness evidence = %#v", got)
	}
	if got := byKind["closed_witness"].witnessJoints; len(got) != 1 || !got[0].resolution.Valid() {
		t.Fatalf("validated witness joints = %#v", got)
	}
	invalidJointDiagnostics := 0
	missingJointDiagnostics := 0
	for _, diagnostic := range normalized.diagnostics {
		if diagnostic.Kind == diagnosticInvalidWitnessJoint {
			invalidJointDiagnostics++
		}
		if diagnostic.Kind == diagnosticMissingWitnessJoint {
			missingJointDiagnostics++
		}
	}
	if invalidJointDiagnostics != 2 || missingJointDiagnostics != 2 {
		t.Fatalf("witness joint diagnostics = %#v", normalized.diagnostics)
	}
	restored, err := compilation.restore(normalized.connections)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || len(restored[0].Evidence) != len(wantEvidence) {
		t.Fatalf("restored filtered connections = %#v", restored)
	}
}

func TestNormalizeResponseAcceptsOnlyWitnessesAdvertisedByTheExactRequest(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndex(t, "go", "request-catalog-go"),
		matchingTestIndex(t, "python", "request-catalog-python"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var request Request
	for _, pair := range compilation.pairs {
		candidateRequest, requestErr := compilation.request(pair.ref)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if len(candidateRequest.WitnessCandidates) > 0 {
			request = candidateRequest
			break
		}
	}
	if len(request.WitnessCandidates) == 0 {
		t.Fatal("fixture compiled no advertised witness candidate")
	}
	candidate := request.WitnessCandidates[0]
	response, err := json.Marshal(struct {
		Connections []responseConnection `json:"connections"`
	}{Connections: []responseConnection{{
		PairRef: request.Pair.Ref, FromGroupRef: request.Pair.LeftGroup.Ref,
		ToGroupRef: request.Pair.RightGroup.Ref, SemanticKind: "uses_exact_request_catalog",
		Label: "uses exact request catalog", Summary: "Selects only an exactly advertised witness.",
		WitnessJointRefs: []string{candidate.Ref},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	tampered := candidate
	tampered.LeftArgumentRef = "a9999"
	for name, advertised := range map[string][]witnessCandidateWire{
		"omitted":  {},
		"tampered": {tampered},
	} {
		t.Run(name, func(t *testing.T) {
			requestCopy := request
			requestCopy.WitnessCandidates = advertised
			normalized, normalizeErr := normalizeResponse(response, compilation, requestCopy)
			if normalizeErr != nil {
				t.Fatal(normalizeErr)
			}
			if len(normalized.connections) != 0 {
				t.Fatalf("unadvertised witness created connections: %#v", normalized.connections)
			}
			assertMatchingDiagnostic(t, normalized.diagnostics, diagnosticInvalidWitnessJoint)
			assertMatchingDiagnostic(t, normalized.diagnostics, diagnosticMissingWitnessJoint)
		})
	}
}

func TestNormalizeResponseRequiresExactUniqueJSONKeys(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndex(t, "go", "strict-json-go"),
		matchingTestIndex(t, "python", "strict-json-python"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var request Request
	for _, pair := range compilation.pairs {
		candidateRequest, requestErr := compilation.request(pair.ref)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if len(candidateRequest.WitnessCandidates) > 0 {
			request = candidateRequest
			break
		}
	}
	if len(request.WitnessCandidates) == 0 {
		t.Fatal("fixture compiled no advertised witness candidate")
	}
	for name, raw := range map[string][]byte{
		"case-folded envelope": []byte(`{"Connections":[]}`),
		"duplicate envelope":   []byte(`{"connections":[],"connections":[]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, normalizeErr := normalizeResponse(raw, compilation, request); normalizeErr == nil {
				t.Fatal("non-exact response envelope was accepted")
			}
		})
	}
	validRow, err := json.Marshal(responseConnection{
		PairRef: request.Pair.Ref, FromGroupRef: request.Pair.LeftGroup.Ref,
		ToGroupRef: request.Pair.RightGroup.Ref, SemanticKind: "strict_json_connection",
		Label: "strict JSON connection", Summary: "Uses an exact unique response key set.",
		WitnessJointRefs: []string{request.WitnessCandidates[0].Ref},
	})
	if err != nil {
		t.Fatal(err)
	}
	duplicateRow := append(append([]byte(nil), validRow[:len(validRow)-1]...), []byte(`,"label":"duplicate"}`)...)
	caseFoldedRow := bytes.Replace(validRow, []byte(`"pair_ref"`), []byte(`"Pair_ref"`), 1)
	for name, row := range map[string][]byte{
		"case-folded row": caseFoldedRow,
		"duplicate row":   duplicateRow,
	} {
		t.Run(name, func(t *testing.T) {
			raw := append([]byte(`{"connections":[`), row...)
			raw = append(raw, []byte(`]}`)...)
			normalized, normalizeErr := normalizeResponse(raw, compilation, request)
			if normalizeErr != nil {
				t.Fatal(normalizeErr)
			}
			if len(normalized.connections) != 0 {
				t.Fatalf("non-exact response row created connections: %#v", normalized.connections)
			}
			assertMatchingDiagnostic(t, normalized.diagnostics, diagnosticMalformedConnection)
		})
	}
}

func TestWitnessJointRequiresBoundaryEdgesAndAnIntersectingValue(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndex(t, "go", "joint-boundary"),
		matchingTestIndexWithoutBoundary(t, "python", "joint-local"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var missingBoundary responseWitnessJoint
	var missingPair pairAuthority
	for _, pair := range compilation.pairs {
		leftRef := firstBoundaryEdgeRef(pair.leftBoundaryEdges)
		rightRef := firstBoundaryEdgeRef(pair.rightBoundaryEdges)
		if leftRef == "" && rightRef == "" {
			continue
		}
		if leftRef == "" {
			leftRef = "e9999"
		}
		if rightRef == "" {
			rightRef = "e9999"
		}
		missingBoundary = responseWitnessJoint{
			Kind: witnessJointArgumentValue, LeftBoundaryEdgeRef: leftRef, LeftArgumentRef: "a1",
			RightBoundaryEdgeRef: rightRef, RightArgumentRef: "a1",
		}
		missingPair = pair
		break
	}
	if missingBoundary.Kind == "" {
		t.Fatal("fixture has no one-sided boundary pair")
	}
	if _, reason, accepted := compilation.normalizeWitnessJoint(missingBoundary, missingPair); accepted ||
		!strings.Contains(reason, "boundary-edge dossier") {
		t.Fatalf("non-boundary joint accepted=%v reason=%q", accepted, reason)
	}

	compilation, err = Compile([]groupindex.Index{
		matchingTestIndex(t, "go", "joint-values-go"),
		matchingTestIndex(t, "python", "joint-values-python"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var nonIntersecting responseWitnessJoint
	var pair pairAuthority
	for _, candidatePair := range compilation.pairs {
		candidateRequest, requestErr := compilation.request(candidatePair.ref)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		valid, validKnown := matchingWitnessJoint(candidateRequest)
		if !validKnown {
			continue
		}
		candidate, candidateKnown := matchingNonIntersectingWitnessJoint(candidateRequest, valid)
		if candidateKnown {
			pair = candidatePair
			nonIntersecting = candidate
			break
		}
	}
	if nonIntersecting.Kind == "" {
		t.Fatal("fixture has no non-intersecting same-pattern argument pair")
	}
	if _, reason, accepted := compilation.normalizeWitnessJoint(nonIntersecting, pair); accepted ||
		!strings.Contains(reason, "no shared") {
		t.Fatalf("non-intersecting joint accepted=%v reason=%q joint=%#v", accepted, reason, nonIntersecting)
	}
}

func TestWitnessJointRejectsForeignBoundaryPatternFromEndpointEvidence(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndexWithForeignBoundaryEvidence(t, "go", "joint-foreign-go"),
		matchingTestIndex(t, "python", "joint-foreign-python"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range compilation.pairs {
		request, requestErr := compilation.request(pair.ref)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		subjectByRef := make(map[string]subjectWire, len(request.Subjects))
		for _, subject := range request.Subjects {
			subjectByRef[subject.Ref] = subject
		}
		edgeByRef := make(map[string]structuralEdgeWire, len(request.StructuralEdges))
		for _, edge := range request.StructuralEdges {
			edgeByRef[edge.Ref] = edge
		}
		type side struct {
			group         groupWire
			opposite      groupWire
			foreignIsLeft bool
		}
		for _, candidateSide := range []side{
			{group: request.Pair.LeftGroup, opposite: request.Pair.RightGroup, foreignIsLeft: true},
			{group: request.Pair.RightGroup, opposite: request.Pair.LeftGroup},
		} {
			members := make(map[string]struct{}, len(candidateSide.group.MemberRefs))
			for _, ref := range candidateSide.group.MemberRefs {
				members[ref] = struct{}{}
			}
			advertised := make(map[string]struct{}, len(candidateSide.group.BoundaryEdgeRefs))
			for _, ref := range candidateSide.group.BoundaryEdgeRefs {
				advertised[ref] = struct{}{}
			}
			for _, evidenceRef := range candidateSide.group.EvidenceRefs {
				foreignPattern := subjectByRef[evidenceRef]
				if foreignPattern.Pattern == nil {
					continue
				}
				if _, sourceIsMember := members[foreignPattern.Pattern.FromRef]; sourceIsMember {
					continue
				}
				for foreignEdgeRef, foreignEdge := range edgeByRef {
					if foreignEdge.FromRef != foreignPattern.Ref || !matchingBoundaryEdgeShape(foreignEdge, subjectByRef) {
						continue
					}
					if _, incorrectlyAdvertised := advertised[foreignEdgeRef]; incorrectlyAdvertised {
						t.Fatalf("foreign evidence edge was advertised as group boundary: %s", foreignEdgeRef)
					}
					for _, oppositeEdgeRef := range candidateSide.opposite.BoundaryEdgeRefs {
						oppositeEdge := edgeByRef[oppositeEdgeRef]
						oppositePattern := subjectByRef[oppositeEdge.FromRef]
						if oppositePattern.Pattern == nil {
							continue
						}
						for _, foreignArgument := range foreignPattern.Pattern.Arguments {
							for _, oppositeArgument := range oppositePattern.Pattern.Arguments {
								if !wireArgumentsIntersect(foreignArgument, oppositeArgument) {
									continue
								}
								candidate := responseWitnessJoint{Kind: witnessJointArgumentValue}
								if candidateSide.foreignIsLeft {
									candidate.LeftBoundaryEdgeRef = foreignEdgeRef
									candidate.LeftArgumentRef = foreignArgument.Ref
									candidate.RightBoundaryEdgeRef = oppositeEdgeRef
									candidate.RightArgumentRef = oppositeArgument.Ref
								} else {
									candidate.LeftBoundaryEdgeRef = oppositeEdgeRef
									candidate.LeftArgumentRef = oppositeArgument.Ref
									candidate.RightBoundaryEdgeRef = foreignEdgeRef
									candidate.RightArgumentRef = foreignArgument.Ref
								}
								if _, reason, accepted := compilation.normalizeWitnessJoint(candidate, pair); accepted ||
									!strings.Contains(reason, "boundary-edge dossier") {
									t.Fatalf("foreign boundary joint accepted=%v reason=%q joint=%#v", accepted, reason, candidate)
								}
								return
							}
						}
					}
				}
			}
		}
	}
	t.Fatal("fixture exposed no foreign boundary edge with a matching endpoint argument")
}

func TestNestedOwnedBoundaryPatternsRemainSeparateFromPresentationMembership(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndexWithNestedBoundary(t, "go", "joint-nested-go"),
		matchingTestIndexWithNestedBoundary(t, "typescript", "joint-nested-ts"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range compilation.pairs {
		request, requestErr := compilation.request(pair.ref)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if request.Pair.LeftGroup.Lane != groupindex.LaneCore || request.Pair.RightGroup.Lane != groupindex.LaneCore {
			continue
		}
		edgeByRef := make(map[string]structuralEdgeWire, len(request.StructuralEdges))
		for _, edge := range request.StructuralEdges {
			edgeByRef[edge.Ref] = edge
		}
		for _, group := range []groupWire{request.Pair.LeftGroup, request.Pair.RightGroup} {
			if len(group.BoundaryEdgeRefs) == 0 {
				t.Fatalf("nested core group has no boundary dossier: %#v", group)
			}
			presentationRefs := make(map[string]struct{})
			for _, ref := range append(append([]string(nil), group.MemberRefs...), group.EvidenceRefs...) {
				presentationRefs[ref] = struct{}{}
			}
			for _, edgeRef := range group.BoundaryEdgeRefs {
				patternRef := edgeByRef[edgeRef].FromRef
				if _, leaked := presentationRefs[patternRef]; leaked {
					t.Fatalf("boundary pattern %s was forced into presentation membership/evidence", patternRef)
				}
			}
		}
		joint, known := matchingWitnessJoint(request)
		if !known {
			t.Fatal("nested endpoint pair has no shared boundary argument")
		}
		raw, marshalErr := json.Marshal(map[string]any{"connections": []responseConnection{{
			PairRef: pair.ref, FromGroupRef: pair.leftGroupRef, ToGroupRef: pair.rightGroupRef,
			SemanticKind: "calls_nested_boundary_for", Label: "calls boundary for",
			Summary:          "The first group calls the nested boundary owned by the second.",
			WitnessJointRefs: []string{request.WitnessCandidates[0].Ref},
		}}})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		normalized, normalizeErr := normalizeResponse(raw, compilation, request)
		if normalizeErr != nil {
			t.Fatal(normalizeErr)
		}
		if len(normalized.connections) != 1 || !reflect.DeepEqual(
			normalized.connections[0].evidenceRefs,
			canonicalStrings([]string{
				pair.leftBoundaryEdges[joint.LeftBoundaryEdgeRef].patternRef,
				pair.rightBoundaryEdges[joint.RightBoundaryEdgeRef].patternRef,
			}),
		) {
			t.Fatalf("nested joint did not survive bilateral normalization: %#v diagnostics=%#v",
				normalized.connections, normalized.diagnostics)
		}
		restored, restoreErr := compilation.restore(normalized.connections)
		if restoreErr != nil {
			t.Fatal(restoreErr)
		}
		if len(restored) != 1 || len(restored[0].Evidence) != 2 {
			t.Fatalf("nested joint evidence was not restored: %#v", restored)
		}
		return
	}
	t.Fatal("fixture exposed no cross-target core pair")
}

func TestBoundaryEdgeRequiresLocalSourceExactPackageAuthorityAndEligibleRole(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndex(t, "go", "boundary-guards-go"),
		matchingTestIndex(t, "python", "boundary-guards-python"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var group groupAuthority
	var edge edgeAuthority
	var authority boundaryEdgeAuthority
	for groupRef, edges := range compilation.boundaryEdgesByGroupRef {
		for edgeRef, candidate := range edges {
			group = compilation.groupByRef[groupRef]
			edge = compilation.edgeByRef[edgeRef]
			authority = candidate
			break
		}
		if authority.edgeRef != "" {
			break
		}
	}
	if authority.edgeRef == "" {
		t.Fatal("fixture has no boundary edge")
	}
	if revalidated, accepted := compilation.boundaryEdgeForGroup(group, edge); !accepted || revalidated != authority {
		t.Fatalf("fixture boundary edge did not revalidate: %#v accepted=%v", revalidated, accepted)
	}

	wrongRole := edge
	wrongRole.edge.Role = groupindex.EdgeRelationTarget
	if _, accepted := compilation.boundaryEdgeForGroup(group, wrongRole); accepted {
		t.Fatal("relation_target edge became a matching boundary")
	}

	platformCompilation := compilation
	platformCompilation.subjectByRef = make(map[string]subjectAuthority, len(compilation.subjectByRef))
	for ref, authority := range compilation.subjectByRef {
		platformCompilation.subjectByRef[ref] = authority
	}
	external := platformCompilation.subjectByRef[authority.externalRef]
	objectCopy := *external.subject.Object
	externalCopy := *objectCopy.External
	externalCopy.AuthorityKind = programindex.ExternalAuthorityPlatform
	// The exact external origin remains unchanged. Platform authority is an
	// explicit adapter fact rather than a package-path naming convention.
	externalCopy.PackagePath = "example.invalid/misleading-package-origin"
	objectCopy.External = &externalCopy
	external.subject.Object = &objectCopy
	platformCompilation.subjectByRef[authority.externalRef] = external
	if _, accepted := platformCompilation.boundaryEdgeForGroup(group, edge); accepted {
		t.Fatal("reserved platform authority became an integration boundary")
	}

	externalSourceCompilation := compilation
	externalSourceCompilation.subjectByRef = make(map[string]subjectAuthority, len(compilation.subjectByRef))
	for ref, authority := range compilation.subjectByRef {
		externalSourceCompilation.subjectByRef[ref] = authority
	}
	pattern := compilation.subjectByRef[authority.patternRef]
	fromRef := compilation.subjectRefByEndpoint[subjectEndpointKey(pattern.targetID, pattern.subject.Pattern.FromID)]
	from := externalSourceCompilation.subjectByRef[fromRef]
	fromObject := *from.subject.Object
	fromObject.Kind = programindex.ObjectExternalSymbol
	fromObject.External = nil
	from.subject.Object = &fromObject
	externalSourceCompilation.subjectByRef[fromRef] = from
	if _, accepted := externalSourceCompilation.boundaryEdgeForGroup(group, edge); accepted {
		t.Fatal("unresolved external_symbol source became a local group-owned boundary")
	}
}

func TestBoundaryOwnershipTraversalTerminatesOnOwnerContainerCycle(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndex(t, "go", "ownership-cycle-go"),
		matchingTestIndex(t, "python", "ownership-cycle-python"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var pattern subjectAuthority
	var group groupAuthority
	for _, candidate := range compilation.subjects {
		if candidate.subject.Pattern != nil && candidate.subject.Pattern.Selector == "dispatch" {
			pattern = candidate
			break
		}
	}
	for _, candidate := range compilation.groups {
		if candidate.targetID == pattern.targetID && candidate.group.Lane == groupindex.LaneCore {
			group = candidate
			break
		}
	}
	if pattern.ref == "" || group.ref == "" {
		t.Fatal("fixture lacks the pattern/group needed for cycle regression")
	}
	fromID := pattern.subject.Pattern.FromID
	var unrelatedID string
	for _, candidate := range compilation.subjects {
		if candidate.targetID == pattern.targetID && candidate.subject.Object != nil &&
			strings.Contains(candidate.subject.Object.Name, "Unrelated") {
			unrelatedID = candidate.subject.ID
			break
		}
	}
	if unrelatedID == "" {
		t.Fatal("fixture lacks unrelated object")
	}
	cycled := compilation
	cycled.subjectByRef = make(map[string]subjectAuthority, len(compilation.subjectByRef))
	for ref, authority := range compilation.subjectByRef {
		cycled.subjectByRef[ref] = authority
	}
	setOwner := func(id, ownerID, containerID string) {
		ref := cycled.subjectRefByEndpoint[subjectEndpointKey(pattern.targetID, id)]
		authority := cycled.subjectByRef[ref]
		object := *authority.subject.Object
		object.OwnerID = ownerID
		object.ContainerID = containerID
		authority.subject.Object = &object
		cycled.subjectByRef[ref] = authority
	}
	setOwner(fromID, unrelatedID, "")
	setOwner(unrelatedID, "", fromID)
	if cycled.patternSourceOwnedByGroup(pattern, group) {
		t.Fatal("owner/container cycle reached an unrelated group member")
	}
}

func TestWitnessJointDerivesExactAndPossibleResolutionLocally(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndex(t, "go", "joint-resolution-go"),
		matchingTestIndex(t, "python", "joint-resolution-python"),
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[programindex.PatternValueResolution]bool)
	for _, pair := range compilation.pairs {
		request, requestErr := compilation.request(pair.ref)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		for _, candidate := range request.WitnessCandidates {
			joint, reason, accepted := compilation.normalizeWitnessJoint(responseWitnessJointFromCandidate(candidate), pair)
			if !accepted {
				t.Fatalf("fixture candidate %s rejected: %s", candidate.Ref, reason)
			}
			if joint.resolution != candidate.SupportResolution {
				t.Fatalf("candidate %s support = %q, revalidated as %q", candidate.Ref, candidate.SupportResolution, joint.resolution)
			}
			seen[joint.resolution] = true
		}
	}
	if !seen[programindex.PatternValueExact] || !seen[programindex.PatternValuePossible] {
		t.Fatalf("derived joint resolutions = %#v, want exact and possible", seen)
	}
}

func TestAlternativeTriggerBoundaryMatchesExactPeerAsPossibleOnly(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndexWithAlternativeTriggerBoundary(t, "python", "alt-trigger-python"),
		matchingTestIndex(t, "typescript", "exact-trigger-typescript"),
	})
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, pair := range compilation.pairs {
		left := compilation.groupByRef[pair.leftGroupRef]
		right := compilation.groupByRef[pair.rightGroupRef]
		if left.group.Lane != groupindex.LaneTriggers || right.group.Lane != groupindex.LaneTriggers {
			continue
		}
		request, requestErr := compilation.request(pair.ref)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if len(request.WitnessCandidates) == 0 {
			t.Fatal("alternative/exact trigger pair has no prevalidated witness candidate")
		}
		if request.WitnessCandidates[0].SupportResolution != programindex.PatternValuePossible {
			t.Fatalf("alternative/exact candidate support = %q, want possible", request.WitnessCandidates[0].SupportResolution)
		}
		candidate, known := matchingWitnessJoint(request)
		if !known {
			t.Fatal("alternative/exact trigger pair has no shared exact argument")
		}
		joint, reason, accepted := compilation.normalizeWitnessJoint(candidate, pair)
		if !accepted || joint.resolution != programindex.PatternValuePossible {
			t.Fatalf("alternative/exact joint = %#v accepted=%v reason=%q", joint, accepted, reason)
		}
		restored, restoreErr := compilation.restore([]connectionInput{{
			pairRef: pair.ref, fromGroupRef: pair.leftGroupRef, toGroupRef: pair.rightGroupRef,
			semanticKind: "matches_possible_trigger", label: "matches possible trigger",
			summary:           "An exact boundary matches a possible trigger binding.",
			supportResolution: joint.resolution,
			evidenceRefs:      canonicalStrings([]string{joint.leftPatternRef, joint.rightPatternRef}),
			witnessJoints:     []witnessJoint{joint},
		}})
		if restoreErr != nil || len(restored) != 1 ||
			restored[0].SupportResolution != programindex.PatternValuePossible {
			t.Fatalf("restored possible support = %#v err=%v", restored, restoreErr)
		}
		matched = true
		break
	}
	if !matched {
		t.Fatal("fixture exposed no trigger-to-trigger pair")
	}

	var possibleGroup groupAuthority
	var possibleEdge edgeAuthority
	for groupRef, edges := range compilation.boundaryEdgesByGroupRef {
		for edgeRef, authority := range edges {
			if authority.basis == boundaryEdgeSemanticTrigger {
				possibleGroup = compilation.groupByRef[groupRef]
				possibleEdge = compilation.edgeByRef[edgeRef]
				break
			}
		}
	}
	if possibleEdge.ref == "" {
		t.Fatal("fixture exposed no semantic trigger boundary")
	}
	nonTriggerLane := possibleGroup
	nonTriggerLane.group.Lane = groupindex.LaneCore
	if _, accepted := compilation.boundaryEdgeForGroup(nonTriggerLane, possibleEdge); accepted {
		t.Fatal("alternative boundary escaped the triggers lane")
	}
	nonTriggerCategory := compilation
	nonTriggerCategory.subjectByRef = make(map[string]subjectAuthority, len(compilation.subjectByRef))
	for ref, subject := range compilation.subjectByRef {
		nonTriggerCategory.subjectByRef[ref] = subject
	}
	patternRef := compilation.subjectRefByEndpoint[subjectEndpointKey(possibleEdge.targetID, possibleEdge.edge.FromSubjectID)]
	pattern := nonTriggerCategory.subjectByRef[patternRef]
	pattern.subject.Categories = []programindex.Category{programindex.CategoryCore}
	nonTriggerCategory.subjectByRef[patternRef] = pattern
	if _, accepted := nonTriggerCategory.boundaryEdgeForGroup(possibleGroup, possibleEdge); accepted {
		t.Fatal("alternative boundary escaped inbound/background categorization")
	}
}

func TestTwoAlternativeTriggerBoundariesCannotEstablishOneJoint(t *testing.T) {
	compilation, err := Compile([]groupindex.Index{
		matchingTestIndexWithAlternativeTriggerBoundary(t, "go", "alt-trigger-go"),
		matchingTestIndexWithAlternativeTriggerBoundary(t, "python", "alt-trigger-python"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range compilation.pairs {
		left := compilation.groupByRef[pair.leftGroupRef]
		right := compilation.groupByRef[pair.rightGroupRef]
		if left.group.Lane != groupindex.LaneTriggers || right.group.Lane != groupindex.LaneTriggers {
			continue
		}
		request, requestErr := compilation.request(pair.ref)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if len(request.WitnessCandidates) != 0 {
			t.Fatalf("alternative/alternative pair advertised invalid candidates: %#v", request.WitnessCandidates)
		}
		candidate, known := matchingRawWitnessJoint(request)
		if !known {
			t.Fatal("alternative trigger pair has no shared exact argument")
		}
		if _, reason, accepted := compilation.normalizeWitnessJoint(candidate, pair); accepted ||
			!strings.Contains(reason, "two possible boundary edges") {
			t.Fatalf("alternative/alternative joint accepted=%v reason=%q", accepted, reason)
		}
		return
	}
	t.Fatal("fixture exposed no trigger-to-trigger pair")
}

func TestRunUsesOneProviderCallPerPairEvenWhenEveryPairFits(t *testing.T) {
	indexes := []groupindex.Index{
		matchingTestIndex(t, "go", "singleton-go"),
		matchingTestIndex(t, "python", "singleton-py"),
		matchingTestIndexWithGroupCopies(t, "typescript", "singleton-ts", 2, 2),
	}
	provider := &matchingPresetProvider{}
	matched, diagnostics, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 4, BatchController: &llm.BatchController{},
	}, provider, indexes)
	if err != nil {
		t.Fatalf("Run singleton pairs: %v", err)
	}
	if len(diagnostics) != 0 || !reflect.DeepEqual(matched, indexes) {
		t.Fatalf("empty sparse result changed authority: diagnostics=%#v", diagnostics)
	}
	provider.mu.Lock()
	attempts := provider.attempts
	requests := append([]Request(nil), provider.requests...)
	provider.mu.Unlock()
	covered := make(map[string]struct{})
	for _, request := range requests {
		covered[request.Pair.Ref] = struct{}{}
	}
	if len(covered) != 20 || attempts != 20 || len(requests) != 20 {
		t.Fatalf("singleton attempts/requests/covered pairs = %d/%d/%d, want 20/20/20", attempts, len(requests), len(covered))
	}
}

func TestRunSkipsCandidateFreePairsWithoutInventingSemanticAuthority(t *testing.T) {
	indexes := []groupindex.Index{
		matchingTestIndex(t, "go", "eligible-go"),
		matchingTestIndex(t, "python", "eligible-python"),
		matchingTestIndexWithoutBoundary(t, "typescript", "candidate-free-ts"),
	}
	compilation, err := Compile(indexes)
	if err != nil {
		t.Fatal(err)
	}
	eligible := 0
	for _, pair := range compilation.pairs {
		if len(pair.witnessCandidates) > 0 {
			eligible++
		}
	}
	if eligible == 0 || eligible >= len(compilation.pairs) {
		t.Fatalf("fixture eligible/total pairs = %d/%d", eligible, len(compilation.pairs))
	}
	provider := &matchingPresetProvider{}
	matched, diagnostics, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 4, BatchController: &llm.BatchController{},
	}, provider, indexes)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 || !reflect.DeepEqual(matched, indexes) {
		t.Fatalf("sparse empty matching changed authority: matched=%#v diagnostics=%#v", matched, diagnostics)
	}
	provider.mu.Lock()
	attempts := provider.attempts
	requests := append([]Request(nil), provider.requests...)
	provider.mu.Unlock()
	if attempts != eligible || len(requests) != eligible {
		t.Fatalf("provider attempts/requests = %d/%d, want %d eligible pairs", attempts, len(requests), eligible)
	}
	for _, request := range requests {
		if len(request.WitnessCandidates) == 0 {
			t.Fatalf("candidate-free pair reached provider: %#v", request.Pair)
		}
	}
}

func TestRunNeedsNoProviderWhenEveryPairIsCandidateFree(t *testing.T) {
	indexes := []groupindex.Index{
		matchingTestIndexWithoutBoundary(t, "go", "candidate-free-go"),
		matchingTestIndexWithoutBoundary(t, "python", "candidate-free-python"),
	}
	matched, diagnostics, err := Run(t.Context(), llm.Executor{}, nil, indexes)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 || !reflect.DeepEqual(matched, indexes) {
		t.Fatalf("candidate-free result changed authority: matched=%#v diagnostics=%#v", matched, diagnostics)
	}
	needsProvider, err := NeedsProvider(indexes)
	if err != nil || needsProvider {
		t.Fatalf("candidate-free NeedsProvider = %v, err=%v", needsProvider, err)
	}
}

func TestProviderPlanCoversExactlyCandidateBearingPairs(t *testing.T) {
	indexes := []groupindex.Index{
		matchingTestIndex(t, "go", "plan-go"),
		matchingTestIndex(t, "python", "plan-python"),
		matchingTestIndexWithoutBoundary(t, "typescript", "plan-ts"),
	}
	compilation, err := Compile(indexes)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := compilation.batchesForProvider(&matchingPresetProvider{})
	if err != nil {
		t.Fatal(err)
	}
	eligible := make(map[string]struct{})
	var candidateFreeRef string
	for _, pair := range compilation.pairs {
		if len(pair.witnessCandidates) == 0 {
			candidateFreeRef = pair.ref
			continue
		}
		eligible[pair.ref] = struct{}{}
	}
	if len(plan) != len(eligible) || len(plan) == 0 || candidateFreeRef == "" {
		t.Fatalf("plan/eligible/total = %d/%d/%d", len(plan), len(eligible), len(compilation.pairs))
	}
	for _, item := range plan {
		if _, expected := eligible[item.pairRef]; !expected {
			t.Fatalf("plan contains candidate-free pair %s", item.pairRef)
		}
	}
	needsProvider, err := NeedsProvider(indexes)
	if err != nil || !needsProvider {
		t.Fatalf("mixed NeedsProvider = %v, err=%v", needsProvider, err)
	}
	if err := compilation.validatePlan(append(append([]pairBatch(nil), plan...), pairBatch{pairRef: candidateFreeRef})); err == nil || !strings.Contains(err.Error(), "candidate-free") {
		t.Fatalf("candidate-free plan inclusion error = %v", err)
	}
	if err := compilation.validatePlan(plan[:len(plan)-1]); err == nil || !strings.Contains(err.Error(), "want 1") {
		t.Fatalf("missing eligible pair error = %v", err)
	}
	duplicate := append(append([]pairBatch(nil), plan...), plan[0])
	if err := compilation.validatePlan(duplicate); err == nil || !strings.Contains(err.Error(), "want 1") {
		t.Fatalf("duplicate eligible pair error = %v", err)
	}
}

func TestRunFailsClosedWhenOneCompletePairIsIndivisible(t *testing.T) {
	indexes := []groupindex.Index{
		matchingTestIndex(t, "go", "atomic-go"),
		matchingTestIndex(t, "python", "atomic-py"),
	}
	provider := &matchingPresetProvider{rejectPreparedPair: true}
	if _, _, err := Run(t.Context(), llm.Executor{}, provider, indexes); err == nil || !strings.Contains(err.Error(), "indivisible") {
		t.Fatalf("indivisible pair error = %v", err)
	}
}

func matchingDirectionResponse(
	t *testing.T,
	pairRef string,
	fromGroupRef string,
	toGroupRef string,
	witnessRefs []string,
) []byte {
	t.Helper()
	wire, err := json.Marshal(struct {
		Connections []responseConnection `json:"connections"`
	}{Connections: []responseConnection{{
		PairRef: pairRef, FromGroupRef: fromGroupRef, ToGroupRef: toGroupRef,
		SemanticKind: "uses_boundary_of", Label: "uses boundary of",
		Summary:          "The source group uses the destination integration boundary.",
		WitnessJointRefs: witnessRefs,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func matchingResponse(request Request) []byte {
	pair := request.Pair
	targetLanguages := make(map[string]string, len(request.Targets))
	for _, target := range request.Targets {
		targetLanguages[target.Ref] = target.Language
	}
	left := pair.LeftGroup
	right := pair.RightGroup
	var goTrigger, pythonCore groupWire
	for _, group := range []groupWire{left, right} {
		switch {
		case targetLanguages[group.TargetRef] == "go" && group.Lane == groupindex.LaneTriggers:
			goTrigger = group
		case targetLanguages[group.TargetRef] == "python" && group.Lane == groupindex.LaneCore:
			pythonCore = group
		}
	}
	if goTrigger.Ref == "" || pythonCore.Ref == "" {
		return []byte(`{"connections":[]}`)
	}
	if len(request.WitnessCandidates) == 0 {
		return []byte(`{"connections":[]}`)
	}
	jointRef := request.WitnessCandidates[0].Ref
	base := map[string]any{
		"pair_ref": pair.Ref, "from_group_ref": pythonCore.Ref, "to_group_ref": goTrigger.Ref,
		"semantic_kind": "translates_domain_commands_for", "label": "translates commands for",
		"summary":            "The Python core translates commands consumed by the Go trigger.",
		"witness_joint_refs": []string{jointRef},
	}
	unknownPair := cloneAnyMap(base)
	unknownPair["pair_ref"] = "p9999"
	unknownPair["semantic_kind"] = "unknown_pair_candidate"
	invalidEndpoint := cloneAnyMap(base)
	invalidEndpoint["to_group_ref"] = "g9999"
	invalidEndpoint["semantic_kind"] = "wrong_pair_endpoint"
	unknownWitness := cloneAnyMap(base)
	unknownWitness["semantic_kind"] = "unknown_witness_candidate"
	unknownWitness["witness_joint_refs"] = []string{"j9999"}
	wire, _ := json.Marshal(map[string]any{"connections": []any{
		base, unknownPair, invalidEndpoint, unknownWitness,
		map[string]any{"pair_ref": 42},
	}})
	return wire
}

func matchingWitnessJoint(request Request) (responseWitnessJoint, bool) {
	if len(request.WitnessCandidates) == 0 {
		return responseWitnessJoint{}, false
	}
	return responseWitnessJointFromCandidate(request.WitnessCandidates[0]), true
}

func responseWitnessJointFromCandidate(candidate witnessCandidateWire) responseWitnessJoint {
	return responseWitnessJoint{
		Kind:                 candidate.Kind,
		LeftBoundaryEdgeRef:  candidate.LeftBoundaryEdgeRef,
		LeftArgumentRef:      candidate.LeftArgumentRef,
		RightBoundaryEdgeRef: candidate.RightBoundaryEdgeRef,
		RightArgumentRef:     candidate.RightArgumentRef,
	}
}

func matchingRawWitnessJoint(request Request) (responseWitnessJoint, bool) {
	subjectByRef := make(map[string]subjectWire, len(request.Subjects))
	for _, subject := range request.Subjects {
		subjectByRef[subject.Ref] = subject
	}
	edgeByRef := make(map[string]structuralEdgeWire, len(request.StructuralEdges))
	for _, edge := range request.StructuralEdges {
		edgeByRef[edge.Ref] = edge
	}
	type choice struct {
		edgeRef string
		pattern subjectWire
	}
	choices := func(refs []string) []choice {
		result := make([]choice, 0, len(refs))
		for _, ref := range refs {
			edge, edgeKnown := edgeByRef[ref]
			pattern, patternKnown := subjectByRef[edge.FromRef]
			if edgeKnown && patternKnown && pattern.Pattern != nil {
				result = append(result, choice{edgeRef: ref, pattern: pattern})
			}
		}
		return result
	}
	for _, left := range choices(request.Pair.LeftGroup.BoundaryEdgeRefs) {
		for _, right := range choices(request.Pair.RightGroup.BoundaryEdgeRefs) {
			for _, leftArgument := range left.pattern.Pattern.Arguments {
				leftValues := matchingWireArgumentValues(leftArgument)
				for _, rightArgument := range right.pattern.Pattern.Arguments {
					rightValues := matchingWireArgumentValues(rightArgument)
					for value := range leftValues {
						if _, shared := rightValues[value]; !shared {
							continue
						}
						return responseWitnessJoint{
							Kind:                witnessJointArgumentValue,
							LeftBoundaryEdgeRef: left.edgeRef, LeftArgumentRef: leftArgument.Ref,
							RightBoundaryEdgeRef: right.edgeRef, RightArgumentRef: rightArgument.Ref,
						}, true
					}
				}
			}
		}
	}
	return responseWitnessJoint{}, false
}

func matchingNonIntersectingWitnessJoint(
	request Request,
	valid responseWitnessJoint,
) (responseWitnessJoint, bool) {
	subjectByRef := make(map[string]subjectWire, len(request.Subjects))
	for _, subject := range request.Subjects {
		subjectByRef[subject.Ref] = subject
	}
	edgeByRef := make(map[string]structuralEdgeWire, len(request.StructuralEdges))
	for _, edge := range request.StructuralEdges {
		edgeByRef[edge.Ref] = edge
	}
	left := subjectByRef[edgeByRef[valid.LeftBoundaryEdgeRef].FromRef]
	right := subjectByRef[edgeByRef[valid.RightBoundaryEdgeRef].FromRef]
	if left.Pattern == nil || right.Pattern == nil {
		return responseWitnessJoint{}, false
	}
	argument := func(pattern *patternWire, ref string) (argumentWire, bool) {
		for _, candidate := range pattern.Arguments {
			if candidate.Ref == ref {
				return candidate, true
			}
		}
		return argumentWire{}, false
	}
	leftSelected, leftKnown := argument(left.Pattern, valid.LeftArgumentRef)
	rightSelected, rightKnown := argument(right.Pattern, valid.RightArgumentRef)
	if !leftKnown || !rightKnown {
		return responseWitnessJoint{}, false
	}
	disjoint := func(leftValues, rightValues map[string]struct{}) bool {
		for value := range leftValues {
			if _, shared := rightValues[value]; shared {
				return false
			}
		}
		return true
	}
	for _, candidate := range left.Pattern.Arguments {
		if candidate.Ref != valid.LeftArgumentRef &&
			disjoint(matchingWireArgumentValues(candidate), matchingWireArgumentValues(rightSelected)) {
			valid.LeftArgumentRef = candidate.Ref
			return valid, true
		}
	}
	for _, candidate := range right.Pattern.Arguments {
		if candidate.Ref != valid.RightArgumentRef &&
			disjoint(matchingWireArgumentValues(leftSelected), matchingWireArgumentValues(candidate)) {
			valid.RightArgumentRef = candidate.Ref
			return valid, true
		}
	}
	return responseWitnessJoint{}, false
}

func matchingWireArgumentValues(argument argumentWire) map[string]struct{} {
	result := make(map[string]struct{}, len(argument.ValueCandidates)+1)
	if argument.Kind == programindex.PatternLiteralString || argument.Kind == programindex.PatternStringTemplate {
		result[patternValueKey(argument.Kind, argument.Value, argument.Parts)] = struct{}{}
	}
	for _, candidate := range argument.ValueCandidates {
		result[patternValueKey(candidate.Kind, candidate.Value, candidate.Parts)] = struct{}{}
	}
	return result
}

func wireArgumentsIntersect(left, right argumentWire) bool {
	leftValues := matchingWireArgumentValues(left)
	for value := range matchingWireArgumentValues(right) {
		if _, shared := leftValues[value]; shared {
			return true
		}
	}
	return false
}

func matchingBoundaryEdgeShape(edge structuralEdgeWire, subjects map[string]subjectWire) bool {
	switch edge.Role {
	case groupindex.EdgePatternTarget, groupindex.EdgePatternReceiver, groupindex.EdgePatternReceiverOrigin:
	default:
		return false
	}
	from, fromKnown := subjects[edge.FromRef]
	to, toKnown := subjects[edge.ToRef]
	return fromKnown && from.Pattern != nil && toKnown && to.Object != nil &&
		to.Object.Kind == programindex.ObjectExternalSymbol && programindex.IsExternalPackageAuthority(to.Object.External)
}

func cloneAnyMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func firstRef(values map[string]struct{}) string {
	refs := make([]string, 0, len(values))
	for ref := range values {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	if len(refs) == 0 {
		return ""
	}
	return refs[0]
}

func firstBoundaryEdgeRef(values map[string]boundaryEdgeAuthority) string {
	refs := make([]string, 0, len(values))
	for ref := range values {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	if len(refs) == 0 {
		return ""
	}
	return refs[0]
}

func matchingTestIndex(t *testing.T, language, selector string) groupindex.Index {
	return matchingTestIndexWithGroupCopies(t, language, selector, 1, 1)
}

func matchingTestIndexWithoutBoundary(t *testing.T, language, selector string) groupindex.Index {
	return matchingTestIndexWithGroupCopiesAndBoundary(t, language, selector, 1, 1, false, false, false, false)
}

func matchingTestIndexWithNestedBoundary(t *testing.T, language, selector string) groupindex.Index {
	return matchingTestIndexWithGroupCopiesAndBoundary(t, language, selector, 1, 1, true, false, true, false)
}

func matchingTestIndexWithAlternativeTriggerBoundary(t *testing.T, language, selector string) groupindex.Index {
	return matchingTestIndexWithGroupCopiesAndBoundary(t, language, selector, 1, 1, true, false, false, true)
}

func matchingTestIndexWithGroupCopies(
	t *testing.T,
	language string,
	selector string,
	triggerCopies int,
	coreCopies int,
) groupindex.Index {
	return matchingTestIndexWithGroupCopiesAndBoundary(t, language, selector, triggerCopies, coreCopies, true, false, false, false)
}

func matchingTestIndexWithForeignBoundaryEvidence(t *testing.T, language, selector string) groupindex.Index {
	return matchingTestIndexWithGroupCopiesAndBoundary(t, language, selector, 1, 1, true, true, false, false)
}

func matchingDirectionTestIndex(
	t *testing.T,
	language string,
	selector string,
	inbound bool,
	outbound bool,
	dualRoleInbound bool,
) groupindex.Index {
	t.Helper()
	if !inbound && !outbound {
		t.Fatal("direction fixture requires at least one boundary role")
	}
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: selector + "/main.src", Line: line, Column: 1}
	}
	objects := []programindex.ObjectInput{
		{
			SourceRef: "root", Kind: programindex.ObjectFunction, Name: selector + "Root",
			Visibility: programindex.VisibilityPublic, Signature: "root()", Location: location(1),
		},
		{
			SourceRef: "external", Kind: programindex.ObjectExternalSymbol, Name: selector + "Boundary",
			Visibility: programindex.VisibilityPublic, Location: location(2),
			External: &programindex.ExternalSymbol{
				AuthorityKind: programindex.ExternalAuthorityPackage,
				PackagePath:   "example.invalid/" + selector,
				Name:          "Boundary",
			},
			SymbolLinkIdentities: []programindex.SymbolLinkIdentityInput{{
				Domain: language, Parts: []string{selector, "Boundary"}, Display: selector + "Boundary",
			}},
		},
	}
	relations := []programindex.RelationInput{}
	evidenceSourceRefs := []string{}
	assignments := []programindex.CategoryAssignment{{
		SubjectID: "", Categories: []programindex.Category{programindex.CategoryDependency},
	}}
	if inbound {
		relations = append(relations, programindex.RelationInput{
			SourceRef: "inbound-relation", Kind: programindex.RelationDecorates,
			FromRef: "root", ToRefs: []string{"root"}, Resolution: programindex.ResolutionExact,
			Invocation: "registration", Location: location(10), TargetsObserved: 1,
			Witnesses: []programindex.Witness{{Kind: "registration", Location: location(10)}}, WitnessesObserved: 1,
			PatternsObserved: 1,
			Patterns: []programindex.RelationPatternInput{{
				SourceRef: "inbound-pattern", Form: programindex.PatternDecoratorCall,
				Selector: "register", Location: location(10), ResultRef: "root",
				ReceiverOriginRefs:       []string{"external"},
				ReceiverOriginResolution: programindex.ResolutionAlternatives, ReceiverOriginsObserved: 1,
				ArgumentsObserved: 1,
				Arguments: []programindex.PatternArgumentInput{{
					Position: 1, Kind: programindex.PatternLiteralString, Value: "/shared-boundary",
				}},
			}},
		})
		evidenceSourceRefs = append(evidenceSourceRefs, "inbound-pattern")
	}
	if outbound {
		relations = append(relations, programindex.RelationInput{
			SourceRef: "outbound-relation", Kind: programindex.RelationInvokesExternal,
			FromRef: "root", ToRefs: []string{"external"}, Resolution: programindex.ResolutionExact,
			Invocation: "sync", Location: location(20), TargetsObserved: 1,
			Witnesses: []programindex.Witness{{Kind: "direct_call", Location: location(20)}}, WitnessesObserved: 1,
			PatternsObserved: 1,
			Patterns: []programindex.RelationPatternInput{{
				SourceRef: "outbound-pattern", Form: programindex.PatternCall,
				Selector: "send", Location: location(20), ResultRef: "external",
				ReceiverOriginRefs:       []string{"external"},
				ReceiverOriginResolution: programindex.ResolutionExact, ReceiverOriginsObserved: 1,
				ArgumentsObserved: 1,
				Arguments: []programindex.PatternArgumentInput{{
					Position: 1, Kind: programindex.PatternLiteralString, Value: "/shared-boundary",
				}},
			}},
		})
		evidenceSourceRefs = append(evidenceSourceRefs, "outbound-pattern")
	}
	program, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: "module", Name: selector, Selector: selector,
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: selector + "/main.src"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "root", Kind: programindex.SeedCallable, Location: location(1)}},
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations),
		},
	})
	if err != nil {
		t.Fatalf("direction programindex.New: %v", err)
	}
	ids := matchingSubjectIDs(program)
	rootCategories := []programindex.Category{programindex.CategoryCore}
	lane := groupindex.LaneCore
	if inbound {
		rootCategories = []programindex.Category{programindex.CategoryInbound}
		lane = groupindex.LaneTriggers
	}
	assignments[0].SubjectID = ids["external"]
	assignments = append(assignments, programindex.CategoryAssignment{SubjectID: ids["root"], Categories: rootCategories})
	if inbound {
		categories := []programindex.Category{programindex.CategoryInbound}
		if dualRoleInbound {
			categories = append(categories, programindex.CategoryDependency)
		}
		assignments = append(assignments, programindex.CategoryAssignment{
			SubjectID: ids["inbound-pattern"], Categories: categories,
		})
	}
	if outbound {
		assignments = append(assignments, programindex.CategoryAssignment{
			SubjectID: ids["outbound-pattern"], Categories: []programindex.Category{programindex.CategoryDependency},
		})
	}
	program, err = programindex.Enrich(program, strings.Repeat("d", 64), assignments)
	if err != nil {
		t.Fatalf("direction programindex.Enrich: %v", err)
	}
	evidenceIDs := make([]string, 0, len(evidenceSourceRefs))
	for _, ref := range evidenceSourceRefs {
		evidenceIDs = append(evidenceIDs, ids[ref])
	}
	index, diagnostics, err := groupindex.Build(program, groupindex.Proposals{Groups: []groupindex.GroupProposal{{
		Key: "boundary", Title: selector + " boundary", Summary: "Owns the fixture integration boundary.",
		Lane: lane, MemberSubjectIDs: []string{ids["root"]}, EvidenceSubjectIDs: evidenceIDs,
	}}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("direction groupindex.Build: diagnostics=%#v err=%v", diagnostics, err)
	}
	return index
}

func matchingTestIndexWithGroupCopiesAndBoundary(
	t *testing.T,
	language string,
	selector string,
	triggerCopies int,
	coreCopies int,
	boundary bool,
	foreignBoundaryEvidence bool,
	nestedBoundarySource bool,
	alternativeTriggerBoundary bool,
) groupindex.Index {
	t.Helper()
	if triggerCopies < 1 || coreCopies < 1 {
		t.Fatal("matching fixture requires positive trigger and core group counts")
	}
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: selector + "/main.src", Line: line, Column: 1}
	}
	objects := []programindex.ObjectInput{
		{SourceRef: "trigger", Kind: programindex.ObjectFunction, Name: selector + "Start", Visibility: programindex.VisibilityPublic, Signature: "start()", Location: location(1)},
		{SourceRef: "core", Kind: programindex.ObjectFunction, Name: selector + "Apply", Visibility: programindex.VisibilityInternal, Location: location(2)},
		{SourceRef: "unrelated", Kind: programindex.ObjectFunction, Name: selector + "Unrelated", Visibility: programindex.VisibilityInternal, Location: location(4)},
		{
			SourceRef: "dependency", Kind: programindex.ObjectExternalSymbol, Name: selector + "Client", Visibility: programindex.VisibilityPublic,
			External: &programindex.ExternalSymbol{
				AuthorityKind: programindex.ExternalAuthorityPackage,
				PackagePath:   "example.invalid/" + selector,
				Name:          "Client",
			}, Location: location(3),
			SymbolLinkIdentities: []programindex.SymbolLinkIdentityInput{{Domain: language, Parts: []string{selector, "Client"}, Display: selector + "Client"}},
		},
	}
	forwardFromRef := "core"
	if nestedBoundarySource {
		objects = append(objects,
			programindex.ObjectInput{
				SourceRef: "boundary-callback", Kind: programindex.ObjectFunction,
				Name: selector + "BoundaryCallback", Visibility: programindex.VisibilityInternal,
				OwnerRef: "core", Location: location(5),
			},
			programindex.ObjectInput{
				SourceRef: "boundary-result", Kind: programindex.ObjectVariable,
				Name: selector + "BoundaryResult", Visibility: programindex.VisibilityInternal,
				ContainerRef: "boundary-callback", Location: location(6),
			},
		)
		forwardFromRef = "boundary-result"
	}
	var receiverOriginRefs []string
	var receiverOriginResolution programindex.Resolution
	var receiverOriginsObserved int
	forwardTargetRef := "trigger"
	if boundary {
		receiverOriginRefs = []string{"dependency"}
		receiverOriginResolution = programindex.ResolutionExact
		if alternativeTriggerBoundary {
			receiverOriginResolution = programindex.ResolutionAlternatives
		}
		receiverOriginsObserved = 1
		forwardTargetRef = "dependency"
	}
	relations := []programindex.RelationInput{{
		SourceRef: "dispatch", Kind: programindex.RelationCalls, FromRef: "trigger", ToRefs: []string{"core"},
		Resolution: programindex.ResolutionExact, Invocation: "sync", Location: location(10), TargetsObserved: 1,
		Witnesses: []programindex.Witness{{Kind: "direct_call", Location: location(10)}}, WitnessesObserved: 1,
		PatternsObserved: 1,
		Patterns: []programindex.RelationPatternInput{
			{
				SourceRef: "dispatch-pattern", Form: programindex.PatternCall, Selector: "dispatch", Location: location(10),
				ResultRef: "core", ReceiverOriginRefs: receiverOriginRefs,
				ReceiverOriginResolution: receiverOriginResolution, ReceiverOriginsObserved: receiverOriginsObserved,
				ArgumentsObserved: 3,
				Arguments: []programindex.PatternArgumentInput{
					{Position: 1, Kind: programindex.PatternLiteralString, Value: "/commands"},
					{
						Position: 2, Kind: programindex.PatternDynamic, ObjectRefs: []string{"dependency"},
						Resolution: programindex.ResolutionExact, ObjectsObserved: 1,
						ValueCandidatesObserved: 1,
						ValueCandidates: []programindex.PatternValueCandidateInput{{
							Kind: programindex.PatternLiteralString, Value: "/commands",
							Resolution:       programindex.PatternValuePossible,
							SourceKind:       programindex.PatternValueSourceInitializer,
							SourceObjectRefs: []string{"dependency"}, SourceObjectsObserved: 1,
						}},
					},
					{Position: 3, Kind: programindex.PatternLiteralString, Value: "/other"},
				},
			},
		},
	}, {
		SourceRef: "forward", Kind: programindex.RelationCalls, FromRef: forwardFromRef, ToRefs: []string{forwardTargetRef},
		Resolution: programindex.ResolutionExact, Invocation: "sync", Location: location(11), TargetsObserved: 1,
		Witnesses: []programindex.Witness{{Kind: "direct_call", Location: location(11)}}, WitnessesObserved: 1,
		PatternsObserved: 1,
		Patterns: []programindex.RelationPatternInput{{
			SourceRef: "forwarded-pattern", Form: programindex.PatternCall, Selector: "forward", Location: location(11),
			ResultRef: forwardTargetRef, ReceiverOriginRefs: receiverOriginRefs,
			ReceiverOriginResolution: receiverOriginResolution, ReceiverOriginsObserved: receiverOriginsObserved,
			ArgumentsObserved: 1,
			Arguments: []programindex.PatternArgumentInput{{
				Position: 1, Kind: programindex.PatternDynamic, ValueCandidatesObserved: 1,
				ValueCandidates: []programindex.PatternValueCandidateInput{{
					Kind: programindex.PatternLiteralString, Value: "/commands",
					Resolution: programindex.PatternValuePossible,
					SourceKind: programindex.PatternValueSourceActualArgument,
					SourceArgumentRefs: []programindex.PatternArgumentRefInput{{
						RelationSourceRef: "dispatch", PatternSourceRef: "dispatch-pattern", Position: 1,
					}},
					SourceArgumentsObserved: 1,
				}},
			}},
		}},
	}}
	program, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: "module", Name: selector, Selector: selector,
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: selector + "/main.src"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "trigger", Kind: programindex.SeedCallable, Location: location(1)}},
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	ids := matchingSubjectIDs(program)
	dispatchCategories := []programindex.Category{programindex.CategoryCore}
	if alternativeTriggerBoundary {
		dispatchCategories = []programindex.Category{programindex.CategoryInbound}
	}
	program, err = programindex.Enrich(program, strings.Repeat("d", 64), []programindex.CategoryAssignment{
		{SubjectID: ids["trigger"], Categories: []programindex.Category{programindex.CategoryInbound}},
		{SubjectID: ids["core"], Categories: []programindex.Category{programindex.CategoryCore}},
		{SubjectID: ids["dependency"], Categories: []programindex.Category{programindex.CategoryDependency}},
		{SubjectID: ids["dispatch-pattern"], Categories: dispatchCategories},
		{SubjectID: ids["forwarded-pattern"], Categories: []programindex.Category{programindex.CategoryCore}},
	})
	if err != nil {
		t.Fatalf("programindex.Enrich: %v", err)
	}
	groups := make([]groupindex.GroupProposal, 0, triggerCopies+coreCopies)
	for position := 1; position <= triggerCopies; position++ {
		groups = append(groups, groupindex.GroupProposal{
			Key: fmt.Sprintf("trigger-%d", position), Title: fmt.Sprintf("%s triggers %d", language, position),
			Summary: "Starts " + selector + " work.", Lane: groupindex.LaneTriggers,
			MemberSubjectIDs: []string{ids["trigger"]}, EvidenceSubjectIDs: []string{ids["dispatch-pattern"]},
		})
	}
	for position := 1; position <= coreCopies; position++ {
		evidenceSubjectIDs := []string{ids["forwarded-pattern"]}
		if nestedBoundarySource {
			evidenceSubjectIDs = []string{}
		}
		if foreignBoundaryEvidence {
			evidenceSubjectIDs = append(evidenceSubjectIDs, ids["dispatch-pattern"])
		}
		groups = append(groups, groupindex.GroupProposal{
			Key: fmt.Sprintf("core-%d", position), Title: fmt.Sprintf("%s core %d", language, position),
			Summary: "Owns " + selector + " domain work.", Lane: groupindex.LaneCore,
			MemberSubjectIDs: []string{ids["core"]}, EvidenceSubjectIDs: evidenceSubjectIDs,
		})
	}
	index, diagnostics, err := groupindex.Build(program, groupindex.Proposals{
		Groups: groups,
		Connections: []groupindex.ConnectionProposal{{
			FromGroupKey: "trigger-1", ToGroupKey: "core-1", SemanticKind: "enters_local_core",
			Label: "enters core", Summary: "The target trigger enters its local core.", EvidenceSubjectIDs: []string{ids["dispatch-pattern"]},
		}},
	})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("groupindex.Build: index=%#v diagnostics=%#v err=%v relation=%#v", index, diagnostics, err, program.Relations[0])
	}
	return index
}

func matchingSubjectIDs(index programindex.Index) map[string]string {
	result := make(map[string]string)
	for _, object := range index.Objects {
		result[object.SourceRef] = object.ID
	}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			result[pattern.SourceRef] = pattern.ID
		}
	}
	return result
}

func snapshotIndexes(indexes []groupindex.Index) []groupindex.Index {
	result := make([]groupindex.Index, len(indexes))
	for position := range indexes {
		result[position] = indexes[position].Snapshot()
	}
	return result
}

func targetLanguage(indexes []groupindex.Index, targetID string) string {
	for _, index := range indexes {
		if index.Target.ID == targetID {
			return index.Target.Language
		}
	}
	return ""
}

func assertMatchingDiagnostic(t *testing.T, diagnostics []groupindex.Diagnostic, kind string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Kind == kind {
			return
		}
	}
	t.Fatalf("diagnostic %q absent from %#v", kind, diagnostics)
}

func assertProviderWiresHideCanonicalIdentities(t *testing.T, indexes []groupindex.Index, wires []string) {
	t.Helper()
	var forbidden []string
	for _, index := range indexes {
		forbidden = append(forbidden, index.Target.ID, index.ProgramIndexSHA256, index.SHA256)
		for _, group := range index.Groups {
			forbidden = append(forbidden, group.ID)
		}
		for _, subject := range index.Subjects {
			forbidden = append(forbidden, subject.ID)
			if subject.Pattern != nil {
				forbidden = append(forbidden, subject.Pattern.RelationID)
				for _, argument := range subject.Pattern.Arguments {
					forbidden = append(forbidden, argument.ID)
					for _, candidate := range argument.ValueCandidates {
						forbidden = append(forbidden, candidate.ID)
					}
				}
			}
		}
		for _, edge := range index.StructuralEdges {
			forbidden = append(forbidden, edge.RelationID)
		}
		for _, connection := range index.Connections {
			forbidden = append(forbidden, connection.ID)
		}
	}
	for _, wire := range wires {
		if strings.Contains(wire, `"program_index_sha256"`) || strings.Contains(wire, `"relation_id"`) || strings.Contains(wire, `"sha256"`) {
			t.Fatalf("provider request exposes canonical identity field: %s", wire)
		}
		for _, value := range forbidden {
			if value != "" && strings.Contains(wire, value) {
				t.Fatalf("provider request exposes canonical value %q", value)
			}
		}
	}
}

func assertValueCandidateProjection(t *testing.T, request Request) {
	t.Helper()
	subjects := make(map[string]struct{}, len(request.Subjects))
	arguments := make(map[string]struct{})
	candidates := make(map[string]struct{})
	for _, subject := range request.Subjects {
		subjects[subject.Ref] = struct{}{}
	}
	for _, subject := range request.Subjects {
		if subject.Pattern == nil {
			continue
		}
		for _, argument := range subject.Pattern.Arguments {
			if !strings.HasPrefix(argument.Ref, "a") {
				t.Fatalf("argument lacks request-local ref: %#v", argument)
			}
			arguments[argument.Ref] = struct{}{}
		}
	}
	initializerCandidates := 0
	actualArgumentCandidates := 0
	for _, subject := range request.Subjects {
		if subject.Pattern == nil {
			continue
		}
		for _, argument := range subject.Pattern.Arguments {
			for _, candidate := range argument.ValueCandidates {
				if !strings.HasPrefix(candidate.Ref, "v") || candidate.Kind != programindex.PatternLiteralString ||
					candidate.Value != "/commands" || candidate.Resolution != programindex.PatternValuePossible {
					t.Fatalf("lossy value-candidate projection: %#v", candidate)
				}
				switch candidate.SourceKind {
				case programindex.PatternValueSourceInitializer:
					initializerCandidates++
					if candidate.SourceObjectsObserved != 1 || candidate.SourceObjectsOmitted != 0 ||
						len(candidate.SourceObjectRefs) != 1 || candidate.SourceArgumentsObserved != 0 ||
						len(candidate.SourceArgumentRefs) != 0 {
						t.Fatalf("lossy initializer provenance: %#v", candidate)
					}
					if _, known := subjects[candidate.SourceObjectRefs[0]]; !known {
						t.Fatalf("candidate source object is not request-local: %#v", candidate)
					}
				case programindex.PatternValueSourceActualArgument:
					actualArgumentCandidates++
					if candidate.SourceArgumentsObserved != 1 || candidate.SourceArgumentsOmitted != 0 ||
						len(candidate.SourceArgumentRefs) != 1 || candidate.SourceObjectsObserved != 0 ||
						len(candidate.SourceObjectRefs) != 0 {
						t.Fatalf("lossy actual-argument provenance: %#v", candidate)
					}
					if _, known := arguments[candidate.SourceArgumentRefs[0]]; !known {
						t.Fatalf("candidate source argument is not request-local: %#v", candidate)
					}
				default:
					t.Fatalf("unexpected candidate source kind: %#v", candidate)
				}
				candidates[candidate.Ref] = struct{}{}
			}
		}
	}
	if len(candidates) < 2 || initializerCandidates+actualArgumentCandidates != len(candidates) {
		t.Fatalf("pair dossier candidate cover = %d initializer=%d actual=%d, want at least one exact endpoint candidate per target",
			len(candidates), initializerCandidates, actualArgumentCandidates)
	}
	objectSourceEdges := 0
	argumentSourceEdges := 0
	for _, edge := range request.StructuralEdges {
		if edge.Role != groupindex.EdgePatternValueSourceObject && edge.Role != groupindex.EdgePatternValueSourceArgument {
			continue
		}
		if _, known := arguments[edge.ArgumentRef]; !known {
			t.Fatalf("value edge has unknown argument ref: %#v", edge)
		}
		if _, known := candidates[edge.ValueCandidateRef]; !known {
			t.Fatalf("value edge has unknown candidate ref: %#v", edge)
		}
		if edge.ValueResolution != programindex.PatternValuePossible {
			t.Fatalf("value edge lost value resolution: %#v", edge)
		}
		switch edge.Role {
		case groupindex.EdgePatternValueSourceObject:
			objectSourceEdges++
			if edge.ValueSourceKind != programindex.PatternValueSourceInitializer || edge.SourceArgumentRef != "" {
				t.Fatalf("initializer edge lost provenance: %#v", edge)
			}
		case groupindex.EdgePatternValueSourceArgument:
			argumentSourceEdges++
			if edge.ValueSourceKind != programindex.PatternValueSourceActualArgument {
				t.Fatalf("actual-argument edge lost provenance: %#v", edge)
			}
			if _, known := arguments[edge.SourceArgumentRef]; !known {
				t.Fatalf("actual-argument edge has unknown source argument: %#v", edge)
			}
		}
	}
	if objectSourceEdges > initializerCandidates || argumentSourceEdges > actualArgumentCandidates ||
		objectSourceEdges+argumentSourceEdges == 0 {
		t.Fatalf("pair dossier incident value-source edges = object:%d argument:%d, candidates=%d/%d",
			objectSourceEdges, argumentSourceEdges, initializerCandidates, actualArgumentCandidates)
	}
}
