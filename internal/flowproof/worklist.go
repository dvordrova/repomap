package flowproof

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/evidence"
)

const SessionVersion = 3

type TaskKind string

const (
	TaskInspectRegistration  TaskKind = "inspect_registration"
	TaskExpandCallable       TaskKind = "expand_callable"
	TaskResolveCallsite      TaskKind = "resolve_callsite"
	TaskFindExternalBoundary TaskKind = "find_external_boundary"
	TaskInspectLifecycle     TaskKind = "inspect_lifecycle"
)

type Task struct {
	Key              string   `json:"key"`
	Kind             TaskKind `json:"kind"`
	FlowID           string   `json:"flow_id"`
	Slot             SlotKind `json:"slot"`
	TargetID         string   `json:"target_id"`
	Depth            int      `json:"depth"`
	ScenarioID       string   `json:"scenario_id"`
	CollectorVersion string   `json:"collector_version"`
}

func NewTask(kind TaskKind, flowID string, slot SlotKind, targetID string, depth int, scenarioID, collectorVersion string) Task {
	task := Task{
		Kind:             kind,
		FlowID:           flowID,
		Slot:             slot,
		TargetID:         targetID,
		Depth:            depth,
		ScenarioID:       scenarioID,
		CollectorVersion: collectorVersion,
	}
	task.Key = taskKey(task)
	return task
}

func taskKey(task Task) string {
	identity := strings.Join([]string{
		string(task.Kind), task.FlowID, string(task.Slot), task.TargetID,
		fmt.Sprintf("%d", task.Depth), task.ScenarioID, task.CollectorVersion,
	}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return "task-" + hex.EncodeToString(sum[:8])
}

type Budget struct {
	MaxTasks      int   `json:"max_tasks"`
	MaxDepth      int   `json:"max_depth"`
	MaxSymbols    int   `json:"max_symbols"`
	MaxFiles      int   `json:"max_files"`
	MaxLLMCalls   int   `json:"max_llm_calls"`
	MaxWallMillis int64 `json:"max_wall_millis"`
	MaxNoProgress int   `json:"max_no_progress"`
}

func DefaultBudget() Budget {
	return Budget{
		MaxTasks:      20,
		MaxDepth:      4,
		MaxSymbols:    30,
		MaxFiles:      24,
		MaxLLMCalls:   2,
		MaxWallMillis: 90_000,
		MaxNoProgress: 2,
	}
}

type Stats struct {
	TasksCompleted int      `json:"tasks_completed"`
	Files          []string `json:"files,omitempty"`
	Symbols        []string `json:"symbols,omitempty"`
	LLMCalls       int      `json:"llm_calls"`
	WallMillis     int64    `json:"wall_millis"`
	NoProgress     int      `json:"no_progress"`
}

type StopReason string

const (
	StopComplete           StopReason = "complete"
	StopMaxTasks           StopReason = "max_tasks"
	StopMaxDepth           StopReason = "max_depth"
	StopMaxSymbols         StopReason = "max_symbols"
	StopMaxFiles           StopReason = "max_files"
	StopMaxLLMCalls        StopReason = "max_llm_calls"
	StopMaxWallTime        StopReason = "max_wall_time"
	StopDuplicateTask      StopReason = "duplicate_task"
	StopNoProgress         StopReason = "no_progress"
	StopNoTask             StopReason = "no_task"
	StopCanceled           StopReason = "canceled"
	StopExecutorError      StopReason = "executor_error"
	StopUnsupportedVersion StopReason = "unsupported_version"
)

type Stop struct {
	Reason  StopReason `json:"reason"`
	Message string     `json:"message,omitempty"`
}

type Session struct {
	Version int      `json:"version"`
	Proof   Proof    `json:"proof"`
	Budget  Budget   `json:"budget"`
	Stats   Stats    `json:"stats"`
	Pending []Task   `json:"pending"`
	Seen    []string `json:"seen_task_keys"`
	Stop    *Stop    `json:"stop,omitempty"`
}

// UpgradeSession preserves a bounded v2 proof for read-only replay while
// moving it to the current schema. Exact surface associations are reconciled
// later against the saved surface catalog; this function never invents them.
func UpgradeSession(session Session) (Session, bool) {
	if session.Version == SessionVersion && session.Proof.Version == Version {
		return session, true
	}
	if session.Version != 2 || session.Proof.Version != 2 {
		return Session{}, false
	}
	upgraded := session
	upgraded.Version = SessionVersion
	upgraded.Proof.Version = Version
	upgraded.Proof.TraceEvidenceSurfaceIDs = nil
	return upgraded, true
}

type TransitionUpdate struct {
	TransitionID string                  `json:"transition_id"`
	Resolution   evidence.ResolutionKind `json:"resolution"`
	Certainty    evidence.Certainty      `json:"certainty,omitempty"`
	Target       *Anchor                 `json:"target,omitempty"`
}

type Result struct {
	TaskKey           string             `json:"task_key"`
	NewAnchors        []Anchor           `json:"new_anchors,omitempty"`
	NewTransitions    []Transition       `json:"new_transitions,omitempty"`
	TransitionUpdates []TransitionUpdate `json:"transition_updates,omitempty"`
	Files             []string           `json:"files,omitempty"`
	Symbols           []string           `json:"symbols,omitempty"`
	LLMCalls          int                `json:"llm_calls,omitempty"`
	WallMillis        int64              `json:"wall_millis,omitempty"`
	Warnings          []string           `json:"warnings,omitempty"`
}

func Start(proof Proof, budget Budget, scenarioID, collectorVersion string) Session {
	session := Session{Version: SessionVersion, Proof: proof, Budget: budget}
	if proof.Version != Version {
		session.Stop = &Stop{
			Reason:  StopUnsupportedVersion,
			Message: fmt.Sprintf("unsupported proof version %d; need %d", proof.Version, Version),
		}
		return session
	}
	refreshCoreVerdicts(&session.Proof)
	refreshTraceState(&session.Proof)
	if session.Proof.Satisfied() {
		session.Stop = &Stop{Reason: StopComplete, Message: "all required proof slots are satisfied"}
		return session
	}
	task, ok := PlanNext(session.Proof, 1, scenarioID, collectorVersion)
	if !ok {
		session.Stop = &Stop{Reason: StopNoTask, Message: "unsatisfied proof has no bounded next task"}
		return session
	}
	session.enqueue(task)
	return session
}

func PlanNext(proof Proof, depth int, scenarioID, collectorVersion string) (Task, bool) {
	for _, kind := range cliSlotOrder {
		slot, ok := proof.Slot(kind)
		if !ok {
			slot = Slot{Kind: kind, Status: SlotMissing}
		}
		if slotSatisfied(proof.Archetype, slot) {
			continue
		}
		target := proof.ID
		switch kind {
		case SlotTrigger, SlotEntrypoint, SlotDispatch, SlotApplicationCallable:
			// Command-framework collection is the bounded producer for these
			// slots. The Go type executor cannot strengthen a missing prefix.
			continue
		case SlotCoreOperation, SlotIOBoundary:
			if transitionID := firstUnresolvedTransition(proof, slot.EvidenceIDs); transitionID != "" {
				target = transitionID
				return NewTask(TaskResolveCallsite, proof.ID, kind, target, depth, scenarioID, collectorVersion), true
			}
			// A resolved target is still only identity evidence. No configured
			// bounded executor can prove the architectural role, so preserve the
			// partial slot and continue to independent slots.
			continue
		case SlotConcurrency:
			if _, complete, guarded := concreteConcurrentLifecycle(proof); complete && guarded {
				// The bounded lifecycle is known, but no configured executor can
				// prove that the selected runtime scenario enters its branch.
				continue
			}
			if len(slot.EvidenceIDs) > 0 {
				target = slot.EvidenceIDs[0]
				return NewTask(TaskInspectLifecycle, proof.ID, kind, target, depth, scenarioID, collectorVersion), true
			}
			continue
		case SlotTermination:
			// Handler/process completion is intentionally outside the current
			// lifecycle executor. Missing termination remains an honest boundary.
			continue
		}
	}
	return Task{}, false
}

func firstTypeInferredTransition(proof Proof, ids []string) string {
	for _, id := range ids {
		transition, ok := proof.Transition(id)
		if ok && transition.Resolution == evidence.ResolutionTypeInferred {
			return id
		}
	}
	return ""
}

func firstUnresolvedTransition(proof Proof, ids []string) string {
	for _, id := range ids {
		transition, ok := proof.Transition(id)
		if ok && (transition.Resolution == evidence.ResolutionUnresolved || transition.Resolution == evidence.ResolutionUnknown) {
			return id
		}
	}
	return ""
}

func (s Session) Next() (Task, bool) {
	if s.Stop != nil || len(s.Pending) == 0 {
		return Task{}, false
	}
	return s.Pending[0], true
}

func (s *Session) Apply(result Result) error {
	if s.Version != SessionVersion || s.Proof.Version != Version {
		return fmt.Errorf(
			"flowproof: unsupported session/proof version %d/%d; need %d/%d",
			s.Version, s.Proof.Version, SessionVersion, Version,
		)
	}
	if s.Stop != nil {
		return fmt.Errorf("flowproof: session already stopped: %s", s.Stop.Reason)
	}
	if len(s.Pending) != 1 || result.TaskKey != s.Pending[0].Key {
		return fmt.Errorf("flowproof: result does not match the single pending task")
	}
	completed := s.Pending[0]
	s.Pending = nil
	s.Stats.TasksCompleted++
	s.Stats.Files = sortUnique(append(s.Stats.Files, result.Files...))
	s.Stats.Symbols = sortUnique(append(s.Stats.Symbols, result.Symbols...))
	s.Stats.LLMCalls += result.LLMCalls
	s.Stats.WallMillis += result.WallMillis
	s.Proof.Warnings = append(s.Proof.Warnings, result.Warnings...)

	changes := s.applyNewEvidence(result.NewAnchors, result.NewTransitions)
	changes += s.applyTransitionUpdates(result.TransitionUpdates)
	changes += refreshCoreVerdicts(&s.Proof)
	changes += refreshTraceState(&s.Proof)
	if changes == 0 {
		s.Stats.NoProgress++
	} else {
		s.Stats.NoProgress = 0
	}
	if s.Stats.NoProgress >= s.Budget.MaxNoProgress {
		s.Stop = &Stop{Reason: StopNoProgress, Message: "consecutive tasks added no new evidence"}
		return nil
	}
	if stop := budgetStop(s.Budget, s.Stats); stop != nil {
		s.Stop = stop
		return nil
	}

	nextDepth := completed.Depth
	if completed.Kind == TaskExpandCallable || completed.Kind == TaskInspectRegistration {
		nextDepth++
	}
	if s.Proof.Satisfied() {
		s.Stop = &Stop{Reason: StopComplete, Message: "all required proof slots are satisfied"}
		return nil
	}
	next, ok := PlanNext(s.Proof, nextDepth, completed.ScenarioID, completed.CollectorVersion)
	if !ok {
		s.Stop = &Stop{Reason: StopNoTask, Message: "unsatisfied proof has no bounded next task"}
		return nil
	}
	if next.Depth > s.Budget.MaxDepth {
		s.Stop = &Stop{Reason: StopMaxDepth, Message: "next proof task exceeds depth budget"}
		return nil
	}
	if s.hasSeen(next.Key) {
		s.Stop = &Stop{Reason: StopDuplicateTask, Message: "planner repeated an already scheduled task"}
		return nil
	}
	s.enqueue(next)
	return nil
}

func (s *Session) applyNewEvidence(anchors []Anchor, transitions []Transition) int {
	changes := 0
	for _, anchor := range anchors {
		if anchor.ID == "" {
			continue
		}
		if _, exists := s.Proof.Anchor(anchor.ID); exists {
			continue
		}
		s.Proof.Anchors = append(s.Proof.Anchors, anchor)
		changes++
	}
	for _, transition := range transitions {
		if transition.ID == "" {
			continue
		}
		if _, exists := s.Proof.Transition(transition.ID); exists {
			continue
		}
		s.Proof.Transitions = append(s.Proof.Transitions, transition)
		changes++
	}
	return changes
}

func (s *Session) applyTransitionUpdates(updates []TransitionUpdate) int {
	changes := 0
	for _, update := range updates {
		for index := range s.Proof.Transitions {
			transition := &s.Proof.Transitions[index]
			if transition.ID != update.TransitionID {
				continue
			}
			if update.Resolution != "" && transition.Resolution != update.Resolution {
				transition.Resolution = update.Resolution
				changes++
			}
			if update.Certainty != "" && transition.Certainty != update.Certainty {
				transition.Certainty = update.Certainty
				changes++
			}
			if update.Target != nil {
				if _, exists := s.Proof.Anchor(update.Target.ID); !exists {
					s.Proof.Anchors = append(s.Proof.Anchors, *update.Target)
					changes++
				}
				if transition.To != update.Target.ID {
					transition.To = update.Target.ID
					changes++
				}
			}
			break
		}
	}
	return changes
}

// refreshCoreVerdicts derives final slot status from normalized facts. A
// language adapter may add anchors, relations, and target resolution, but it
// cannot declare an architectural role verified.
func refreshCoreVerdicts(proof *Proof) int {
	changes := refreshArchitecturalRoleVerdicts(proof)
	concurrency, ok := proof.Slot(SlotConcurrency)
	if !ok {
		return changes
	}
	if concurrency.Status == SlotNotApplicable && slotSatisfied(proof.Archetype, concurrency) {
		if !hasConcurrentStart(*proof) {
			return changes
		}
		appendProofWarning(proof, "concurrent lifecycle absence contradicts concrete task-start facts")
	}

	evidenceIDs, complete, guarded := concreteConcurrentLifecycle(*proof)
	status := SlotMissing
	summary := ""
	missing := "goroutine or async lifecycle"
	if complete && !guarded {
		status = SlotVerified
		summary = "task start, body, cancellation target, and join are linked"
		missing = ""
	} else if complete {
		status = SlotPartial
		summary = "task lifecycle is linked on a guarded branch"
		missing = "selected scenario satisfying the branch condition"
	} else if hasConcurrentStart(*proof) {
		status = SlotPartial
		summary = "async task start is visible"
		missing = "task body, cancellation, and join linked to one task"
	}

	for index := range proof.Slots {
		slot := &proof.Slots[index]
		if slot.Kind != SlotConcurrency {
			continue
		}
		if slot.Status == status && slot.Summary == summary && slot.Missing == missing &&
			strings.Join(slot.EvidenceIDs, "\x00") == strings.Join(evidenceIDs, "\x00") &&
			slot.ApplicabilityReason == "" {
			return changes
		}
		*slot = Slot{
			Kind:        SlotConcurrency,
			Status:      status,
			Summary:     summary,
			EvidenceIDs: append([]string{}, evidenceIDs...),
			Missing:     missing,
		}
		return changes + 1
	}
	return changes
}

func refreshTraceState(proof *Proof) int {
	quality := AssessTraceQuality(*proof)
	frontier := proof.CurrentFrontier
	if quality == TraceQualityComplete {
		frontier = ""
	} else if frontier == "" {
		for _, kind := range cliSlotOrder {
			slot, ok := proof.Slot(kind)
			if !ok || slot.Status == SlotVerified || slot.Status == SlotNotApplicable {
				continue
			}
			frontier = strings.TrimSpace(slot.Missing)
			if frontier == "" {
				frontier = strings.TrimSpace(slot.Summary)
			}
			if frontier != "" {
				break
			}
		}
	}
	if proof.TraceQuality == quality && proof.CurrentFrontier == frontier {
		return 0
	}
	proof.TraceQuality = quality
	proof.CurrentFrontier = frontier
	return 1
}

func refreshArchitecturalRoleVerdicts(proof *Proof) int {
	changes := 0
	for index := range proof.Slots {
		slot := &proof.Slots[index]
		if slot.Status != SlotPartial || len(slot.EvidenceIDs) == 0 {
			continue
		}
		var summary, missing string
		switch slot.Kind {
		case SlotCoreOperation:
			summary = "call targets resolved; core-operation role remains unverified"
			missing = "domain-role witness"
		case SlotIOBoundary:
			summary = "internal call targets resolved; external I/O or persistence boundary remains unverified"
			missing = "external resource or persistence-boundary witness"
		default:
			continue
		}
		allResolved := true
		for _, transitionID := range slot.EvidenceIDs {
			transition, ok := proof.Transition(transitionID)
			if !ok || !targetIdentityResolved(transition.Resolution) {
				allResolved = false
				break
			}
		}
		if !allResolved || slot.Summary == summary && slot.Missing == missing {
			continue
		}
		slot.Summary = summary
		slot.Missing = missing
		changes++
	}
	return changes
}

func targetIdentityResolved(resolution evidence.ResolutionKind) bool {
	switch resolution {
	case evidence.ResolutionStatic,
		evidence.ResolutionFrameworkRule,
		evidence.ResolutionRuntimeObserved:
		return true
	default:
		return false
	}
}

func concreteConcurrentLifecycle(proof Proof) ([]string, bool, bool) {
	for _, task := range proof.Anchors {
		if task.Kind != AnchorTask {
			continue
		}
		var startID, bodyID, joinID string
		cancelByTarget := make(map[string]string)
		for _, transition := range proof.Transitions {
			if !supportedLifecycleTransition(transition) {
				continue
			}
			switch {
			case transition.Relation == evidence.RelationStartsGoroutine && transition.To == task.ID:
				startID = transition.ID
			case transition.Relation == evidence.RelationCallback && transition.From == task.ID &&
				transition.Invocation == evidence.InvocationGoroutine:
				bodyID = transition.ID
			case transition.Relation == evidence.RelationCancels && transition.From != task.ID:
				cancelByTarget[transition.To] = transition.ID
			case transition.Relation == evidence.RelationJoins && transition.To == task.ID:
				joinID = transition.ID
			}
		}
		for _, transition := range proof.Transitions {
			if transition.Relation != evidence.RelationUsesCancellation || transition.From != task.ID ||
				!supportedLifecycleTransition(transition) {
				continue
			}
			cancelID := cancelByTarget[transition.To]
			if startID != "" && bodyID != "" && cancelID != "" && joinID != "" {
				ids := []string{startID, bodyID, cancelID, transition.ID, joinID}
				return ids, true, lifecycleIsGuarded(proof, ids)
			}
		}
	}
	return concurrentStartEvidence(proof), false, false
}

func lifecycleIsGuarded(proof Proof, transitionIDs []string) bool {
	for _, transitionID := range transitionIDs {
		transition, ok := proof.Transition(transitionID)
		if ok && transition.Condition != nil {
			return true
		}
	}
	return false
}

func supportedLifecycleTransition(transition Transition) bool {
	if transition.Certainty != evidence.CertaintyStatic &&
		transition.Certainty != evidence.CertaintyObserved &&
		transition.Certainty != evidence.CertaintyVerified {
		return false
	}
	return transition.Resolution != evidence.ResolutionUnknown &&
		transition.Resolution != evidence.ResolutionUnresolved &&
		transition.Resolution != evidence.ResolutionModelSuggested
}

func hasConcurrentStart(proof Proof) bool {
	return len(concurrentStartEvidence(proof)) > 0
}

func concurrentStartEvidence(proof Proof) []string {
	var ids []string
	for _, transition := range proof.Transitions {
		if transition.Relation == evidence.RelationStartsGoroutine ||
			transition.Invocation == evidence.InvocationGoroutine {
			ids = append(ids, transition.ID)
		}
	}
	return ids
}

func appendProofWarning(proof *Proof, warning string) {
	for _, existing := range proof.Warnings {
		if existing == warning {
			return
		}
	}
	proof.Warnings = append(proof.Warnings, warning)
}

func budgetStop(budget Budget, stats Stats) *Stop {
	switch {
	case stats.TasksCompleted >= budget.MaxTasks:
		return &Stop{Reason: StopMaxTasks, Message: "task budget exhausted"}
	case len(stats.Symbols) >= budget.MaxSymbols:
		return &Stop{Reason: StopMaxSymbols, Message: "symbol budget exhausted"}
	case len(stats.Files) >= budget.MaxFiles:
		return &Stop{Reason: StopMaxFiles, Message: "file budget exhausted"}
	case stats.LLMCalls > budget.MaxLLMCalls:
		return &Stop{Reason: StopMaxLLMCalls, Message: "model-call budget exhausted"}
	case stats.WallMillis >= budget.MaxWallMillis:
		return &Stop{Reason: StopMaxWallTime, Message: "wall-time budget exhausted"}
	default:
		return nil
	}
}

var ErrUnsupportedTask = errors.New("flowproof: unsupported task")

// Executor is defined at the worklist consumer. Language adapters implement
// only the task kinds they can prove; an unsupported next task stops the
// bounded run instead of falling through to an autonomous agent loop.
type Executor interface {
	Execute(ctx context.Context, repoPath string, proof Proof, task Task) (Result, error)
}

func Run(ctx context.Context, repoPath string, session *Session, executor Executor) error {
	if session == nil {
		return fmt.Errorf("flowproof: nil session")
	}
	if session.Version != SessionVersion || session.Proof.Version != Version {
		session.Stop = &Stop{
			Reason: StopUnsupportedVersion,
			Message: fmt.Sprintf(
				"unsupported session/proof version %d/%d; need %d/%d",
				session.Version, session.Proof.Version, SessionVersion, Version,
			),
		}
		return nil
	}
	if executor == nil {
		session.Stop = &Stop{Reason: StopNoTask, Message: "no proof task executor is configured"}
		return nil
	}
	runCtx := ctx
	cancel := func() {}
	if session.Budget.MaxWallMillis > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(session.Budget.MaxWallMillis)*time.Millisecond)
	}
	defer cancel()

	for {
		task, ok := session.Next()
		if !ok {
			return nil
		}
		if err := runCtx.Err(); err != nil {
			session.Stop = &Stop{Reason: StopCanceled, Message: err.Error()}
			return nil
		}
		started := time.Now()
		result, err := executor.Execute(runCtx, repoPath, session.Proof, task)
		result.WallMillis += time.Since(started).Milliseconds()
		if errors.Is(err, ErrUnsupportedTask) {
			session.Stop = &Stop{Reason: StopNoTask, Message: "next proof task has no configured deterministic executor"}
			return nil
		}
		if err != nil {
			session.Stop = &Stop{Reason: StopExecutorError, Message: err.Error()}
			return err
		}
		if result.TaskKey == "" {
			result.TaskKey = task.Key
		}
		if err := session.Apply(result); err != nil {
			session.Stop = &Stop{Reason: StopExecutorError, Message: err.Error()}
			return err
		}
	}
}

func (s *Session) enqueue(task Task) {
	s.Pending = []Task{task}
	s.Seen = append(s.Seen, task.Key)
}

func (s Session) hasSeen(key string) bool {
	for _, seen := range s.Seen {
		if seen == key {
			return true
		}
	}
	return false
}
