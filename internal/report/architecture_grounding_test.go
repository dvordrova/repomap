package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
)

func TestValidateArchitectureGroundingAcceptsPersistedVersions(t *testing.T) {
	t.Parallel()

	for _, version := range []int{1, legacyArchitectureGroundingVersion, ArchitectureGroundingVersion} {
		grounding := ArchitectureGrounding{
			Version:             version,
			RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
			GroundingMode:       componentmap.GroundingPackages,
		}
		if err := validateArchitectureGrounding(grounding); err != nil {
			t.Errorf("version %d: %v", version, err)
		}
	}
}
