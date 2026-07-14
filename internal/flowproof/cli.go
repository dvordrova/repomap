package flowproof

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
)

const CLICollectorVersion = "command-trace-v2"

type CLIStep struct {
	Symbol           string
	Relation         string
	CallsiteLocation *evidence.Location
	TargetLocation   evidence.Location
}

type CLICall struct {
	Symbol     string
	Path       string
	Line       int
	Relation   string
	Condition  *evidence.Condition
	Resolved   bool
	TargetPath string
	TargetLine int
}

// ConcurrentLifecyclePresence is a normalized bounded fact about the selected
// handler scope. Adapters report presence; the proof core decides whether that
// fact makes an optional slot not applicable.
type ConcurrentLifecyclePresence string

const (
	ConcurrentLifecycleUnknown ConcurrentLifecyclePresence = ""
	ConcurrentLifecyclePresent ConcurrentLifecyclePresence = "present"
	ConcurrentLifecycleAbsent  ConcurrentLifecyclePresence = "absent"
)

type ConcurrentLifecycleFact struct {
	Presence   ConcurrentLifecyclePresence
	Provenance []evidence.Provenance
}

type CLISeed struct {
	FlowID              string
	Goal                string
	Command             string
	SeedSurfaceID       string
	Framework           string
	CollectorVersion    string
	ScenarioID          string
	Steps               []CLIStep
	Calls               []CLICall
	ConcurrentLifecycle ConcurrentLifecycleFact
}

// BuildCLI creates the initial proof from bounded command-framework evidence.
// It does not claim that source-order callsites form a runtime sequence.
func BuildCLI(seed CLISeed) Proof {
	proof := Proof{
		Version:       Version,
		ID:            seed.FlowID,
		Archetype:     ArchetypeCLI,
		Goal:          seed.Goal,
		Command:       seed.Command,
		Slots:         newCLISlots(),
		SeedSurfaceID: seed.SeedSurfaceID,
	}
	provider := seed.Framework
	if provider == "" {
		provider = "command_framework"
	}

	stepAnchors := make([]string, 0, len(seed.Steps))
	for index, step := range seed.Steps {
		anchor := Anchor{
			ID:       anchorID("step", index, step.Symbol, step.TargetLocation.Path, step.TargetLocation.Line),
			Kind:     AnchorFunction,
			Label:    step.Symbol,
			Location: location(step.TargetLocation.Path, step.TargetLocation.Line),
		}
		proof.Anchors = append(proof.Anchors, anchor)
		stepAnchors = append(stepAnchors, anchor.ID)
		if index == 0 {
			continue
		}
		resolution := evidence.ResolutionStatic
		if step.Relation == string(evidence.RelationRegisters) || step.Relation == string(evidence.RelationCallback) {
			resolution = evidence.ResolutionFrameworkRule
		}
		callsite := stepCallsite(step)
		proof.Transitions = append(proof.Transitions, Transition{
			ID:         transitionID("dispatch", index, callsite.Path, callsite.Line, step.Symbol),
			From:       stepAnchors[index-1],
			To:         anchor.ID,
			Relation:   relationKind(step.Relation),
			Resolution: resolution,
			Invocation: invocationMode(step.Relation),
			Certainty:  evidence.CertaintyStatic,
			Evidence:   callsite,
			Provider:   provider,
		})
	}

	handlerID := ""
	if len(stepAnchors) > 0 {
		handlerID = stepAnchors[len(stepAnchors)-1]
	}
	callTransitionIDs := make([]string, 0, len(seed.Calls))
	for index, call := range seed.Calls {
		anchor := Anchor{
			ID:       anchorID("call", index, call.Symbol, call.Path, call.Line),
			Kind:     AnchorCallsite,
			Label:    call.Symbol,
			Location: location(call.Path, call.Line),
		}
		if call.Resolved && call.TargetPath != "" && call.TargetLine > 0 {
			anchor.Kind = AnchorFunction
			anchor.Location = location(call.TargetPath, call.TargetLine)
		}
		proof.Anchors = append(proof.Anchors, anchor)
		resolution := evidence.ResolutionUnresolved
		if call.Resolved {
			resolution = evidence.ResolutionStatic
		}
		transition := Transition{
			ID:         transitionID("handler", index, call.Path, call.Line, call.Symbol),
			From:       handlerID,
			To:         anchor.ID,
			Relation:   relationKind(call.Relation),
			Resolution: resolution,
			Invocation: invocationMode(call.Relation),
			Condition:  cloneCondition(call.Condition),
			Certainty:  evidence.CertaintyStatic,
			Evidence:   evidence.Location{Path: call.Path, Line: call.Line},
			Provider:   "go_syntax",
		}
		proof.Transitions = append(proof.Transitions, transition)
		callTransitionIDs = append(callTransitionIDs, transition.ID)
	}

	fillCLISlots(&proof, seed, stepAnchors, callTransitionIDs)
	applyConcurrentLifecyclePresence(&proof, seed.ConcurrentLifecycle)
	refreshCoreVerdicts(&proof)
	refreshTraceState(&proof)
	return proof
}

func cloneCondition(condition *evidence.Condition) *evidence.Condition {
	if condition == nil {
		return nil
	}
	copy := *condition
	return &copy
}

func newCLISlots() []Slot {
	slots := make([]Slot, 0, len(cliSlotOrder))
	for _, kind := range cliSlotOrder {
		slots = append(slots, Slot{Kind: kind, Status: SlotMissing})
	}
	return slots
}

func fillCLISlots(proof *Proof, seed CLISeed, stepAnchors, callTransitionIDs []string) {
	if seed.Command != "" && len(stepAnchors) >= 3 && hasStepCallsite(seed.Steps, 2) {
		setSlot(proof, SlotTrigger, SlotVerified, "registered command "+seed.Command, []string{stepAnchors[2]}, "")
	} else {
		setSlot(proof, SlotTrigger, SlotMissing, "", nil, "registered command trigger with exact callsite")
	}
	if len(stepAnchors) >= 1 {
		setSlot(proof, SlotEntrypoint, SlotVerified, "exact process entrypoint", []string{stepAnchors[0]}, "")
	} else {
		setSlot(proof, SlotEntrypoint, SlotMissing, "", nil, "exact process entrypoint")
	}
	if len(stepAnchors) >= 3 && hasStepCallsite(seed.Steps, 1) && hasStepCallsite(seed.Steps, 2) {
		setSlot(proof, SlotDispatch, SlotVerified, "root command and subcommand registration", stepAnchors[1:3], "")
	} else {
		setSlot(proof, SlotDispatch, SlotMissing, "", append([]string{}, stepAnchors...), "root command to subcommand registration callsites")
	}
	if len(stepAnchors) >= 4 && hasStepCallsite(seed.Steps, 3) {
		setSlot(proof, SlotApplicationCallable, SlotVerified, "Run/RunE callback", []string{stepAnchors[3]}, "")
	} else {
		setSlot(proof, SlotApplicationCallable, SlotMissing, "", nil, "Run/RunE application callback callsite")
	}

	var coreIDs, ioIDs, concurrencyIDs []string
	for _, transitionID := range callTransitionIDs {
		transition, _ := proof.Transition(transitionID)
		anchor, _ := proof.Anchor(transition.To)
		if isCoreCall(anchor.Label) {
			coreIDs = append(coreIDs, transitionID)
		}
		if isIOCall(anchor.Label) {
			ioIDs = append(ioIDs, transitionID)
		}
		if transition.Relation == evidence.RelationStartsGoroutine || transition.Invocation == evidence.InvocationGoroutine {
			concurrencyIDs = append(concurrencyIDs, transitionID)
		}
	}
	coreIDs = selectCoreEvidence(proof, coreIDs, 6)
	if len(ioIDs) > 2 {
		ioIDs = ioIDs[:2]
	}
	if len(coreIDs) == 0 {
		setSlot(proof, SlotCoreOperation, SlotMissing, "", nil, "first domain-level operation")
	} else {
		setSlot(proof, SlotCoreOperation, SlotPartial, "candidate core-operation callsites", coreIDs, "target identity and domain-role witness")
	}
	if len(ioIDs) == 0 {
		setSlot(proof, SlotIOBoundary, SlotMissing, "", nil, "repository, network, or storage boundary")
	} else {
		setSlot(proof, SlotIOBoundary, SlotPartial, "candidate I/O-boundary callsites", ioIDs, "target identity and external resource or persistence-boundary witness")
	}
	if len(concurrencyIDs) == 0 {
		setSlot(proof, SlotConcurrency, SlotMissing, "", nil, "goroutine or async lifecycle")
	} else {
		setSlot(proof, SlotConcurrency, SlotPartial, "goroutine start is visible", concurrencyIDs, "join, cancellation, and ownership")
	}
	setSlot(proof, SlotTermination, SlotMissing, "", nil, "return, shutdown, or completion path")
}

func stepCallsite(step CLIStep) evidence.Location {
	if step.CallsiteLocation == nil || step.CallsiteLocation.Path == "" || step.CallsiteLocation.Line <= 0 {
		return evidence.Location{}
	}
	return *step.CallsiteLocation
}

func hasStepCallsite(steps []CLIStep, index int) bool {
	return index >= 0 && index < len(steps) &&
		steps[index].CallsiteLocation != nil &&
		steps[index].CallsiteLocation.Path != "" && steps[index].CallsiteLocation.Line > 0
}

func setSlot(proof *Proof, kind SlotKind, status SlotStatus, summary string, evidenceIDs []string, missing string) {
	for index := range proof.Slots {
		if proof.Slots[index].Kind != kind {
			continue
		}
		proof.Slots[index].Status = status
		proof.Slots[index].Summary = summary
		proof.Slots[index].EvidenceIDs = append([]string{}, evidenceIDs...)
		proof.Slots[index].Provenance = nil
		proof.Slots[index].Missing = missing
		proof.Slots[index].ApplicabilityReason = ""
		return
	}
}

func applyConcurrentLifecyclePresence(proof *Proof, fact ConcurrentLifecycleFact) {
	if fact.Presence == ConcurrentLifecycleUnknown || fact.Presence == ConcurrentLifecyclePresent {
		return
	}
	if fact.Presence != ConcurrentLifecycleAbsent {
		proof.Warnings = append(proof.Warnings, fmt.Sprintf(
			"invalid concurrent lifecycle presence %q", fact.Presence,
		))
		return
	}
	if !validApplicabilityProvenance(fact.Provenance) {
		proof.Warnings = append(proof.Warnings, "concurrent lifecycle absence has no valid provenance")
		return
	}
	if hasConcurrentStart(*proof) {
		appendProofWarning(proof, "concurrent lifecycle absence contradicts concrete task-start facts")
		return
	}
	for index := range proof.Slots {
		if proof.Slots[index].Kind != SlotConcurrency {
			continue
		}
		proof.Slots[index] = Slot{
			Kind:                SlotConcurrency,
			Status:              SlotNotApplicable,
			Summary:             "not applicable to the selected flow scope",
			Provenance:          append([]evidence.Provenance(nil), fact.Provenance...),
			ApplicabilityReason: ApplicabilityNoConcurrentLifecycleInScope,
		}
		return
	}
}

func relationKind(value string) evidence.RelationKind {
	switch value {
	case string(evidence.RelationRegisters):
		return evidence.RelationRegisters
	case string(evidence.RelationCallback):
		return evidence.RelationCallback
	case string(evidence.RelationConstructs):
		return evidence.RelationConstructs
	case string(evidence.RelationStartsGoroutine):
		return evidence.RelationStartsGoroutine
	case string(evidence.RelationDispatches):
		return evidence.RelationDispatches
	default:
		return evidence.RelationCalls
	}
}

func invocationMode(relation string) evidence.InvocationMode {
	switch relation {
	case string(evidence.RelationCallback):
		return evidence.InvocationCallback
	case string(evidence.RelationStartsGoroutine):
		return evidence.InvocationGoroutine
	default:
		return evidence.InvocationSynchronous
	}
}

func isCoreCall(symbol string) bool {
	lower := strings.ToLower(symbol)
	for _, noise := range []string{"progress", "printer", "terminal", "logger", "format", "deletesnapshot"} {
		if strings.Contains(lower, noise) {
			return false
		}
	}
	for _, term := range []string{
		"snapshot", "archiver.", "archive", "repository", "repo.",
		"loadindex", "openwith", "newscanner", ".scan",
	} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func selectCoreEvidence(proof *Proof, ids []string, limit int) []string {
	if len(ids) <= limit {
		return ids
	}
	type candidate struct {
		id    string
		score int
	}
	candidates := make([]candidate, 0, len(ids))
	for _, id := range ids {
		transition, _ := proof.Transition(id)
		anchor, _ := proof.Anchor(transition.To)
		candidates = append(candidates, candidate{id: id, score: coreCallScore(anchor.Label)})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	selected := make(map[string]struct{}, limit)
	for _, candidate := range candidates[:limit] {
		selected[candidate.id] = struct{}{}
	}
	result := make([]string, 0, limit)
	for _, id := range ids {
		if _, ok := selected[id]; ok {
			result = append(result, id)
		}
	}
	return result
}

func coreCallScore(symbol string) int {
	lower := strings.ToLower(symbol)
	switch {
	case strings.HasSuffix(lower, ".snapshot"):
		return 100
	case lower == "archiver.new":
		return 95
	case strings.Contains(lower, "newscanner"):
		return 90
	case strings.HasSuffix(lower, ".scan"):
		return 85
	case strings.Contains(lower, "loadindex"):
		return 80
	case strings.Contains(lower, "openwith"):
		return 75
	case strings.Contains(lower, "snapshot"):
		return 50
	default:
		return 10
	}
}

func isIOCall(symbol string) bool {
	lower := strings.ToLower(symbol)
	for _, term := range []string{"open", "load", "save", "read", "write", "repo.", "repository", "cache", "index"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

var nonID = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func anchorID(prefix string, index int, symbol, path string, line int) string {
	return fmt.Sprintf("%s-%02d-%s-%d", prefix, index+1, slug(filepath.Base(path)+"-"+symbol), line)
}

func transitionID(prefix string, index int, path string, line int, symbol string) string {
	return fmt.Sprintf("%s-%02d-%s-%d", prefix, index+1, slug(filepath.Base(path)+"-"+symbol), line)
}

func slug(value string) string {
	value = strings.ToLower(nonID.ReplaceAllString(value, "-"))
	return strings.Trim(value, "-")
}

func location(path string, line int) *evidence.Location {
	if path == "" || line <= 0 {
		return nil
	}
	return &evidence.Location{Path: path, Line: line}
}

func sortUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
