package programgrouping

import (
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

type presetProvider struct {
	mu                  sync.Mutex
	requests            []Request
	prompts             []llm.Prompt
	maxInitialGroupRefs int
	respond             func(Request) []byte
}

func (provider *presetProvider) State() []byte {
	return []byte(`{"provider":"program-grouping-preset-v1"}`)
}

func (provider *presetProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var request Request
	if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
		return llm.Prepared{}, err
	}
	if provider.maxInitialGroupRefs > 0 && request.Phase == phaseGrouping &&
		len(request.GroupRefs) > provider.maxInitialGroupRefs {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitRequestBytes, Limit: provider.maxInitialGroupRefs,
			Observed: len(request.GroupRefs), ObservedKnown: true,
		})
	}
	wire, err := json.Marshal(struct {
		System    string `json:"system"`
		User      string `json:"user"`
		JSON      bool   `json:"json"`
		MaxTokens int    `json:"max_tokens"`
	}{prompt.System, prompt.User, prompt.ResponseFormatJSON, limits.MaxOutputTokens})
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

func (provider *presetProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
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
	if err := validatePresetRequest(request); err != nil {
		return llm.Completion{}, err
	}
	provider.mu.Lock()
	provider.requests = append(provider.requests, request)
	provider.prompts = append(provider.prompts, llm.Prompt{
		System: envelope.System, User: envelope.User, ResponseFormatJSON: true,
	})
	provider.mu.Unlock()
	response := []byte(`{"groups":[],"connections":[]}`)
	if provider.respond != nil {
		response = provider.respond(request)
	}
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, Latency: time.Millisecond},
	}, nil
}

func validatePresetRequest(request Request) error {
	if request.Version != requestVersion ||
		request.Phase != phaseGrouping && request.Phase != phaseMerge ||
		request.GroupRefs == nil || request.Subjects == nil || request.Edges == nil ||
		request.CandidateGroups == nil || request.CandidateConnections == nil {
		return fmt.Errorf("preset: incomplete request")
	}
	subjects := make(map[string]subjectWire, len(request.Subjects))
	arguments := make(map[string]struct{})
	for _, subject := range request.Subjects {
		if subject.Ref == "" {
			return fmt.Errorf("preset: empty subject ref")
		}
		subjects[subject.Ref] = subject
		for _, argument := range subject.Arguments {
			if argument.Ref == "" {
				return fmt.Errorf("preset: empty argument ref")
			}
			if _, duplicate := arguments[argument.Ref]; duplicate {
				return fmt.Errorf("preset: duplicate argument ref %s", argument.Ref)
			}
			arguments[argument.Ref] = struct{}{}
		}
	}
	for _, ref := range request.GroupRefs {
		subject, known := subjects[ref]
		if !known || len(subject.Categories) == 0 {
			return fmt.Errorf("preset: group ref %s is absent or unclassified", ref)
		}
	}
	for _, edge := range request.Edges {
		if _, known := subjects[edge.FromRef]; !known {
			return fmt.Errorf("preset: edge %s has no source subject", edge.Ref)
		}
		if _, known := subjects[edge.ToRef]; !known {
			return fmt.Errorf("preset: edge %s has no destination subject", edge.Ref)
		}
		if edge.SourceArgument != nil {
			if _, known := subjects[edge.SourceArgument.PatternRef]; !known {
				return fmt.Errorf("preset: edge %s source argument has no owning pattern subject", edge.Ref)
			}
		}
	}
	for _, subject := range request.Subjects {
		for _, argument := range subject.Arguments {
			for _, ref := range argument.ObjectRefs {
				if _, known := subjects[ref]; !known {
					return fmt.Errorf("preset: argument has unknown object ref %s", ref)
				}
			}
			for _, candidate := range argument.ValueCandidates {
				for _, ref := range candidate.SourceObjectRefs {
					if _, known := subjects[ref]; !known {
						return fmt.Errorf("preset: candidate has unknown source object ref %s", ref)
					}
				}
				for _, ref := range candidate.SourceArgumentRefs {
					if _, known := arguments[ref]; !known {
						return fmt.Errorf("preset: candidate has unknown source argument ref %s", ref)
					}
				}
			}
		}
	}
	return nil
}

func TestRunBuildsSparseOverlappingGroupsAndOpenSemanticConnections(t *testing.T) {
	index := groupingTestIndex(t, "go")
	provider := &presetProvider{}
	provider.respond = func(request Request) []byte {
		refs := requestRefs(request)
		return []byte(fmt.Sprintf(`{
  "groups": [
    {"key":"triggers","title":"Order triggers","summary":"Starts order work","lane":"triggers","member_refs":[%q,%q],"evidence_refs":[%q]},
    {"key":"core","title":"Order core","summary":"Processes orders","lane":"core","member_refs":[%q],"evidence_refs":[]},
    {"key":"core","title":"Order core","summary":"Processes orders","lane":"core","member_refs":[%q],"evidence_refs":[%q]},
    {"key":"unknown","title":"Unknown","summary":"Broken ref","lane":"core","member_refs":["s9999"],"evidence_refs":[]},
    {"key":"context-member","title":"Context","summary":"Must not select context","lane":"core","member_refs":[%q],"evidence_refs":[]},
    {"key":42,"title":"Malformed","summary":"Wrong key type","lane":"core","member_refs":[%q],"evidence_refs":[]}
  ],
  "connections": [
    {"from_group_key":"triggers","to_group_key":"core","semantic_kind":"dispatches_work_to","label":"dispatches orders","summary":"Triggers hand work to the order core","evidence_refs":[%q]},
    {"from_group_key":"triggers","to_group_key":"core","semantic_kind":"dispatches_work_to","label":"dispatches orders","summary":"Triggers hand work to the order core","evidence_refs":[%q]},
    {"from_group_key":"triggers","to_group_key":"missing","semantic_kind":"uses","label":"broken","summary":"Unknown group key","evidence_refs":[]},
    {"from_group_key":"triggers","to_group_key":"core","semantic_kind":"Not Snake","label":"broken","summary":"Invalid kind","evidence_refs":[]}
  ]
}`,
			refs["pattern:HandleFunc"], refs["object:runWorker"], refs["object:normalize"],
			refs["object:processOrder"], refs["pattern:HandleFunc"], refs["object:normalize"],
			refs["object:normalize"], refs["object:processOrder"],
			refs["pattern:HandleFunc"], refs["object:processOrder"],
		))
	}

	grouped, diagnostics, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 4, BatchController: &llm.BatchController{},
	}, provider, index)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := grouped.Validate(); err != nil {
		t.Fatalf("GroupsIndex.Validate: %v", err)
	}
	trigger := groupByTitle(t, grouped, "Order triggers")
	core := groupByTitle(t, grouped, "Order core")
	if trigger.Lane != groupindex.LaneTriggers || len(trigger.MemberSubjectIDs) != 2 {
		t.Fatalf("trigger group = %#v", trigger)
	}
	if core.Lane != groupindex.LaneCore || len(core.MemberSubjectIDs) != 2 {
		t.Fatalf("compatible duplicate core rows were not unioned: %#v", core)
	}
	patternID := subjectIDBySourceRef(t, index, "registration-pattern")
	if !containsString(trigger.MemberSubjectIDs, patternID) || !containsString(core.MemberSubjectIDs, patternID) {
		t.Fatalf("overlapping core/trigger membership was lost: trigger=%#v core=%#v", trigger, core)
	}
	if len(grouped.Connections) != 1 || grouped.Connections[0].SemanticKind != "dispatches_work_to" {
		t.Fatalf("open semantic connection = %#v", grouped.Connections)
	}
	wantEvidence := []string{
		subjectIDBySourceRef(t, index, "process"),
		subjectIDBySourceRef(t, index, "registration-pattern"),
	}
	sort.Strings(wantEvidence)
	var gotEvidence []string
	for _, evidence := range grouped.Connections[0].Evidence {
		gotEvidence = append(gotEvidence, evidence.SubjectID)
	}
	if !reflect.DeepEqual(gotEvidence, wantEvidence) {
		t.Fatalf("compatible duplicate connection evidence = %#v, want %#v", gotEvidence, wantEvidence)
	}
	for _, omitted := range []string{"audit", "storage"} {
		id := subjectIDBySourceRef(t, index, omitted)
		for _, group := range grouped.Groups {
			if containsString(group.MemberSubjectIDs, id) {
				t.Fatalf("sparse omitted subject %q appeared in group %#v", omitted, group)
			}
		}
	}
	assertDiagnosticKind(t, diagnostics, diagnosticUnknownMemberRef)
	assertDiagnosticKind(t, diagnostics, diagnosticUnselectableMember)
	assertDiagnosticKind(t, diagnostics, diagnosticMalformedGroup)
	assertDiagnosticKind(t, diagnostics, diagnosticUnknownGroupKey)
	assertDiagnosticKind(t, diagnostics, diagnosticInvalidConnection)

	provider.mu.Lock()
	requests := append([]Request(nil), provider.requests...)
	prompts := append([]llm.Prompt(nil), provider.prompts...)
	provider.mu.Unlock()
	if len(requests) != 1 || requests[0].Phase != phaseGrouping {
		t.Fatalf("provider requests = %#v", requests)
	}
	refs := requestRefs(requests[0])
	helper := subjectByRef(t, requests[0], refs["object:normalize"])
	if len(helper.Categories) != 0 {
		t.Fatalf("unclassified context categories = %#v", helper.Categories)
	}
	if !requestHasGlobalSourceArgument(requests[0], refs["pattern:HandleFunc"]) {
		t.Fatalf("cross-relation source argument was not restored through its owning pattern: %#v", requests[0].Edges)
	}
	registration := subjectByRef(t, requests[0], refs["pattern:HandleFunc"])
	if len(registration.Arguments) == 0 || registration.Arguments[0].Ref == "" ||
		len(registration.Arguments[0].ValueCandidates) != 2 ||
		registration.Arguments[0].ValueCandidatesObserved != 2 || registration.Arguments[0].ValueCandidatesOmitted != 0 {
		t.Fatalf("resolved value candidate was not projected losslessly: %#v", registration.Arguments)
	}
	argumentsByRef := make(map[string]argumentWire)
	for _, subject := range requests[0].Subjects {
		for _, argument := range subject.Arguments {
			argumentsByRef[argument.Ref] = argument
		}
	}
	var initializer, actual valueCandidateWire
	for _, candidate := range registration.Arguments[0].ValueCandidates {
		switch candidate.SourceKind {
		case programindex.PatternValueSourceInitializer:
			initializer = candidate
		case programindex.PatternValueSourceActualArgument:
			actual = candidate
		}
	}
	if initializer.Value != "/orders" || len(initializer.SourceObjectRefs) != 1 ||
		initializer.SourceObjectsObserved != 1 || initializer.SourceObjectsOmitted != 0 {
		t.Fatalf("initializer value candidate = %#v", initializer)
	}
	if actual.Value != "/products/runtime" || actual.Resolution != programindex.PatternValuePossible ||
		len(actual.SourceArgumentRefs) != 1 || actual.SourceArgumentsObserved != 1 || actual.SourceArgumentsOmitted != 0 {
		t.Fatalf("actual-argument value candidate = %#v", actual)
	}
	if source, known := argumentsByRef[actual.SourceArgumentRefs[0]]; !known ||
		source.Kind != programindex.PatternLiteralString || source.Value != actual.Value {
		t.Fatalf("actual source ref does not resolve inside request: %q => %#v", actual.SourceArgumentRefs[0], source)
	}
	for _, object := range index.Objects {
		if strings.Contains(prompts[0].User, object.ID) {
			t.Fatalf("provider request leaked canonical ProgramIndex object ID %q", object.ID)
		}
	}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if strings.Contains(prompts[0].User, argument.ID) {
					t.Fatalf("provider request leaked canonical ProgramIndex argument ID %q", argument.ID)
				}
				for _, candidate := range argument.ValueCandidates {
					if strings.Contains(prompts[0].User, candidate.ID) {
						t.Fatalf("provider request leaked canonical ProgramIndex value ID %q", candidate.ID)
					}
				}
			}
		}
	}
	if strings.Contains(prompts[0].System, "one row for every") ||
		!strings.Contains(prompts[0].System, "open snake_case vocabulary") {
		t.Fatalf("prompt lost sparse/open-vocabulary contract")
	}
}

func TestNormalizeResponseFiltersSetValuedRefsWithoutDiscardingValidRows(t *testing.T) {
	index := groupingTestIndex(t, "go")
	compilation, err := Compile(index)
	if err != nil {
		t.Fatal(err)
	}
	request, err := compilation.request(
		phaseGrouping, compilation.categorizedRefs, proposalSet{},
	)
	if err != nil {
		t.Fatal(err)
	}
	refs := requestRefs(request)
	processRef := refs["object:processOrder"]
	handlerRef := refs["object:handleOrder"]
	workerRef := refs["object:runWorker"]
	storageRef := refs["object:Store.Put"]
	platformRef := refs["object:platform:javascript.requestAnimationFrame"]
	platformPatternRef := refs["pattern:requestAnimationFrame"]
	contextRef := refs["object:normalize"]
	if processRef == "" || handlerRef == "" || workerRef == "" || storageRef == "" ||
		platformRef == "" || platformPatternRef == "" || contextRef == "" {
		t.Fatalf("fixture request refs = %#v", refs)
	}
	raw, err := json.Marshal(struct {
		Groups      []responseGroup      `json:"groups"`
		Connections []responseConnection `json:"connections"`
	}{
		Groups: []responseGroup{
			{
				Key: "mixed", Title: "Mixed", Summary: "Keeps advertised selectable members", Lane: groupindex.LaneCore,
				MemberRefs:   []string{processRef, workerRef, contextRef, "s9999", handlerRef, processRef},
				EvidenceRefs: []string{contextRef, "s9998", contextRef},
			},
			{
				Key: "all-invalid", Title: "Invalid", Summary: "Has no selectable members", Lane: groupindex.LaneCore,
				MemberRefs: []string{contextRef, "s9997"}, EvidenceRefs: []string{},
			},
			{
				Key: "peer", Title: "Peer", Summary: "Connection endpoint", Lane: groupindex.LaneTriggers,
				MemberRefs: []string{workerRef}, EvidenceRefs: []string{},
			},
			{
				Key: "dependency", Title: "Storage dependency", Summary: "Filters platform-only evidence", Lane: groupindex.LaneDependencies,
				MemberRefs:   []string{storageRef},
				EvidenceRefs: []string{platformRef, platformPatternRef, processRef},
			},
		},
		Connections: []responseConnection{{
			FromGroupKey: "mixed", ToGroupKey: "peer", SemanticKind: "dispatches_to",
			Label: "dispatches", Summary: "Mixed reaches peer",
			EvidenceRefs: []string{contextRef, "s9996", contextRef},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	normalized, err := normalizeResponse(raw, compilation, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.groups) != 3 {
		t.Fatalf("filtered groups = %#v", normalized.groups)
	}
	groupsByKey := make(map[string]groupProposal, len(normalized.groups))
	for _, group := range normalized.groups {
		groupsByKey[group.Key] = group
	}
	mixed, ok := groupsByKey["mixed"]
	if !ok {
		t.Fatalf("mixed group was discarded: %#v", normalized.groups)
	}
	wantMembers := canonicalStrings([]string{
		compilation.subjectByRef[processRef].id,
		compilation.subjectByRef[handlerRef].id,
	})
	if !reflect.DeepEqual(mixed.MemberSubjectIDs, wantMembers) {
		t.Fatalf("mixed member subset = %#v, want %#v", mixed.MemberSubjectIDs, wantMembers)
	}
	wantEvidence := []string{compilation.subjectByRef[contextRef].id}
	if !reflect.DeepEqual(mixed.EvidenceSubjectIDs, wantEvidence) {
		t.Fatalf("mixed evidence subset = %#v, want %#v", mixed.EvidenceSubjectIDs, wantEvidence)
	}
	if _, exists := groupsByKey["all-invalid"]; exists {
		t.Fatalf("all-invalid group survived: %#v", normalized.groups)
	}
	dependency, exists := groupsByKey["dependency"]
	if !exists || !reflect.DeepEqual(dependency.EvidenceSubjectIDs, []string{compilation.subjectByRef[processRef].id}) {
		t.Fatalf("dependency evidence retained platform authority: %#v", dependency)
	}
	if len(normalized.connections) != 1 ||
		!reflect.DeepEqual(normalized.connections[0].EvidenceSubjectIDs, wantEvidence) {
		t.Fatalf("connection after evidence filtering = %#v", normalized.connections)
	}
	assertDiagnosticKind(t, normalized.diagnostics, diagnosticUnknownMemberRef)
	assertDiagnosticKind(t, normalized.diagnostics, diagnosticUnselectableMember)
	assertDiagnosticKind(t, normalized.diagnostics, diagnosticLaneMismatch)
	assertDiagnosticKind(t, normalized.diagnostics, diagnosticUnsupportedEvidence)
	unknownEvidenceDiagnostics := 0
	for _, diagnostic := range normalized.diagnostics {
		if diagnostic.Kind == diagnosticUnknownEvidenceRef {
			unknownEvidenceDiagnostics++
		}
	}
	if unknownEvidenceDiagnostics != 2 {
		t.Fatalf("unknown evidence diagnostics = %#v", normalized.diagnostics)
	}
}

func TestNormalizeMergeResponseRejectsCandidateMemberLoss(t *testing.T) {
	index := groupingTestIndex(t, "jsts")
	compilation, err := Compile(index)
	if err != nil {
		t.Fatal(err)
	}
	handlerID := subjectIDBySourceRef(t, index, "handler")
	processID := subjectIDBySourceRef(t, index, "process")
	candidates := proposalSet{groups: []groupProposal{
		{
			Key: "http-run", Title: "HTTP service layer", Summary: "Runs one level",
			Lane: groupindex.LaneCore, MemberSubjectIDs: []string{handlerID}, EvidenceSubjectIDs: []string{},
		},
		{
			Key: "http-read", Title: "HTTP service layer", Summary: "Reads levels",
			Lane: groupindex.LaneCore, MemberSubjectIDs: []string{processID}, EvidenceSubjectIDs: []string{},
		},
	}}
	request, err := compilation.mergeRequest(candidates)
	if err != nil {
		t.Fatal(err)
	}
	handlerRef := compilation.refBySubjectID[handlerID]
	processRef := compilation.refBySubjectID[processID]
	response := func(memberRefs []string, lane groupindex.Lane) []byte {
		t.Helper()
		wire, marshalErr := json.Marshal(struct {
			Groups      []responseGroup      `json:"groups"`
			Connections []responseConnection `json:"connections"`
		}{
			Groups: []responseGroup{{
				Key: "http", Title: "HTTP service layer", Summary: "Reads and runs levels",
				Lane: lane, MemberRefs: memberRefs, EvidenceRefs: []string{},
			}},
			Connections: []responseConnection{},
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return wire
	}

	if _, err := normalizeResponse(
		response([]string{processRef}, groupindex.LaneCore), compilation, request,
	); err == nil || !strings.Contains(err.Error(), "omitted validated candidate memberships") {
		t.Fatalf("lossy merge error = %v", err)
	}
	if _, err := normalizeResponse(
		response([]string{handlerRef, processRef}, groupindex.LaneTriggers), compilation, request,
	); err == nil || !strings.Contains(err.Error(), "omitted validated candidate memberships") {
		t.Fatalf("cross-lane merge error = %v", err)
	}
	merged, err := normalizeResponse(
		response([]string{handlerRef, processRef}, groupindex.LaneCore), compilation, request,
	)
	if err != nil {
		t.Fatalf("complete merge: %v", err)
	}
	if len(merged.groups) != 1 ||
		!reflect.DeepEqual(merged.groups[0].MemberSubjectIDs, canonicalStrings([]string{handlerID, processID})) {
		t.Fatalf("complete consolidated merge = %#v", merged.groups)
	}
}

func TestPromptMakesEveryMembershipLaneCompatibleAndMergePreserving(t *testing.T) {
	for _, required := range []string{
		"immutable restored group memberships",
		"sparse initial-selection rule does not authorize",
		"must not retract membership already selected",
		"Every `member_ref` must itself carry a category compatible",
		"`inbound` or `background_activity` for `triggers`",
		"`dependency` for `dependencies`",
		"not become membership",
		"Every candidate member is already individually compatible",
		"candidate lane/member sets are",
		"immutable lower bounds",
	} {
		if !strings.Contains(promptText, required) {
			t.Fatalf("grouping prompt lost merge preservation rule %q", required)
		}
	}
}

func TestNormalizeResponseRequiresExactUniqueJSONKeys(t *testing.T) {
	index := groupingTestIndex(t, "go")
	compilation, err := Compile(index)
	if err != nil {
		t.Fatal(err)
	}
	request, err := compilation.request(phaseGrouping, compilation.categorizedRefs, proposalSet{})
	if err != nil {
		t.Fatal(err)
	}
	refs := requestRefs(request)
	processRef := refs["object:processOrder"]
	workerRef := refs["object:runWorker"]

	for name, raw := range map[string][]byte{
		"duplicate envelope key":   []byte(`{"groups":[],"groups":[],"connections":[]}`),
		"case folded envelope key": []byte(`{"Groups":[],"connections":[]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeResponse(raw, compilation, request); err == nil {
				t.Fatal("closed envelope was accepted")
			}
		})
	}

	for name, raw := range map[string][]byte{
		"duplicate group key":      []byte(fmt.Sprintf(`{"groups":[{"key":"core","key":"other","title":"Core","summary":"Core work","lane":"core","member_refs":[%q],"evidence_refs":[]}],"connections":[]}`, processRef)),
		"case folded group key":    []byte(fmt.Sprintf(`{"groups":[{"Key":"core","title":"Core","summary":"Core work","lane":"core","member_refs":[%q],"evidence_refs":[]}],"connections":[]}`, processRef)),
		"duplicate connection key": []byte(fmt.Sprintf(`{"groups":[{"key":"core","title":"Core","summary":"Core work","lane":"core","member_refs":[%q],"evidence_refs":[]},{"key":"trigger","title":"Trigger","summary":"Starts work","lane":"triggers","member_refs":[%q],"evidence_refs":[]}],"connections":[{"from_group_key":"trigger","from_group_key":"core","to_group_key":"core","semantic_kind":"dispatches_to","label":"dispatches","summary":"Starts core work","evidence_refs":[]}]}`, processRef, workerRef)),
	} {
		t.Run(name, func(t *testing.T) {
			normalized, normalizeErr := normalizeResponse(raw, compilation, request)
			if normalizeErr != nil {
				t.Fatalf("row-local malformed value became envelope error: %v", normalizeErr)
			}
			if len(normalized.diagnostics) == 0 {
				t.Fatalf("malformed row was accepted: %#v", normalized)
			}
		})
	}
}

func TestRunUsesOneLanguageNeutralContractForGoPythonAndJSTS(t *testing.T) {
	var systemPrompt string
	for _, language := range []string{"go", "python", "jsts"} {
		t.Run(language, func(t *testing.T) {
			index := groupingTestIndex(t, language)
			provider := &presetProvider{}
			grouped, diagnostics, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, index)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(grouped.Groups) != 0 || len(grouped.Connections) != 0 || len(diagnostics) != 0 {
				t.Fatalf("sparse empty output = %#v / %#v", grouped, diagnostics)
			}
			provider.mu.Lock()
			requests := append([]Request(nil), provider.requests...)
			prompts := append([]llm.Prompt(nil), provider.prompts...)
			provider.mu.Unlock()
			if len(requests) != 1 || requests[0].Target.Language != language || len(requests[0].GroupRefs) == 0 {
				t.Fatalf("language-neutral request = %#v", requests)
			}
			if systemPrompt == "" {
				systemPrompt = prompts[0].System
			} else if prompts[0].System != systemPrompt {
				t.Fatal("language changed the shared grouping prompt")
			}
		})
	}
}

func TestRunExhaustivelyBatchesAndConvergentlyMerges(t *testing.T) {
	index := groupingTestIndex(t, "python")
	provider := &presetProvider{maxInitialGroupRefs: 1}
	provider.respond = func(request Request) []byte {
		if request.Phase == phaseGrouping {
			ref := request.GroupRefs[0]
			subject := subjectByRef(t, request, ref)
			lane := laneForCategories(subject.Categories)
			return []byte(fmt.Sprintf(`{"groups":[{"key":"g1","title":%q,"summary":"Shard group","lane":%q,"member_refs":[%q],"evidence_refs":[]}],"connections":[]}`,
				"Group "+ref, lane, ref))
		}
		membersByLane := map[groupindex.Lane][]string{}
		for _, ref := range request.GroupRefs {
			subject := subjectByRef(t, request, ref)
			membersByLane[laneForCategories(subject.Categories)] = append(membersByLane[laneForCategories(subject.Categories)], ref)
		}
		keys := []groupindex.Lane{groupindex.LaneTriggers, groupindex.LaneCore, groupindex.LaneDependencies}
		groups := make([]responseGroup, 0)
		for _, lane := range keys {
			members := membersByLane[lane]
			if len(members) == 0 {
				continue
			}
			groups = append(groups, responseGroup{
				Key: string(lane), Title: strings.ToUpper(string(lane)), Summary: "Merged target lane",
				Lane: lane, MemberRefs: members, EvidenceRefs: []string{},
			})
		}
		connections := []responseConnection{}
		if len(membersByLane[groupindex.LaneTriggers]) > 0 && len(membersByLane[groupindex.LaneCore]) > 0 {
			connections = append(connections, responseConnection{
				FromGroupKey: string(groupindex.LaneTriggers), ToGroupKey: string(groupindex.LaneCore),
				SemanticKind: "initiates_domain_work", Label: "starts work",
				Summary: "Trigger groups initiate core domain behavior", EvidenceRefs: []string{},
			})
		}
		wire, err := json.Marshal(struct {
			Groups      []responseGroup      `json:"groups"`
			Connections []responseConnection `json:"connections"`
		}{groups, connections})
		if err != nil {
			t.Fatalf("marshal merge response: %v", err)
		}
		return wire
	}

	grouped, diagnostics, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 4, BatchController: &llm.BatchController{},
	}, provider, index)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if len(grouped.Groups) != 3 || len(grouped.Connections) != 1 ||
		grouped.Connections[0].SemanticKind != "initiates_domain_work" {
		t.Fatalf("merged GroupsIndex = groups %#v connections %#v", grouped.Groups, grouped.Connections)
	}
	trigger := groupByTitle(t, grouped, strings.ToUpper(string(groupindex.LaneTriggers)))
	if len(trigger.MemberSubjectIDs) != 3 {
		t.Fatalf("inbound + background activity were not merged into triggers: %#v", trigger)
	}

	provider.mu.Lock()
	requests := append([]Request(nil), provider.requests...)
	provider.mu.Unlock()
	compilation, err := Compile(index)
	if err != nil {
		t.Fatal(err)
	}
	groupingCalls := 0
	mergeCalls := 0
	seen := make(map[string]int)
	for _, request := range requests {
		switch request.Phase {
		case phaseGrouping:
			groupingCalls++
			if len(request.GroupRefs) != 1 {
				t.Fatalf("forced grouping shard = %#v", request.GroupRefs)
			}
			assertCompleteIncidentEdges(t, compilation, request)
			seen[request.GroupRefs[0]]++
		case phaseMerge:
			mergeCalls++
			if len(request.CandidateGroups) < 2 {
				t.Fatalf("merge request did not provide a cross-shard meeting point: %#v", request.CandidateGroups)
			}
		}
	}
	if groupingCalls != len(compilation.categorizedRefs) || mergeCalls == 0 {
		t.Fatalf("calls grouping=%d merge=%d, want %d and >0", groupingCalls, mergeCalls, len(compilation.categorizedRefs))
	}
	for _, ref := range compilation.categorizedRefs {
		if seen[ref] != 1 {
			t.Fatalf("categorized subject %s appeared in %d primary shards", ref, seen[ref])
		}
	}
}

func assertCompleteIncidentEdges(t *testing.T, compilation Compilation, request Request) {
	t.Helper()
	ownedIDs := make(map[string]struct{}, len(request.GroupRefs))
	for _, ref := range request.GroupRefs {
		ownedIDs[compilation.subjectByRef[ref].id] = struct{}{}
	}
	want := make(map[string]struct{})
	for _, edge := range compilation.edges {
		_, fromOwned := ownedIDs[edge.fromID]
		_, toOwned := ownedIDs[edge.toID]
		if fromOwned || toOwned {
			want[edge.ref] = struct{}{}
		}
	}
	got := make(map[string]struct{}, len(request.Edges))
	for _, edge := range request.Edges {
		got[edge.Ref] = struct{}{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("incident edge cover = %#v, want %#v for %v", got, want, request.GroupRefs)
	}
}

func groupingTestIndex(t *testing.T, language string) programindex.Index {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "src/orders.lang", Line: line, Column: 2}
	}
	objects := []programindex.ObjectInput{
		{SourceRef: "module", Kind: programindex.ObjectModule, Name: "orders", Visibility: programindex.VisibilityPublic, Location: location(1)},
		{SourceRef: "bootstrap", Kind: programindex.ObjectFunction, Name: "configure", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(5)},
		{SourceRef: "handler", Kind: programindex.ObjectFunction, Name: "handleOrder", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(20)},
		{SourceRef: "worker", Kind: programindex.ObjectFunction, Name: "runWorker", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(30)},
		{SourceRef: "process", Kind: programindex.ObjectFunction, Name: "processOrder", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(40)},
		{SourceRef: "audit", Kind: programindex.ObjectFunction, Name: "auditOrder", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(50)},
		{SourceRef: "helper", Kind: programindex.ObjectFunction, Name: "normalize", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(60)},
		{SourceRef: "route-path", Kind: programindex.ObjectVariable, Name: "ordersPath", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(8)},
		{SourceRef: "register", Kind: programindex.ObjectExternalSymbol, Name: "Router.HandleFunc", Visibility: programindex.VisibilityPublic, External: &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: "example.org/router", Receiver: "Router", Name: "HandleFunc"}},
		{SourceRef: "storage", Kind: programindex.ObjectExternalSymbol, Name: "Store.Put", Visibility: programindex.VisibilityPublic, External: &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: "example.org/storage", Receiver: "Store", Name: "Put"}},
		{
			SourceRef: "platform-raf", Kind: programindex.ObjectExternalSymbol,
			Name: "platform:javascript.requestAnimationFrame", Visibility: programindex.VisibilityPublic,
			External: &programindex.ExternalSymbol{
				AuthorityKind: programindex.ExternalAuthorityPlatform,
				PackagePath:   "platform:javascript", Name: "requestAnimationFrame",
			},
		},
	}
	relations := []programindex.RelationInput{
		{
			SourceRef: "registration", Kind: programindex.RelationInvokesExternal, FromRef: "bootstrap",
			ToRefs: []string{"register"}, Resolution: programindex.ResolutionExact,
			Location: location(10), TargetsObserved: 1,
			Witnesses: []programindex.Witness{{Kind: "call", Detail: "route registration", Location: location(10)}}, WitnessesObserved: 1,
			Patterns: []programindex.RelationPatternInput{{
				SourceRef: "registration-pattern", Form: programindex.PatternCall,
				Selector: "HandleFunc", Location: location(10), ArgumentsObserved: 2,
				Arguments: []programindex.PatternArgumentInput{
					{
						Position: 1, Kind: programindex.PatternDynamic,
						ValueCandidatesObserved: 2,
						ValueCandidates: []programindex.PatternValueCandidateInput{
							{
								Kind: programindex.PatternLiteralString, Value: "/orders",
								Resolution:       programindex.PatternValueExact,
								SourceKind:       programindex.PatternValueSourceInitializer,
								SourceObjectRefs: []string{"route-path"}, SourceObjectsObserved: 1,
							},
							{
								Kind: programindex.PatternLiteralString, Value: "/products/runtime",
								Resolution: programindex.PatternValuePossible, SourceKind: programindex.PatternValueSourceActualArgument,
								SourceArgumentRefs: []programindex.PatternArgumentRefInput{{
									RelationSourceRef: "actual-call", PatternSourceRef: "actual-pattern", Position: 1,
								}},
								SourceArgumentsObserved: 1,
							},
						},
					},
					{Position: 2, Kind: programindex.PatternDynamic, ObjectRefs: []string{"handler"}, Resolution: programindex.ResolutionExact, ObjectsObserved: 1},
				},
			}}, PatternsObserved: 1,
		},
		{
			SourceRef: "actual-call", Kind: programindex.RelationCalls, FromRef: "helper",
			ToRefs: []string{"bootstrap"}, Resolution: programindex.ResolutionExact,
			Location: location(9), TargetsObserved: 1,
			Witnesses: []programindex.Witness{{Kind: "call", Location: location(9)}}, WitnessesObserved: 1,
			PatternsObserved: 1, Patterns: []programindex.RelationPatternInput{{
				SourceRef: "actual-pattern", Form: programindex.PatternCall, Selector: "configure", Location: location(9),
				ArgumentsObserved: 1, Arguments: []programindex.PatternArgumentInput{{
					Position: 1, Kind: programindex.PatternLiteralString, Value: "/products/runtime",
				}},
			}},
		},
		{
			SourceRef: "callback-handoff", Kind: programindex.RelationPassesCallback, FromRef: "bootstrap",
			ToRefs: []string{"handler"}, Resolution: programindex.ResolutionExact,
			Location: location(11), TargetsObserved: 1,
			Witnesses: []programindex.Witness{{Kind: "callback", Location: location(11)}}, WitnessesObserved: 1,
			SourceArgument: &programindex.PatternArgumentRefInput{
				RelationSourceRef: "registration", PatternSourceRef: "registration-pattern", Position: 2,
			},
		},
		{SourceRef: "handler-process", Kind: programindex.RelationCalls, FromRef: "handler", ToRefs: []string{"process"}, Resolution: programindex.ResolutionExact, Location: location(22), TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "call", Location: location(22)}}, WitnessesObserved: 1},
		{SourceRef: "worker-process", Kind: programindex.RelationCalls, FromRef: "worker", ToRefs: []string{"process"}, Resolution: programindex.ResolutionExact, Location: location(32), TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "call", Location: location(32)}}, WitnessesObserved: 1},
		{SourceRef: "process-helper", Kind: programindex.RelationCalls, FromRef: "process", ToRefs: []string{"helper"}, Resolution: programindex.ResolutionExact, Location: location(42), TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "call", Location: location(42)}}, WitnessesObserved: 1},
		{SourceRef: "process-storage", Kind: programindex.RelationInvokesExternal, FromRef: "process", ToRefs: []string{"storage"}, Resolution: programindex.ResolutionExact, Location: location(44), TargetsObserved: 1, Witnesses: []programindex.Witness{{Kind: "call", Location: location(44)}}, WitnessesObserved: 1},
		{
			SourceRef: "worker-platform", Kind: programindex.RelationInvokesExternal,
			FromRef: "worker", ToRefs: []string{"platform-raf"}, Resolution: programindex.ResolutionExact,
			Location: location(34), TargetsObserved: 1,
			Witnesses: []programindex.Witness{{Kind: "call", Location: location(34)}}, WitnessesObserved: 1,
			PatternsObserved: 1, Patterns: []programindex.RelationPatternInput{{
				SourceRef: "worker-platform-pattern", Form: programindex.PatternCall,
				Selector: "requestAnimationFrame", Location: location(34),
				Arguments: []programindex.PatternArgumentInput{}, ArgumentsObserved: 0,
			}},
		},
	}
	base, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: "application", Name: "orders", Selector: language + ":orders",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "src/orders.lang"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "bootstrap", Kind: programindex.SeedCallable, Location: location(5)}},
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	assignments := []programindex.CategoryAssignment{
		{SubjectID: subjectIDBySourceRef(t, base, "registration-pattern"), Categories: []programindex.Category{programindex.CategoryInbound, programindex.CategoryCore}},
		{SubjectID: subjectIDBySourceRef(t, base, "handler"), Categories: []programindex.Category{programindex.CategoryCore}},
		{SubjectID: subjectIDBySourceRef(t, base, "worker"), Categories: []programindex.Category{programindex.CategoryBackgroundActivity}},
		{SubjectID: subjectIDBySourceRef(t, base, "process"), Categories: []programindex.Category{programindex.CategoryCore}},
		{SubjectID: subjectIDBySourceRef(t, base, "audit"), Categories: []programindex.Category{programindex.CategoryCore}},
		{SubjectID: subjectIDBySourceRef(t, base, "storage"), Categories: []programindex.Category{programindex.CategoryDependency}},
		{SubjectID: subjectIDBySourceRef(t, base, "platform-raf"), Categories: []programindex.Category{programindex.CategoryCore}},
		{SubjectID: subjectIDBySourceRef(t, base, "worker-platform-pattern"), Categories: []programindex.Category{programindex.CategoryBackgroundActivity}},
	}
	enriched, err := programindex.Enrich(base, strings.Repeat("c", 64), assignments)
	if err != nil {
		t.Fatalf("programindex.Enrich: %v", err)
	}
	return enriched
}

func requestRefs(request Request) map[string]string {
	result := make(map[string]string)
	for _, subject := range request.Subjects {
		if subject.Kind == subjectObject {
			result["object:"+subject.Name] = subject.Ref
		} else {
			result["pattern:"+subject.Selector] = subject.Ref
		}
	}
	return result
}

func subjectByRef(t *testing.T, request Request, ref string) subjectWire {
	t.Helper()
	for _, subject := range request.Subjects {
		if subject.Ref == ref {
			return subject
		}
	}
	t.Fatalf("request has no subject %q", ref)
	return subjectWire{}
}

func subjectIDBySourceRef(t *testing.T, index programindex.Index, sourceRef string) string {
	t.Helper()
	for _, object := range index.Objects {
		if object.SourceRef == sourceRef {
			return object.ID
		}
	}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.SourceRef == sourceRef {
				return pattern.ID
			}
		}
	}
	t.Fatalf("ProgramIndex has no subject source ref %q", sourceRef)
	return ""
}

func groupByTitle(t *testing.T, index groupindex.Index, title string) groupindex.Group {
	t.Helper()
	for _, group := range index.Groups {
		if group.Title == title {
			return group
		}
	}
	t.Fatalf("GroupsIndex has no group titled %q: %#v", title, index.Groups)
	return groupindex.Group{}
}

func assertDiagnosticKind(t *testing.T, diagnostics []groupindex.Diagnostic, kind string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Kind == kind {
			return
		}
	}
	t.Fatalf("diagnostics have no %q: %#v", kind, diagnostics)
}

func requestHasGlobalSourceArgument(request Request, wantPatternRef string) bool {
	for _, edge := range request.Edges {
		if edge.SourceArgument != nil && edge.SourceArgument.PatternRef == wantPatternRef &&
			edge.SourceArgument.Position == 2 {
			return true
		}
	}
	return false
}

func laneForCategories(categories []programindex.Category) groupindex.Lane {
	for _, category := range categories {
		if category == programindex.CategoryInbound || category == programindex.CategoryBackgroundActivity {
			return groupindex.LaneTriggers
		}
	}
	for _, category := range categories {
		if category == programindex.CategoryCore {
			return groupindex.LaneCore
		}
	}
	return groupindex.LaneDependencies
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
