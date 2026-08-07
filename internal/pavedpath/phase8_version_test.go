package pavedpath

import (
	"testing"
)

// Phase 8 reviewer finding (backend-authority leakage): Paved Paths version
// is backend-owned — a model response that omits it is stamped with
// ProposalVersion, and the prompt no longer asks for a version echo.
func TestDecodeProposalStampsBackendOwnedVersion(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"paths":[{"title":"t","goal":"g","actions":[{"evidence_id":"e1","instruction":"run it"}],"ordering_basis":"editorial"}]}`)
	proposal, err := DecodeProposal(raw)
	if err != nil {
		t.Fatalf("DecodeProposal without version rejected: %v", err)
	}
	if proposal.Version != ProposalVersion {
		t.Fatalf("backend-owned version not stamped: got %d want %d", proposal.Version, ProposalVersion)
	}
	// Legacy echo still validated.
	rawLegacy := []byte(`{"version":1,"paths":[{"title":"t","goal":"g","actions":[{"evidence_id":"e1","instruction":"run it"}],"ordering_basis":"editorial"}]}`)
	if proposal, err := DecodeProposal(rawLegacy); err != nil || proposal.Version != ProposalVersion {
		t.Fatalf("legacy version echo rejected: %v", err)
	}
	// Wrong version still rejected.
	rawWrong := []byte(`{"version":99,"paths":[]}`)
	if _, err := DecodeProposal(rawWrong); err == nil {
		t.Fatalf("wrong version echo accepted")
	}
}
