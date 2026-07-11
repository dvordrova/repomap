package flowproof

import (
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestBuildCLIPlansFirstUnresolvedCoreCallsite(t *testing.T) {
	proof := BuildCLI(resticSeed())

	for _, kind := range []SlotKind{SlotTrigger, SlotEntrypoint, SlotDispatch, SlotApplicationCallable} {
		slot, ok := proof.Slot(kind)
		if !ok || slot.Status != SlotVerified {
			t.Fatalf("slot %s = %#v", kind, slot)
		}
	}
	core, _ := proof.Slot(SlotCoreOperation)
	if core.Status != SlotPartial {
		t.Fatalf("core slot = %#v", core)
	}
	var registration Transition
	for _, transition := range proof.Transitions {
		if transition.Relation == evidence.RelationRegisters {
			registration = transition
			break
		}
	}
	target, ok := proof.Anchor(registration.To)
	if !ok || target.Location == nil || registration.Evidence.Path != "cmd/restic/main.go" ||
		registration.Evidence.Line != 98 || target.Location.Path != "cmd/restic/cmd_backup.go" ||
		target.Location.Line != 35 {
		t.Fatalf("registration callsite/target = %#v / %#v", registration.Evidence, target.Location)
	}

	session := Start(proof, DefaultBudget(), "restic-default", CLICollectorVersion)
	task, ok := session.Next()
	if !ok {
		t.Fatalf("no next task: stop=%#v", session.Stop)
	}
	if task.Kind != TaskResolveCallsite || task.Slot != SlotCoreOperation {
		t.Fatalf("next task = %#v", task)
	}
	transition, ok := proof.Transition(task.TargetID)
	if !ok {
		t.Fatalf("target transition %q not found", task.TargetID)
	}
	anchor, _ := proof.Anchor(transition.To)
	if anchor.Label != "repo.LoadIndex" || transition.Evidence.Line != 577 {
		t.Fatalf("first proof hole = %#v via %#v", anchor, transition)
	}
}

func TestSessionStopsWhenNoProgressRepeatsSameTask(t *testing.T) {
	session := Start(BuildCLI(resticSeed()), DefaultBudget(), "restic-default", CLICollectorVersion)
	first, _ := session.Next()
	if err := session.Apply(Result{TaskKey: first.Key}); err != nil {
		t.Fatal(err)
	}
	if session.Stop == nil || session.Stop.Reason != StopDuplicateTask {
		t.Fatalf("stop = %#v", session.Stop)
	}
}

func TestSessionAppliesResolutionMonotonically(t *testing.T) {
	session := Start(BuildCLI(resticSeed()), DefaultBudget(), "restic-default", CLICollectorVersion)
	task, _ := session.Next()
	target := Anchor{
		ID:            "method-restic-repository-load-index",
		Kind:          AnchorMethod,
		Label:         "LoadIndex",
		QualifiedName: "github.com/restic/restic/internal/repository.Repository.LoadIndex",
		Location:      &evidence.Location{Path: "internal/repository/repository.go", Line: 421},
	}
	if err := session.Apply(Result{
		TaskKey: task.Key,
		TransitionUpdates: []TransitionUpdate{{
			TransitionID: task.TargetID,
			Resolution:   evidence.ResolutionTypeInferred,
			Certainty:    evidence.CertaintyStatic,
			Target:       &target,
		}},
		Files:   []string{"internal/repository/repository.go", "cmd/restic/cmd_backup.go"},
		Symbols: []string{target.QualifiedName},
	}); err != nil {
		t.Fatal(err)
	}
	transition, _ := session.Proof.Transition(task.TargetID)
	if transition.To != target.ID || transition.Resolution != evidence.ResolutionTypeInferred {
		t.Fatalf("transition = %#v", transition)
	}
	if len(session.Stats.Files) != 2 || len(session.Stats.Symbols) != 1 || session.Stats.NoProgress != 0 {
		t.Fatalf("stats = %#v", session.Stats)
	}
}

func TestPartialTargetCannotCompleteProof(t *testing.T) {
	proof := proofWithOnePartialRole()
	session := Start(proof, DefaultBudget(), "fixture", CLICollectorVersion)
	task, ok := session.Next()
	if !ok || task.Kind != TaskResolveCallsite || task.Slot != SlotCoreOperation {
		t.Fatalf("next task = %#v, stop = %#v", task, session.Stop)
	}
	target := Anchor{
		ID:            "resolved-domain-call",
		Kind:          AnchorFunction,
		Label:         "DoDomainWork",
		QualifiedName: "example.test/app.DoDomainWork",
		Location:      &evidence.Location{Path: "app/domain.go", Line: 21},
	}
	if err := session.Apply(Result{
		TaskKey: task.Key,
		TransitionUpdates: []TransitionUpdate{{
			TransitionID: task.TargetID,
			Resolution:   evidence.ResolutionStatic,
			Certainty:    evidence.CertaintyStatic,
			Target:       &target,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	core, _ := session.Proof.Slot(SlotCoreOperation)
	if core.Status != SlotPartial || core.Missing != "domain-role witness" ||
		core.Summary != "call targets resolved; core-operation role remains unverified" {
		t.Fatalf("resolved target changed role verdict: %#v", core)
	}
	if session.Stop != nil && session.Stop.Reason == StopComplete {
		t.Fatalf("resolution-only proof stopped complete: %#v", session.Stop)
	}
	if session.Proof.Satisfied() {
		t.Fatal("resolution-only proof is satisfied")
	}
	if next, ok := session.Next(); ok && next.Slot == SlotCoreOperation {
		t.Fatalf("resolved-but-partial core role blocked independent slots: %#v", next)
	}
}

func TestInitConcurrencyIsNotApplicable(t *testing.T) {
	seed := resticSeed()
	seed.FlowID = "restic-init"
	seed.Goal = "understand restic init"
	seed.Command = "init"
	seed.Calls = []CLICall{{
		Symbol: "global.CreateRepository", Path: "cmd/restic/cmd_init.go", Line: 84,
		Relation: "calls",
	}}
	seed.NotApplicableSlots = map[SlotKind]ApplicabilityReason{
		SlotConcurrency: ApplicabilityNoConcurrentLifecycleInScope,
	}

	proof := BuildCLI(seed)
	concurrency, ok := proof.Slot(SlotConcurrency)
	if !ok {
		t.Fatal("concurrency slot missing")
	}
	if concurrency.Status != SlotNotApplicable ||
		concurrency.ApplicabilityReason != ApplicabilityNoConcurrentLifecycleInScope {
		t.Fatalf("concurrency slot = %#v", concurrency)
	}

	for index := range proof.Slots {
		if proof.Slots[index].Kind == SlotConcurrency {
			continue
		}
		proof.Slots[index].Status = SlotVerified
		proof.Slots[index].Missing = ""
	}
	session := Start(proof, DefaultBudget(), "restic-init", CLICollectorVersion)
	if session.Stop == nil || session.Stop.Reason != StopComplete {
		t.Fatalf("justified not-applicable slot did not satisfy proof: %#v", session.Stop)
	}
}

func proofWithOnePartialRole() Proof {
	proof := Proof{
		Version:   Version,
		ID:        "partial-role",
		Archetype: ArchetypeCLI,
		Slots:     newCLISlots(),
		Anchors: []Anchor{
			{ID: "handler", Kind: AnchorFunction, Label: "run"},
			{ID: "callsite", Kind: AnchorCallsite, Label: "DoDomainWork"},
		},
		Transitions: []Transition{{
			ID: "domain-call", From: "handler", To: "callsite",
			Relation: evidence.RelationCalls, Resolution: evidence.ResolutionUnresolved,
			Invocation: evidence.InvocationSynchronous, Certainty: evidence.CertaintyStatic,
			Evidence: evidence.Location{Path: "app/run.go", Line: 12}, Provider: "fixture",
		}},
	}
	for index := range proof.Slots {
		proof.Slots[index].Status = SlotVerified
	}
	setSlot(&proof, SlotCoreOperation, SlotPartial, "candidate domain call", []string{"domain-call"}, "domain-role witness")
	return proof
}

func resticSeed() CLISeed {
	return CLISeed{
		FlowID:    "restic-backup",
		Goal:      "understand restic backup",
		Command:   "backup",
		Framework: "cobra",
		Steps: []CLIStep{
			{Symbol: "main", Relation: "entrypoint", TargetLocation: evidence.Location{Path: "cmd/restic/main.go", Line: 161}},
			{Symbol: "newRootCommand", Relation: "calls", CallsiteLocation: proofLocation("cmd/restic/main.go", 185), TargetLocation: evidence.Location{Path: "cmd/restic/main.go", Line: 36}},
			{Symbol: "newBackupCommand", Relation: "registers_command", CallsiteLocation: proofLocation("cmd/restic/main.go", 98), TargetLocation: evidence.Location{Path: "cmd/restic/cmd_backup.go", Line: 35}},
			{Symbol: "runBackup", Relation: "callback", CallsiteLocation: proofLocation("cmd/restic/cmd_backup.go", 61), TargetLocation: evidence.Location{Path: "cmd/restic/cmd_backup.go", Line: 498}},
		},
		Calls: []CLICall{
			{Symbol: "openWithAppendLock", Path: "cmd/restic/cmd_backup.go", Line: 542, Relation: "calls", Resolved: true},
			{Symbol: "repo.LoadIndex", Path: "cmd/restic/cmd_backup.go", Line: 577, Relation: "calls"},
			{Symbol: "archiver.NewScanner", Path: "cmd/restic/cmd_backup.go", Line: 643, Relation: "constructs"},
			{Symbol: "wg.Go", Path: "cmd/restic/cmd_backup.go", Line: 652, Relation: "starts_goroutine"},
			{Symbol: "archiver.New", Path: "cmd/restic/cmd_backup.go", Line: 655, Relation: "constructs"},
			{Symbol: "arch.Snapshot", Path: "cmd/restic/cmd_backup.go", Line: 698, Relation: "calls"},
		},
	}
}

func proofLocation(path string, line int) *evidence.Location {
	return &evidence.Location{Path: path, Line: line}
}
