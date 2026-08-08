package report

// Decision 226/242 provider-free acceptance: an honest first-hop fan-out is
// built only from saved local evidence; contract fields come from the closed
// sets; no edge is invented; an explicit frontier is a successful result.
import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/localization"
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
				ProofMode: componentmap.AnchorProofProcessEntry, Label: "process entry example.main",
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
				Members: []componentmap.Candidate{
					{
						ID: entryMember, Role: "conceptual_member", Name: "main",
						Facts: []componentmap.LocalFact{{
							Kind: componentmap.FactDeclaration, Value: "example.main",
							Location: &evidence.Location{Path: "main.go", Line: 36, Column: 1},
						}},
					},
					{
						ID: startMember, Role: "conceptual_member", Name: "service.Start",
						Facts: []componentmap.LocalFact{{
							Kind: componentmap.FactDeclaration, Value: "service.Start",
							Location: &evidence.Location{Path: "service.go", Line: 20, Column: 6},
						}},
					},
				},
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
					Scenarios:  []componentmap.ScenarioContext{{ID: "go:linux:tags=", Name: "Recorded Go build scenario"}},
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

func TestProjectMechanismFragmentsHonestFirstHopFanout(t *testing.T) {
	data := mechanismFragmentFixture()
	fragments, err := ProjectMechanismFragments(data.ArchitectureCanvas)
	if err != nil {
		t.Fatalf("ProjectMechanismFragments: %v", err)
	}
	if len(fragments) != 1 {
		t.Fatalf("fragments = %#v, want one", fragments)
	}
	fragment := &fragments[0]
	if fragment.Version != MechanismFragmentVersion {
		t.Fatalf("version = %d, want %d", fragment.Version, MechanismFragmentVersion)
	}
	if fragment.ID == "" || len(fragment.ComponentIDs) != 1 ||
		fragment.ComponentIDs[0] != "component-app" {
		t.Fatalf("identity/accounting = %#v", fragment)
	}
	// Entry carries the closed contract fields. A process entry is a
	// declaration/entry anchor: claim_kind process_entry, support_mode
	// resolved_static, evidence from the actual proof mode.
	entry := fragment.Entry
	if entry.ClaimKind != "process_entry" || entry.SupportMode != "resolved_static" ||
		entry.Path != "main.go" || entry.Line != 36 || entry.Ordering != "exact_local_order" ||
		entry.Evidence != "behavior anchor proof_mode process_entry" || entry.Symbol != "example.main" ||
		strings.Contains(entry.Label, "member-main") {
		t.Fatalf("entry = %#v", entry)
	}
	// Only the handoff FROM the entry symbol is a supported transition; the
	// second handoff (from service.Start) is NOT reachable from the entry by
	// local evidence alone, so it must NOT be invented into the path. The
	// local evidence alone, so it must NOT be invented into the path.
	if len(fragment.Handoffs) != 1 {
		t.Fatalf("handoffs = %d, want 1 supported direct handoff: %#v", len(fragment.Handoffs), fragment.Handoffs)
	}
	transition := fragment.Handoffs[0]
	if transition.ClaimKind != "direct_static_call" || transition.SupportMode != "resolved_static" ||
		transition.Path != "main.go" || transition.Line != 150 ||
		transition.Ordering != "resolved_path_order" {
		t.Fatalf("transition = %#v", transition)
	}
	if transition.Evidence != "go_ssa surface-ssa-v12 connect_architecture_anchors" {
		t.Fatalf("transition evidence = %q", transition.Evidence)
	}
	if transition.Target == nil || transition.Target.Label != "service.Start" ||
		transition.Target.Path != "service.go" || transition.Target.Line != 20 ||
		transition.Target.Symbol != "service.Start" {
		t.Fatalf("exact Canvas handoff target = %#v", transition.Target)
	}
	collectOpenablePaths(data)
	for _, path := range []string{"main.go", "service.go"} {
		if !containsString(data.OpenablePaths, path) {
			t.Fatalf("mechanism source path %q is not openable: %#v", path, data.OpenablePaths)
		}
	}
	// Frontier is explicit: the handoff from service.Start exists locally
	// but is not connected to the entry path — honest disconnect.
	if fragment.Frontier.Ordering != "not_established" ||
		len(fragment.Frontier.Unresolved) == 0 {
		t.Fatalf("frontier = %#v", fragment.Frontier)
	}
	// Round-trip drift rejection.
	if err := ValidateMechanismFragments(data.ArchitectureCanvas, fragments); err != nil {
		t.Fatalf("ValidateMechanismFragments: %v", err)
	}
	fragment.Version = MechanismFragmentVersion - 1
	if err := ValidateMechanismFragments(data.ArchitectureCanvas, fragments); err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("historical fragment version error = %v", err)
	}
	fragment.Version = MechanismFragmentVersion
	fragment.Handoffs[0].Path = "mutated.go"
	if err := ValidateMechanismFragments(data.ArchitectureCanvas, fragments); err == nil {
		t.Fatal("ValidateMechanismFragments accepted drifted fragment")
	}
}

func TestMechanismFragmentsRemainExactAfterPresentationLocalization(t *testing.T) {
	t.Parallel()

	canonical := mechanismFragmentFixture()
	canonical.FormatVersion = CurrentFormatVersion
	if err := ensureMechanismFragments(canonical); err != nil {
		t.Fatalf("ensure canonical fragments: %v", err)
	}
	canonicalEntry := canonical.ArchitectureCanvas.MechanismFragments[0].Entry
	prepared, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	projected, result, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		russianPresentationProjection(prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback {
		t.Fatalf("presentation localization unexpectedly fell back: %#v", result)
	}
	if projected.ArchitectureCanvas.BehaviorAnchors[0].Label ==
		canonical.ArchitectureCanvas.BehaviorAnchors[0].Label {
		t.Fatal("fixture did not localize the process-entry anchor label")
	}
	if got := projected.ArchitectureCanvas.MechanismFragments[0].Entry; !reflect.DeepEqual(got, canonicalEntry) {
		t.Fatalf("presentation localization changed persisted mechanism identity:\ngot  %#v\nwant %#v", got, canonicalEntry)
	}
	if _, err := RenderHTML(projected); err != nil {
		t.Fatalf("RenderHTML rejected localized report with exact mechanism fragments: %v", err)
	}
}

func TestEnsureMechanismFragmentsAbsentWithoutProcessEntry(t *testing.T) {
	data := mechanismFragmentFixture()
	data.ArchitectureCanvas.BehaviorAnchors = nil
	if err := ensureMechanismFragments(data); err != nil {
		t.Fatalf("ensureMechanismFragments: %v", err)
	}
	if data.ArchitectureCanvas.MechanismFragments != nil {
		t.Fatalf("fragments must be absent without a process entry: %#v", data.ArchitectureCanvas.MechanismFragments)
	}
}

func TestEnsureMechanismFragmentsAbsentForZeroHopEntrypoint(t *testing.T) {
	t.Parallel()

	data := mechanismFragmentFixture()
	data.ArchitectureCanvas.StructuralEdges = nil
	if err := ensureMechanismFragments(data); err != nil {
		t.Fatalf("ensureMechanismFragments: %v", err)
	}
	if data.ArchitectureCanvas.MechanismFragments != nil {
		t.Fatalf("zero-hop entry became a mechanism: %#v", data.ArchitectureCanvas.MechanismFragments)
	}
}

func TestMechanismFragmentsPersistOnlyOnArchitectureCanvas(t *testing.T) {
	t.Parallel()

	data := mechanismFragmentFixture()
	if err := ensureMechanismFragments(data); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		ArchitectureCanvas struct {
			MechanismFragments []MechanismFragmentProjection `json:"mechanism_fragments"`
		} `json:"architecture_canvas"`
		LegacyMechanism json.RawMessage `json:"mechanism_fragment"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.ArchitectureCanvas.MechanismFragments) != 1 || wire.LegacyMechanism != nil {
		t.Fatalf("mechanism wire authority = %#v, legacy=%s", wire.ArchitectureCanvas.MechanismFragments, wire.LegacyMechanism)
	}
}

func TestEnsureMechanismFragmentsRejectsPersistedProjectionOnHistoricalCanvas(t *testing.T) {
	t.Parallel()

	data := mechanismFragmentFixture()
	fragments, err := ProjectMechanismFragments(data.ArchitectureCanvas)
	if err != nil {
		t.Fatal(err)
	}
	data.ArchitectureCanvas.Version--
	if err := ensureMechanismFragments(data); err != nil {
		t.Fatalf("historical canvas without a v3 projection should remain renderable: %v", err)
	}
	data.ArchitectureCanvas.MechanismFragments = fragments
	if err := ensureMechanismFragments(data); err == nil ||
		!strings.Contains(err.Error(), "unsupported canvas version") {
		t.Fatalf("persisted v3 projection on historical canvas error = %v", err)
	}
}

func TestEnsureMechanismFragmentsUsesExactEntryHandoffOutsideCanvas(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithEntryHandoff()
	entryMember := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "entry-member"}
	canvas := &ArchitectureCanvas{
		Version: ArchitectureCanvasVersion,
		BehaviorAnchors: []componentmap.BehaviorAnchor{{
			ID:        "entry-anchor",
			Kind:      componentmap.AnchorProcessEntry,
			ProofMode: componentmap.AnchorProofProcessEntry,
			Label:     "process entry main",
			Location: evidence.Location{
				Path: grounding.EntryHandoffs[0].ProcessEntrypoint.Location.Path,
				Line: grounding.EntryHandoffs[0].ProcessEntrypoint.Location.Line,
			},
			Scenario:  componentmap.ScenarioContext{ID: grounding.EntryHandoffs[0].Scenario.ID},
			MemberIDs: []componentmap.MemberID{entryMember},
		}},
		Components: []ArchitectureComponent{
			{
				ID: "application", Name: "Application",
				Members: []componentmap.Candidate{{
					ID: entryMember, Role: componentmap.CandidateRoleConceptualMember, Name: "main",
					Facts: []componentmap.LocalFact{{
						Kind:     componentmap.FactDeclaration,
						Value:    grounding.EntryHandoffs[0].ProcessEntrypoint.ID,
						Location: &grounding.EntryHandoffs[0].ProcessEntrypoint.Location,
					}},
				}},
			},
			{
				ID: "callee", Name: "Exact callee",
				Members: []componentmap.Candidate{{
					ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "callee-member"},
					Role: componentmap.CandidateRoleConceptualMember,
					Name: grounding.EntryHandoffs[0].Callee.ID,
					Facts: []componentmap.LocalFact{{
						Kind:     componentmap.FactDeclaration,
						Value:    grounding.EntryHandoffs[0].Callee.ID,
						Location: &grounding.EntryHandoffs[0].Callee.Location,
					}},
				}},
			},
			{
				ID: "shared-entry", Name: "Shared entry participant",
				SharedMembers: []componentmap.Candidate{{
					ID: entryMember, Role: componentmap.CandidateRoleConceptualMember, Name: "main",
				}},
			},
			{
				ID: "false-callee", Name: "Same declaration identity, wrong location",
				Members: []componentmap.Candidate{{
					ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "false-callee-member"},
					Role: componentmap.CandidateRoleConceptualMember,
					Name: grounding.EntryHandoffs[0].Callee.ID,
					Facts: []componentmap.LocalFact{{
						Kind:  componentmap.FactDeclaration,
						Value: grounding.EntryHandoffs[0].Callee.ID,
						Location: &evidence.Location{
							Path: grounding.EntryHandoffs[0].Callee.Location.Path,
							Line: grounding.EntryHandoffs[0].Callee.Location.Line + 1,
						},
					}},
				}},
			},
			{
				ID: "local-remainder", Name: "Local remainder",
				SharedMembers: []componentmap.Candidate{{
					ID: entryMember, Role: componentmap.CandidateRoleConceptualMember, Name: "main",
				}, {
					ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "remainder-callee"},
					Role: componentmap.CandidateRoleConceptualMember,
					Name: grounding.EntryHandoffs[0].Callee.ID,
					Facts: []componentmap.LocalFact{{
						Kind:     componentmap.FactDeclaration,
						Value:    grounding.EntryHandoffs[0].Callee.ID,
						Location: &grounding.EntryHandoffs[0].Callee.Location,
					}},
				}},
			},
		},
		LocalRemainderComponentID: "local-remainder",
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
				Scenarios: []componentmap.ScenarioContext{{ID: grounding.EntryHandoffs[0].Scenario.ID}},
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
				Scenarios: []componentmap.ScenarioContext{{ID: grounding.EntryHandoffs[0].Scenario.ID}},
			},
			WitnessIDs: []string{"same-line-distinct-call-witness"}, WitnessCount: 1,
		}},
	}
	data := &ReportData{
		ArchitectureCanvas:    canvas,
		ArchitectureGrounding: &grounding,
		ArchitectureAssociations: &ArchitectureAssociationProjection{
			Version: ArchitectureAssociationVersion,
			Components: []ArchitectureComponentAssociations{{
				ComponentID: "application",
				Name:        "Application",
				Associations: []ArchitectureBoundaryResourceRow{
					{Kind: "operation", OwningUnit: "github.com/example/application"},
					{Kind: "surface", OwningUnit: "github.com/example/application"},
				},
			}},
		},
	}
	if err := ensureMechanismFragments(data); err != nil {
		t.Fatal(err)
	}
	if len(data.ArchitectureCanvas.MechanismFragments) != 1 ||
		len(data.ArchitectureCanvas.MechanismFragments[0].Handoffs) != 2 {
		t.Fatalf("fragments = %#v, want exact handoff and distinct same-line call; generic association rows must stay out", data.ArchitectureCanvas.MechanismFragments)
	}
	fragment := data.ArchitectureCanvas.MechanismFragments[0]
	if len(fragment.ComponentIDs) != 3 || fragment.ComponentIDs[0] != "application" ||
		fragment.ComponentIDs[1] != "callee" || fragment.ComponentIDs[2] != "shared-entry" {
		t.Fatalf("exact component joins = %#v", fragment)
	}
	transition := fragment.Handoffs[0]
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
	if transition.Target == nil ||
		transition.Target.Label != handoff.Callee.Name ||
		transition.Target.Path != handoff.Callee.Location.Path ||
		transition.Target.Line != handoff.Callee.Location.Line ||
		transition.Target.Column != handoff.Callee.Location.Column ||
		transition.Target.Symbol != handoff.Callee.ID {
		t.Fatalf("entry handoff target = %#v, want exact locally joined callee", transition.Target)
	}
	withoutCallee := *canvas
	withoutCallee.MechanismFragments = nil
	withoutCallee.Components = append(
		[]ArchitectureComponent(nil),
		canvas.Components[0],
		canvas.Components[2],
		canvas.Components[3],
		canvas.Components[4],
	)
	withoutCalleeFragments, err := projectMechanismFragmentsForProduct(&withoutCallee, &grounding)
	if err != nil {
		t.Fatal(err)
	}
	if len(withoutCalleeFragments) != 1 || len(withoutCalleeFragments[0].Handoffs) == 0 ||
		withoutCalleeFragments[0].Handoffs[0].Target == nil ||
		withoutCalleeFragments[0].Handoffs[0].Target.Symbol != "" {
		t.Fatalf("unjoined target published local symbol: %#v", withoutCalleeFragments)
	}
	if len(canvas.StructuralEdges) != 2 || canvas.StructuralEdges[0].ID != "legacy-overlap" {
		t.Fatalf("exact entry handoff mutated canonical Canvas: %#v", canvas.StructuralEdges)
	}
	if err := validateMechanismFragmentsForProduct(
		canvas,
		data.ArchitectureCanvas.MechanismFragments,
		&grounding,
	); err != nil {
		t.Fatalf("grounded fragment validation: %v", err)
	}
}

func TestMechanismEntryHandoffJoinsExactDeclarationIdentity(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithEntryHandoff()
	handoff := grounding.EntryHandoffs[0]
	matchingMember := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "matching-entry"}
	otherMember := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "other-entry"}
	entryLocation := handoff.ProcessEntrypoint.Location
	canvas := &ArchitectureCanvas{
		Version: ArchitectureCanvasVersion,
		BehaviorAnchors: []componentmap.BehaviorAnchor{
			{
				ID: "matching-anchor", Kind: componentmap.AnchorProcessEntry,
				ProofMode: componentmap.AnchorProofProcessEntry,
				Location:  entryLocation, Scenario: componentmap.ScenarioContext{ID: handoff.Scenario.ID},
				MemberIDs: []componentmap.MemberID{matchingMember},
			},
			{
				ID: "same-location-other-identity", Kind: componentmap.AnchorProcessEntry,
				ProofMode: componentmap.AnchorProofProcessEntry,
				Location:  entryLocation, Scenario: componentmap.ScenarioContext{ID: handoff.Scenario.ID},
				MemberIDs: []componentmap.MemberID{otherMember},
			},
		},
		Components: []ArchitectureComponent{
			{
				ID: "matching", Name: "Matching entry",
				Members: []componentmap.Candidate{{
					ID: matchingMember, Facts: []componentmap.LocalFact{{
						Kind:  componentmap.FactDeclaration,
						Value: handoff.ProcessEntrypoint.ID, Location: &entryLocation,
					}},
				}},
			},
			{
				ID: "other", Name: "Different entry on same source line",
				Members: []componentmap.Candidate{{
					ID: otherMember, Facts: []componentmap.LocalFact{{
						Kind:  componentmap.FactDeclaration,
						Value: "example.com/app.other", Location: &entryLocation,
					}},
				}},
			},
		},
	}
	data := &ReportData{ArchitectureCanvas: canvas, ArchitectureGrounding: &grounding}
	if err := ensureMechanismFragments(data); err != nil {
		t.Fatal(err)
	}
	if len(canvas.MechanismFragments) != 1 ||
		canvas.MechanismFragments[0].Entry.Symbol != handoff.ProcessEntrypoint.ID {
		t.Fatalf("same-location entry handoff identity join = %#v, want only exact declaration", canvas.MechanismFragments)
	}
}

func TestEnsureMechanismFragmentsProjectsEveryProcessEntryDeterministically(t *testing.T) {
	t.Parallel()

	var canonical []MechanismFragmentProjection
	for _, reverse := range []bool{false, true} {
		data := mechanismFragmentFixture()
		helperMember := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "helper-main"}
		data.ArchitectureCanvas.BehaviorAnchors = append(
			data.ArchitectureCanvas.BehaviorAnchors,
			componentmap.BehaviorAnchor{
				ID:        "secondary-entry-anchor",
				Kind:      componentmap.AnchorProcessEntry,
				ProofMode: componentmap.AnchorProofProcessEntry,
				Label:     "different process entry",
				Location:  evidence.Location{Path: "cmd/helper/main.go", Line: 7, Column: 1},
				Scenario:  componentmap.ScenarioContext{ID: "go:linux:tags="},
				MemberIDs: []componentmap.MemberID{helperMember},
			},
		)
		data.ArchitectureCanvas.Components = append(data.ArchitectureCanvas.Components, ArchitectureComponent{
			ID: "component-helper", Name: "Helper",
			Members: []componentmap.Candidate{{
				ID: helperMember, Role: componentmap.CandidateRoleConceptualMember, Name: "helper.main",
			}, {
				ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "helper-run"},
				Role: componentmap.CandidateRoleConceptualMember, Name: "helper.run",
			}},
		})
		data.ArchitectureCanvas.StructuralEdges = append(
			data.ArchitectureCanvas.StructuralEdges,
			ArchitectureStructuralEdge{Witness: componentmap.LocalRelation{
				Kind:      componentmap.StructuralRelationBehaviorHandoff,
				From:      helperMember,
				To:        componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "helper-run"},
				Location:  &evidence.Location{Path: "cmd/helper/main.go", Line: 9},
				Scenarios: []componentmap.ScenarioContext{{ID: "go:linux:tags="}},
			}},
		)
		if reverse {
			anchors := data.ArchitectureCanvas.BehaviorAnchors
			anchors[0], anchors[len(anchors)-1] = anchors[len(anchors)-1], anchors[0]
		}
		if err := ensureMechanismFragments(data); err != nil {
			t.Fatal(err)
		}
		fragments := data.ArchitectureCanvas.MechanismFragments
		if len(fragments) != 2 {
			t.Fatalf("fragments (reverse=%t) = %#v, want two", reverse, fragments)
		}
		if fragments[0].ID == "" || fragments[1].ID == "" || fragments[0].ID == fragments[1].ID {
			t.Fatalf("fragment IDs (reverse=%t) = %#v", reverse, fragments)
		}
		componentsByEntry := make(map[string][]componentmap.ComponentID, len(fragments))
		for _, fragment := range fragments {
			componentsByEntry[fragment.Entry.Path] = fragment.ComponentIDs
		}
		if got := componentsByEntry["main.go"]; len(got) != 1 || got[0] != "component-app" {
			t.Fatalf("main component IDs (reverse=%t) = %#v", reverse, got)
		}
		if got := componentsByEntry["cmd/helper/main.go"]; len(got) != 1 || got[0] != "component-helper" {
			t.Fatalf("helper component IDs (reverse=%t) = %#v", reverse, got)
		}
		if canonical == nil {
			canonical = append([]MechanismFragmentProjection(nil), fragments...)
		} else if !reflect.DeepEqual(canonical, fragments) {
			t.Fatalf("fragment projection changed with anchor order:\nfirst=%#v\nreverse=%#v", canonical, fragments)
		}
	}
}

func TestProjectMechanismFragmentsUsesEveryExactEntryMemberAndScenario(t *testing.T) {
	t.Parallel()

	entryA := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "entry-a"}
	entryB := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "entry-b"}
	target := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "target"}
	otherTarget := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "other-target"}
	makeCanvas := func(reverse bool) *ArchitectureCanvas {
		memberIDs := []componentmap.MemberID{entryB, entryA}
		if reverse {
			memberIDs[0], memberIDs[1] = memberIDs[1], memberIDs[0]
		}
		return &ArchitectureCanvas{
			Version:                   ArchitectureCanvasVersion,
			LocalRemainderComponentID: "local-remainder",
			BehaviorAnchors: []componentmap.BehaviorAnchor{{
				ID:        "entry",
				Kind:      componentmap.AnchorProcessEntry,
				ProofMode: componentmap.AnchorProofProcessEntry,
				Label:     "process entry",
				Location:  evidence.Location{Path: "cmd/app/main.go", Line: 9},
				Scenario:  componentmap.ScenarioContext{ID: "go:linux/amd64:tags="},
				MemberIDs: memberIDs,
			}},
			Components: []ArchitectureComponent{
				{ID: "entry-a-component", Members: []componentmap.Candidate{{ID: entryA}}},
				{ID: "entry-b-component", SharedMembers: []componentmap.Candidate{{ID: entryB}}},
				{ID: "target-component", Members: []componentmap.Candidate{{ID: target}}},
				{ID: "local-remainder", Members: []componentmap.Candidate{{ID: entryA}, {ID: target}}},
			},
			StructuralEdges: []ArchitectureStructuralEdge{
				{Witness: componentmap.LocalRelation{
					Kind: componentmap.StructuralRelationBehaviorHandoff,
					From: entryA, To: target,
					Location:  &evidence.Location{Path: "cmd/app/main.go", Line: 11},
					Scenarios: []componentmap.ScenarioContext{{ID: "go:linux/amd64:tags="}},
				}},
				{Witness: componentmap.LocalRelation{
					Kind: componentmap.StructuralRelationBehaviorHandoff,
					From: entryB, To: otherTarget,
					Location:  &evidence.Location{Path: "cmd/app/main.go", Line: 12},
					Scenarios: []componentmap.ScenarioContext{{ID: "go:linux/amd64:tags="}},
				}},
				{Witness: componentmap.LocalRelation{
					Kind: componentmap.StructuralRelationBehaviorHandoff,
					From: entryA, To: otherTarget,
					Location:  &evidence.Location{Path: "cmd/app/main.go", Line: 13},
					Scenarios: []componentmap.ScenarioContext{{ID: "go:darwin/arm64:tags="}},
				}},
			},
		}
	}

	first, err := ProjectMechanismFragments(makeCanvas(false))
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := ProjectMechanismFragments(makeCanvas(true))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, reversed) {
		t.Fatalf("member order changed fragments:\nfirst=%#v\nreversed=%#v", first, reversed)
	}
	if len(first) != 1 || first[0].Entry.Symbol != "" || len(first[0].Handoffs) != 2 {
		t.Fatalf("multi-member exact fragment = %#v", first)
	}
	wantComponents := []componentmap.ComponentID{"entry-a-component", "entry-b-component", "target-component"}
	if !reflect.DeepEqual(first[0].ComponentIDs, wantComponents) {
		t.Fatalf("component IDs = %#v, want %#v (remainder excluded)", first[0].ComponentIDs, wantComponents)
	}
	for _, handoff := range first[0].Handoffs {
		if handoff.Scenario != "go:linux/amd64:tags=" || handoff.Line == 13 {
			t.Fatalf("cross-scenario handoff leaked: %#v", handoff)
		}
	}
}

func TestProjectMechanismFragmentsRejectsInexactProcessEntry(t *testing.T) {
	t.Parallel()

	data := mechanismFragmentFixture()
	data.ArchitectureCanvas.BehaviorAnchors[0].ProofMode = componentmap.AnchorProofCallTarget
	if _, err := ProjectMechanismFragments(data.ArchitectureCanvas); err == nil ||
		!strings.Contains(err.Error(), "process entry anchor") {
		t.Fatalf("inexact process entry error = %v", err)
	}
}

func TestProjectMechanismFragmentsRejectsGlobalHandoffMultiplication(t *testing.T) {
	t.Parallel()

	entry := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "shared-entry"}
	canvas := &ArchitectureCanvas{
		Version: ArchitectureCanvasVersion,
		BehaviorAnchors: []componentmap.BehaviorAnchor{
			{
				ID: "entry-a", Kind: componentmap.AnchorProcessEntry,
				ProofMode: componentmap.AnchorProofProcessEntry,
				Location:  evidence.Location{Path: "cmd/a/main.go", Line: 1},
				Scenario:  componentmap.ScenarioContext{ID: "go:linux/amd64:tags="},
				MemberIDs: []componentmap.MemberID{entry},
			},
			{
				ID: "entry-b", Kind: componentmap.AnchorProcessEntry,
				ProofMode: componentmap.AnchorProofProcessEntry,
				Location:  evidence.Location{Path: "cmd/b/main.go", Line: 1},
				Scenario:  componentmap.ScenarioContext{ID: "go:linux/amd64:tags="},
				MemberIDs: []componentmap.MemberID{entry},
			},
		},
		Components: []ArchitectureComponent{{
			ID: "entry-component", Members: []componentmap.Candidate{{ID: entry}},
		}},
	}
	for index := 0; index < maxMechanismHandoffsTotal/2+1; index++ {
		canvas.StructuralEdges = append(canvas.StructuralEdges, ArchitectureStructuralEdge{
			Witness: componentmap.LocalRelation{
				Kind: componentmap.StructuralRelationBehaviorHandoff,
				From: entry,
				To:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "target"},
				Location: &evidence.Location{
					Path: "internal/app/run.go", Line: index + 1,
				},
				Scenarios: []componentmap.ScenarioContext{{ID: "go:linux/amd64:tags="}},
			},
		})
	}
	if _, err := ProjectMechanismFragments(canvas); err == nil ||
		!strings.Contains(err.Error(), "total handoff count") {
		t.Fatalf("global handoff bound error = %v", err)
	}
}

func TestArchitectureDeclarationLocationsCompatible(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		left, right evidence.Location
		want        bool
	}{
		{
			name:  "missing column is compatible",
			left:  evidence.Location{Path: "cmd/dive/main.go", Line: 43},
			right: evidence.Location{Path: "cmd/dive/main.go", Line: 43, Column: 6},
			want:  true,
		},
		{
			name:  "equal exact columns",
			left:  evidence.Location{Path: "cmd/dive/main.go", Line: 43, Column: 6},
			right: evidence.Location{Path: "cmd/dive/main.go", Line: 43, Column: 6},
			want:  true,
		},
		{
			name:  "different known columns",
			left:  evidence.Location{Path: "cmd/dive/main.go", Line: 43, Column: 6},
			right: evidence.Location{Path: "cmd/dive/main.go", Line: 43, Column: 7},
			want:  false,
		},
		{
			name:  "different declaration line",
			left:  evidence.Location{Path: "cmd/dive/main.go", Line: 43},
			right: evidence.Location{Path: "cmd/dive/main.go", Line: 44},
			want:  false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := architectureDeclarationLocationsCompatible(test.left, test.right); got != test.want {
				t.Fatalf("compatible(%#v, %#v) = %t, want %t", test.left, test.right, got, test.want)
			}
		})
	}
}
