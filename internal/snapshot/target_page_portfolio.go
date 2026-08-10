package snapshot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	TargetPagePortfolioVersion          = 1
	TargetPagePortfolioArtifactFilename = "target_page_portfolio.v1.json"
	MaxTargetPagePortfolioBytes         = 4 << 20

	maxTargetPageRunIDBytes = 255
)

// TargetPageState is the closed publication state of one selected target.
type TargetPageState string

const (
	TargetPageReady       TargetPageState = "ready"
	TargetPageUnavailable TargetPageState = "unavailable"
)

// TargetPageUnavailableCode deliberately records no runtime error prose.
// Detailed failures remain local console diagnostics.
type TargetPageUnavailableCode string

const (
	TargetPageUnavailableTargetRunFailed TargetPageUnavailableCode = "target_run_failed"
)

// TargetPageOutcome is the backend-only input used after every selected
// target run has reached a terminal state. Default ownership and target order
// are always restored from TargetRunContainer rather than accepted here.
type TargetPageOutcome struct {
	TargetRef       string
	State           TargetPageState
	RunID           string
	UnavailableCode TargetPageUnavailableCode
}

// TargetPage is one selected target's sibling-page handoff. A ready page owns
// only a safe sibling run identifier. An unavailable page owns only its closed
// failure code, so no partial run can become navigable authority.
type TargetPage struct {
	TargetRef       string                    `json:"target_ref"`
	Default         bool                      `json:"default"`
	State           TargetPageState           `json:"state"`
	RunID           string                    `json:"run_id,omitempty"`
	UnavailableCode TargetPageUnavailableCode `json:"unavailable_code,omitempty"`
}

// TargetPagePortfolio is the sealed cross-page index for one selected target
// container. It intentionally contains no path, display prose, report digest,
// or manifest digest: sibling manifests bind the exact container and portfolio
// artifact bytes without introducing a report/manifest hash cycle.
type TargetPagePortfolio struct {
	Version                  int          `json:"version"`
	TargetRunContainerSHA256 string       `json:"target_run_container_sha256"`
	TargetCatalogRef         string       `json:"target_catalog_ref"`
	Targets                  []TargetPage `json:"targets"`
	SHA256                   string       `json:"sha256"`
}

// TargetPageSiblingAuthority is reconstructed from one successful sibling's
// verified run manifest. The two artifact digests are hashes of the exact
// canonical artifact bytes, not either artifact's internal self-seal.
type TargetPageSiblingAuthority struct {
	RunID                             string
	AnalysisTargetRef                 string
	TargetRunContainerArtifactSHA256  string
	TargetPagePortfolioArtifactSHA256 string
}

// BuildTargetPagePortfolio restores canonical container order, derives the
// sole default marker, and seals one terminal outcome for every selected
// target. It performs no filesystem access.
func BuildTargetPagePortfolio(
	container TargetRunContainer,
	outcomes []TargetPageOutcome,
) (TargetPagePortfolio, error) {
	if err := container.Validate(); err != nil {
		return TargetPagePortfolio{}, fmt.Errorf("target page portfolio: container: %w", err)
	}
	if len(outcomes) != len(container.Targets) {
		return TargetPagePortfolio{}, fmt.Errorf("target page portfolio: outcome set does not cover selected targets")
	}

	byTargetRef := make(map[string]TargetPageOutcome, len(outcomes))
	for _, outcome := range outcomes {
		if !validTargetPageTargetRef(outcome.TargetRef) {
			return TargetPagePortfolio{}, fmt.Errorf("target page portfolio: outcome has invalid target ref")
		}
		if _, duplicate := byTargetRef[outcome.TargetRef]; duplicate {
			return TargetPagePortfolio{}, fmt.Errorf("target page portfolio: duplicate outcome target ref")
		}
		byTargetRef[outcome.TargetRef] = outcome
	}

	portfolio := TargetPagePortfolio{
		Version:                  TargetPagePortfolioVersion,
		TargetRunContainerSHA256: container.SHA256,
		TargetCatalogRef:         container.CatalogRef,
		Targets:                  make([]TargetPage, 0, len(container.Targets)),
	}
	for _, projection := range container.Targets {
		outcome, found := byTargetRef[projection.Target.Ref]
		if !found {
			return TargetPagePortfolio{}, fmt.Errorf("target page portfolio: selected target outcome is missing")
		}
		portfolio.Targets = append(portfolio.Targets, TargetPage{
			TargetRef:       projection.Target.Ref,
			Default:         projection.Target.Ref == container.DefaultTargetRef,
			State:           outcome.State,
			RunID:           outcome.RunID,
			UnavailableCode: outcome.UnavailableCode,
		})
	}

	digest, err := targetPagePortfolioDigest(portfolio)
	if err != nil {
		return TargetPagePortfolio{}, err
	}
	portfolio.SHA256 = digest
	if err := portfolio.ValidateAgainstContainer(container); err != nil {
		return TargetPagePortfolio{}, err
	}
	if _, err := portfolio.CanonicalJSON(); err != nil {
		return TargetPagePortfolio{}, err
	}
	return portfolio, nil
}

// Validate verifies the standalone closed shape and self-seal. The exact
// selected target order and default authority are additionally verified by
// ValidateAgainstContainer.
func (portfolio TargetPagePortfolio) Validate() error {
	if portfolio.Version != TargetPagePortfolioVersion ||
		!validTargetPageDigest(portfolio.TargetRunContainerSHA256) ||
		!validTargetPageCatalogRef(portfolio.TargetCatalogRef) ||
		len(portfolio.Targets) == 0 {
		return fmt.Errorf("target page portfolio: invalid identity")
	}

	seenTargetRefs := make(map[string]struct{}, len(portfolio.Targets))
	seenRunIDs := make(map[string]struct{}, len(portfolio.Targets))
	defaultCount := 0
	readyCount := 0
	for index, page := range portfolio.Targets {
		if !validTargetPageTargetRef(page.TargetRef) {
			return fmt.Errorf("target page portfolio: target %d has invalid ref", index)
		}
		if _, duplicate := seenTargetRefs[page.TargetRef]; duplicate {
			return fmt.Errorf("target page portfolio: duplicate target ref")
		}
		seenTargetRefs[page.TargetRef] = struct{}{}
		if page.Default {
			defaultCount++
		}

		switch page.State {
		case TargetPageReady:
			readyCount++
			if err := ValidateTargetPageRunID(page.RunID); err != nil {
				return fmt.Errorf("target page portfolio: target %d: %w", index, err)
			}
			if page.UnavailableCode != "" {
				return fmt.Errorf("target page portfolio: ready target has unavailable code")
			}
			if _, duplicate := seenRunIDs[page.RunID]; duplicate {
				return fmt.Errorf("target page portfolio: duplicate ready run id")
			}
			seenRunIDs[page.RunID] = struct{}{}
		case TargetPageUnavailable:
			if page.RunID != "" || page.UnavailableCode != TargetPageUnavailableTargetRunFailed {
				return fmt.Errorf("target page portfolio: unavailable target has invalid closed state")
			}
		default:
			return fmt.Errorf("target page portfolio: target has invalid state")
		}
	}
	if defaultCount != 1 {
		return fmt.Errorf("target page portfolio: requires exactly one default target")
	}
	if readyCount == 0 {
		return fmt.Errorf("target page portfolio: requires at least one ready sibling")
	}
	if !validTargetPageDigest(portfolio.SHA256) {
		return fmt.Errorf("target page portfolio: invalid seal")
	}
	want, err := targetPagePortfolioDigest(portfolio)
	if err != nil {
		return err
	}
	if portfolio.SHA256 != want {
		return fmt.Errorf("target page portfolio: seal binding mismatch")
	}
	return nil
}

// ValidateAgainstContainer binds the standalone portfolio to the exact
// selected set, canonical catalog order, and backend-owned default marker.
func (portfolio TargetPagePortfolio) ValidateAgainstContainer(container TargetRunContainer) error {
	if err := portfolio.Validate(); err != nil {
		return err
	}
	if err := container.Validate(); err != nil {
		return fmt.Errorf("target page portfolio: container: %w", err)
	}
	if portfolio.TargetRunContainerSHA256 != container.SHA256 ||
		portfolio.TargetCatalogRef != container.CatalogRef ||
		len(portfolio.Targets) != len(container.Targets) {
		return fmt.Errorf("target page portfolio: container binding mismatch")
	}
	for index, projection := range container.Targets {
		page := portfolio.Targets[index]
		if page.TargetRef != projection.Target.Ref ||
			page.Default != (projection.Target.Ref == container.DefaultTargetRef) {
			return fmt.Errorf("target page portfolio: target %d container projection mismatch", index)
		}
	}
	return nil
}

// ValidateSiblingAuthorities proves that every ready page has exactly one
// successful sibling manifest bound to the same target, container artifact,
// and byte-identical portfolio artifact. Unavailable targets must have none.
func (portfolio TargetPagePortfolio) ValidateSiblingAuthorities(
	container TargetRunContainer,
	authorities []TargetPageSiblingAuthority,
) error {
	if err := portfolio.ValidateAgainstContainer(container); err != nil {
		return err
	}
	containerArtifactSHA256, err := targetRunContainerArtifactSHA256(container)
	if err != nil {
		return err
	}
	portfolioArtifactSHA256, err := portfolio.ArtifactSHA256()
	if err != nil {
		return err
	}

	readyByTargetRef := make(map[string]TargetPage)
	for _, page := range portfolio.Targets {
		if page.State == TargetPageReady {
			readyByTargetRef[page.TargetRef] = page
		}
	}
	if len(authorities) != len(readyByTargetRef) {
		return fmt.Errorf("target page portfolio: sibling authority set does not cover ready targets")
	}

	seenTargetRefs := make(map[string]struct{}, len(authorities))
	seenRunIDs := make(map[string]struct{}, len(authorities))
	for index, authority := range authorities {
		if !validTargetPageTargetRef(authority.AnalysisTargetRef) {
			return fmt.Errorf("target page portfolio: sibling %d has invalid target ref", index)
		}
		if err := ValidateTargetPageRunID(authority.RunID); err != nil {
			return fmt.Errorf("target page portfolio: sibling %d: %w", index, err)
		}
		if !validTargetPageDigest(authority.TargetRunContainerArtifactSHA256) ||
			!validTargetPageDigest(authority.TargetPagePortfolioArtifactSHA256) {
			return fmt.Errorf("target page portfolio: sibling %d has invalid artifact binding", index)
		}
		if _, duplicate := seenTargetRefs[authority.AnalysisTargetRef]; duplicate {
			return fmt.Errorf("target page portfolio: duplicate sibling target ref")
		}
		seenTargetRefs[authority.AnalysisTargetRef] = struct{}{}
		if _, duplicate := seenRunIDs[authority.RunID]; duplicate {
			return fmt.Errorf("target page portfolio: duplicate sibling run id")
		}
		seenRunIDs[authority.RunID] = struct{}{}

		page, found := readyByTargetRef[authority.AnalysisTargetRef]
		if !found || page.RunID != authority.RunID ||
			authority.TargetRunContainerArtifactSHA256 != containerArtifactSHA256 ||
			authority.TargetPagePortfolioArtifactSHA256 != portfolioArtifactSHA256 {
			return fmt.Errorf("target page portfolio: sibling %d binding mismatch", index)
		}
	}
	return nil
}

// ValidateTargetPageRunID accepts one portable safe path segment. It rejects
// traversal, separators, escaping syntax, hidden names, and ambiguous trailing
// punctuation rather than attempting to clean an unsafe value.
func ValidateTargetPageRunID(runID string) error {
	if runID == "" || len(runID) > maxTargetPageRunIDBytes || runID != strings.TrimSpace(runID) ||
		!targetPageRunIDAlphanumeric(runID[0]) || !targetPageRunIDAlphanumeric(runID[len(runID)-1]) {
		return fmt.Errorf("target page portfolio: invalid sibling run id")
	}
	for index := 1; index < len(runID)-1; index++ {
		character := runID[index]
		if !targetPageRunIDAlphanumeric(character) && character != '-' && character != '_' && character != '.' {
			return fmt.Errorf("target page portfolio: invalid sibling run id")
		}
	}
	base, _, _ := strings.Cut(strings.ToUpper(runID), ".")
	if targetPageRunIDReservedOnWindows(base) {
		return fmt.Errorf("target page portfolio: invalid sibling run id")
	}
	return nil
}

// CanonicalJSON returns the exact compact artifact bytes copied unchanged to
// every ready sibling directory.
func (portfolio TargetPagePortfolio) CanonicalJSON() ([]byte, error) {
	if err := portfolio.Validate(); err != nil {
		return nil, err
	}
	wire, err := json.Marshal(portfolio)
	if err != nil {
		return nil, fmt.Errorf("target page portfolio: encode artifact: %w", err)
	}
	if len(wire) > MaxTargetPagePortfolioBytes {
		return nil, fmt.Errorf("target page portfolio: artifact exceeds bounded envelope")
	}
	return wire, nil
}

// ArtifactSHA256 returns the digest of the exact canonical artifact bytes.
// This is the digest sibling manifests bind; SHA256 is the internal self-seal.
func (portfolio TargetPagePortfolio) ArtifactSHA256() (string, error) {
	wire, err := portfolio.CanonicalJSON()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

// DecodeTargetPagePortfolio accepts only exact canonical artifact bytes.
func DecodeTargetPagePortfolio(data []byte) (TargetPagePortfolio, error) {
	if len(data) == 0 || len(data) > MaxTargetPagePortfolioBytes {
		return TargetPagePortfolio{}, fmt.Errorf("target page portfolio: artifact exceeds bounded envelope")
	}
	var portfolio TargetPagePortfolio
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&portfolio); err != nil {
		return TargetPagePortfolio{}, fmt.Errorf("target page portfolio: invalid artifact JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return TargetPagePortfolio{}, fmt.Errorf("target page portfolio: invalid trailing artifact data")
	}
	if err := portfolio.Validate(); err != nil {
		return TargetPagePortfolio{}, err
	}
	canonical, err := portfolio.CanonicalJSON()
	if err != nil {
		return TargetPagePortfolio{}, err
	}
	if !bytes.Equal(data, canonical) {
		return TargetPagePortfolio{}, fmt.Errorf("target page portfolio: artifact is not canonical")
	}
	return portfolio, nil
}

func targetPagePortfolioDigest(portfolio TargetPagePortfolio) (string, error) {
	identity := portfolio
	identity.SHA256 = ""
	wire, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("target page portfolio: encode identity: %w", err)
	}
	digest := sha256.Sum256(append([]byte("target-page-portfolio-v1\x00"), wire...))
	return hex.EncodeToString(digest[:]), nil
}

func targetRunContainerArtifactSHA256(container TargetRunContainer) (string, error) {
	wire, err := container.CanonicalJSON()
	if err != nil {
		return "", fmt.Errorf("target page portfolio: encode container artifact: %w", err)
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

func validTargetPageDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validTargetPageCatalogRef(value string) bool {
	return validTargetPageRef(value, "atc-")
}

func validTargetPageTargetRef(value string) bool {
	return validTargetPageRef(value, "at-")
}

func validTargetPageRef(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+24 {
		return false
	}
	hexPart := strings.TrimPrefix(value, prefix)
	if strings.ToLower(hexPart) != hexPart {
		return false
	}
	decoded, err := hex.DecodeString(hexPart)
	return err == nil && len(decoded) == 12
}

func targetPageRunIDAlphanumeric(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

func targetPageRunIDReservedOnWindows(base string) bool {
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
		base[3] >= '1' && base[3] <= '9'
}
