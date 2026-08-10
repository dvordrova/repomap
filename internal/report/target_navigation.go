package report

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/snapshot"
)

const TargetNavigationVersion = 1

// RenderOptions contains presentation-only inputs that are authorized by the
// caller but are not part of one target's canonical ReportData. Keeping target
// navigation here preserves the invariant that one report page owns exactly
// one ReportData value.
type RenderOptions struct {
	TargetNavigation *TargetNavigationPortfolio
}

// TargetNavigationPortfolio is the render-only sibling-page directory for one
// manifest-bound target portfolio. It is flat backend authority; the browser
// groups entries by ModuleDir without deriving identity or links from display
// text.
type TargetNavigationPortfolio struct {
	Version          int                    `json:"version"`
	DefaultTargetRef string                 `json:"default_target_ref"`
	CurrentTargetRef string                 `json:"current_target_ref"`
	Targets          []TargetNavigationItem `json:"targets"`
}

// TargetNavigationItem is one target page. Href is an exact backend-produced
// relative sibling-report route. An unavailable item deliberately has no href
// and renders as disabled rather than linking to a missing report.
type TargetNavigationItem struct {
	TargetRef   string `json:"target_ref"`
	ModuleDir   string `json:"module_dir"`
	DisplayPath string `json:"display_path"`
	Available   bool   `json:"available"`
	Href        string `json:"href,omitempty"`
}

// BuildTargetNavigation projects one manifest-bound sibling-page portfolio
// into transient report presentation state. Both persisted HTML generation
// and the local report server use this exact builder so a served report cannot
// silently lose or reinterpret the target directory embedded in its static
// counterpart.
func BuildTargetNavigation(
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	currentTargetRef string,
) (*TargetNavigationPortfolio, error) {
	if err := portfolio.ValidateAgainstContainer(container); err != nil {
		return nil, err
	}
	navigation := &TargetNavigationPortfolio{
		Version:          TargetNavigationVersion,
		DefaultTargetRef: container.DefaultTargetRef,
		CurrentTargetRef: currentTargetRef,
		Targets:          make([]TargetNavigationItem, 0, len(container.Targets)),
	}
	currentFound := false
	for index, projection := range container.Targets {
		page := portfolio.Targets[index]
		item := TargetNavigationItem{
			TargetRef:   projection.Target.Ref,
			ModuleDir:   projection.Target.ModuleDir,
			DisplayPath: projection.DisplayPath,
			Available:   page.State == snapshot.TargetPageReady,
		}
		if item.Available {
			if item.TargetRef == currentTargetRef {
				item.Href = "#/map"
				currentFound = true
			} else {
				item.Href = "../" + page.RunID + "/report.html#/map"
			}
		}
		navigation.Targets = append(navigation.Targets, item)
	}
	if !currentFound {
		return nil, fmt.Errorf("report: current target page is unavailable")
	}
	return navigation, nil
}

// LoadManifestTargetNavigation restores transient target navigation only from
// the exact container and portfolio bytes bound by one verified run manifest.
// It never reads report.html and never persists the projection into
// report.json.
func LoadManifestTargetNavigation(
	runDir string,
	manifest RunManifest,
) (*TargetNavigationPortfolio, error) {
	if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err != nil {
		return nil, err
	}
	if manifest.MaterialInputs.TargetPagePortfolioSHA256 == "" {
		return nil, nil
	}

	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, fmt.Errorf("report: open target navigation run: %w", err)
	}
	defer root.Close()
	containerRaw, err := readManifestFile(
		root,
		snapshot.TargetRunContainerArtifactFilename,
		snapshot.MaxTargetRunContainerBytes,
	)
	if err != nil || manifestSHA256(containerRaw) != manifest.MaterialInputs.TargetRunContainerSHA256 {
		return nil, fmt.Errorf("report: target navigation container authority mismatch")
	}
	portfolioRaw, err := readManifestFile(
		root,
		snapshot.TargetPagePortfolioArtifactFilename,
		snapshot.MaxTargetPagePortfolioBytes,
	)
	if err != nil || manifestSHA256(portfolioRaw) != manifest.MaterialInputs.TargetPagePortfolioSHA256 {
		return nil, fmt.Errorf("report: target navigation portfolio authority mismatch")
	}
	container, err := snapshot.DecodeTargetRunContainer(containerRaw)
	if err != nil {
		return nil, fmt.Errorf("report: target navigation container: %w", err)
	}
	portfolio, err := snapshot.DecodeTargetPagePortfolio(portfolioRaw)
	if err != nil {
		return nil, fmt.Errorf("report: target navigation portfolio: %w", err)
	}
	return BuildTargetNavigation(
		container,
		portfolio,
		manifest.MaterialInputs.AnalysisTargetRef,
	)
}

func validateTargetNavigation(data *ReportData, navigation *TargetNavigationPortfolio) error {
	if navigation == nil {
		return nil
	}
	if data == nil || data.AnalysisTarget == nil {
		return fmt.Errorf("report: target navigation requires one exact analysis target")
	}
	if navigation.Version != TargetNavigationVersion ||
		!validTargetNavigationRef(navigation.DefaultTargetRef) ||
		!validTargetNavigationRef(navigation.CurrentTargetRef) ||
		len(navigation.Targets) == 0 {
		return fmt.Errorf("report: invalid target navigation identity")
	}

	seen := make(map[string]struct{}, len(navigation.Targets))
	defaultFound := false
	currentFound := false
	for index, item := range navigation.Targets {
		if !validTargetNavigationRef(item.TargetRef) {
			return fmt.Errorf("report: target navigation item %d has invalid identity", index)
		}
		if _, duplicate := seen[item.TargetRef]; duplicate {
			return fmt.Errorf("report: target navigation contains duplicate target identity")
		}
		seen[item.TargetRef] = struct{}{}
		if !validTargetNavigationPath(item.ModuleDir) ||
			!validTargetNavigationPath(item.DisplayPath) ||
			!targetNavigationPathInsideModule(item.DisplayPath, item.ModuleDir) {
			return fmt.Errorf("report: target navigation item %d has invalid display scope", index)
		}
		if item.Available != (item.Href != "") {
			return fmt.Errorf("report: target navigation item %d has inconsistent availability", index)
		}
		if item.Available && !validTargetNavigationHref(item.Href, item.TargetRef == navigation.CurrentTargetRef) {
			return fmt.Errorf("report: target navigation item %d has invalid sibling report route", index)
		}

		if item.TargetRef == navigation.DefaultTargetRef {
			defaultFound = true
		}
		if item.TargetRef == navigation.CurrentTargetRef {
			currentFound = true
			if !item.Available || item.TargetRef != data.AnalysisTarget.Ref ||
				item.ModuleDir != data.AnalysisTarget.ModuleDir ||
				item.DisplayPath != data.AnalysisTarget.PackageDir {
				return fmt.Errorf("report: target navigation current page does not match analysis target")
			}
		}
	}
	if !defaultFound || !currentFound {
		return fmt.Errorf("report: target navigation default or current target is unavailable")
	}
	return nil
}

func validTargetNavigationRef(value string) bool {
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

func validTargetNavigationPath(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.Contains(value, "\\") &&
		!strings.HasPrefix(value, "/") && path.Clean(value) == value &&
		(value == "." || (value != ".." && !strings.HasPrefix(value, "../")))
}

func targetNavigationPathInsideModule(displayPath, moduleDir string) bool {
	return moduleDir == "." || displayPath == moduleDir || strings.HasPrefix(displayPath, moduleDir+"/")
}

func validTargetNavigationHref(value string, current bool) bool {
	if current {
		return value == "#/map"
	}
	if !utf8.ValidString(value) || strings.Contains(value, "\\") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Scheme != "" || parsed.Host != "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "/map" {
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
	for _, character := range runID {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}
