package report

// Decision 226/242 provider-free acceptance: an honest first-hop fan-out is
// built only from exact D210 entry handoffs; generic Canvas behavior relations
// cannot become direct-entry evidence.
import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/localization"
)

func entrypointHandoffGroupFixture() *ReportData {
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
				Scenario:  componentmap.ScenarioContext{ID: "go:linux/amd64:tags="},
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
	grounding := architectureGroundingWithEntryHandoff()
	handoff := &grounding.EntryHandoffs[0]
	handoff.ProcessEntrypoint = ArchitectureAnchorMember{
		ID: "example.main", Package: "example", Name: "main",
		Location: evidence.Location{Path: "main.go", Line: 36, Column: 1},
	}
	handoff.Callee = ArchitectureAnchorMember{
		ID: "service.Start", Package: "example/service", Name: "service.Start",
		Location: evidence.Location{Path: "service.go", Line: 20, Column: 6},
	}
	handoff.RepresentativeCallsite = evidence.Location{Path: "main.go", Line: 150, Column: 16}
	handoff.TargetPackage = handoff.Callee.Package
	handoff.ID = architectureEntryHandoffID(*handoff)
	grounding.Coverage.EntryHandoffs.CandidateSetSHA256 =
		architectureEntryHandoffCandidateSetSHA256(grounding.EntryHandoffs)
	data := &ReportData{ArchitectureCanvas: canvas, ArchitectureGrounding: &grounding}
	associations, err := ProjectArchitectureAssociations(canvas, nil)
	if err != nil {
		panic(err)
	}
	data.ArchitectureAssociations = associations
	return data
}

func TestProjectEntrypointHandoffGroupsHonestFirstHopFanout(t *testing.T) {
	data := entrypointHandoffGroupFixture()
	groups, err := ProjectEntrypointHandoffGroups(data.ArchitectureCanvas, data.ArchitectureGrounding, data.RepositoryGraph)
	if err != nil {
		t.Fatalf("ProjectEntrypointHandoffGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %#v, want one", groups)
	}
	group := &groups[0]
	if group.Version != EntrypointHandoffGroupVersion {
		t.Fatalf("version = %d, want %d", group.Version, EntrypointHandoffGroupVersion)
	}
	if group.ID == "" || len(group.ComponentIDs) != 1 ||
		group.ComponentIDs[0] != "component-app" {
		t.Fatalf("identity/accounting = %#v", group)
	}
	// Entry carries the closed contract fields. A process entry is a
	// declaration/entry anchor: claim_kind process_entry, support_mode
	// resolved_static, evidence from the actual proof mode.
	entry := group.Entry
	if entry.ClaimKind != "process_entry" || entry.SupportMode != "resolved_static" ||
		entry.Path != "main.go" || entry.Line != 36 || entry.Column != 1 || entry.Ordering != "exact_local_order" ||
		entry.Evidence != "behavior anchor proof_mode process_entry" || entry.Symbol != "example.main" ||
		!reflect.DeepEqual(entry.ComponentIDs, []componentmap.ComponentID{"component-app"}) ||
		strings.Contains(entry.Label, "member-main") {
		t.Fatalf("entry = %#v", entry)
	}
	// Only the handoff FROM the entry symbol is a supported transition; the
	// second handoff (from service.Start) is NOT reachable from the entry by
	// local evidence alone, so it must NOT be invented into the path.
	if len(group.EntryHandoffs) != 1 {
		t.Fatalf("handoffs = %d, want 1 supported direct handoff: %#v", len(group.EntryHandoffs), group.EntryHandoffs)
	}
	transition := group.EntryHandoffs[0]
	if transition.ClaimKind != "direct_static_call" || transition.SupportMode != "resolved_static" ||
		transition.Path != "main.go" || transition.Line != 150 ||
		!reflect.DeepEqual(transition.ComponentIDs, []componentmap.ComponentID{"component-app"}) ||
		transition.Ordering != "resolved_path_order" {
		t.Fatalf("transition = %#v", transition)
	}
	handoff := data.ArchitectureGrounding.EntryHandoffs[0]
	if transition.EvidenceRef == nil || transition.EvidenceRef.Kind != entrypointEvidenceRefEntryHandoff ||
		transition.EvidenceRef.ID != handoff.ID || transition.Provenance == nil ||
		transition.Provenance.Provider != handoff.Producer.Provider ||
		transition.Provenance.Version != handoff.Producer.Version ||
		transition.Provenance.Operation != handoff.Producer.Operation ||
		transition.Scenario != handoff.Scenario.ID {
		t.Fatalf("typed transition authority = %#v", transition)
	}
	if transition.Target == nil || transition.Target.Label != "service.Start" ||
		transition.Target.Path != "service.go" || transition.Target.Line != 20 ||
		transition.Target.Symbol != "service.Start" {
		t.Fatalf("exact Canvas handoff target = %#v", transition.Target)
	}
	collectOpenablePaths(data)
	for _, path := range []string{"main.go", "service.go"} {
		if !containsString(data.OpenablePaths, path) {
			t.Fatalf("entry-handoff source path %q is not openable: %#v", path, data.OpenablePaths)
		}
	}
	// Frontier is explicit: the handoff from service.Start exists locally
	// but is not connected to the entry path — honest disconnect.
	if group.Frontier.Ordering != "not_established" ||
		len(group.Frontier.Unresolved) == 0 {
		t.Fatalf("frontier = %#v", group.Frontier)
	}
	// Round-trip drift rejection.
	if err := ValidateEntrypointHandoffGroups(data.ArchitectureCanvas, data.ArchitectureGrounding, data.RepositoryGraph, groups); err != nil {
		t.Fatalf("ValidateEntrypointHandoffGroups: %v", err)
	}
	group.Version = EntrypointHandoffGroupVersion - 1
	if err := ValidateEntrypointHandoffGroups(data.ArchitectureCanvas, data.ArchitectureGrounding, data.RepositoryGraph, groups); err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("historical group version error = %v", err)
	}
	group.Version = EntrypointHandoffGroupVersion
	group.EntryHandoffs[0].EvidenceRef.ID = "entry-handoff-tampered"
	if err := ValidateEntrypointHandoffGroups(data.ArchitectureCanvas, data.ArchitectureGrounding, data.RepositoryGraph, groups); err == nil {
		t.Fatal("ValidateEntrypointHandoffGroups accepted a drifted D210 relation ref")
	}
	group.EntryHandoffs[0].EvidenceRef.ID = handoff.ID
	group.EntryHandoffs[0].Path = "mutated.go"
	if err := ValidateEntrypointHandoffGroups(data.ArchitectureCanvas, data.ArchitectureGrounding, data.RepositoryGraph, groups); err == nil {
		t.Fatal("ValidateEntrypointHandoffGroups accepted drifted group")
	}
}

func TestEntrypointHandoffGroupsRemainExactAfterPresentationLocalization(t *testing.T) {
	t.Parallel()

	canonical := entrypointHandoffGroupFixture()
	canonical.FormatVersion = CurrentFormatVersion
	if err := ensureEntrypointHandoffGroups(canonical); err != nil {
		t.Fatalf("ensure canonical groups: %v", err)
	}
	canonicalEntry := canonical.ArchitectureCanvas.EntryHandoffGroups[0].Entry
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
	if got := projected.ArchitectureCanvas.EntryHandoffGroups[0].Entry; !reflect.DeepEqual(got, canonicalEntry) {
		t.Fatalf("presentation localization changed persisted entry identity:\ngot  %#v\nwant %#v", got, canonicalEntry)
	}
	if _, err := RenderHTML(projected); err != nil {
		t.Fatalf("RenderHTML rejected localized report with exact entry handoff groups: %v", err)
	}
}

func TestEnsureEntrypointHandoffGroupsAbsentWithoutProcessEntry(t *testing.T) {
	data := entrypointHandoffGroupFixture()
	data.ArchitectureCanvas.BehaviorAnchors = nil
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		t.Fatalf("ensureEntrypointHandoffGroups: %v", err)
	}
	if data.ArchitectureCanvas.EntryHandoffGroups != nil {
		t.Fatalf("groups must be absent without a process entry: %#v", data.ArchitectureCanvas.EntryHandoffGroups)
	}
}

func TestCanvasBehaviorHandoffAloneCannotCreateEntrypointHandoffGroup(t *testing.T) {
	t.Parallel()

	data := entrypointHandoffGroupFixture()
	data.ArchitectureGrounding = nil
	if len(data.ArchitectureCanvas.StructuralEdges) == 0 {
		t.Fatal("fixture requires an adversarial Canvas behavior handoff")
	}
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		t.Fatalf("ensureEntrypointHandoffGroups: %v", err)
	}
	if data.ArchitectureCanvas.EntryHandoffGroups != nil {
		t.Fatalf("Canvas behavior handoff became an entry-handoff group: %#v", data.ArchitectureCanvas.EntryHandoffGroups)
	}
}

func TestEntrypointHandoffGroupsPersistOnlyOnArchitectureCanvas(t *testing.T) {
	t.Parallel()

	data := entrypointHandoffGroupFixture()
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	wireJSON := string(encoded)
	if !strings.Contains(wireJSON, `"entry_handoff_groups"`) ||
		!strings.Contains(wireJSON, `"entry_handoffs"`) ||
		strings.Contains(wireJSON, `"mechanism_fragments"`) ||
		strings.Contains(wireJSON, `"mechanism_fragment"`) {
		t.Fatalf("entry-handoff wire keys are not exclusive: %s", wireJSON)
	}
	var wire struct {
		ArchitectureCanvas struct {
			EntryHandoffGroups []EntrypointHandoffGroup `json:"entry_handoff_groups"`
		} `json:"architecture_canvas"`
		LegacyMechanismFragments json.RawMessage `json:"mechanism_fragments"`
		LegacyMechanismFragment  json.RawMessage `json:"mechanism_fragment"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.ArchitectureCanvas.EntryHandoffGroups) != 1 ||
		wire.LegacyMechanismFragments != nil || wire.LegacyMechanismFragment != nil {
		t.Fatalf(
			"entry-handoff wire authority = %#v, legacy plural=%s singular=%s",
			wire.ArchitectureCanvas.EntryHandoffGroups,
			wire.LegacyMechanismFragments,
			wire.LegacyMechanismFragment,
		)
	}
	persisted := wire.ArchitectureCanvas.EntryHandoffGroups[0]
	if strings.Count(wireJSON, `"component_ids"`) != 3 ||
		!reflect.DeepEqual(persisted.Entry.ComponentIDs, []componentmap.ComponentID{"component-app"}) ||
		len(persisted.EntryHandoffs) != 1 ||
		!reflect.DeepEqual(persisted.EntryHandoffs[0].ComponentIDs, []componentmap.ComponentID{"component-app"}) {
		t.Fatalf("transition-level component_ids wire shape = %#v; JSON=%s", persisted, wireJSON)
	}
}

func TestEnsureEntrypointHandoffGroupsRejectsPersistedProjectionOnHistoricalCanvas(t *testing.T) {
	t.Parallel()

	data := entrypointHandoffGroupFixture()
	groups, err := ProjectEntrypointHandoffGroups(data.ArchitectureCanvas, data.ArchitectureGrounding, data.RepositoryGraph)
	if err != nil {
		t.Fatal(err)
	}
	data.ArchitectureCanvas.Version--
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		t.Fatalf("historical canvas without a v1 projection should remain renderable: %v", err)
	}
	data.ArchitectureCanvas.EntryHandoffGroups = groups
	if err := ensureEntrypointHandoffGroups(data); err == nil ||
		!strings.Contains(err.Error(), "unsupported canvas version") {
		t.Fatalf("persisted v1 projection on historical canvas error = %v", err)
	}
}

func TestEnsureEntrypointHandoffGroupsUsesExactEntryHandoffOutsideCanvas(t *testing.T) {
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
				ID: "shared-callee", Name: "Second exact callee owner",
				SharedMembers: []componentmap.Candidate{{
					ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "shared-callee-member"},
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
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		t.Fatal(err)
	}
	if len(data.ArchitectureCanvas.EntryHandoffGroups) != 1 ||
		len(data.ArchitectureCanvas.EntryHandoffGroups[0].EntryHandoffs) != 1 {
		t.Fatalf("groups = %#v, want only the exact D210 handoff; Canvas and association rows must stay out", data.ArchitectureCanvas.EntryHandoffGroups)
	}
	group := data.ArchitectureCanvas.EntryHandoffGroups[0]
	if !reflect.DeepEqual(group.ComponentIDs, []componentmap.ComponentID{
		"application", "callee", "shared-callee", "shared-entry",
	}) {
		t.Fatalf("exact component joins = %#v", group)
	}
	if !reflect.DeepEqual(group.Entry.ComponentIDs, []componentmap.ComponentID{
		"application", "shared-entry",
	}) {
		t.Fatalf("exact entry component joins = %#v", group.Entry.ComponentIDs)
	}
	transition := group.EntryHandoffs[0]
	handoff := grounding.EntryHandoffs[0]
	if transition.ClaimKind != "direct_static_call" ||
		transition.Path != handoff.RepresentativeCallsite.Path ||
		transition.Line != handoff.RepresentativeCallsite.Line ||
		transition.Column != handoff.RepresentativeCallsite.Column ||
		transition.Symbol != handoff.Callee.Name ||
		!reflect.DeepEqual(transition.ComponentIDs, []componentmap.ComponentID{"callee", "shared-callee"}) ||
		transition.WitnessCount != handoff.WitnessCount ||
		transition.Evidence != handoff.Producer.Provider+" "+handoff.Producer.Version+" "+handoff.Producer.Operation ||
		transition.EvidenceRef == nil || transition.EvidenceRef.Kind != entrypointEvidenceRefEntryHandoff ||
		transition.EvidenceRef.ID != handoff.ID || transition.Provenance == nil ||
		transition.Provenance.Provider != handoff.Producer.Provider ||
		transition.Provenance.Version != handoff.Producer.Version ||
		transition.Provenance.Operation != handoff.Producer.Operation ||
		transition.Scenario != handoff.Scenario.ID ||
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
	withoutCallee.EntryHandoffGroups = nil
	withoutCallee.Components = append(
		[]ArchitectureComponent(nil),
		canvas.Components[0],
		canvas.Components[3],
		canvas.Components[4],
		canvas.Components[5],
	)
	withoutCalleeGroups, err := projectEntrypointHandoffGroupsForProduct(&withoutCallee, &grounding, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(withoutCalleeGroups) != 1 || len(withoutCalleeGroups[0].EntryHandoffs) == 0 ||
		withoutCalleeGroups[0].EntryHandoffs[0].Target == nil ||
		withoutCalleeGroups[0].EntryHandoffs[0].Target.Symbol != "" ||
		len(withoutCalleeGroups[0].EntryHandoffs[0].ComponentIDs) != 0 {
		t.Fatalf("unjoined target published local symbol: %#v", withoutCalleeGroups)
	}
	unjoinedJSON, err := json.Marshal(withoutCalleeGroups[0].EntryHandoffs[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unjoinedJSON), `"component_ids":[]`) {
		t.Fatalf("unjoined handoff must publish an explicit empty component_ids array: %s", unjoinedJSON)
	}
	if len(canvas.StructuralEdges) != 2 || canvas.StructuralEdges[0].ID != "legacy-overlap" {
		t.Fatalf("exact entry handoff mutated canonical Canvas: %#v", canvas.StructuralEdges)
	}
	if err := validateEntrypointHandoffGroupsForProduct(
		canvas,
		data.ArchitectureCanvas.EntryHandoffGroups,
		&grounding,
		data.RepositoryGraph,
	); err != nil {
		t.Fatalf("grounded group validation: %v", err)
	}
}

func TestEntrypointHandoffJoinsExactDeclarationIdentity(t *testing.T) {
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
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		t.Fatal(err)
	}
	if len(canvas.EntryHandoffGroups) != 1 ||
		canvas.EntryHandoffGroups[0].Entry.Symbol != handoff.ProcessEntrypoint.ID {
		t.Fatalf("same-location entry handoff identity join = %#v, want only exact declaration", canvas.EntryHandoffGroups)
	}
}

func TestProjectEntrypointHandoffGroupsRejectsMalformedD210Authority(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(*ArchitectureEntryHandoff)
	}{
		{name: "relation id", mutate: func(handoff *ArchitectureEntryHandoff) {
			handoff.ID = "entry-handoff-tampered"
		}},
		{name: "producer operation", mutate: func(handoff *ArchitectureEntryHandoff) {
			handoff.Producer.Operation = "connect_architecture_anchors"
			handoff.ID = architectureEntryHandoffID(*handoff)
		}},
		{name: "scenario", mutate: func(handoff *ArchitectureEntryHandoff) {
			handoff.Scenario.ID = "go:unknown"
			handoff.ID = architectureEntryHandoffID(*handoff)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := entrypointHandoffGroupFixture()
			test.mutate(&data.ArchitectureGrounding.EntryHandoffs[0])
			if _, err := ProjectEntrypointHandoffGroups(data.ArchitectureCanvas, data.ArchitectureGrounding, data.RepositoryGraph); err == nil ||
				!strings.Contains(err.Error(), "not exact D210 evidence") {
				t.Fatalf("malformed D210 authority error = %v", err)
			}
		})
	}
}

func TestEnsureEntrypointHandoffGroupsProjectsEveryProcessEntryDeterministically(t *testing.T) {
	t.Parallel()

	var canonical []EntrypointHandoffGroup
	for _, reverse := range []bool{false, true} {
		data := entrypointHandoffGroupFixture()
		helperMember := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "helper-main"}
		helperRunMember := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "helper-run"}
		helperEntry := ArchitectureAnchorMember{
			ID: "example/helper.main", Package: "example/helper", Name: "main",
			Location: evidence.Location{Path: "cmd/helper/main.go", Line: 7, Column: 1},
		}
		helperRun := ArchitectureAnchorMember{
			ID: "example/helper.run", Package: "example/helper", Name: "run",
			Location: evidence.Location{Path: "cmd/helper/run.go", Line: 3, Column: 1},
		}
		data.ArchitectureCanvas.BehaviorAnchors = append(
			data.ArchitectureCanvas.BehaviorAnchors,
			componentmap.BehaviorAnchor{
				ID:        "secondary-entry-anchor",
				Kind:      componentmap.AnchorProcessEntry,
				ProofMode: componentmap.AnchorProofProcessEntry,
				Label:     "different process entry",
				Location:  helperEntry.Location,
				Scenario:  componentmap.ScenarioContext{ID: "go:linux/amd64:tags="},
				MemberIDs: []componentmap.MemberID{helperMember},
			},
		)
		data.ArchitectureCanvas.Components = append(data.ArchitectureCanvas.Components, ArchitectureComponent{
			ID: "component-helper", Name: "Helper",
			Members: []componentmap.Candidate{{
				ID: helperMember, Role: componentmap.CandidateRoleConceptualMember, Name: "helper.main",
				Facts: []componentmap.LocalFact{{
					Kind: componentmap.FactDeclaration, Value: helperEntry.ID, Location: &helperEntry.Location,
				}},
			}, {
				ID:   helperRunMember,
				Role: componentmap.CandidateRoleConceptualMember, Name: "helper.run",
				Facts: []componentmap.LocalFact{{
					Kind: componentmap.FactDeclaration, Value: helperRun.ID, Location: &helperRun.Location,
				}},
			}},
		})
		helperHandoff := ArchitectureEntryHandoff{
			ProcessEntrypoint:      helperEntry,
			Callee:                 helperRun,
			RepresentativeCallsite: evidence.Location{Path: "cmd/helper/main.go", Line: 9, Column: 2},
			WitnessCount:           1,
			TargetPackage:          helperRun.Package,
			Scenario:               data.ArchitectureGrounding.EntryHandoffs[0].Scenario,
			Certainty:              evidence.CertaintyStatic,
			Producer:               data.ArchitectureGrounding.EntryHandoffs[0].Producer,
			Limitations:            []string{architectureEntryHandoffLimitation},
		}
		helperHandoff.ID = architectureEntryHandoffID(helperHandoff)
		data.ArchitectureGrounding.EntryHandoffs = append(data.ArchitectureGrounding.EntryHandoffs, helperHandoff)
		if reverse {
			anchors := data.ArchitectureCanvas.BehaviorAnchors
			anchors[0], anchors[len(anchors)-1] = anchors[len(anchors)-1], anchors[0]
			handoffs := data.ArchitectureGrounding.EntryHandoffs
			handoffs[0], handoffs[len(handoffs)-1] = handoffs[len(handoffs)-1], handoffs[0]
		}
		if err := ensureEntrypointHandoffGroups(data); err != nil {
			t.Fatal(err)
		}
		groups := data.ArchitectureCanvas.EntryHandoffGroups
		if len(groups) != 2 {
			t.Fatalf("groups (reverse=%t) = %#v, want two", reverse, groups)
		}
		if groups[0].ID == "" || groups[1].ID == "" || groups[0].ID == groups[1].ID {
			t.Fatalf("group IDs (reverse=%t) = %#v", reverse, groups)
		}
		componentsByEntry := make(map[string][]componentmap.ComponentID, len(groups))
		for _, group := range groups {
			componentsByEntry[group.Entry.Path] = group.ComponentIDs
		}
		if got := componentsByEntry["main.go"]; len(got) != 1 || got[0] != "component-app" {
			t.Fatalf("main component IDs (reverse=%t) = %#v", reverse, got)
		}
		if got := componentsByEntry["cmd/helper/main.go"]; len(got) != 1 || got[0] != "component-helper" {
			t.Fatalf("helper component IDs (reverse=%t) = %#v", reverse, got)
		}
		if canonical == nil {
			canonical = append([]EntrypointHandoffGroup(nil), groups...)
		} else if !reflect.DeepEqual(canonical, groups) {
			t.Fatalf("group projection changed with anchor order:\nfirst=%#v\nreverse=%#v", canonical, groups)
		}
	}
}

func TestProjectEntrypointHandoffGroupsUsesEveryExactEntryMemberAndScenario(t *testing.T) {
	t.Parallel()

	entryA := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "entry-a"}
	entryB := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "entry-b"}
	target := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "target"}
	makeEvidence := func(reverse bool) (*ArchitectureCanvas, *ArchitectureGrounding) {
		grounding := architectureGroundingWithEntryHandoff()
		handoff := &grounding.EntryHandoffs[0]
		handoff.ProcessEntrypoint.Location = evidence.Location{Path: "cmd/app/main.go", Line: 9, Column: 6}
		handoff.RepresentativeCallsite = evidence.Location{Path: "cmd/app/main.go", Line: 11, Column: 2}
		handoff.ID = architectureEntryHandoffID(*handoff)
		wrongScenario := *handoff
		wrongScenario.Scenario = architectureGroundingScenario{
			ID: "go:darwin/arm64:tags=", GOOS: "darwin", GOARCH: "arm64", Tags: []string{},
		}
		wrongScenario.ID = architectureEntryHandoffID(wrongScenario)
		grounding.EntryHandoffs = []ArchitectureEntryHandoff{*handoff, wrongScenario}
		if reverse {
			grounding.EntryHandoffs[0], grounding.EntryHandoffs[1] =
				grounding.EntryHandoffs[1], grounding.EntryHandoffs[0]
		}
		memberIDs := []componentmap.MemberID{entryB, entryA}
		if reverse {
			memberIDs[0], memberIDs[1] = memberIDs[1], memberIDs[0]
		}
		canvas := &ArchitectureCanvas{
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
				{ID: "entry-a-component", Members: []componentmap.Candidate{{
					ID: entryA, Facts: []componentmap.LocalFact{{
						Kind: componentmap.FactDeclaration, Value: handoff.ProcessEntrypoint.ID,
						Location: &handoff.ProcessEntrypoint.Location,
					}},
				}}},
				{ID: "entry-b-component", SharedMembers: []componentmap.Candidate{{
					ID: entryB, Facts: []componentmap.LocalFact{{
						Kind: componentmap.FactDeclaration, Value: "example.com/app.other",
						Location: &handoff.ProcessEntrypoint.Location,
					}},
				}}},
				{ID: "target-component", Members: []componentmap.Candidate{{
					ID: target, Facts: []componentmap.LocalFact{{
						Kind: componentmap.FactDeclaration, Value: handoff.Callee.ID,
						Location: &handoff.Callee.Location,
					}},
				}}},
				{ID: "local-remainder", Members: []componentmap.Candidate{{ID: entryA}, {ID: target}}},
			},
		}
		return canvas, &grounding
	}

	firstCanvas, firstGrounding := makeEvidence(false)
	first, err := ProjectEntrypointHandoffGroups(firstCanvas, firstGrounding, nil)
	if err != nil {
		t.Fatal(err)
	}
	reversedCanvas, reversedGrounding := makeEvidence(true)
	reversed, err := ProjectEntrypointHandoffGroups(reversedCanvas, reversedGrounding, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, reversed) {
		t.Fatalf("member order changed groups:\nfirst=%#v\nreversed=%#v", first, reversed)
	}
	if len(first) != 1 || first[0].Entry.Symbol != "" || len(first[0].EntryHandoffs) != 1 {
		t.Fatalf("multi-member exact group = %#v", first)
	}
	wantComponents := []componentmap.ComponentID{"entry-a-component", "entry-b-component", "target-component"}
	if !reflect.DeepEqual(first[0].ComponentIDs, wantComponents) {
		t.Fatalf("component IDs = %#v, want %#v (remainder excluded)", first[0].ComponentIDs, wantComponents)
	}
	if handoff := first[0].EntryHandoffs[0]; handoff.Scenario != "go:linux/amd64:tags=" ||
		handoff.EvidenceRef == nil || handoff.EvidenceRef.ID != firstGrounding.EntryHandoffs[0].ID {
		t.Fatalf("cross-scenario handoff leaked or exact ref was lost: %#v", handoff)
	}
}

func TestProjectEntrypointHandoffGroupsRejectsInexactProcessEntry(t *testing.T) {
	t.Parallel()

	data := entrypointHandoffGroupFixture()
	data.ArchitectureCanvas.BehaviorAnchors[0].ProofMode = componentmap.AnchorProofCallTarget
	if _, err := ProjectEntrypointHandoffGroups(data.ArchitectureCanvas, data.ArchitectureGrounding, data.RepositoryGraph); err == nil ||
		!strings.Contains(err.Error(), "process entry anchor") {
		t.Fatalf("inexact process entry error = %v", err)
	}
}

func TestProjectEntrypointHandoffGroupsNeverMultipliesCanvasHandoffsAcrossEntries(t *testing.T) {
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
	for index := 0; index < maxEntrypointHandoffsTotal/2+1; index++ {
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
	groups, err := ProjectEntrypointHandoffGroups(canvas, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if groups != nil {
		t.Fatalf("Canvas-only handoffs became entry-handoff groups: %#v", groups)
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
