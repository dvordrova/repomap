package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
)

func TestValidateArchitectureGroundingAcceptsPersistedVersions(t *testing.T) {
	t.Parallel()

	for _, version := range []int{1, legacyArchitectureGroundingVersion, previousArchitectureGroundingVersion, ArchitectureGroundingVersion} {
		grounding := ArchitectureGrounding{
			Version:             version,
			RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
			GroundingMode:       componentmap.GroundingPackages,
		}
		if version == ArchitectureGroundingVersion {
			grounding.Coverage.Complete = true
		}
		if err := validateArchitectureGrounding(grounding); err != nil {
			t.Errorf("version %d: %v", version, err)
		}
	}
}

func TestValidateArchitectureGroundingRequiresTypedProofModeInV4(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithProofMode(componentmap.AnchorProofCallTarget)
	if err := validateArchitectureGrounding(grounding); err != nil {
		t.Fatalf("valid v4 proof mode: %v", err)
	}

	grounding.BehaviorAnchors[0].ProofMode = ""
	if err := validateArchitectureGrounding(grounding); err == nil {
		t.Fatal("missing v4 proof mode must be rejected")
	}
}

func TestValidateArchitectureGroundingRejectsLegacyUntypedAnchors(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithProofMode(componentmap.AnchorProofCallTarget)
	grounding.Version = previousArchitectureGroundingVersion
	grounding.BehaviorAnchors[0].ProofMode = ""
	grounding.Coverage = ArchitectureGroundingCoverage{}
	if err := validateArchitectureGrounding(grounding); err == nil {
		t.Fatal("legacy untyped behavior anchor must not be reinterpreted")
	}
}

func TestValidateArchitectureGroundingKeepsProofModeOrthogonalToNonProcessKind(t *testing.T) {
	t.Parallel()

	grounding := architectureGroundingWithProofMode(componentmap.AnchorProofDeclarationFamily)
	grounding.BehaviorAnchors[0].Kind = componentmap.AnchorLifecycleStart
	if err := validateArchitectureGrounding(grounding); err != nil {
		t.Fatalf("declaration-family lifecycle anchor: %v", err)
	}

	grounding.BehaviorAnchors[0].Kind = componentmap.AnchorProcessEntry
	if err := validateArchitectureGrounding(grounding); err == nil {
		t.Fatal("process entry must use process-entry proof mode")
	}
}

func architectureGroundingWithProofMode(mode componentmap.AnchorProofMode) ArchitectureGrounding {
	location := evidence.Location{Path: "service/start.go", Line: 12, Column: 1}
	declarationFamilyMembers := 0
	if mode == componentmap.AnchorProofDeclarationFamily {
		declarationFamilyMembers = 1
	}
	return ArchitectureGrounding{
		Version:             ArchitectureGroundingVersion,
		RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
		GroundingMode:       componentmap.GroundingBehavior,
		BehaviorAnchors: []ArchitectureBehaviorAnchor{{
			ID: "anchor-1", Kind: componentmap.AnchorLifecycleStart, ProofMode: mode,
			Label: "service start", Location: location,
			Scenario:          architectureGroundingScenario{ID: "scenario-1", GOOS: "linux", GOARCH: "amd64"},
			Certainty:         evidence.CertaintyStatic,
			AssociatedMembers: []ArchitectureAnchorMember{{ID: "member-1", Location: location}},
			Limitations:       []string{"Static local proof only."},
		}},
		Coverage: ArchitectureGroundingCoverage{
			Complete: true, AnchorsConsidered: 1, AnchorsPublished: 1,
			DeclarationFamilyMembersConsidered: declarationFamilyMembers,
			DeclarationFamilyMembersPublished:  declarationFamilyMembers,
		},
	}
}
