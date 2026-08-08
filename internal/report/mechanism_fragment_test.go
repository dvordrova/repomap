package report

// Decision 226 provider-free acceptance: the honest vertical fragment is
// built only from saved local evidence; contract fields come from the
// closed sets; no edge is invented; a disconnected fragment with an
// explicit frontier is a successful honest result.
import (
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func mechanismFragmentFixture() *ReportData {
	entryMember := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "member-main"}
	startMember := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "member-start"}
	tlsMember := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "member-tls"}
	canvas := &ArchitectureCanvas{
		Version: ArchitectureCanvasVersion,
		BehaviorAnchors: []componentmap.BehaviorAnchor{
			{
				ID: "anchor-entry", Kind: componentmap.AnchorProcessEntry,
				ProofMode: "call_target", Label: "process entry example.main",
				Location:  evidence.Location{Path: "main.go", Line: 36, Column: 1},
				Scenario:  componentmap.ScenarioContext{ID: "go:linux:tags="},
				MemberIDs: []componentmap.MemberID{entryMember},
			},
			{
				ID: "anchor-lifecycle", Kind: componentmap.AnchorLifecycleStart,
				ProofMode: "declaration_family", Label: "lifecycle_start example.service.Start",
				Location:  evidence.Location{Path: "main.go", Line: 150, Column: 16},
				MemberIDs: []componentmap.MemberID{startMember},
			},
		},
		Components: []ArchitectureComponent{
			{
				ID: componentmap.ComponentID("component-app"), Name: "Primary application",
				Members: []componentmap.Candidate{{ID: entryMember, Role: "conceptual_member", Name: "main"}},
			},
		},
		StructuralEdges: []ArchitectureStructuralEdge{
			{
				ID: "edge-1", FromComponentID: "component-app", ToComponentID: "component-app",
				Witness: componentmap.LocalRelation{
					ID: "handoff-1", Kind: componentmap.StructuralRelationBehaviorHandoff,
					From: entryMember, To: startMember,
					Location:   &evidence.Location{Path: "main.go", Line: 150, Column: 16},
					Provenance: []evidence.Provenance{{Provider: "go_ssa", Version: "surface-ssa-v12", Operation: "connect_architecture_anchors"}},
					Scenarios:  []componentmap.ScenarioContext{{ID: "go:linux", Name: "Recorded Go build scenario"}},
				},
			},
			{
				ID: "edge-2", FromComponentID: "component-app", ToComponentID: "component-app",
				Witness: componentmap.LocalRelation{
					ID: "handoff-2", Kind: componentmap.StructuralRelationBehaviorHandoff,
					From: startMember, To: tlsMember,
					Location:   &evidence.Location{Path: "ldap/server.go", Line: 61, Column: 30},
					Provenance: []evidence.Provenance{{Provider: "go_ssa", Version: "surface-ssa-v12", Operation: "connect_architecture_anchors"}},
					Scenarios:  []componentmap.ScenarioContext{{ID: "go:linux", Name: "Recorded Go build scenario"}},
				},
			},
		},
	}
	data := &ReportData{ArchitectureCanvas: canvas}
	associations, err := ProjectArchitectureAssociations(canvas, nil)
	if err != nil {
		panic(err)
	}
	data.ArchitectureAssociations = associations
	return data
}

func TestProjectMechanismFragmentHonestVerticalSlice(t *testing.T) {
	data := mechanismFragmentFixture()
	fragment, err := ProjectMechanismFragment(
		data.ArchitectureCanvas, data.ArchitectureAssociations, nil,
	)
	if err != nil {
		t.Fatalf("ProjectMechanismFragment: %v", err)
	}
	if fragment == nil {
		t.Fatal("fragment missing")
	}
	if fragment.Version != MechanismFragmentVersion {
		t.Fatalf("version = %d, want %d", fragment.Version, MechanismFragmentVersion)
	}
	// Entry carries the closed contract fields. A process entry is a
	// declaration/entry anchor: claim_kind process_entry, support_mode
	// resolved_static, evidence from the actual proof mode.
	entry := fragment.Entry
	if entry.ClaimKind != "process_entry" || entry.SupportMode != "resolved_static" ||
		entry.Path != "main.go" || entry.Line != 36 || entry.Ordering != "exact_local_order" ||
		entry.Evidence != "behavior anchor proof_mode call_target" {
		t.Fatalf("entry = %#v", entry)
	}
	// Only the handoff FROM the entry symbol is a supported transition; the
	// second handoff (from service.Start) is NOT reachable from the entry by
	// local evidence alone, so it must NOT be invented into the path. The
	// unresolved continuation is an explicit contract transition.
	if len(fragment.Transitions) != 2 {
		t.Fatalf("transitions = %d, want 2 (supported handoff + unresolved continuation): %#v", len(fragment.Transitions), fragment.Transitions)
	}
	transition := fragment.Transitions[0]
	if transition.ClaimKind != "direct_static_call" || transition.SupportMode != "resolved_static" ||
		transition.Path != "main.go" || transition.Line != 150 ||
		transition.Ordering != "resolved_path_order" {
		t.Fatalf("transition = %#v", transition)
	}
	if transition.Evidence != "go_ssa surface-ssa-v12 connect_architecture_anchors" {
		t.Fatalf("transition evidence = %q", transition.Evidence)
	}
	continuation := fragment.Transitions[len(fragment.Transitions)-1]
	if continuation.ClaimKind != "unresolved_continuation" || continuation.SupportMode != "unknown" ||
		continuation.Ordering != "not_established" {
		t.Fatalf("unresolved continuation = %#v", continuation)
	}
	// Frontier is explicit: the handoff from service.Start exists locally
	// but is not connected to the entry path — honest disconnect.
	if fragment.Frontier.Ordering != "not_established" ||
		len(fragment.Frontier.Unresolved) == 0 {
		t.Fatalf("frontier = %#v", fragment.Frontier)
	}
	// Round-trip drift rejection.
	if err := ValidateMechanismFragment(
		data.ArchitectureCanvas, data.ArchitectureAssociations, nil, fragment,
	); err != nil {
		t.Fatalf("ValidateMechanismFragment: %v", err)
	}
	fragment.Transitions[0].Path = "mutated.go"
	if err := ValidateMechanismFragment(
		data.ArchitectureCanvas, data.ArchitectureAssociations, nil, fragment,
	); err == nil {
		t.Fatal("ValidateMechanismFragment accepted drifted fragment")
	}
}

func TestEnsureMechanismFragmentAbsentWithoutProcessEntry(t *testing.T) {
	data := mechanismFragmentFixture()
	data.ArchitectureCanvas.BehaviorAnchors = nil
	if err := ensureMechanismFragment(data); err != nil {
		t.Fatalf("ensureMechanismFragment: %v", err)
	}
	if data.MechanismFragment != nil {
		t.Fatalf("fragment must be absent without a process entry: %#v", data.MechanismFragment)
	}
}

func TestEnsureMechanismFragmentUsesExactEntryHandoffOutsideCanvas(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithEntryHandoff()
	entryMember := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "entry-member"}
	canvas := &ArchitectureCanvas{
		Version: ArchitectureCanvasVersion,
		BehaviorAnchors: []componentmap.BehaviorAnchor{{
			ID:        "decoy-entry-anchor",
			Kind:      componentmap.AnchorProcessEntry,
			ProofMode: componentmap.AnchorProofCallTarget,
			Label:     "different process entry",
			Location:  evidence.Location{Path: "cmd/helper/main.go", Line: 7, Column: 1},
			Scenario:  componentmap.ScenarioContext{ID: "go:linux:tags="},
			MemberIDs: []componentmap.MemberID{{Kind: componentmap.MemberSymbol, Value: "helper-main"}},
		}, {
			ID:        "entry-anchor",
			Kind:      componentmap.AnchorProcessEntry,
			ProofMode: componentmap.AnchorProofCallTarget,
			Label:     "process entry main",
			Location:  grounding.EntryHandoffs[0].ProcessEntrypoint.Location,
			Scenario:  componentmap.ScenarioContext{ID: grounding.EntryHandoffs[0].Scenario.ID},
			MemberIDs: []componentmap.MemberID{entryMember},
		}},
		Components: []ArchitectureComponent{{
			ID: "application", Name: "Application",
			Members: []componentmap.Candidate{{
				ID: entryMember, Role: componentmap.CandidateRoleConceptualMember, Name: "main",
			}},
		}},
		StructuralEdges: []ArchitectureStructuralEdge{{
			ID: "legacy-overlap",
			Witness: componentmap.LocalRelation{
				ID: "legacy-overlap-witness", Kind: componentmap.StructuralRelationBehaviorHandoff,
				From: entryMember,
				To:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "opaque-callee-id"},
				Location: &evidence.Location{
					Path:   grounding.EntryHandoffs[0].RepresentativeCallsite.Path,
					Line:   grounding.EntryHandoffs[0].RepresentativeCallsite.Line,
					Column: grounding.EntryHandoffs[0].RepresentativeCallsite.Column,
				},
			},
			WitnessIDs: []string{"legacy-overlap-witness"}, WitnessCount: 1,
		}, {
			ID: "same-line-distinct-call",
			Witness: componentmap.LocalRelation{
				ID: "same-line-distinct-call-witness", Kind: componentmap.StructuralRelationBehaviorHandoff,
				From: entryMember,
				To:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "second-callee"},
				Location: &evidence.Location{
					Path:   grounding.EntryHandoffs[0].RepresentativeCallsite.Path,
					Line:   grounding.EntryHandoffs[0].RepresentativeCallsite.Line,
					Column: grounding.EntryHandoffs[0].RepresentativeCallsite.Column + 1,
				},
			},
			WitnessIDs: []string{"same-line-distinct-call-witness"}, WitnessCount: 1,
		}},
	}
	data := &ReportData{
		ArchitectureCanvas:       canvas,
		ArchitectureGrounding:    &grounding,
		ArchitectureAssociations: &ArchitectureAssociationProjection{Version: ArchitectureAssociationVersion},
		RepositoryAtlas: &repositoryatlas.Atlas{Evidence: []repositoryatlas.Evidence{{
			ID: "selected-entry", Location: grounding.EntryHandoffs[0].ProcessEntrypoint.Location,
		}}},
		Navigator: &NavigatorReportProduct{
			Recommendation: &navigator.RecommendationAction{EvidenceIDs: []string{"selected-entry"}},
		},
	}
	if err := ensureMechanismFragment(data); err != nil {
		t.Fatal(err)
	}
	if data.MechanismFragment == nil || len(data.MechanismFragment.Transitions) != 3 {
		t.Fatalf("fragment = %#v, want exact handoff, distinct same-line call, plus frontier", data.MechanismFragment)
	}
	transition := data.MechanismFragment.Transitions[0]
	handoff := grounding.EntryHandoffs[0]
	if transition.ClaimKind != "direct_static_call" ||
		transition.Path != handoff.RepresentativeCallsite.Path ||
		transition.Line != handoff.RepresentativeCallsite.Line ||
		transition.Column != handoff.RepresentativeCallsite.Column ||
		transition.Symbol != handoff.Callee.Name ||
		transition.WitnessCount != handoff.WitnessCount ||
		transition.Evidence != handoff.Producer.Provider+" "+handoff.Producer.Version+" "+handoff.Producer.Operation ||
		transition.Limitation != handoff.Limitations[0] {
		t.Fatalf("entry handoff transition = %#v", transition)
	}
	if len(canvas.StructuralEdges) != 2 || canvas.StructuralEdges[0].ID != "legacy-overlap" {
		t.Fatalf("exact entry handoff mutated canonical Canvas: %#v", canvas.StructuralEdges)
	}
	if err := validateMechanismFragmentForProduct(
		canvas,
		data.ArchitectureAssociations,
		data.RepositoryAtlas,
		data.MechanismFragment,
		&grounding,
		data.Navigator,
	); err != nil {
		t.Fatalf("grounded fragment validation: %v", err)
	}
}
