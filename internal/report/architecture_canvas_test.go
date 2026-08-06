package report

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
)

func TestProjectArchitectureCanvasResticScanOnlyAsync(t *testing.T) {
	t.Parallel()

	canvas := projectArchitectureFixture(t, architectureResticProof(), nil)
	flow := architectureFlow(t, canvas, "backup")
	scan := architectureStep(t, flow, "scan")
	if branch := architectureBranch(t, flow, scan.BranchID); branch.Kind != "task" || branch.RootAnchorID != "scanner-task" {
		t.Fatalf("Scan branch = %#v, want scanner task", branch)
	}
	if architectureBranch(t, flow, architectureStep(t, flow, "handler").BranchID).Kind != "main" {
		t.Fatalf("handler was not kept on the main branch: %#v", flow.Branches)
	}
	contextStep := architectureStep(t, flow, "scanner-context")
	if branch := architectureBranch(t, flow, contextStep.BranchID); branch.Kind != "shared" {
		t.Fatalf("cancellation context branch = %#v, want explicit shared branch", branch)
	}
	callback := architectureFlowEdge(t, canvas, "scan-body")
	if callback.FromBranchID != scan.BranchID || callback.ToBranchID != scan.BranchID || callback.CrossBranch {
		t.Fatalf("Scan callback edge = %#v, want task-local edge", callback)
	}
}

func TestProjectArchitectureCanvasCancelAndJoinRemainDistinct(t *testing.T) {
	t.Parallel()

	canvas := projectArchitectureFixture(t, architectureResticProof(), nil)
	cancel := architectureFlowEdge(t, canvas, "cancel-scanner")
	join := architectureFlowEdge(t, canvas, "join-scanner")
	waitCall := architectureFlowEdge(t, canvas, "call-wait")
	start := architectureFlowEdge(t, canvas, "start-scanner")

	if cancel.Relation != evidence.RelationCancels || cancel.From != "cancel" || cancel.To != "scanner-context" {
		t.Fatalf("cancel edge = %#v", cancel)
	}
	if join.Relation != evidence.RelationJoins || join.From != "wait" || join.To != "scanner-task" {
		t.Fatalf("join edge = %#v, want Wait -> scanner task", join)
	}
	if waitCall.Relation != evidence.RelationCalls || waitCall.From != "handler" || waitCall.To != "wait" {
		t.Fatalf("Wait invocation = %#v, want separate handler call", waitCall)
	}
	if !start.CrossBranch || !join.CrossBranch || start.FromBranchID == start.ToBranchID || join.FromBranchID == join.ToBranchID {
		t.Fatalf("task start/join must cross main and task branches: start=%#v join=%#v", start, join)
	}
}

func TestProjectArchitectureCanvasCreatesOneBranchPerTask(t *testing.T) {
	t.Parallel()

	proof := architectureResticProof()
	proof.Anchors = append(proof.Anchors,
		flowproof.Anchor{ID: "upload-task", Kind: flowproof.AnchorTask, Label: "upload task", Location: architectureLocation("cmd/restic/cmd_backup.go", 710)},
		flowproof.Anchor{ID: "upload", Kind: flowproof.AnchorFunction, Label: "upload", Location: architectureLocation("internal/archiver/upload.go", 40)},
	)
	proof.Transitions = append(proof.Transitions,
		architectureTransition("start-upload", "handler", "upload-task", evidence.RelationStartsGoroutine, evidence.InvocationGoroutine, 710),
		architectureTransition("upload-body", "upload-task", "upload", evidence.RelationCallback, evidence.InvocationGoroutine, 711),
	)
	canvas := projectArchitectureFixture(t, proof, func(bundle *componentmap.CandidateBundle) {
		bundle.AnchorBindings = append(bundle.AnchorBindings,
			architectureBinding("backup", "upload-task", architectureMember(componentmap.MemberPackage, "cmd"), "cmd/restic/cmd_backup.go", 710),
			architectureBinding("backup", "upload", architectureMember(componentmap.MemberPackage, "archiver"), "internal/archiver/upload.go", 40),
		)
	})
	flow := architectureFlow(t, canvas, "backup")
	var taskRoots []string
	for _, branch := range flow.Branches {
		if branch.Kind == "task" {
			taskRoots = append(taskRoots, branch.RootAnchorID)
		}
	}
	if !reflect.DeepEqual(taskRoots, []string{"scanner-task", "upload-task"}) {
		t.Fatalf("task roots = %v, want one branch per task", taskRoots)
	}
	scanBranch := architectureStep(t, flow, "scan").BranchID
	uploadBranch := architectureStep(t, flow, "upload").BranchID
	if scanBranch == uploadBranch || architectureBranch(t, flow, scanBranch).RootAnchorID != "scanner-task" ||
		architectureBranch(t, flow, uploadBranch).RootAnchorID != "upload-task" {
		t.Fatalf("task bodies were merged: scan=%q upload=%q branches=%#v", scanBranch, uploadBranch, flow.Branches)
	}
}

func TestProjectArchitectureCanvasOmitsMalformedAndMissingEndpointEdges(t *testing.T) {
	t.Parallel()

	proof := architectureResticProof()
	proof.Transitions = append(proof.Transitions,
		architectureTransition("missing-target", "handler", "not-an-anchor", evidence.RelationCalls, evidence.InvocationSynchronous, 720),
		architectureTransition("invalid-relation", "handler", "wait", evidence.RelationKind("teleports"), evidence.InvocationSynchronous, 721),
	)
	invalidEvidence := architectureTransition("invalid-evidence", "handler", "wait", evidence.RelationCalls, evidence.InvocationSynchronous, 0)
	proof.Transitions = append(proof.Transitions, invalidEvidence)
	canvas := projectArchitectureFixture(t, proof, nil)
	if architectureHasFlowEdge(canvas, "missing-target") || architectureHasFlowEdge(canvas, "invalid-relation") ||
		architectureHasFlowEdge(canvas, "invalid-evidence") {
		t.Fatalf("malformed transitions became drawable: %#v", canvas.FlowEdges)
	}
	if !architectureHasFrontier(canvas, "missing_endpoint", "missing-target") ||
		!architectureHasFrontier(canvas, "invalid_transition", "invalid-relation") ||
		!architectureHasFrontier(canvas, "invalid_transition", "invalid-evidence") {
		t.Fatalf("frontiers = %#v, want missing and malformed transition frontiers", canvas.Frontiers)
	}
}

func TestProjectArchitectureCanvasValidatesProofIdentity(t *testing.T) {
	t.Parallel()

	t.Run("proof id mismatch", func(t *testing.T) {
		proof := architectureResticProof()
		proof.ID = "restore"
		canvas := projectArchitectureFixture(t, proof, nil)
		if len(canvas.Flows) != 0 || !architectureHasDiagnostic(canvas, "flow.proof_id_mismatch") {
			t.Fatalf("canvas = %#v, want rejected mismatched proof", canvas)
		}
	})

	t.Run("stale proof version", func(t *testing.T) {
		proof := architectureResticProof()
		proof.Version = 1
		canvas := projectArchitectureFixture(t, proof, nil)
		if len(canvas.Flows) != 0 || !architectureHasDiagnostic(canvas, "flow.unsupported_proof_version") {
			t.Fatalf("canvas = %#v, want visible stale-version rejection", canvas)
		}
	})

	t.Run("duplicate anchor id", func(t *testing.T) {
		proof := architectureResticProof()
		proof.Anchors = append(proof.Anchors, proof.Anchors[0])
		canvas := projectArchitectureFixture(t, proof, nil)
		if architectureHasFlowEdge(canvas, "start-scanner") || !architectureHasDiagnostic(canvas, "flow.duplicate_anchor_id") {
			t.Fatalf("canvas = %#v, want duplicate anchor omitted before edge projection", canvas)
		}
	})
}

func TestProjectArchitectureCanvasUsesExactBindingNotPathCoincidence(t *testing.T) {
	t.Parallel()

	proof := architectureResticProof()
	for index := range proof.Anchors {
		if proof.Anchors[index].ID == "scan" {
			proof.Anchors[index].Location = architectureLocation("cmd/restic/cmd_backup.go", 500)
		}
	}
	canvas := projectArchitectureFixture(t, proof, func(bundle *componentmap.CandidateBundle) {
		bindings := bundle.AnchorBindings[:0]
		for _, binding := range bundle.AnchorBindings {
			if binding.AnchorID != "scan" {
				bindings = append(bindings, binding)
			}
		}
		bundle.AnchorBindings = bindings
	})
	flow := architectureFlow(t, canvas, "backup")
	scan := architectureStep(t, flow, "scan")
	handler := architectureStep(t, flow, "handler")
	if handler.Binding == nil || handler.ComponentID != "" || len(handler.ParticipatingComponentIDs) != 1 {
		t.Fatal("exact handler binding was not projected")
	}
	if scan.ComponentID != "" || len(scan.ParticipatingComponentIDs) != 0 || scan.Binding != nil {
		t.Fatalf("path coincidence assigned Scan to handler component: %#v", scan)
	}
	if !architectureHasAnchorFrontier(canvas, "unassigned_component", "scan") {
		t.Fatalf("frontiers = %#v, want unassigned Scan binding", canvas.Frontiers)
	}
}

func TestProjectArchitectureCanvasSharedBindingUsesParticipantsWithoutOwnershipOrCrossProduct(t *testing.T) {
	t.Parallel()

	project := func(reverse bool) ArchitectureCanvas {
		bundle := architectureBundle()
		bundle.Relations[0].Kind = componentmap.StructuralRelationBehaviorHandoff
		cmd := architectureMember(componentmap.MemberPackage, "cmd")
		archiver := architectureMember(componentmap.MemberPackage, "archiver")
		subsystems := []componentmap.ProposedSubsystem{
			{
				Name: "Command Surface",
				Components: []componentmap.ProposedComponent{{
					Name: "Backup command and scan", MemberIDs: []componentmap.MemberID{cmd, archiver},
				}},
			},
			{
				Name: "Repository Engine",
				Components: []componentmap.ProposedComponent{{
					Name: "Repository scanner", MemberIDs: []componentmap.MemberID{archiver},
				}},
			},
		}
		if reverse {
			subsystems[0], subsystems[1] = subsystems[1], subsystems[0]
		}
		landscape, err := componentmap.Apply(bundle, componentmap.Proposal{
			Version: componentmap.ProposalVersion, Subsystems: subsystems,
		})
		if err != nil {
			t.Fatal(err)
		}
		canvas, err := ProjectArchitectureCanvas(ArchitectureCanvasInput{
			CandidateBundle: bundle,
			Landscape:       landscape,
			Flows: []ArchitectureFlowInput{{
				ID: "backup", Name: "Backup operation",
				Session: flowproof.Session{Version: flowproof.SessionVersion, Proof: architectureResticProof()},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return canvas
	}

	first := project(false)
	second := project(true)
	firstStep := architectureStep(t, architectureFlow(t, first, "backup"), "scan")
	secondStep := architectureStep(t, architectureFlow(t, second, "backup"), "scan")
	if firstStep.Binding == nil || firstStep.ComponentID != "" || len(firstStep.ParticipatingComponentIDs) != 2 {
		t.Fatalf("shared bound step = %#v", firstStep)
	}
	if !reflect.DeepEqual(firstStep.ParticipatingComponentIDs, secondStep.ParticipatingComponentIDs) {
		t.Fatalf(
			"participant order changed with proposal order: first=%v second=%v",
			firstStep.ParticipatingComponentIDs,
			secondStep.ParticipatingComponentIDs,
		)
	}
	if len(first.StructuralFacts) != 1 || len(first.StructuralEdges) != 1 {
		t.Fatalf("shared endpoint lost its exact member relation: %#v", first)
	}
	edge := first.StructuralEdges[0]
	if edge.Witness.From != first.StructuralFacts[0].From || edge.Witness.To != first.StructuralFacts[0].To ||
		edge.FromComponentID == "" || edge.ToComponentID != "" ||
		len(edge.FromComponentIDs) != 1 || len(edge.ToComponentIDs) != 2 {
		t.Fatalf("shared endpoint relation chose or expanded conceptual endpoints: %#v", edge)
	}
	if architectureHasAnchorFrontier(first, "ambiguous_component", "scan") {
		t.Fatalf("valid shared binding was presented as unresolved: %#v", first.Frontiers)
	}
}

func TestProjectArchitectureCanvasKeepsStructuralLocatorSeparateWithPluralExactParticipants(t *testing.T) {
	t.Parallel()

	packageID := architectureMember(componentmap.MemberPackage, "service")
	fileID := architectureMember(componentmap.MemberFile, "service.go")
	symbolID := architectureMember(componentmap.MemberSymbol, "Serve")
	bundle := componentmap.CandidateBundle{
		Version:             componentmap.ContractVersion,
		RepositoryArchetype: componentmap.ArchetypeApplication,
		GroundingMode:       componentmap.GroundingPackages,
		Candidates: []componentmap.Candidate{
			{
				ID: packageID, Role: componentmap.CandidateRoleConceptualMember, Name: "service",
				Facts: []componentmap.LocalFact{architectureFact(componentmap.FactRepositoryPath, "service", "service.go", 1)},
			},
			{
				ID: fileID, Role: componentmap.CandidateRoleStructuralLocator, Name: "service.go", ParentID: &packageID,
				Facts: []componentmap.LocalFact{architectureFact(componentmap.FactRepositoryPath, "service.go", "service.go", 1)},
			},
			{
				ID: symbolID, Role: componentmap.CandidateRoleConceptualMember, Name: "Serve", ParentID: &fileID,
				Facts: []componentmap.LocalFact{architectureFact(componentmap.FactDeclaration, "Serve", "service.go", 20)},
			},
		},
		Relations: []componentmap.LocalRelation{{
			ID: "package-to-handler", From: packageID, To: symbolID,
			Kind: componentmap.StructuralRelationBehaviorHandoff, Certainty: evidence.CertaintyStatic,
			Provenance: architectureProvenance("fixture", "containment_witness", "service.go", 20),
			Scenarios:  []componentmap.ScenarioContext{{ID: "fixture", Name: "fixture"}},
		}},
	}
	project := func(reverse bool) ArchitectureCanvas {
		subsystems := []componentmap.ProposedSubsystem{
			{Name: "Service package", Components: []componentmap.ProposedComponent{{Name: "Service package", MemberIDs: []componentmap.MemberID{packageID}}}},
			{Name: "Request handling", Components: []componentmap.ProposedComponent{{Name: "Request handling", MemberIDs: []componentmap.MemberID{symbolID}}}},
		}
		if reverse {
			subsystems[0], subsystems[1] = subsystems[1], subsystems[0]
		}
		landscape, err := componentmap.Apply(bundle, componentmap.Proposal{
			Version: componentmap.ProposalVersion, Subsystems: subsystems,
		})
		if err != nil {
			t.Fatal(err)
		}
		canvas, err := ProjectArchitectureCanvas(ArchitectureCanvasInput{
			CandidateBundle: bundle, Landscape: landscape,
		})
		if err != nil {
			t.Fatal(err)
		}
		return canvas
	}

	first := project(false)
	second := project(true)
	if len(first.StructuralLocators) != 1 {
		t.Fatalf("structural locators = %#v", first.StructuralLocators)
	}
	locator := first.StructuralLocators[0]
	if locator.Locator.ID != fileID || locator.Locator.Role != componentmap.CandidateRoleStructuralLocator ||
		len(locator.ParticipatingComponentIDs) != 2 {
		t.Fatalf("structural locator projection = %#v", locator)
	}
	if !reflect.DeepEqual(locator.ParticipatingComponentIDs, second.StructuralLocators[0].ParticipatingComponentIDs) {
		t.Fatalf(
			"structural locator participants changed with proposal order: first=%v second=%v",
			locator.ParticipatingComponentIDs,
			second.StructuralLocators[0].ParticipatingComponentIDs,
		)
	}
	for _, component := range first.Components {
		for _, member := range component.Members {
			if member.ID == fileID {
				t.Fatalf("structural locator became a conceptual member: %#v", component)
			}
		}
	}
	if len(first.StructuralFacts) != 1 || first.StructuralFacts[0].ID != "package-to-handler" {
		t.Fatalf("local structural facts changed: %#v", first.StructuralFacts)
	}

	localLandscape, err := componentmap.Canonical(bundle)
	if err != nil {
		t.Fatal(err)
	}
	localCanvas, err := ProjectArchitectureCanvas(ArchitectureCanvasInput{
		CandidateBundle: bundle, Landscape: localLandscape,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(localCanvas.StructuralLocators) != 1 ||
		localCanvas.StructuralLocators[0].Locator.ID != fileID ||
		len(localCanvas.StructuralLocators[0].ParticipatingComponentIDs) == 0 {
		t.Fatalf("local Canvas lost structural locator participation: %#v", localCanvas.StructuralLocators)
	}
	for _, component := range localCanvas.Components {
		for _, member := range component.Members {
			if member.ID == fileID {
				t.Fatalf("local Canvas made structural locator conceptual: %#v", component)
			}
		}
	}
	rejectedLandscape, err := componentmap.Apply(bundle, componentmap.Proposal{
		Version: componentmap.ProposalVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	rejectedCanvas, err := ProjectArchitectureCanvas(ArchitectureCanvasInput{
		CandidateBundle: bundle, Landscape: rejectedLandscape,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rejectedCanvas.StructuralLocators) != 1 ||
		rejectedCanvas.StructuralLocators[0].Locator.ID != fileID ||
		!reflect.DeepEqual(
			rejectedCanvas.StructuralLocators[0].ParticipatingComponentIDs,
			localCanvas.StructuralLocators[0].ParticipatingComponentIDs,
		) {
		t.Fatalf(
			"rejected/local Canvas structural locator mismatch: rejected=%#v local=%#v",
			rejectedCanvas.StructuralLocators,
			localCanvas.StructuralLocators,
		)
	}
}

func TestProjectArchitectureCanvasKeepsProducerOwnedFileAsModuleConceptual(t *testing.T) {
	t.Parallel()

	fileID := architectureMember(componentmap.MemberFile, "module.go")
	bundle := componentmap.CandidateBundle{
		Version:             componentmap.ContractVersion,
		RepositoryArchetype: componentmap.ArchetypeLibraryFramework,
		GroundingMode:       componentmap.GroundingPackages,
		Candidates: []componentmap.Candidate{{
			ID: fileID, Role: componentmap.CandidateRoleConceptualMember, Name: "module.go",
			Facts: []componentmap.LocalFact{architectureFact(componentmap.FactRepositoryPath, "module.go", "module.go", 1)},
		}},
	}
	landscape, err := componentmap.Apply(bundle, componentmap.Proposal{
		Version: componentmap.ProposalVersion,
		Subsystems: []componentmap.ProposedSubsystem{{
			Name: "Library module", Components: []componentmap.ProposedComponent{{
				Name: "Library module", MemberIDs: []componentmap.MemberID{fileID},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	canvas, err := ProjectArchitectureCanvas(ArchitectureCanvasInput{CandidateBundle: bundle, Landscape: landscape})
	if err != nil {
		t.Fatal(err)
	}
	if len(canvas.StructuralLocators) != 0 || len(canvas.Components) != 1 ||
		len(canvas.Components[0].Members) != 1 || canvas.Components[0].Members[0].ID != fileID {
		t.Fatalf("file-as-module conceptual projection = %#v", canvas)
	}
}

func TestProjectArchitectureCanvasUsesLandscapeSubsystemsAndFlowLabels(t *testing.T) {
	t.Parallel()

	canvas := projectArchitectureFixture(t, architectureResticProof(), nil)
	if len(canvas.Subsystems) != 2 || canvas.Subsystems[0].Name != "Command Surface" || canvas.Subsystems[1].Name != "Repository Engine" {
		t.Fatalf("subsystems = %#v, want Landscape groups rather than role lanes", canvas.Subsystems)
	}
	flow := architectureFlow(t, canvas, "backup")
	if flow.Name != "Backup operation" || flow.Archetype != flowproof.ArchetypeCLI ||
		flow.Trigger != "user runs backup" ||
		flow.Scope != "one CLI handler" || flow.MentalModel != "scan then save" {
		t.Fatalf("flow labels = %#v", flow)
	}
	for _, component := range canvas.Components {
		if !reflect.DeepEqual(component.ParticipatingFlowIDs, []componentmap.FlowID{"backup"}) {
			t.Fatalf("component %q participating flows = %v", component.ID, component.ParticipatingFlowIDs)
		}
	}
}

func TestProjectArchitectureCanvasLabelsPackageFallbackHonestly(t *testing.T) {
	t.Parallel()

	bundle := architectureBundle()
	landscape, err := componentmap.Deterministic(bundle, componentmap.FallbackInsufficientAnchors)
	if err != nil {
		t.Fatal(err)
	}
	canvas, err := ProjectArchitectureCanvas(ArchitectureCanvasInput{CandidateBundle: bundle, Landscape: landscape})
	if err != nil {
		t.Fatal(err)
	}
	if canvas.ArchitectureSource != componentmap.SourcePackageFallback || canvas.ArchitectureLevel != 4 ||
		canvas.Title != "Package landscape" ||
		canvas.Subtitle != "Behavioral grounding was insufficient or the architecture proposal was rejected" {
		t.Fatalf("package fallback presentation = %#v", canvas)
	}
}

func TestProjectArchitectureCanvasLabelsAcceptedPackageGroupingHonestly(t *testing.T) {
	t.Parallel()

	input, _ := architectureCanvasInput(t, architectureResticProof(), nil)
	canvas, err := ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if canvas.ArchitectureSource != componentmap.SourceValidatedModel ||
		canvas.Title != "Conceptual architecture" ||
		canvas.Subtitle != "Model-assisted grouping of exact local repository members" {
		t.Fatalf("accepted package grouping presentation = %#v", canvas)
	}
}

func TestProjectArchitectureCanvasPreservesDiagnosticSubsystemCategory(t *testing.T) {
	t.Parallel()

	input, landscape := architectureCanvasInput(t, architectureResticProof(), nil)
	landscape.Subsystems[0].Category = componentmap.SubsystemCategoryDiagnostic
	input.Landscape = landscape
	canvas, err := ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if canvas.Subsystems[0].Category != componentmap.SubsystemCategoryDiagnostic {
		t.Fatalf("subsystem category = %q, want diagnostic", canvas.Subsystems[0].Category)
	}
	if canvas.LocalRemainderComponentID != "" {
		t.Fatalf("ordinary local diagnostic component became D206 remainder %q", canvas.LocalRemainderComponentID)
	}
	retainedFlowAssociation := false
	for _, component := range canvas.Components {
		if len(component.ParticipatingFlowIDs) != 0 {
			retainedFlowAssociation = true
			break
		}
	}
	if !retainedFlowAssociation {
		t.Fatal("ordinary local diagnostic components lost exact flow associations")
	}
}

func TestProjectArchitectureCanvasKeepsPartialRemainderOutOfModelAssociations(t *testing.T) {
	t.Parallel()

	bundle := architectureBundle()
	landscape, err := componentmap.Apply(bundle, componentmap.Proposal{
		Version: componentmap.ProposalVersion,
		Subsystems: []componentmap.ProposedSubsystem{{
			Name: "Command Surface",
			Components: []componentmap.ProposedComponent{{
				Name: "Backup command",
				MemberIDs: []componentmap.MemberID{
					architectureMember(componentmap.MemberPackage, "cmd"),
				},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if landscape.ValidationOutcome != componentmap.ValidationAcceptedPartial ||
		len(landscape.LocalRemainderMemberIDs) != 1 ||
		landscape.LocalRemainderMemberIDs[0] != architectureMember(componentmap.MemberPackage, "archiver") {
		t.Fatalf("partial landscape = %#v", landscape)
	}
	canvas, err := ProjectArchitectureCanvas(ArchitectureCanvasInput{
		CandidateBundle: bundle,
		Landscape:       landscape,
		Flows: []ArchitectureFlowInput{{
			ID: "backup", Name: "Backup operation",
			Session: flowproof.Session{Version: flowproof.SessionVersion, Proof: architectureResticProof()},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canvas.StructuralFacts, bundle.Relations) {
		t.Fatalf("partial enrichment changed exact local relations: %#v", canvas.StructuralFacts)
	}
	if len(canvas.StructuralLocators) != 0 {
		t.Fatalf("partial enrichment invented structural locators: %#v", canvas.StructuralLocators)
	}
	diagnosticComponents := make(map[componentmap.ComponentID]struct{})
	for _, subsystem := range canvas.Subsystems {
		if subsystem.Category != componentmap.SubsystemCategoryDiagnostic {
			continue
		}
		for _, componentID := range subsystem.ComponentIDs {
			diagnosticComponents[componentID] = struct{}{}
		}
	}
	if len(diagnosticComponents) != 1 {
		t.Fatalf("partial remainder components = %#v", diagnosticComponents)
	}
	if _, exists := diagnosticComponents[canvas.LocalRemainderComponentID]; canvas.LocalRemainderComponentID == "" || !exists {
		t.Fatalf("persisted local remainder component = %q, diagnostics = %#v", canvas.LocalRemainderComponentID, diagnosticComponents)
	}
	for _, component := range canvas.Components {
		if _, remainder := diagnosticComponents[component.ID]; !remainder {
			continue
		}
		if len(component.Members) != 1 ||
			component.Members[0].ID != architectureMember(componentmap.MemberPackage, "archiver") ||
			len(component.ParticipatingFlowIDs) != 0 || len(component.AnchorIDs) != 0 {
			t.Fatalf("local remainder gained model association: %#v", component)
		}
	}
	scan := architectureStep(t, architectureFlow(t, canvas, "backup"), "scan")
	if scan.Binding == nil || scan.ComponentID != "" || len(scan.ParticipatingComponentIDs) != 0 {
		t.Fatalf("local remainder became flow ownership/participation: %#v", scan)
	}
}

func TestArchitectureCanvasVersionRejectsHistoricalRemainderSemantics(t *testing.T) {
	t.Parallel()

	// Decision 231 (Archive 9): shared participation projection advanced
	// the canvas version to 10 (shared scope + shared members).
	if ArchitectureCanvasVersion != 10 {
		t.Fatalf("ArchitectureCanvasVersion = %d, want 10 for shared participation projection", ArchitectureCanvasVersion)
	}
	if err := validateSemanticSearchCanvasVersion(&ArchitectureCanvas{Version: ArchitectureCanvasVersion - 1}); err == nil || !strings.Contains(err.Error(), "unsupported architecture canvas version") {
		t.Fatalf("historical Architecture Canvas version error = %v", err)
	}
}

func TestArchitectureCanvasRejectsRemainderIdentityNotBoundToExactMembers(t *testing.T) {
	t.Parallel()

	bundle := architectureBundle()
	landscape, err := componentmap.Apply(bundle, componentmap.Proposal{
		Version: componentmap.ProposalVersion,
		Subsystems: []componentmap.ProposedSubsystem{{
			Name: "Command Surface",
			Components: []componentmap.ProposedComponent{{
				Name:      "Backup command",
				MemberIDs: []componentmap.MemberID{architectureMember(componentmap.MemberPackage, "cmd")},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	landscape.LocalRemainderMemberIDs = []componentmap.MemberID{
		architectureMember(componentmap.MemberPackage, "cmd"),
	}
	if _, err := architectureLocalRemainderComponentID(landscape); err == nil ||
		!strings.Contains(err.Error(), "no diagnostic component matches") {
		t.Fatalf("mismatched local remainder identity error = %v", err)
	}
}

func TestProjectArchitectureCanvasKeepsPackageImportAsSupportingFact(t *testing.T) {
	t.Parallel()

	input, landscape := architectureCanvasInput(t, architectureResticProof(), nil)
	canvas, err := ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(canvas.StructuralFacts) != 1 || len(canvas.StructuralEdges) != 0 {
		t.Fatalf("facts=%#v edges=%#v", canvas.StructuralFacts, canvas.StructuralEdges)
	}
	relation := landscape.Relations[0]
	fact := canvas.StructuralFacts[0]
	if fact.ID != relation.ID || fact.From != relation.From || fact.To != relation.To ||
		fact.Location == nil || fact.Location.Line != 12 || !reflect.DeepEqual(fact.Provenance, relation.Provenance) ||
		!reflect.DeepEqual(fact.Scenarios, relation.Scenarios) {
		t.Fatalf("structural fact lost witness data: %#v", fact)
	}
}

func projectArchitectureFixture(
	t *testing.T,
	proof flowproof.Proof,
	mutateBundle func(*componentmap.CandidateBundle),
) ArchitectureCanvas {
	t.Helper()
	input, _ := architectureCanvasInput(t, proof, mutateBundle)
	canvas, err := ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	return canvas
}

func architectureCanvasInput(
	t *testing.T,
	proof flowproof.Proof,
	mutateBundle func(*componentmap.CandidateBundle),
) (ArchitectureCanvasInput, componentmap.Landscape) {
	t.Helper()
	bundle := architectureBundle()
	if mutateBundle != nil {
		mutateBundle(&bundle)
	}
	landscape, err := componentmap.Apply(bundle, componentmap.Proposal{
		Version: componentmap.ContractVersion,
		Subsystems: []componentmap.ProposedSubsystem{
			{
				Name: "Command Surface",
				Components: []componentmap.ProposedComponent{{
					Name: "Backup command", MemberIDs: []componentmap.MemberID{architectureMember(componentmap.MemberPackage, "cmd")},
				}},
			},
			{
				Name: "Repository Engine",
				Components: []componentmap.ProposedComponent{{
					Name: "Repository scanner", MemberIDs: []componentmap.MemberID{architectureMember(componentmap.MemberPackage, "archiver")},
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ArchitectureCanvasInput{
		CandidateBundle: bundle,
		Landscape:       landscape,
		Flows: []ArchitectureFlowInput{{
			ID: "backup", Name: "Backup operation", Trigger: "user runs backup",
			Scope: "one CLI handler", MentalModel: "scan then save",
			Session: flowproof.Session{Version: flowproof.SessionVersion, Proof: proof},
		}},
	}, landscape
}

func architectureBundle() componentmap.CandidateBundle {
	cmd := architectureMember(componentmap.MemberPackage, "cmd")
	archiver := architectureMember(componentmap.MemberPackage, "archiver")
	return componentmap.CandidateBundle{
		Version:             componentmap.ContractVersion,
		RepositoryArchetype: componentmap.ArchetypeApplication,
		GroundingMode:       componentmap.GroundingPackages,
		Flows: []componentmap.Flow{{
			ID: "backup", Name: "Backup",
			Facts: []componentmap.LocalFact{architectureFact(componentmap.FactDeclaration, "flowproof-v2", "cmd/restic/cmd_backup.go", 500)},
		}},
		Candidates: []componentmap.Candidate{
			{
				ID: cmd, Role: componentmap.CandidateRoleConceptualMember, Name: "cmd/restic",
				Participations: []componentmap.FlowParticipation{{
					FlowID: "backup", Evidence: architectureFact(componentmap.FactFlowParticipation, "backup", "cmd/restic/cmd_backup.go", 500),
				}},
				Facts: []componentmap.LocalFact{architectureFact(componentmap.FactRepositoryPath, "cmd/restic", "cmd/restic/cmd_backup.go", 1)},
			},
			{
				ID: archiver, Role: componentmap.CandidateRoleConceptualMember, Name: "internal/archiver",
				Participations: []componentmap.FlowParticipation{{
					FlowID: "backup", Evidence: architectureFact(componentmap.FactFlowParticipation, "backup", "internal/archiver/scanner.go", 44),
				}},
				Facts: []componentmap.LocalFact{architectureFact(componentmap.FactRepositoryPath, "internal/archiver", "internal/archiver/scanner.go", 1)},
			},
		},
		Relations: []componentmap.LocalRelation{{
			ID: "cmd-imports-archiver", From: cmd, To: archiver,
			Kind:     componentmap.StructuralRelationPackageImport,
			Location: architectureLocation("cmd/restic/cmd_backup.go", 12), Certainty: evidence.CertaintyStatic,
			Provenance: architectureProvenance("go_list", "package_import", "cmd/restic/cmd_backup.go", 12),
			Scenarios: []componentmap.ScenarioContext{{
				ID: "go-default", Name: "default Go build", Build: evidence.BuildContext{GOOS: "darwin", GOARCH: "amd64"},
			}},
		}},
		AnchorBindings: []componentmap.FlowAnchorBinding{
			architectureBinding("backup", "handler", cmd, "cmd/restic/cmd_backup.go", 500),
			architectureBinding("backup", "scanner-task", cmd, "cmd/restic/cmd_backup.go", 686),
			architectureBinding("backup", "scan", archiver, "internal/archiver/scanner.go", 44),
			architectureBinding("backup", "cancel", cmd, "cmd/restic/cmd_backup.go", 701),
			architectureBinding("backup", "wait", cmd, "cmd/restic/cmd_backup.go", 704),
			architectureBinding("backup", "scanner-context", cmd, "cmd/restic/cmd_backup.go", 682),
		},
	}
}

func architectureResticProof() flowproof.Proof {
	condition := &evidence.Condition{Expression: "!opts.NoScan", Location: evidence.Location{Path: "cmd/restic/cmd_backup.go", Line: 685}}
	return flowproof.Proof{
		Version: flowproof.Version, ID: "backup", Archetype: flowproof.ArchetypeCLI,
		Goal: "back up repository data", Command: "backup",
		Slots: []flowproof.Slot{{
			Kind: flowproof.SlotConcurrency, Status: flowproof.SlotPartial,
			Summary: "guarded scanner lifecycle", Missing: "selected branch condition",
			EvidenceIDs: []string{"start-scanner", "scan-body", "join-scanner"},
		}},
		Anchors: []flowproof.Anchor{
			{ID: "handler", Kind: flowproof.AnchorFunction, Label: "runBackup", Location: architectureLocation("cmd/restic/cmd_backup.go", 500)},
			{ID: "scanner-task", Kind: flowproof.AnchorTask, Label: "Scanner.Scan task", Location: architectureLocation("cmd/restic/cmd_backup.go", 686)},
			{ID: "scan", Kind: flowproof.AnchorMethod, Label: "Scanner.Scan", Location: architectureLocation("internal/archiver/scanner.go", 44)},
			{ID: "cancel", Kind: flowproof.AnchorOperation, Label: "cancel", Location: architectureLocation("cmd/restic/cmd_backup.go", 701)},
			{ID: "wait", Kind: flowproof.AnchorOperation, Label: "group.Wait", Location: architectureLocation("cmd/restic/cmd_backup.go", 704)},
			{ID: "scanner-context", Kind: flowproof.AnchorOperation, Label: "scanCtx", Location: architectureLocation("cmd/restic/cmd_backup.go", 682)},
		},
		Transitions: []flowproof.Transition{
			architectureConditionalTransition("start-scanner", "handler", "scanner-task", evidence.RelationStartsGoroutine, evidence.InvocationGoroutine, 686, condition),
			architectureConditionalTransition("scan-body", "scanner-task", "scan", evidence.RelationCallback, evidence.InvocationGoroutine, 690, condition),
			architectureTransition("call-cancel", "handler", "cancel", evidence.RelationCalls, evidence.InvocationSynchronous, 701),
			architectureTransition("cancel-scanner", "cancel", "scanner-context", evidence.RelationCancels, evidence.InvocationSynchronous, 701),
			architectureTransition("call-wait", "handler", "wait", evidence.RelationCalls, evidence.InvocationSynchronous, 704),
			architectureConditionalTransition("join-scanner", "wait", "scanner-task", evidence.RelationJoins, evidence.InvocationSynchronous, 704, condition),
			architectureConditionalTransition("uses-context", "scanner-task", "scanner-context", evidence.RelationUsesCancellation, evidence.InvocationUnknown, 690, condition),
		},
	}
}

func architectureTransition(
	id, from, to string,
	relation evidence.RelationKind,
	invocation evidence.InvocationMode,
	line int,
) flowproof.Transition {
	return architectureConditionalTransition(id, from, to, relation, invocation, line, nil)
}

func architectureConditionalTransition(
	id, from, to string,
	relation evidence.RelationKind,
	invocation evidence.InvocationMode,
	line int,
	condition *evidence.Condition,
) flowproof.Transition {
	return flowproof.Transition{
		ID: id, From: from, To: to, Relation: relation,
		Resolution: evidence.ResolutionStatic, Invocation: invocation, Condition: condition,
		Certainty: evidence.CertaintyStatic,
		Evidence:  evidence.Location{Path: "cmd/restic/cmd_backup.go", Line: line},
		Provider:  "go_types",
	}
}

func architectureBinding(
	flowID componentmap.FlowID,
	anchorID string,
	memberID componentmap.MemberID,
	path string,
	line int,
) componentmap.FlowAnchorBinding {
	return componentmap.FlowAnchorBinding{
		FlowID: flowID, AnchorID: anchorID, MemberID: memberID,
		Location: architectureLocation(path, line), Certainty: evidence.CertaintyStatic,
		Provenance: architectureProvenance("go_types", "bind_flow_anchor", path, line),
		Scenarios: []componentmap.ScenarioContext{{
			ID: "go-default", Name: "default Go build", Build: evidence.BuildContext{GOOS: "darwin", GOARCH: "amd64"},
		}},
	}
}

func architectureFact(kind componentmap.FactKind, value, path string, line int) componentmap.LocalFact {
	return componentmap.LocalFact{
		Kind: kind, Value: value, Location: architectureLocation(path, line),
		Certainty:  evidence.CertaintyStatic,
		Provenance: architectureProvenance("fixture", "collect_fact", path, line),
	}
}

func architectureProvenance(provider, operation, path string, line int) []evidence.Provenance {
	return []evidence.Provenance{{
		Provider: provider, Version: "v1", Operation: operation,
		Location: architectureLocation(path, line),
	}}
}

func architectureLocation(path string, line int) *evidence.Location {
	return &evidence.Location{Path: path, Line: line}
}

func architectureMember(kind componentmap.MemberKind, value string) componentmap.MemberID {
	return componentmap.MemberID{Kind: kind, Value: value}
}

func architectureFlow(t *testing.T, canvas ArchitectureCanvas, id componentmap.FlowID) ArchitectureFlow {
	t.Helper()
	for _, flow := range canvas.Flows {
		if flow.ID == id {
			return flow
		}
	}
	t.Fatalf("flow %q not found in %#v", id, canvas.Flows)
	return ArchitectureFlow{}
}

func architectureStep(t *testing.T, flow ArchitectureFlow, id string) ArchitectureFlowStep {
	t.Helper()
	for _, step := range flow.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("step %q not found in %#v", id, flow.Steps)
	return ArchitectureFlowStep{}
}

func architectureBranch(t *testing.T, flow ArchitectureFlow, id string) ArchitectureFlowBranch {
	t.Helper()
	for _, branch := range flow.Branches {
		if branch.ID == id {
			return branch
		}
	}
	t.Fatalf("branch %q not found in %#v", id, flow.Branches)
	return ArchitectureFlowBranch{}
}

func architectureFlowEdge(t *testing.T, canvas ArchitectureCanvas, id string) ArchitectureFlowEdge {
	t.Helper()
	for _, edge := range canvas.FlowEdges {
		if edge.ID == id {
			return edge
		}
	}
	t.Fatalf("edge %q not found in %#v", id, canvas.FlowEdges)
	return ArchitectureFlowEdge{}
}

func architectureHasFlowEdge(canvas ArchitectureCanvas, id string) bool {
	for _, edge := range canvas.FlowEdges {
		if edge.ID == id {
			return true
		}
	}
	return false
}

func architectureHasFrontier(canvas ArchitectureCanvas, kind, transitionID string) bool {
	for _, frontier := range canvas.Frontiers {
		if frontier.Kind == kind && frontier.TransitionID == transitionID {
			return true
		}
	}
	return false
}

func architectureHasAnchorFrontier(canvas ArchitectureCanvas, kind, anchorID string) bool {
	for _, frontier := range canvas.Frontiers {
		if frontier.Kind == kind && frontier.AnchorID == anchorID {
			return true
		}
	}
	return false
}

func architectureHasDiagnostic(canvas ArchitectureCanvas, code string) bool {
	for _, diagnostic := range canvas.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
