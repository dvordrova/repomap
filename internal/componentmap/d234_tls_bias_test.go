package componentmap

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/evidence"
)

// Decision 234 (Archive 9, owner corrective 3 — TLS-family dominance bias):
// a TLS/security component grouped by SHARED unit participation (D231) holds
// only its exact anchors' exclusive members — the shared unit's other
// observations stay owned by their real components. A declaration-family
// anchor is participation/support scope, never package-wide exclusive
// ownership. No TLS string exists in any rule; the bias is the shared
// participation contract itself.
func TestTLSBroadAnchorDoesNotClaimPackageObservations(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	// Add one broad TLS declaration-family anchor inside the shared unit's
	// package (not owning the production members).
	bundle.BehaviorAnchors = append(bundle.BehaviorAnchors, BehaviorAnchor{
		ID: "a-tls", Kind: AnchorSecurityBoundary, Label: "tls transport boundary",
		ProofMode: AnchorProofDeclarationFamily,
		Location:  evidence.Location{Path: "pkg/security/tls.go", Line: 30},
		Scenario:  ScenarioContext{ID: "go:test", Name: "test build"},
		Producer:  evidence.Provenance{Provider: "fixture", Version: "v1", Operation: "classify_tls_boundary"},
		Certainty: evidence.CertaintyStatic,
		// The broad TLS anchor binds ONE symbol inside the shared unit —
		// never the whole unit.
		MemberIDs:   []MemberID{{Kind: MemberSymbol, Value: "member-symbol-prod-handler"}},
		Limitations: []string{"Static fixture evidence; a declaration-family anchor is support scope, not package ownership."},
	})
	request, _, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if len(request.Units) == 0 {
		t.Fatal("fixture has no units catalog")
	}
	prodID := MemberID{Kind: MemberPackage, Value: "member-package-prod-a"}
	unitCatalog, unitErr := CompileUnitCatalog(bundle)
	if unitErr != nil {
		t.Fatalf("unit catalog: %v", unitErr)
	}
	membersByRef := unitCatalogUnitMembersByWireRef(unitCatalog)
	var sharedUnitRef string
	for ref, members := range membersByRef {
		for _, memberID := range members {
			if memberID == prodID {
				sharedUnitRef = ref
			}
		}
	}
	if sharedUnitRef == "" {
		t.Fatal("no unit ref covers the anchored production member")
	}
	// The wire anchor ref is allocated by sorted anchor ID: "a-tls" sorts
	// before "anchor-process", so it receives "a1".
	anchorRef := SynthesisAnchorRef{Kind: AnchorSecurityBoundary, Ref: "a1"}
	// TLS component and Core component share the SAME unit; only the TLS
	// component carries the TLS anchor.
	wire := synthesisWireProposal{Records: []synthesisWireRecord{
		{Kind: synthesisWireSubsystemRecord, Ref: "g1", Name: "Security", Description: ""},
		{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g1",
			Name: "TLS transport", Description: "certificate and transport TLS configuration",
			UnitRefs:   []SynthesisUnitRef{{Kind: MemberPackage, Ref: sharedUnitRef}},
			AnchorRefs: []SynthesisAnchorRef{anchorRef},
			Hypothesis: false,
		},
		{Kind: synthesisWireSubsystemRecord, Ref: "g2", Name: "Application", Description: ""},
		{
			Kind: synthesisWireComponentRecord, SubsystemRef: "g2",
			Name: "Core application", Description: "api and storage",
			UnitRefs:   []SynthesisUnitRef{{Kind: MemberPackage, Ref: sharedUnitRef}},
			AnchorRefs: []SynthesisAnchorRef{},
			Hypothesis: false,
		},
	}}
	response, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "revision-tls", "test", "test", time.Millisecond, response)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if result.Landscape.Fallback {
		t.Fatalf("shared unit must not reject whole-stage: %#v", result.Landscape.Diagnostics)
	}
	var tlsComponent *Component
	var coreComponent *Component
	for _, subsystem := range result.Landscape.Subsystems {
		for index := range subsystem.Components {
			component := &subsystem.Components[index]
			if strings.Contains(strings.ToLower(component.Name), "tls") {
				tlsComponent = component
			}
			if strings.Contains(strings.ToLower(component.Name), "core") {
				coreComponent = component
			}
		}
	}
	if tlsComponent == nil || coreComponent == nil {
		diags := make([]string, 0, len(result.Landscape.Diagnostics))
		for _, diagnostic := range result.Landscape.Diagnostics {
			diags = append(diags, diagnostic.Code)
		}
		t.Fatalf("TLS and core components missing: TLS=%v Core=%v diags=%v", tlsComponent, coreComponent, diags)
	}
	// Shared participation must NOT grant the TLS component exclusive
	// ownership of the whole unit: it has no exclusive members beyond its
	// own anchors, and the unit's other observations stay with Core.
	if len(tlsComponent.Members) != 0 {
		t.Fatalf("TLS component gained exclusive members %v — shared participation must not claim package observations", memberIDs(tlsComponent.Members))
	}
	if len(tlsComponent.SharedMemberIDs) == 0 {
		t.Fatalf("TLS component must participate in the shared unit (SharedMemberIDs empty)")
	}
	if len(coreComponent.Members) == 0 && len(coreComponent.SharedMemberIDs) == 0 {
		t.Fatalf("core component lost all coverage")
	}
	// The TLS anchor itself is exact and counted: the component names it.
	if len(tlsComponent.AnchorIDs) == 0 {
		t.Fatalf("TLS component must retain its exact anchor IDs")
	}
}

func memberIDs(members []Candidate) []MemberID {
	ids := make([]MemberID, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.ID)
	}
	return ids
}
