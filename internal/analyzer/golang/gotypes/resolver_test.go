package gotypes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
)

// This is a replaceable adapter contract test: it constrains the observable
// exact-callsite result, not go/packages traversal or internal AST choices.
func TestExecutorResolvesConcreteMethodAtExactCallsite(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "go.mod", "module example.test/resolver\n\ngo 1.24\n")
	writeFile(t, repo, "main.go", `package main

type Repository struct{}

func (*Repository) LoadIndex() error { return nil }

func run(repo *Repository) error {
	return repo.LoadIndex()
}

func main() {}
`)
	proof := flowproof.Proof{
		Version:   flowproof.Version,
		ID:        "fixture",
		Archetype: flowproof.ArchetypeCLI,
		Anchors: []flowproof.Anchor{{
			ID: "callsite", Kind: flowproof.AnchorCallsite, Label: "repo.LoadIndex",
		}},
		Transitions: []flowproof.Transition{{
			ID: "call", From: "handler", To: "callsite", Relation: evidence.RelationCalls,
			Resolution: evidence.ResolutionUnresolved, Certainty: evidence.CertaintyStatic,
			Evidence: evidence.Location{Path: "main.go", Line: 8},
		}},
	}
	task := flowproof.NewTask(flowproof.TaskResolveCallsite, proof.ID, flowproof.SlotCoreOperation, "call", 1, "fixture", "fixture-v1")

	result, err := NewExecutor().Execute(context.Background(), repo, proof, task)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TransitionUpdates) != 1 {
		t.Fatalf("updates = %#v", result.TransitionUpdates)
	}
	update := result.TransitionUpdates[0]
	if update.Resolution != evidence.ResolutionStatic || update.Target == nil || update.Target.Label != "LoadIndex" {
		t.Fatalf("update = %#v", update)
	}
	if update.Target.Location == nil || update.Target.Location.Path != "main.go" || update.Target.Location.Line != 5 {
		t.Fatalf("target = %#v", update.Target)
	}
}

func TestBackupContainsMainAndScannerBranches(t *testing.T) {
	result, session := executeLifecycleFixture(t)
	task := anchorByKind(t, result.NewAnchors, flowproof.AnchorTask)
	start := transitionByRelation(t, session.Proof.Transitions, evidence.RelationStartsGoroutine)
	callback := transitionByRelation(t, result.NewTransitions, evidence.RelationCallback)
	join := transitionByRelation(t, result.NewTransitions, evidence.RelationJoins)
	registration := transitionToAnchorLabel(t, result.NewTransitions, result.NewAnchors, evidence.RelationCalls, "Go")
	cancelCall := transitionToAnchorLabel(t, result.NewTransitions, result.NewAnchors, evidence.RelationCalls, "cancel")
	waitCall := transitionToAnchorLabel(t, result.NewTransitions, result.NewAnchors, evidence.RelationCalls, "group.Wait")

	if start.From != "handler" || start.To != task.ID {
		t.Fatalf("task start = %#v, task = %#v", start, task)
	}
	if start.Condition == nil || start.Condition.Expression != "scan" {
		t.Fatalf("task start condition = %#v, want scan branch", start.Condition)
	}
	if callback.From != task.ID || callback.Invocation != evidence.InvocationGoroutine {
		t.Fatalf("task body = %#v, task = %#v", callback, task)
	}
	if registration.Condition == nil || callback.Condition == nil || join.Condition == nil {
		t.Fatalf("guarded scanner branch was flattened: registration=%#v callback=%#v join=%#v", registration, callback, join)
	}
	if registration.From != "handler" || registration.To == task.ID {
		t.Fatalf("registration call = %#v, task = %#v", registration, task)
	}
	if cancelCall.From != "handler" || waitCall.From != "handler" {
		t.Fatalf("main-branch calls: cancel=%#v wait=%#v", cancelCall, waitCall)
	}
	if cancelCall.Condition != nil || waitCall.Condition != nil {
		t.Fatalf("unconditional main branch inherited scanner guard: cancel=%#v wait=%#v", cancelCall, waitCall)
	}
	for _, transition := range result.NewTransitions {
		if transition.Relation == evidence.RelationReturns {
			t.Fatalf("source order must not become a return edge: %#v", transition)
		}
		if transition.To == callback.To && transition.Invocation == evidence.InvocationSynchronous {
			t.Fatalf("scanner body also appears on the synchronous branch: %#v", transition)
		}
	}
	concurrency, ok := session.Proof.Slot(flowproof.SlotConcurrency)
	if !ok || concurrency.Status != flowproof.SlotPartial ||
		concurrency.Missing != "selected scenario satisfying the branch condition" {
		t.Fatalf("concurrency after core evaluation = %#v", concurrency)
	}
	if session.Stop == nil || session.Stop.Reason != flowproof.StopNoTask {
		t.Fatalf("guarded lifecycle stop = %#v, want honest no_task boundary", session.Stop)
	}
}

func TestBackupCancelDoesNotJoinWait(t *testing.T) {
	result, session := executeLifecycleFixture(t)
	cancel := transitionByRelation(t, result.NewTransitions, evidence.RelationCancels)
	use := transitionByRelation(t, result.NewTransitions, evidence.RelationUsesCancellation)
	join := transitionByRelation(t, result.NewTransitions, evidence.RelationJoins)
	cancelSource := anchorByID(t, result.NewAnchors, cancel.From)
	cancelTarget := anchorByID(t, result.NewAnchors, cancel.To)
	wait := anchorByID(t, result.NewAnchors, join.From)

	if cancelSource.Label != "cancel" || cancelTarget.Label != "cancelCtx" {
		t.Fatalf("cancellation = %#v, source = %#v, target = %#v", cancel, cancelSource, cancelTarget)
	}
	task := anchorByKind(t, result.NewAnchors, flowproof.AnchorTask)
	if use.From != task.ID || use.To != cancelTarget.ID {
		t.Fatalf("task cancellation association = %#v, task=%#v target=%#v", use, task, cancelTarget)
	}
	if wait.Label != "group.Wait" {
		t.Fatalf("join source = %#v, anchor = %#v", join, wait)
	}
	if join.From == cancel.From || join.From == cancel.To {
		t.Fatalf("cancel and Wait were flattened into one join edge: cancel=%#v join=%#v", cancel, join)
	}
	for _, transition := range result.NewTransitions {
		if transition.From == cancel.From && transition.To == wait.ID {
			t.Fatalf("source order was encoded as an edge from cancel to Wait: %#v", transition)
		}
	}

	broken := session.Proof
	broken.Anchors = append([]flowproof.Anchor(nil), broken.Anchors...)
	broken.Transitions = append([]flowproof.Transition(nil), broken.Transitions...)
	broken.Slots = append([]flowproof.Slot(nil), broken.Slots...)
	unrelated := flowproof.Anchor{ID: "unrelated-context", Kind: flowproof.AnchorOperation, Label: "otherCtx"}
	broken.Anchors = append(broken.Anchors, unrelated)
	for index := range broken.Transitions {
		if broken.Transitions[index].Relation == evidence.RelationUsesCancellation {
			broken.Transitions[index].To = unrelated.ID
		}
	}
	for index := range broken.Slots {
		if broken.Slots[index].Kind == flowproof.SlotConcurrency {
			broken.Slots[index].Status = flowproof.SlotPartial
		}
	}
	brokenSession := flowproof.Start(broken, flowproof.DefaultBudget(), "fixture", "fixture-v1")
	brokenConcurrency, _ := brokenSession.Proof.Slot(flowproof.SlotConcurrency)
	if brokenConcurrency.Status == flowproof.SlotVerified {
		t.Fatalf("unrelated cancellation target verified scanner lifecycle: %#v", brokenConcurrency)
	}
}

func TestBackupWaitJoinsScannerTask(t *testing.T) {
	first, _ := executeLifecycleFixture(t)
	second, _ := executeLifecycleFixture(t)
	firstTask := anchorByKind(t, first.NewAnchors, flowproof.AnchorTask)
	secondTask := anchorByKind(t, second.NewAnchors, flowproof.AnchorTask)
	join := transitionByRelation(t, first.NewTransitions, evidence.RelationJoins)

	if join.To != firstTask.ID {
		t.Fatalf("join = %#v, task = %#v", join, firstTask)
	}
	if firstTask.ID == "" || firstTask.ID != secondTask.ID {
		t.Fatalf("task identity is not stable: first=%#v second=%#v", firstTask, secondTask)
	}
}

func executeLifecycleFixture(t *testing.T) (flowproof.Result, flowproof.Session) {
	t.Helper()
	repo := t.TempDir()
	writeFile(t, repo, "go.mod", "module example.test/lifecycle\n\ngo 1.24\n")
	writeFile(t, repo, "main.go", `package main

import "context"

type Group struct{}

func (*Group) Go(callback func() error) {}
func (*Group) Wait() error { return nil }
func work(context.Context) error { return nil }

func run(parent context.Context, group *Group, scan bool) error {
	cancelCtx, cancel := context.WithCancel(parent)
	if scan {
		group.Go(func() error { return work(cancelCtx) })
	}
	cancel()
	if err := group.Wait(); err != nil {
		return err
	}
	return nil
}

func main() {}
`)
	proof := flowproof.Proof{
		Version:   flowproof.Version,
		ID:        "lifecycle",
		Archetype: flowproof.ArchetypeCLI,
		Slots: []flowproof.Slot{
			{Kind: flowproof.SlotTrigger, Status: flowproof.SlotVerified},
			{Kind: flowproof.SlotEntrypoint, Status: flowproof.SlotVerified},
			{Kind: flowproof.SlotDispatch, Status: flowproof.SlotVerified},
			{Kind: flowproof.SlotApplicationCallable, Status: flowproof.SlotVerified},
			{Kind: flowproof.SlotCoreOperation, Status: flowproof.SlotVerified},
			{Kind: flowproof.SlotIOBoundary, Status: flowproof.SlotVerified},
			{Kind: flowproof.SlotConcurrency, Status: flowproof.SlotPartial, EvidenceIDs: []string{"start"}},
			{Kind: flowproof.SlotTermination, Status: flowproof.SlotVerified},
		},
		Anchors: []flowproof.Anchor{
			{ID: "handler", Kind: flowproof.AnchorFunction, Label: "run"},
			{ID: "start-callsite", Kind: flowproof.AnchorCallsite, Label: "group.Go"},
		},
		Transitions: []flowproof.Transition{{
			ID: "start", From: "handler", To: "start-callsite", Relation: evidence.RelationStartsGoroutine,
			Resolution: evidence.ResolutionUnresolved, Invocation: evidence.InvocationGoroutine,
			Condition: &evidence.Condition{
				Expression: "scan", Location: evidence.Location{Path: "main.go", Line: 13, Column: 5},
			},
			Certainty: evidence.CertaintyStatic, Evidence: evidence.Location{Path: "main.go", Line: 14},
		}},
	}
	session := flowproof.Start(proof, flowproof.DefaultBudget(), "fixture", "fixture-v1")
	task, ok := session.Next()
	if !ok || task.Kind != flowproof.TaskInspectLifecycle || task.TargetID != "start" {
		t.Fatalf("lifecycle task = %#v, stop = %#v", task, session.Stop)
	}

	result, err := NewExecutor().Execute(context.Background(), repo, proof, task)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Apply(result); err != nil {
		t.Fatal(err)
	}
	return result, session
}

func transitionByRelation(t *testing.T, transitions []flowproof.Transition, relation evidence.RelationKind) flowproof.Transition {
	t.Helper()
	var matches []flowproof.Transition
	for _, transition := range transitions {
		if transition.Relation == relation {
			matches = append(matches, transition)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("%s transitions = %#v", relation, matches)
	}
	return matches[0]
}

func transitionToAnchorLabel(t *testing.T, transitions []flowproof.Transition, anchors []flowproof.Anchor, relation evidence.RelationKind, label string) flowproof.Transition {
	t.Helper()
	var matches []flowproof.Transition
	for _, transition := range transitions {
		if transition.Relation != relation {
			continue
		}
		anchor := anchorByID(t, anchors, transition.To)
		if anchor.Label == label {
			matches = append(matches, transition)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("%s transitions to %q = %#v", relation, label, matches)
	}
	return matches[0]
}

func anchorByKind(t *testing.T, anchors []flowproof.Anchor, kind flowproof.AnchorKind) flowproof.Anchor {
	t.Helper()
	var matches []flowproof.Anchor
	for _, anchor := range anchors {
		if anchor.Kind == kind {
			matches = append(matches, anchor)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("%s anchors = %#v", kind, matches)
	}
	return matches[0]
}

func anchorByID(t *testing.T, anchors []flowproof.Anchor, id string) flowproof.Anchor {
	t.Helper()
	for _, anchor := range anchors {
		if anchor.ID == id {
			return anchor
		}
	}
	t.Fatalf("anchor %q not found in %#v", id, anchors)
	return flowproof.Anchor{}
}

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
