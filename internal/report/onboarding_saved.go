package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	RepositoryOnboardingFile     = "repository_onboarding.json"
	RepositoryOnboardingVersion  = 1
	maxRepositoryOnboardingBytes = 512 << 10
)

// RepositoryOnboardingEditorial is replayable presentation state. The
// preferred artifact is selected locally; model-authored compressions contain
// only memberships over accepted statement IDs. FinalizeRepositoryOnboarding
// remains the local authority for every visible phase and Start Here choice.
type RepositoryOnboardingEditorial struct {
	Version             int                    `json:"version"`
	PreferredArtifactID string                 `json:"preferred_artifact_id,omitempty"`
	Compressions        []NarrativeCompression `json:"compressions"`
}

func readRepositoryOnboardingEditorial(path string) (RepositoryOnboardingEditorial, string) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RepositoryOnboardingEditorial{}, ""
		}
		return RepositoryOnboardingEditorial{}, "repository onboarding unavailable: cannot inspect saved editorial"
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxRepositoryOnboardingBytes {
		return RepositoryOnboardingEditorial{}, "repository onboarding unavailable: saved editorial is not a bounded regular file"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return RepositoryOnboardingEditorial{}, "repository onboarding unavailable: cannot read saved editorial"
	}
	editorial, err := DecodeRepositoryOnboardingEditorial(raw)
	if err != nil {
		return RepositoryOnboardingEditorial{}, fmt.Sprintf("repository onboarding unavailable: %v", err)
	}
	return editorial, ""
}

// DecodeRepositoryOnboardingEditorial strictly decodes one bounded model or
// replay result. Semantic membership is validated later against current
// canonical mechanisms.
func DecodeRepositoryOnboardingEditorial(raw []byte) (RepositoryOnboardingEditorial, error) {
	if len(raw) == 0 || len(raw) > maxRepositoryOnboardingBytes {
		return RepositoryOnboardingEditorial{}, fmt.Errorf("saved editorial is outside bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var editorial RepositoryOnboardingEditorial
	if err := decoder.Decode(&editorial); err != nil {
		return RepositoryOnboardingEditorial{}, fmt.Errorf("saved editorial is invalid JSON: %w", err)
	}
	if err := requireRepositoryOnboardingEOF(decoder); err != nil {
		return RepositoryOnboardingEditorial{}, err
	}
	if editorial.Version != RepositoryOnboardingVersion {
		return RepositoryOnboardingEditorial{}, fmt.Errorf("unsupported saved editorial version %d", editorial.Version)
	}
	if len(editorial.Compressions) > 16 {
		return RepositoryOnboardingEditorial{}, fmt.Errorf("saved editorial has too many mechanism compressions")
	}
	editorial.PreferredArtifactID = strings.TrimSpace(editorial.PreferredArtifactID)
	if len(editorial.PreferredArtifactID) > 256 {
		return RepositoryOnboardingEditorial{}, fmt.Errorf("saved editorial preferred artifact ID is outside bounds")
	}
	return editorial, nil
}

func applyRepositoryOnboardingEditorial(
	data *ReportData,
	editorial RepositoryOnboardingEditorial,
) {
	if data == nil {
		return
	}
	data.StartHereArtifactID = editorial.PreferredArtifactID
	FinalizeRepositoryOnboarding(data, editorial.Compressions)
}

func requireRepositoryOnboardingEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("saved editorial contains trailing JSON")
		}
		return fmt.Errorf("saved editorial has invalid trailing content: %w", err)
	}
	return nil
}
