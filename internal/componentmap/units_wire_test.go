package componentmap

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestEtcdUnitWireIsBoundedMeasures the Decision 216 wire win on the saved
// etcd-sized fixture: the unit catalog projection must be far smaller than
// the raw candidate list while preserving full coverage.
func TestEtcdUnitWireIsBounded(t *testing.T) {
	data, err := os.ReadFile("testdata/etcd_architecture_parity_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Bundle CandidateBundle `json:"bundle"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	// Upgrade the saved legacy bundle into the current role/proof contract
	// exactly as the structural bridge gate does.
	fixture.Bundle.Version = ContractVersion
	for index := range fixture.Bundle.Candidates {
		fixture.Bundle.Candidates[index].Role = CandidateRoleConceptualMember
	}
	for index := range fixture.Bundle.BehaviorAnchors {
		if fixture.Bundle.BehaviorAnchors[index].Kind == AnchorProcessEntry {
			fixture.Bundle.BehaviorAnchors[index].ProofMode = AnchorProofProcessEntry
		} else {
			fixture.Bundle.BehaviorAnchors[index].ProofMode = AnchorProofCallTarget
		}
	}
	catalog, err := CompileUnitCatalog(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	rawMembers := 0
	for _, candidate := range fixture.Bundle.Candidates {
		if candidate.Role == CandidateRoleConceptualMember {
			rawMembers++
		}
	}
	if catalog.TotalMembers != len(fixture.Bundle.Candidates) {
		t.Fatalf("catalog total members = %d, want %d", catalog.TotalMembers, len(fixture.Bundle.Candidates))
	}
	unitJSON, err := json.Marshal(catalog.WireUnits)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("etcd fixture: raw conceptual members=%d units=%d unit_wire_bytes=%d", rawMembers, len(catalog.WireUnits), len(unitJSON))
	if len(catalog.WireUnits) > targetMaxUnits {
		t.Fatalf("unit catalog exceeds hard bound: %d > %d", len(catalog.WireUnits), targetMaxUnits)
	}
	if len(catalog.WireUnits) == 0 {
		t.Fatal("empty unit catalog")
	}
}

// TestMaximumLegalUnitProposalFitsWithinCeiling proves the maximum legal
// unit-grouping response (8 subsystems, 24 components, every unit referenced
// once per component, bounded anchor refs) serializes well below the global
// 64k provider ceiling (Decision 216 envelope).
func TestMaximumLegalUnitProposalFitsWithinCeiling(t *testing.T) {
	t.Parallel()

	records := make([]synthesisWireRecord, 0, 8+24)
	for subsystem := 0; subsystem < 8; subsystem++ {
		subsystemRef := fmt.Sprintf("g%d", subsystem+1)
		records = append(records, synthesisWireRecord{
			Kind: synthesisWireSubsystemRecord, Ref: subsystemRef,
			Name: "subsystem-" + string(rune('a'+subsystem)),
		})
		for component := 0; component < 3; component++ {
			unitRefs := make([]SynthesisUnitRef, 0, 64)
			for unit := 0; unit < 64; unit++ {
				unitRefs = append(unitRefs, SynthesisUnitRef{Kind: MemberPackage, Ref: fmt.Sprintf("u%d", unit+1)})
			}
			records = append(records, synthesisWireRecord{
				Kind: synthesisWireComponentRecord, SubsystemRef: subsystemRef,
				Name: "component", Description: strings.Repeat("d", 1024),
				UnitRefs: unitRefs, AnchorRefs: []SynthesisAnchorRef{},
				Hypothesis: true,
			})
		}
	}
	wire := synthesisWireProposal{Records: records}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("maximum legal unit proposal bytes = %d", len(encoded))
	if len(encoded) > maxSynthesisResponseBytes {
		t.Fatalf("maximum legal unit proposal %d bytes exceeds ceiling %d", len(encoded), maxSynthesisResponseBytes)
	}
	// The same proposal decoded and resolved must remain valid under the
	// bounded schema (round trip through the exact contract decoder).
	decoded, err := decodeSynthesisWireProposalJSON(encoded)
	if err != nil {
		t.Fatalf("maximum legal unit proposal failed schema decode: %v", err)
	}
	if len(decoded.Records) != len(records) {
		t.Fatalf("decoded record count = %d, want %d", len(decoded.Records), len(records))
	}
}
