package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/localization"
)

func TestPresentationInventoryKeepsExactCatalogSurfaceNameOpaque(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		ProjectGuess: "A repository service.",
		ArchitectureCanvas: &ArchitectureCanvas{Surfaces: []ArchitectureSurface{
			{ID: "surface-main", Name: "main", Source: surfaceSourceCatalog},
			{ID: "trace-start", Name: "A request starts here", Source: surfaceSourceTraceStart},
		}},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}

	assertPresentationAddressAbsent(t, prepared, "architecture/surfaces/surface-main/name")
	assertPresentationAddressPresent(t, prepared, "architecture/surfaces/trace-start/name")
}

func TestPresentationInventoryProtectsTypedTriggerProtocolOnlyForItsOwner(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		ProjectGuess: "A repository service.",
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{
			{
				ID:                     "surface-http",
				Transport:              "http",
				UnavailableReason:      "The http server is not available.",
				TraceReadinessReason:   "The http handler can be inspected.",
				TraceUnavailableReason: "The http handoff is unresolved.",
			},
			{
				ID:                "surface-unknown",
				UnavailableReason: "The http server is not available.",
			},
		}},
		ArchitectureCanvas: &ArchitectureCanvas{
			Flows: []ArchitectureFlow{{
				ID:                         "flow-http",
				RelatedComponentSurfaceIDs: []string{"surface-http"},
				CurrentFrontier:            "The http server handoff is unresolved.",
			}},
			Frontiers: []ArchitectureFrontier{{
				ID: "frontier-http", FlowID: "flow-http",
				Reason: "The http server handoff is unresolved.",
			}},
		},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}

	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"surfaces/triggers/surface-http/unavailable_reason",
		"http",
	)
	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"architecture/flows/flow-http/current_frontier",
		"http",
	)
	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"architecture/frontiers/frontier-http/reason",
		"http",
	)
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"surfaces/triggers/surface-unknown/unavailable_reason",
		"http",
	)
}

func TestPresentationInventoryProtectsRelationEnumByProducerIdentity(t *testing.T) {
	t.Parallel()

	newRelation := func(id, provider, operation string) componentmap.LocalRelation {
		return componentmap.LocalRelation{
			ID:   id,
			From: componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "from"},
			To:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "to"},
			Kind: componentmap.StructuralRelationBehaviorHandoff,
			Provenance: []evidence.Provenance{{
				Provider:  provider,
				Operation: operation,
				Detail:    "1 exact bounded_direct_call witness across one package",
			}},
		}
	}
	data := &ReportData{
		ProjectGuess: "A repository service.",
		ArchitectureCanvas: &ArchitectureCanvas{StructuralFacts: []componentmap.LocalRelation{
			newRelation("relation-owned", "go_ssa", "connect_architecture_anchors"),
			newRelation("relation-other", "other", "other_operation"),
		}},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}

	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"architecture/structural_relations/relation-owned/provenance/0/detail",
		"bounded_direct_call",
	)
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"architecture/structural_relations/relation-other/provenance/0/detail",
		"bounded_direct_call",
	)
}

func TestPresentationInventoryScopesRepositoryAliasWithoutInferringOpaqueProse(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		RepoName:     "github.com/casdoor/casdoor",
		ProjectGuess: "Casdoor provides identity services.",
		ArchitectureCanvas: &ArchitectureCanvas{Suggestions: []ArchitectureSuggestion{{
			ID:     "users",
			Reason: "Casdoor exposes user management here.",
		}}},
		StudyMap: &RepositoryStudyMap{
			Brief: RepositoryBrief{DomainTerms: []RepositoryBriefDomainTerm{
				{
					Term:    "MCP",
					Meaning: "Model Context Protocol, a protocol for context exchange.",
				},
				{
					Term:    "WebAuthn",
					Meaning: "Web Authentication, a standard for browser credentials.",
				},
			}},
			Directions: []StudyDirection{{
				ID:       "users",
				Question: "How does Casdoor manage users?",
			}},
		},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}

	// The exact repository path owns its display spelling only within objects
	// that explicitly receive repository protection; it is not a lexical word.
	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"architecture/suggestions/users/reason",
		"Casdoor",
	)
	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"study_direction/users/question",
		"Casdoor",
	)
	// DomainTerm.Meaning is human prose. Punctuation does not turn its leading
	// words into an opaque identity; only the typed Term value is protected when
	// it actually occurs in the prose.
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"brief_domain_term/repository-brief:domain-term:",
		"Model Context Protocol",
	)
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"brief_domain_term/repository-brief:domain-term:",
		"Web Authentication",
	)
}

func TestPresentationInventoryOmitsCatalogRenderedGroundingEnum(t *testing.T) {
	t.Parallel()

	data := &ReportData{Components: []Component{{
		ID: "component-users",
		AnchorGroups: []AnchorGroup{{
			ID: "users-entry", Grounding: anchorGroundingPath,
			ModelNotes: []string{"cmd/main.go is the go_main_function anchor"},
		}},
	}}}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	assertPresentationAddressAbsent(t, prepared, "anchor_groups/users-entry/grounding")
	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"components/component-users/anchor_groups/users-entry/model_notes/0",
		"go_main_function",
	)
}

func TestPresentationInventoryOmitsCatalogRenderedBundleReason(t *testing.T) {
	t.Parallel()

	data := &ReportData{Flows: []FlowData{{
		ID: "users",
		FilesToRead: []FileItem{{
			Path: "controllers/user.go", Reason: "Shows how user updates are handled.",
		}},
		BundleFiles: []FileItem{{
			Path: "controllers/user.go", Reason: "likely_file from candidate_flow",
		}},
	}}}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	assertPresentationAddressAbsent(t, prepared, "flows/users/bundle_files/")
	assertPresentationAddressPresent(t, prepared, "flows/users/files_to_read/")
}

func TestPresentationInventoryDoesNotInferCatalogReasonFromEnglishShape(t *testing.T) {
	t.Parallel()

	const prose = "exact basename match 'Controllers should be translated'"
	data := &ReportData{Flows: []FlowData{{
		ID: "users",
		BundleFiles: []FileItem{{
			Path: "controllers/user.go", Reason: prose,
		}},
	}}}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	assertPresentationAddressPresent(t, prepared, "flows/users/bundle_files/")
	found := false
	for _, field := range prepared.Canonical.Fields {
		if field.Text == prose {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("human bundle prose was inferred to be fixed product copy")
	}
}

func TestPresentationInventoryJoinsGuidedTourToTypedTriggerProtocolOnce(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		ProjectGuess: "A repository service.",
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "surface-http", Transport: "http", Framework: "net/http",
			RelatedTraceID: "flow-http",
		}}},
		GuidedTour: &guidedtour.Story{
			CandidateID: "flow-http",
			Trigger:     "An http request enters the service.",
			Title:       "The http request path",
			Summary:     "The net/http server accepts an http request.",
			GapSummary: []guidedtour.StoryGapSummary{{
				GapIDs: []string{"gap-http"},
				Gaps: []guidedtour.Gap{{
					ID: "gap-http", Label: "http dispatch",
					Detail: "The http server handoff is unresolved.",
				}},
			}},
		},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{
		"guided_tour/flow-http/trigger",
		"guided_tour/flow-http/gaps/gap-http/label",
		"guided_tour/flow-http/gaps/gap-http/detail",
	} {
		assertPresentationFieldProtectsValue(t, prepared, owner, "http")
	}
	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"guided_tour/flow-http/summary",
		"net/http",
	)

	bindings, err := buildPresentationLocalizationBindings(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{
		"guided_tour/flow-http/trigger",
		"guided_tour/flow-http/gaps/gap-http/label",
		"guided_tour/flow-http/gaps/gap-http/detail",
	} {
		fieldID, err := localization.FieldID(
			localization.OwnerPresentationText,
			owner,
			localization.FieldText,
		)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(bindings.byID[fieldID].setters); got != 1 {
			t.Fatalf("%s has %d setters, want one typed registration", owner, got)
		}
	}
}

func TestPresentationInventoryDoesNotInferPackageIdentityFromDisplayName(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		ProjectGuess: "A repository service.",
		Components: []Component{{
			ID: "component-routers", Name: "routers",
			PrimaryPackage: "example.test/repo/routers",
			ModelPurpose:   "The example.test/repo/routers package dispatches requests.",
			AnchorGroups:   []AnchorGroup{{ID: "routers-auth", Path: "routers/auth.go"}},
		}},
		CandidateDirections: []CandidateDirection{
			{
				ID: "owned", Name: "Request routing",
				LikelyFiles: []string{"routers/auth.go"},
				Evidence:    []string{"routers package owns request dispatch"},
			},
			{
				ID: "unrelated", Name: "Router concepts",
				Evidence: []string{"routers can also be an ordinary model label"},
			},
		},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"orientation/directions/owned/evidence/0",
		"routers",
	)
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"orientation/directions/unrelated/evidence/0",
		"routers",
	)
	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"components/component-routers/model_purpose",
		"example.test/repo/routers",
	)
	// Legacy component labels remain model-authored presentation prose even
	// when their spelling happens to match an exact package basename.
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"components/component-routers/name",
		"routers",
	)
}

func assertPresentationAddressAbsent(
	t *testing.T,
	prepared PreparedPresentationLocalization,
	ownerNeedle string,
) {
	t.Helper()
	for _, field := range prepared.Canonical.Fields {
		if strings.Contains(field.OwnerID, ownerNeedle) {
			t.Fatalf("unexpected presentation field %q for opaque address %q", field.ID, ownerNeedle)
		}
	}
}

func assertPresentationAddressPresent(
	t *testing.T,
	prepared PreparedPresentationLocalization,
	ownerNeedle string,
) {
	t.Helper()
	for _, field := range prepared.Canonical.Fields {
		if strings.Contains(field.OwnerID, ownerNeedle) {
			return
		}
	}
	t.Fatalf("missing presentation field for %q", ownerNeedle)
}
