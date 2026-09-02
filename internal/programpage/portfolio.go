// Package programpage owns the persisted, language-neutral binding from exact
// ProgramIndex targets to their published page runs.
package programpage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	Version          = 1
	ArtifactFilename = "program-page-portfolio.json"

	MaxPages              = 4_096
	MaxRunIDBytes         = 255
	AdvisoryArtifactBytes = 8 * 1024 * 1024
	// MaxArtifactBytes is a compatibility sentinel for report readers. Zero
	// means the complete validated local artifact is read without a byte cutoff.
	MaxArtifactBytes = 0
)

const digestDomain = "program-page-portfolio-v1\x00"

// Page binds one exact, independently validated ProgramIndex target to the
// safe sibling run that publishes its page. The target is retained in full so
// consumers never need to recover language, kind, selector, or source scope
// from a report projection.
type Page struct {
	Target programindex.Target `json:"target"`
	RunID  string              `json:"run_id"`
}

// Portfolio is the complete canonical page inventory for one repository run.
// DefaultTargetID is explicit consumer authority; slice position never chooses
// a default target.
type Portfolio struct {
	Version         int    `json:"version"`
	DefaultTargetID string `json:"default_target_id"`
	Pages           []Page `json:"pages"`
	SHA256          string `json:"sha256"`
}

// Build canonicalizes and seals a complete set of validated target pages. It
// performs no filesystem access and never infers a default from page order.
func Build(defaultTargetID string, pages []Page) (Portfolio, error) {
	if len(pages) == 0 {
		return Portfolio{}, fmt.Errorf("program page portfolio: page bound exceeded")
	}
	portfolio := Portfolio{
		Version:         Version,
		DefaultTargetID: defaultTargetID,
		Pages:           clonePages(pages),
	}
	sort.Slice(portfolio.Pages, func(i, j int) bool {
		if portfolio.Pages[i].Target.ID != portfolio.Pages[j].Target.ID {
			return portfolio.Pages[i].Target.ID < portfolio.Pages[j].Target.ID
		}
		return portfolio.Pages[i].RunID < portfolio.Pages[j].RunID
	})
	if err := portfolio.validateShape(); err != nil {
		return Portfolio{}, err
	}
	digest, err := portfolioDigest(portfolio)
	if err != nil {
		return Portfolio{}, err
	}
	portfolio.SHA256 = digest
	if err := portfolio.Validate(); err != nil {
		return Portfolio{}, err
	}
	if _, err := portfolio.CanonicalJSON(); err != nil {
		return Portfolio{}, err
	}
	return portfolio, nil
}

// Snapshot returns a consumer-owned deep copy, including nested target source
// and seed storage.
func (portfolio Portfolio) Snapshot() Portfolio {
	result := portfolio
	result.Pages = clonePages(portfolio.Pages)
	return result
}

// Validate checks the exact target identities, complete default binding,
// canonical ordering, unique safe run bindings, byte bounds, and self-seal.
func (portfolio Portfolio) Validate() error {
	if err := portfolio.validateShape(); err != nil {
		return err
	}
	want, err := portfolioDigest(portfolio)
	if err != nil {
		return err
	}
	if !validSHA256(portfolio.SHA256) || portfolio.SHA256 != want {
		return fmt.Errorf("program page portfolio: sha256 mismatch")
	}
	return nil
}

// CanonicalJSON returns the exact compact bytes suitable for persistence.
func (portfolio Portfolio) CanonicalJSON() ([]byte, error) {
	if err := portfolio.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(portfolio)
	if err != nil {
		return nil, fmt.Errorf("program page portfolio: encode artifact: %w", err)
	}
	return encoded, nil
}

// Decode accepts only exact canonical artifact bytes. Unknown fields,
// trailing values, alternate whitespace, invalid targets, and seal tampering
// all fail closed.
func Decode(encoded []byte) (Portfolio, error) {
	if len(encoded) == 0 {
		return Portfolio{}, fmt.Errorf("program page portfolio: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var portfolio Portfolio
	if err := decoder.Decode(&portfolio); err != nil {
		return Portfolio{}, fmt.Errorf("program page portfolio: decode artifact: %w", err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Portfolio{}, fmt.Errorf("program page portfolio: trailing JSON value")
		}
		return Portfolio{}, fmt.Errorf("program page portfolio: trailing data: %w", err)
	}
	if err := portfolio.Validate(); err != nil {
		return Portfolio{}, err
	}
	canonical, err := portfolio.CanonicalJSON()
	if err != nil {
		return Portfolio{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return Portfolio{}, fmt.Errorf("program page portfolio: artifact is not canonical")
	}
	return portfolio, nil
}

// ValidateRunID accepts one portable safe path segment. Run IDs may be used
// to address sibling publication directories, so traversal, separators,
// escaping syntax, hidden names, and Windows device names are rejected.
func ValidateRunID(runID string) error {
	if runID == "" || len(runID) > MaxRunIDBytes || runID != strings.TrimSpace(runID) ||
		!runIDAlphanumeric(runID[0]) || !runIDAlphanumeric(runID[len(runID)-1]) {
		return fmt.Errorf("program page portfolio: invalid run id")
	}
	for index := 1; index < len(runID)-1; index++ {
		character := runID[index]
		if !runIDAlphanumeric(character) && character != '-' && character != '_' && character != '.' {
			return fmt.Errorf("program page portfolio: invalid run id")
		}
	}
	base, _, _ := strings.Cut(strings.ToUpper(runID), ".")
	if reservedRunIDBase(base) {
		return fmt.Errorf("program page portfolio: invalid run id")
	}
	return nil
}

func (portfolio Portfolio) validateShape() error {
	if portfolio.Version != Version || !validText(portfolio.DefaultTargetID) {
		return fmt.Errorf("program page portfolio: invalid identity")
	}
	if portfolio.Pages == nil || len(portfolio.Pages) == 0 {
		return fmt.Errorf("program page portfolio: page bound exceeded")
	}
	defaultMatches := 0
	runIDs := make(map[string]struct{}, len(portfolio.Pages))
	for position, page := range portfolio.Pages {
		if err := page.Target.Validate(); err != nil {
			return fmt.Errorf("program page portfolio: page %d target: %w", position, err)
		}
		if position > 0 && portfolio.Pages[position-1].Target.ID >= page.Target.ID {
			return fmt.Errorf("program page portfolio: pages are not canonical")
		}
		if page.Target.ID == portfolio.DefaultTargetID {
			defaultMatches++
		}
		if err := ValidateRunID(page.RunID); err != nil {
			return fmt.Errorf("program page portfolio: page %d: %w", position, err)
		}
		if _, duplicate := runIDs[page.RunID]; duplicate {
			return fmt.Errorf("program page portfolio: duplicate run id")
		}
		runIDs[page.RunID] = struct{}{}
	}
	if defaultMatches != 1 {
		return fmt.Errorf("program page portfolio: default target must match exactly one page")
	}
	return nil
}

func portfolioDigest(portfolio Portfolio) (string, error) {
	payload := portfolio.Snapshot()
	payload.SHA256 = ""
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("program page portfolio: encode digest material: %w", err)
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(digestDomain))
	_, _ = digest.Write(encoded)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func clonePages(pages []Page) []Page {
	if pages == nil {
		return nil
	}
	result := make([]Page, len(pages))
	for position, page := range pages {
		result[position] = Page{Target: page.Target.Snapshot(), RunID: page.RunID}
	}
	return result
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func runIDAlphanumeric(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

func reservedRunIDBase(base string) bool {
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
		base[3] >= '1' && base[3] <= '9'
}
