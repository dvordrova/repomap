package report

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

const TargetNavigationVersion = 4

// RenderOptions contains presentation-only inputs that are authorized by the
// caller but are not part of one target's canonical ReportData. Keeping target
// navigation here preserves the invariant that one report page owns exactly
// one ReportData value.
type RenderOptions struct {
	TargetNavigation *TargetNavigationPortfolio
	// ReportSHA256 is the digest of the report.json bytes this page is
	// rendered from. It is stamped into the page so a reader (and the
	// publication check) can prove which data the page shows.
	ReportSHA256 string
	// LocalRoots are caller-authorized workstation roots that must be scrubbed
	// from the browser payload. They affect neither report content nor source
	// authority; the local server keeps those paths only in its open catalog.
	LocalRoots []string
}

// TargetNavigationPortfolio is the render-only sibling-page directory for one
// manifest-bound target portfolio. It is flat backend authority; the browser
// receives ProgramTarget identities and never derives routes from display text.
type TargetNavigationPortfolio struct {
	Version         int                    `json:"version"`
	DefaultTargetID string                 `json:"default_target_id"`
	CurrentTargetID string                 `json:"current_target_id"`
	Targets         []TargetNavigationItem `json:"targets"`
}

// TargetNavigationItem is one target page. Href is an exact backend-produced
// relative sibling-report route into that page's Program workspace.
type TargetNavigationItem struct {
	TargetID    string `json:"target_id"`
	Language    string `json:"language"`
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
	Href        string `json:"href"`
}

// TargetNavigationPage is the validated language-neutral authority for one
// published ProgramPortfolio page. Backend-specific target refs may select
// the run that owns this value, but they never cross the browser boundary.
type TargetNavigationPage struct {
	RunID            string
	ProgramTarget    programindex.Target
	ArtifactFilename string
}

// PreparedTargetNavigationPage projects the small page identity from an
// already restored current-run ReportData value. It is the process-local
// counterpart of LoadTargetNavigationPage and performs no artifact reads.
func PreparedTargetNavigationPage(
	runDir string,
	data *ReportData,
) (TargetNavigationPage, error) {
	if data == nil || data.ProgramPortfolio == nil {
		return TargetNavigationPage{}, fmt.Errorf("report: prepared target navigation data is incomplete")
	}
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return TargetNavigationPage{}, fmt.Errorf("report: resolve prepared target navigation run: %w", err)
	}
	absoluteRunDir = filepath.Clean(absoluteRunDir)
	if filepath.Clean(data.ArtifactsDir) != absoluteRunDir {
		return TargetNavigationPage{}, fmt.Errorf("report: prepared target navigation data does not belong to the run")
	}
	entry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return TargetNavigationPage{}, fmt.Errorf("report: prepared target navigation program portfolio: %w", err)
	}
	page := TargetNavigationPage{
		RunID:            filepath.Base(absoluteRunDir),
		ProgramTarget:    entry.Target.Snapshot(),
		ArtifactFilename: data.defaultProgramIndexArtifactFilename,
	}
	if err := validateTargetNavigationPage(page); err != nil {
		return TargetNavigationPage{}, err
	}
	return page, nil
}

// LoadTargetNavigationPage restores one page identity only from its validated
// ProgramPortfolio default and the exact ProgramIndex artifact-set binding.
func LoadTargetNavigationPage(runDir, runID string) (TargetNavigationPage, error) {
	if err := programpage.ValidateRunID(runID); err != nil {
		return TargetNavigationPage{}, fmt.Errorf("report: target navigation run: %w", err)
	}
	data, err := ReadRunDir(runDir)
	if err != nil {
		return TargetNavigationPage{}, fmt.Errorf("report: target navigation program page: %w", err)
	}
	if data.ProgramPortfolio == nil {
		return TargetNavigationPage{}, fmt.Errorf("report: target navigation program portfolio is missing")
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return TargetNavigationPage{}, fmt.Errorf("report: target navigation program portfolio: %w", err)
	}
	setRaw, _, err := readBoundedProgramArtifact(
		filepath.Join(runDir, programindex.ArtifactSetFilename),
		programindex.MaxArtifactSetBytes,
		"program index set",
		false,
	)
	if err != nil {
		return TargetNavigationPage{}, err
	}
	set, err := programindex.DecodeArtifactSet(setRaw)
	if err != nil {
		return TargetNavigationPage{}, fmt.Errorf("report: target navigation program index set: %w", err)
	}
	if set.DefaultTargetID != defaultEntry.Target.ID {
		return TargetNavigationPage{}, fmt.Errorf("report: target navigation default program target mismatch")
	}
	artifactFilename := ""
	for _, entry := range set.Entries {
		if entry.TargetID == set.DefaultTargetID {
			artifactFilename = entry.Filename
			break
		}
	}
	page := TargetNavigationPage{
		RunID:            runID,
		ProgramTarget:    defaultEntry.Target.Snapshot(),
		ArtifactFilename: artifactFilename,
	}
	if err := validateTargetNavigationPage(page); err != nil {
		return TargetNavigationPage{}, err
	}
	return page, nil
}

// BuildTargetNavigation projects validated ProgramTarget pages into transient
// presentation state. Order and routes remain backend-owned; identity and
// display facts come only from each page's ProgramPortfolio/ArtifactSet.
func BuildTargetNavigation(
	pages []TargetNavigationPage,
	defaultTargetID string,
	currentTargetID string,
) (*TargetNavigationPortfolio, error) {
	if len(pages) == 0 || !validTargetNavigationText(defaultTargetID) ||
		!validTargetNavigationText(currentTargetID) {
		return nil, fmt.Errorf("report: target navigation page identity is incomplete")
	}
	navigation := &TargetNavigationPortfolio{
		Version:         TargetNavigationVersion,
		DefaultTargetID: defaultTargetID,
		CurrentTargetID: currentTargetID,
		Targets:         make([]TargetNavigationItem, 0, len(pages)),
	}
	seenTargets := make(map[string]struct{}, len(pages))
	seenRuns := make(map[string]struct{}, len(pages))
	defaultFound := false
	currentFound := false
	for _, page := range pages {
		if err := validateTargetNavigationPage(page); err != nil {
			return nil, err
		}
		if _, duplicate := seenTargets[page.ProgramTarget.ID]; duplicate {
			return nil, fmt.Errorf("report: target navigation contains duplicate program target identity")
		}
		seenTargets[page.ProgramTarget.ID] = struct{}{}
		if _, duplicate := seenRuns[page.RunID]; duplicate {
			return nil, fmt.Errorf("report: target navigation contains duplicate run identity")
		}
		seenRuns[page.RunID] = struct{}{}
		item := TargetNavigationItem{
			TargetID:    page.ProgramTarget.ID,
			Language:    page.ProgramTarget.Language,
			Kind:        page.ProgramTarget.Kind,
			DisplayName: page.ProgramTarget.Name,
		}
		if item.TargetID == defaultTargetID {
			defaultFound = true
		}
		if item.TargetID == currentTargetID {
			item.Href = "#/program"
			currentFound = true
		} else {
			item.Href = "../" + page.RunID + "/report.html#/program"
		}
		navigation.Targets = append(navigation.Targets, item)
	}
	if !defaultFound || !currentFound {
		return nil, fmt.Errorf("report: target navigation default or current program target is absent")
	}
	return navigation, nil
}

// LoadManifestTargetNavigation restores transient target navigation only from
// the language-neutral page portfolio bound by one verified run manifest. It
// never reads report.html and never persists the projection into report.json.
func LoadManifestTargetNavigation(
	runDir string,
	manifest RunManifest,
) (*TargetNavigationPortfolio, error) {
	return loadManifestProgramPageNavigation(runDir, manifest)
}

func loadManifestProgramPageNavigation(
	runDir string,
	manifest RunManifest,
) (*TargetNavigationPortfolio, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, fmt.Errorf("report: open program page navigation run: %w", err)
	}
	portfolioRaw, err := readManifestFile(
		root, programpage.ArtifactFilename, programpage.MaxArtifactBytes,
	)
	_ = root.Close()
	if err != nil || manifestSHA256(portfolioRaw) != manifest.MaterialInputs.ProgramPagePortfolioSHA256 {
		return nil, fmt.Errorf("report: program page navigation portfolio authority mismatch")
	}
	portfolio, err := programpage.Decode(portfolioRaw)
	if err != nil {
		return nil, fmt.Errorf("report: program page navigation portfolio: %w", err)
	}
	pages := make([]TargetNavigationPage, 0, len(portfolio.Pages))
	currentTargetID := ""
	runsDir := filepath.Dir(filepath.Clean(runDir))
	for index, binding := range portfolio.Pages {
		pageRunDir := filepath.Join(runsDir, binding.RunID)
		page, pageErr := LoadTargetNavigationPage(pageRunDir, binding.RunID)
		if pageErr != nil {
			return nil, fmt.Errorf("report: program page navigation page %d: %w", index, pageErr)
		}
		if !reflect.DeepEqual(page.ProgramTarget, binding.Target) {
			return nil, fmt.Errorf("report: program page navigation page %d target authority mismatch", index)
		}
		outcomeRaw, _, outcomeErr := readBoundedProgramArtifact(
			filepath.Join(pageRunDir, targetoutcome.ArtifactFilename),
			targetoutcome.MaxArtifactBytes,
			"program page navigation target outcome portfolio",
			false,
		)
		if outcomeErr != nil || manifestSHA256(outcomeRaw) != manifest.MaterialInputs.TargetOutcomePortfolioSHA256 {
			return nil, fmt.Errorf("report: program page navigation page %d target outcome authority mismatch", index)
		}
		if filepath.Clean(pageRunDir) == filepath.Clean(runDir) {
			if binding.Target.ID != manifest.MaterialInputs.ProgramTargetID {
				return nil, fmt.Errorf("report: current program page navigation authority mismatch")
			}
			currentTargetID = binding.Target.ID
		}
		pages = append(pages, page)
	}
	if currentTargetID == "" {
		return nil, fmt.Errorf("report: current program page is absent from portfolio")
	}
	return BuildTargetNavigation(pages, portfolio.DefaultTargetID, currentTargetID)
}

func validateTargetNavigation(data *ReportData, navigation *TargetNavigationPortfolio) error {
	if data == nil {
		return fmt.Errorf("report: target navigation requires report data")
	}
	if data.TargetOutcomePortfolio == nil {
		return fmt.Errorf("report: target navigation requires the exhaustive target outcome portfolio")
	}
	if navigation == nil {
		return fmt.Errorf("report: target outcome portfolio requires complete target navigation")
	}
	if navigation.Version != TargetNavigationVersion ||
		!validTargetNavigationText(navigation.DefaultTargetID) ||
		!validTargetNavigationText(navigation.CurrentTargetID) ||
		len(navigation.Targets) == 0 {
		return fmt.Errorf("report: invalid target navigation identity")
	}

	// ProgramPagePortfolio owns the exact targets and sibling run paths. Target
	// navigation is a ProgramPortfolio contract; there is no route rewrite or
	// browser fallback.
	if data.ProgramPortfolio == nil {
		return fmt.Errorf("report: target navigation requires one exact ProgramPortfolio page")
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report: target navigation ProgramPortfolio: %w", err)
	}
	seen := make(map[string]struct{}, len(navigation.Targets))
	defaultFound := false
	currentFound := false
	for index, item := range navigation.Targets {
		if !validTargetNavigationText(item.TargetID) ||
			!validTargetNavigationText(item.Language) ||
			!validTargetNavigationText(item.Kind) ||
			!validTargetNavigationText(item.DisplayName) {
			return fmt.Errorf("report: target navigation item %d has invalid identity", index)
		}
		if _, duplicate := seen[item.TargetID]; duplicate {
			return fmt.Errorf("report: target navigation contains duplicate target identity")
		}
		seen[item.TargetID] = struct{}{}
		if item.Href == "" {
			return fmt.Errorf("report: target navigation item %d is not a complete published page", index)
		}
		if !validTargetNavigationHref(
			item.Href,
			item.TargetID == navigation.CurrentTargetID,
		) {
			return fmt.Errorf("report: target navigation item %d has invalid sibling report route", index)
		}

		if item.TargetID == navigation.DefaultTargetID {
			defaultFound = true
		}
		if item.TargetID == navigation.CurrentTargetID {
			currentFound = true
			if item.TargetID != defaultEntry.Target.ID ||
				item.Language != defaultEntry.Target.Language ||
				item.Kind != defaultEntry.Target.Kind ||
				item.DisplayName != defaultEntry.Target.Name {
				return fmt.Errorf("report: target navigation current page does not match the default program target")
			}
		}
	}
	if !defaultFound || !currentFound {
		return fmt.Errorf("report: target navigation default or current target is absent")
	}
	if err := data.TargetOutcomePortfolio.validateTargetNavigation(navigation); err != nil {
		return fmt.Errorf("report: target outcome portfolio target navigation: %w", err)
	}
	return nil
}

func validateTargetNavigationPage(page TargetNavigationPage) error {
	if err := programpage.ValidateRunID(page.RunID); err != nil {
		return fmt.Errorf("report: target navigation page run: %w", err)
	}
	if err := page.ProgramTarget.Validate(); err != nil {
		return fmt.Errorf("report: target navigation page program target: %w", err)
	}
	if !validTargetNavigationArtifactFilename(page.ArtifactFilename) {
		return fmt.Errorf("report: target navigation page artifact filename is invalid")
	}
	return nil
}

func validTargetNavigationText(value string) bool {
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

func validTargetNavigationArtifactFilename(value string) bool {
	return value == programindex.ArtifactFilename
}

func validTargetNavigationHref(value string, current bool) bool {
	if current {
		return value == "#/program"
	}
	if !utf8.ValidString(value) || strings.Contains(value, "\\") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Scheme != "" || parsed.Host != "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "/program" {
		return false
	}
	const prefix = "../"
	const suffix = "/report.html"
	if !strings.HasPrefix(parsed.Path, prefix) || !strings.HasSuffix(parsed.Path, suffix) {
		return false
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(parsed.Path, prefix), suffix)
	if runID == "" || strings.Contains(runID, "/") {
		return false
	}
	return programpage.ValidateRunID(runID) == nil
}
