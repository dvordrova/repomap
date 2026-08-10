package componentmap

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTelebotLikeNestedRootRolesDeriveScopeWithoutPackageMembership(t *testing.T) {
	t.Parallel()

	rootPackage := unitTestCandidate(
		MemberPackage, "telebot-root-package-private", "gopkg.in/telebot.v3", CandidateRoleConceptualMember, nil,
	)
	startFile := unitTestCandidate(
		MemberFile, "telebot-start-file-private", "bot.go", CandidateRoleStructuralLocator, &rootPackage.ID,
	)
	startSymbol := unitTestCandidate(
		MemberSymbol, "telebot-start-symbol-private", "gopkg.in/telebot.v3.(*Bot).Start", CandidateRoleConceptualMember, &startFile.ID,
	)
	webhookFile := unitTestCandidate(
		MemberFile, "telebot-webhook-file-private", "webhook.go", CandidateRoleStructuralLocator, &rootPackage.ID,
	)
	webhookSymbol := unitTestCandidate(
		MemberSymbol, "telebot-webhook-symbol-private", "gopkg.in/telebot.v3.(*Webhook).ServeHTTP", CandidateRoleConceptualMember, &webhookFile.ID,
	)
	middlewarePackage := unitTestCandidate(
		MemberPackage, "telebot-middleware-package-private", "gopkg.in/telebot.v3/middleware", CandidateRoleConceptualMember, nil,
	)
	reactPackage := unitTestCandidate(
		MemberPackage, "telebot-react-package-private", "gopkg.in/telebot.v3/react", CandidateRoleConceptualMember, nil,
	)
	layoutPackage := unitTestCandidate(
		MemberPackage, "telebot-layout-package-private", "gopkg.in/telebot.v3/layout", CandidateRoleConceptualMember, nil,
	)
	const startAnchorID = "telebot-start-anchor-private"
	const webhookAnchorID = "telebot-webhook-anchor-private"
	bundle := unitTestBundle(
		[]Candidate{
			rootPackage, startFile, startSymbol, webhookFile, webhookSymbol,
			middlewarePackage, reactPackage, layoutPackage,
		},
		[]BehaviorAnchor{
			unitTestAnchor(startAnchorID, AnchorLifecycleStart, startSymbol.ID),
			unitTestAnchor(webhookAnchorID, AnchorRequestDispatchRoot, webhookSymbol.ID),
		},
	)
	catalog, err := buildSynthesisPrivateCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	type component struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		MemberRefs  []string `json:"member_refs"`
		AnchorRefs  []string `json:"anchor_refs"`
	}
	type subsystem struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Components  []component `json:"components"`
	}
	response, err := json.Marshal(struct {
		Subsystems []subsystem `json:"subsystems"`
	}{Subsystems: []subsystem{
		{
			Name: "Core package", Description: "Root responsibilities",
			Components: []component{
				{
					Name: "Lifecycle", Description: "Bot lifecycle",
					MemberRefs: []string{catalog.membersByID[startSymbol.ID].Ref},
					AnchorRefs: []string{catalog.anchorsByID[startAnchorID].Ref},
				},
				{
					Name: "Request handling", Description: "Webhook request dispatch",
					MemberRefs: []string{catalog.membersByID[webhookSymbol.ID].Ref},
					AnchorRefs: []string{catalog.anchorsByID[webhookAnchorID].Ref},
				},
			},
		},
		{
			Name: "Additional modules", Description: "Optional module packages",
			Components: []component{
				{Name: "Middleware", Description: "Middleware", MemberRefs: []string{catalog.membersByID[middlewarePackage.ID].Ref}, AnchorRefs: []string{}},
				{Name: "React", Description: "React", MemberRefs: []string{catalog.membersByID[reactPackage.ID].Ref}, AnchorRefs: []string{}},
				{Name: "Layout", Description: "Layout", MemberRefs: []string{catalog.membersByID[layoutPackage.ID].Ref}, AnchorRefs: []string{}},
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	result, err := RecordSynthesisResponse(bundle, "telebot-nested-parent-scope", "test", "test", 0, response)
	if err != nil {
		t.Fatal(err)
	}
	if result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationAcceptedPartial {
		t.Fatalf("Telebot-like response did not publish: %#v", result.Landscape)
	}
	for _, code := range []string{
		"proposal.supporting_only_unit_coverage_salvaged",
		"proposal.supporting_only_unit_coverage",
		"proposal.empty_primary_scope_coverage",
	} {
		if d241HasDiagnostic(result.Landscape.Diagnostics, code) {
			t.Fatalf("Telebot-like response retained stale coverage diagnostic %q: %#v", code, result.Landscape.Diagnostics)
		}
	}
	counts := result.Membership
	if !counts.Counted || counts.MemberOccurrences != 5 || counts.DistinctMembers != 5 ||
		counts.RequestedPrimaryScope != 4 || counts.CoveredPrimaryScope != 4 ||
		counts.UncoveredPrimaryScope != 0 || counts.CoveredSupportingEvidence != 2 ||
		!reflect.DeepEqual(counts.UncoveredMemberIDs, []MemberID{rootPackage.ID}) {
		t.Fatalf("Telebot-like exact membership/scope = %#v", counts)
	}
	for _, name := range []string{"Lifecycle", "Request handling"} {
		component := d241FindComponent(result.Landscape, name)
		if component == nil || len(component.Members) != 1 ||
			component.Members[0].ID == rootPackage.ID {
			t.Fatalf("derived root package entered %q component membership: %#v", name, component)
		}
	}
	if !d241ContainsMember(result.Landscape.LocalRemainderMemberIDs, rootPackage.ID) {
		t.Fatalf("unselected root package left exact local remainder: %#v", result.Landscape.LocalRemainderMemberIDs)
	}
}
