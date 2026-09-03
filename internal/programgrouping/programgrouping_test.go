package programgrouping

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
	requests            []groupingRequest
	prompts             []llm.Prompt
	maxInitialGroupRefs int
	respond             func(groupingRequest) []byte
}

// groupingRequest is whichever phase the fake provider was handed. The two
// phases share a wire envelope and differ in what they carry: a grouping
// request carries subjects to select from, a consolidation request carries
// only candidate descriptions.
type groupingRequest struct {
	Request
	Candidates []consolidateCandidate `json:"candidates"`
}

func (provider *presetProvider) State() []byte {
	return []byte(`{"provider":"program-grouping-preset-v1"}`)
}

func (provider *presetProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var request groupingRequest
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
	var request groupingRequest
	if err := json.Unmarshal([]byte(envelope.User), &request); err != nil {
		return llm.Completion{}, err
	}
	if request.Phase != phaseConsolidate {
		if err := validatePresetRequest(request.Request); err != nil {
			return llm.Completion{}, err
		}
	} else if err := validatePresetConsolidation(request); err != nil {
		return llm.Completion{}, err
	}
	provider.mu.Lock()
	provider.requests = append(provider.requests, request)
	provider.prompts = append(provider.prompts, llm.Prompt{
		System: envelope.System, User: envelope.User, ResponseFormatJSON: true,
	})
	provider.mu.Unlock()
	response := []byte(`{"assign":[],"links":[]}`)
	if provider.respond != nil {
		response = provider.respond(request)
	}
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, Latency: time.Millisecond},
	}, nil
}

// validatePresetConsolidation pins the one property that makes consolidation
// safe: the request describes candidates and never carries a member ref, so
// no answer to it can select or drop a member.
func validatePresetConsolidation(request groupingRequest) error {
	if request.Version != requestVersion || len(request.Candidates) == 0 {
		return fmt.Errorf("preset: incomplete consolidation request")
	}
	if len(request.GroupRefs) != 0 || len(request.Subjects) != 0 ||
		len(request.CandidateGroups) != 0 {
		return fmt.Errorf("preset: consolidation request carries member selection")
	}
	seen := make(map[string]struct{}, len(request.Candidates))
	for _, candidate := range request.Candidates {
		if candidate.Ref == "" || candidate.Title == "" || !candidate.Lane.Valid() {
			return fmt.Errorf("preset: incomplete consolidation candidate %#v", candidate)
		}
		if _, duplicate := seen[candidate.Ref]; duplicate {
			return fmt.Errorf("preset: duplicate consolidation candidate %s", candidate.Ref)
		}
		seen[candidate.Ref] = struct{}{}
	}
	return nil
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

func TestRunPartitionsSelectableRefsAndKeepsOpenLinks(t *testing.T) {
	index := groupingTestIndex(t, "go")
	provider := &presetProvider{}
	provider.respond = func(request groupingRequest) []byte {
		refs := requestRefs(request.Request)
		if request.Phase == phaseConsolidate {
			assign := make([]consolidateAssign, 0, len(request.Candidates))
			for _, candidate := range request.Candidates {
				assign = append(assign, consolidateAssign{Ref: candidate.Ref, Cluster: candidate.Title})
			}
			wire, err := json.Marshal(consolidateResponse{Assign: assign})
			if err != nil {
				t.Fatal(err)
			}
			return wire
		}
		// One label over two lanes, one ref repeated, one invented, one that
		// is context and not selectable, and a link to a label nothing was
		// assigned to.
		return []byte(fmt.Sprintf(`{
  "assign": [
    {"ref":%q,"group":"Order work"},
    {"ref":%q,"group":"Order work"},
    {"ref":%q,"group":"Order core"},
    {"ref":%q,"group":"Order core"},
    {"ref":"s9999","group":"Invented"},
    {"ref":%q,"group":"Context"}
  ],
  "links": [
    {"from":"Order work","to":"Order core","label":"dispatches orders"},
    {"from":"Order work","to":"Nowhere","label":"broken"},
    {"from":"Order work","to":"Order work","label":"itself"}
  ]
}`,
			refs["pattern:HandleFunc"], refs["object:runWorker"],
			refs["object:processOrder"], refs["object:processOrder"],
			refs["object:normalize"],
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

	// Every placed ref appears in exactly one group, and no group holds a ref
	// the request never offered.
	seen := make(map[string]int)
	for _, group := range grouped.Groups {
		for _, member := range group.MemberSubjectIDs {
			seen[member]++
		}
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("subject %s is in %d groups; grouping is a partition", id, count)
		}
	}
	context := subjectIDBySourceRef(t, index, "helper")
	if _, placed := seen[context]; placed {
		t.Fatalf("an unselectable context subject was placed: %#v", grouped.Groups)
	}
	if len(grouped.Connections) != 1 || grouped.Connections[0].Label != "dispatches orders" {
		t.Fatalf("links to an unassigned label or to itself survived: %#v", grouped.Connections)
	}
	assertDiagnosticKind(t, diagnostics, diagnosticUnselectableMember)
	assertDiagnosticKind(t, diagnostics, diagnosticUnknownMemberRef)
	assertDiagnosticKind(t, diagnostics, diagnosticUnknownGroupKey)

	provider.mu.Lock()
	requests := append([]groupingRequest(nil), provider.requests...)
	prompts := append([]llm.Prompt(nil), provider.prompts...)
	provider.mu.Unlock()
	// One shard, then the consolidation that runs over every shard result
	// however few there are: it cannot lose a member, and skipping it left a
	// small target with one raw draw of a stage whose draws vary widely.
	if len(requests) != 2 || requests[0].Phase != phaseGrouping ||
		requests[1].Phase != phaseConsolidate {
		t.Fatalf("provider requests = %#v", requests)
	}
	refs := requestRefs(requests[0].Request)
	helper := subjectByRef(t, requests[0].Request, refs["object:normalize"])
	if len(helper.Categories) != 0 {
		t.Fatalf("unclassified context categories = %#v", helper.Categories)
	}
	if !requestHasGlobalSourceArgument(requests[0].Request, refs["pattern:HandleFunc"]) {
		t.Fatalf("cross-relation source argument was not restored through its owning pattern: %#v", requests[0].Edges)
	}
	registration := subjectByRef(t, requests[0].Request, refs["pattern:HandleFunc"])
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
	// The whole point of the cube: the model is asked for membership and
	// nothing it could invent instead.
	if !strings.Contains(prompts[0].System, "Do not return lanes, member lists, evidence") ||
		!strings.Contains(prompts[0].System, "is a partition, not a sample") {
		t.Fatalf("prompt lost the assignment-only contract")
	}
}

func TestNormalizeResponseKeepsValidAssignmentsAndRejectsTheRest(t *testing.T) {
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
	contextRef := refs["object:normalize"]
	if processRef == "" || workerRef == "" || contextRef == "" {
		t.Fatalf("fixture request refs = %#v", refs)
	}

	// processOrder is core and runWorker is background activity, so one label
	// over both becomes one group per lane and nobody moves. normalize is not
	// categorized, so it is context and cannot be assigned at all.
	raw, err := json.Marshal(struct {
		Assign []responseAssign `json:"assign"`
		Links  []responseLink   `json:"links"`
	}{
		Assign: []responseAssign{
			{Ref: processRef, Group: "Order work"},
			{Ref: workerRef, Group: "Order work"},
			{Ref: processRef, Group: "Second home"},
			{Ref: contextRef, Group: "Context"},
			{Ref: "s9999", Group: "Invented"},
		},
		Links: []responseLink{
			{From: "Order work", To: "Nothing", Label: "goes nowhere"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	normalized, err := normalizeResponse(raw, compilation, request)
	if err != nil {
		t.Fatal(err)
	}
	byLane := make(map[groupindex.Lane]groupProposal, len(normalized.groups))
	for _, group := range normalized.groups {
		if group.Title == "Order work" {
			byLane[group.Lane] = group
		}
	}
	if len(byLane) != 2 {
		t.Fatalf("one label over two lanes did not become two groups: %#v", normalized.groups)
	}
	if !reflect.DeepEqual(byLane[groupindex.LaneCore].MemberSubjectIDs,
		[]string{compilation.subjectByRef[processRef].id}) {
		t.Fatalf("core group members = %#v", byLane[groupindex.LaneCore])
	}
	if !reflect.DeepEqual(byLane[groupindex.LaneTriggers].MemberSubjectIDs,
		[]string{compilation.subjectByRef[workerRef].id}) {
		t.Fatalf("triggers group members = %#v", byLane[groupindex.LaneTriggers])
	}
	for _, group := range normalized.groups {
		if group.Title == "Context" || group.Title == "Invented" || group.Title == "Second home" {
			t.Fatalf("an unselectable, invented or repeated ref was placed: %#v", group)
		}
	}
	if len(normalized.connections) != 0 {
		t.Fatalf("a link naming an unassigned label survived: %#v", normalized.connections)
	}
	assertDiagnosticKind(t, normalized.diagnostics, diagnosticUnselectableMember)
	assertDiagnosticKind(t, normalized.diagnostics, diagnosticUnknownMemberRef)
	assertDiagnosticKind(t, normalized.diagnostics, diagnosticUnknownGroupKey)
}

func TestPromptMakesEveryMembershipLaneCompatibleAndConsolidationLossless(t *testing.T) {
	for _, required := range []string{
		"grouping` is a partition, not a sample",
		"`inbound` or `background_activity` refs make a",
		"You never write a lane",
		"Do not return lanes, member lists, evidence",
		"Read it as a graph and not as a list of names",
		"Answer with one line per candidate and nothing else",
		"appears exactly once, none is\nleft out and none is repeated",
		"Do not return titles, summaries, lanes,\nmembers, connections or prose",
		"Candidates in one group must share a `lane`",
	} {
		if !strings.Contains(promptText, required) {
			t.Fatalf("grouping prompt lost consolidation rule %q", required)
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
		"conflicting envelope key": []byte(`{"groups":[],"groups":[{"key":"x"}],"connections":[]}`),
		"case folded envelope key": []byte(`{"Groups":[],"connections":[]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeResponse(raw, compilation, request); err == nil {
				t.Fatal("closed envelope was accepted")
			}
		})
	}

	// A shard whose groups say nothing to each other returns no links at all.
	// Refusing that answer failed the whole target on a live run.
	t.Run("links may be absent", func(t *testing.T) {
		raw := []byte(fmt.Sprintf(`{"assign":[{"ref":%q,"group":"Core"}]}`, processRef))
		normalized, normalizeErr := normalizeResponse(raw, compilation, request)
		if normalizeErr != nil {
			t.Fatalf("an answer with no links was refused: %v", normalizeErr)
		}
		if len(normalized.groups) != 1 || len(normalized.connections) != 0 {
			t.Fatalf("answer with no links = %#v", normalized)
		}
	})

	// A key repeated verbatim is one answer said twice. Refusing it cost every
	// group of one real target, and with them every connection that named one.
	t.Run("repeated identical keys are one answer", func(t *testing.T) {
		raw := []byte(fmt.Sprintf(
			`{"assign":[{"ref":%q,"group":"Core","group":"Core"}],"links":[]}`,
			processRef,
		))
		normalized, normalizeErr := normalizeResponse(raw, compilation, request)
		if normalizeErr != nil {
			t.Fatalf("repeated identical key became an envelope error: %v", normalizeErr)
		}
		if len(normalized.groups) != 1 || len(normalized.diagnostics) != 0 {
			t.Fatalf("repeated identical key discarded the group: %#v", normalized)
		}
	})

	for name, raw := range map[string][]byte{
		"duplicate assign key":   []byte(fmt.Sprintf(`{"assign":[{"ref":%q,"ref":"other","group":"Core"}],"links":[]}`, processRef)),
		"case folded assign key": []byte(fmt.Sprintf(`{"assign":[{"Ref":%q,"group":"Core"}],"links":[]}`, processRef)),
		"duplicate link key":     []byte(fmt.Sprintf(`{"assign":[{"ref":%q,"group":"Core"},{"ref":%q,"group":"Trigger"}],"links":[{"from":"Trigger","from":"Core","to":"Core","label":"dispatches"}]}`, processRef, workerRef)),
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
			requests := append([]groupingRequest(nil), provider.requests...)
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

func TestRunExhaustivelyBatchesAndConvergentlyConsolidates(t *testing.T) {
	index := groupingTestIndex(t, "python")
	provider := &presetProvider{maxInitialGroupRefs: 1}
	provider.respond = func(request groupingRequest) []byte {
		if request.Phase == phaseGrouping {
			ref := request.GroupRefs[0]
			subject := subjectByRef(t, request.Request, ref)
			lane := laneForCategories(subject.Categories)
			_ = lane
			return []byte(fmt.Sprintf(`{"assign":[{"ref":%q,"group":%q}],"links":[]}`,
				ref, "Group "+ref))
		}
		// Consolidation answers one label per candidate; here every candidate
		// of one lane gets that lane as its label.
		assign := make([]consolidateAssign, 0, len(request.Candidates))
		for _, candidate := range request.Candidates {
			assign = append(assign, consolidateAssign{
				Ref: candidate.Ref, Cluster: strings.ToUpper(string(candidate.Lane)),
			})
		}
		wire, err := json.Marshal(consolidateResponse{Assign: assign})
		if err != nil {
			t.Fatalf("marshal consolidation response: %v", err)
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
	// Consolidation joins what the shards proposed and invents nothing. These
	// shards owned one subject each and so proposed no connection; a phase
	// that returned one here would be making it up.
	if len(grouped.Groups) != 3 || len(grouped.Connections) != 0 {
		t.Fatalf("consolidated GroupsIndex = groups %#v connections %#v", grouped.Groups, grouped.Connections)
	}
	// The model assigns labels, not names: a consolidated group carries the
	// words of the largest candidate it absorbed, so this asserts the join by
	// lane and membership rather than by a title the model never wrote.
	var trigger *groupindex.Group
	for position := range grouped.Groups {
		if grouped.Groups[position].Lane == groupindex.LaneTriggers {
			trigger = &grouped.Groups[position]
		}
	}
	if trigger == nil || len(trigger.MemberSubjectIDs) != 3 {
		t.Fatalf("inbound + background activity were not merged into triggers: %#v", grouped.Groups)
	}

	provider.mu.Lock()
	requests := append([]groupingRequest(nil), provider.requests...)
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
			assertCompleteIncidentEdges(t, compilation, request.Request)
			seen[request.GroupRefs[0]]++
		case phaseConsolidate:
			mergeCalls++
			if len(request.Candidates) < 2 {
				t.Fatalf("consolidation did not provide a cross-shard meeting point: %#v", request.Candidates)
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
