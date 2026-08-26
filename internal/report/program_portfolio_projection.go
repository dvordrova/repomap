package report

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/programindex"
)

const ProgramPortfolioVersion = 1

// ProgramSemanticState is the closed presentation capability of one exact
// ProgramTarget. It describes whether the ordinary semantic cube result must
// be present for that target; it is not a browser-side inference or a runtime
// fallback state.
type ProgramSemanticState string

const (
	ProgramSemanticAvailable        ProgramSemanticState = "available"
	ProgramSemanticProgramAvailable ProgramSemanticState = "program_semantic_available"
	ProgramSemanticStructuralOnly   ProgramSemanticState = "structural_only"
)

// ProgramPortfolio is the complete browser-facing projection of the selected
// language-neutral ProgramIndex artifact set. It never chooses, drops, or
// repairs a target: every set entry has one exact Target/View pair.
type ProgramPortfolio struct {
	Version         int                     `json:"version"`
	DefaultTargetID string                  `json:"default_target_id"`
	Entries         []ProgramPortfolioEntry `json:"entries"`
}

type ProgramPortfolioEntry struct {
	Target        programindex.Target  `json:"target"`
	View          ProgramView          `json:"view"`
	SemanticState ProgramSemanticState `json:"semantic_state"`
}

// NewProgramPortfolio projects every validated selected index and preserves
// the artifact-set default by exact ProgramTarget ID.
func NewProgramPortfolio(defaultTargetID string, indexes []programindex.Index) (*ProgramPortfolio, error) {
	if len(indexes) == 0 || len(indexes) > programindex.MaxArtifactSetEntries {
		return nil, fmt.Errorf("program portfolio: entry bound exceeded")
	}
	result := &ProgramPortfolio{
		Version: ProgramPortfolioVersion, DefaultTargetID: defaultTargetID,
		Entries: make([]ProgramPortfolioEntry, 0, len(indexes)),
	}
	for _, index := range indexes {
		view, err := NewProgramView(index)
		if err != nil {
			return nil, fmt.Errorf("program portfolio: project target %q: %w", index.Target.ID, err)
		}
		result.Entries = append(result.Entries, ProgramPortfolioEntry{
			Target:        index.Target.Snapshot(),
			View:          *view,
			SemanticState: programSemanticState(index.Target, index.Target.ID == defaultTargetID),
		})
	}
	sort.Slice(result.Entries, func(left, right int) bool {
		return result.Entries[left].Target.ID < result.Entries[right].Target.ID
	})
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return result, nil
}

func (portfolio ProgramPortfolio) Validate() error {
	if portfolio.Version != ProgramPortfolioVersion ||
		!validProgramViewText(portfolio.DefaultTargetID) ||
		portfolio.Entries == nil || len(portfolio.Entries) == 0 ||
		len(portfolio.Entries) > programindex.MaxArtifactSetEntries {
		return fmt.Errorf("program portfolio: invalid identity or entry bound")
	}
	defaultMatches := 0
	previousID := ""
	for _, entry := range portfolio.Entries {
		if err := entry.Target.Validate(); err != nil {
			return fmt.Errorf("program portfolio: target: %w", err)
		}
		if err := entry.View.Validate(); err != nil {
			return fmt.Errorf("program portfolio: view for %q: %w", entry.Target.ID, err)
		}
		if entry.View.TargetID != entry.Target.ID {
			return fmt.Errorf("program portfolio: target/view identity mismatch")
		}
		if entry.SemanticState != programSemanticState(entry.Target, entry.Target.ID == portfolio.DefaultTargetID) {
			return fmt.Errorf("program portfolio: target %q has invalid semantic state", entry.Target.ID)
		}
		if previousID != "" && previousID >= entry.Target.ID {
			return fmt.Errorf("program portfolio: entries are not canonical")
		}
		previousID = entry.Target.ID
		if entry.Target.ID == portfolio.DefaultTargetID {
			defaultMatches++
		}
	}
	if defaultMatches != 1 {
		return fmt.Errorf("program portfolio: default target must have exactly one entry")
	}
	return nil
}

func programSemanticState(target programindex.Target, isDefault bool) ProgramSemanticState {
	if programSemanticLanguage(target.Language) && isDefault {
		return ProgramSemanticProgramAvailable
	}
	return ProgramSemanticStructuralOnly
}

func programSemanticLanguage(language string) bool {
	switch language {
	case "go", "python", "javascript", "typescript":
		return true
	default:
		return false
	}
}

func (portfolio ProgramPortfolio) defaultEntry() (ProgramPortfolioEntry, error) {
	if err := portfolio.Validate(); err != nil {
		return ProgramPortfolioEntry{}, err
	}
	for _, entry := range portfolio.Entries {
		if entry.Target.ID == portfolio.DefaultTargetID {
			return entry, nil
		}
	}
	return ProgramPortfolioEntry{}, fmt.Errorf("program portfolio: default target is missing")
}

// validateProgramSemanticPresentation closes the semantic capability declared
// by every portfolio entry against the page-local CubeMap or CoreMap
// presentation carried by ReportData. A semantic-capable non-default entry is
// not representable and must fail instead of appearing as an unexplained empty
// map.
func validateProgramSemanticPresentation(
	portfolio *ProgramPortfolio,
	analysisTarget *analysistarget.Target,
	cubeMapView *CubeMapView,
	coreMapView *CoreMapView,
	activityEntrypointView *ActivityEntrypointView,
	integrationUsageView *IntegrationUsageView,
	activityPathView *ActivityPathView,
	jsTSSemanticViews ...jstsSemanticPresentation,
) error {
	if len(jsTSSemanticViews) > 1 {
		return fmt.Errorf("report: JavaScript/TypeScript semantic presentation is ambiguous")
	}
	var jsTSSurfaceCatalogView *JSTSSurfaceCatalogView
	var crossSurfacePathView *CrossSurfacePathView
	if len(jsTSSemanticViews) == 1 {
		jsTSSurfaceCatalogView = jsTSSemanticViews[0].surfaceCatalog
		crossSurfacePathView = jsTSSemanticViews[0].crossSurfacePaths
	}
	if portfolio == nil {
		if analysisTarget != nil || cubeMapView != nil || coreMapView != nil ||
			activityEntrypointView != nil || integrationUsageView != nil || activityPathView != nil ||
			jsTSSurfaceCatalogView != nil || crossSurfacePathView != nil {
			return fmt.Errorf("report: semantic view requires a complete program portfolio")
		}
		return nil
	}
	defaultEntry, err := portfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report: %w", err)
	}
	for _, entry := range portfolio.Entries {
		if entry.SemanticState == ProgramSemanticAvailable && entry.Target.ID != portfolio.DefaultTargetID {
			return fmt.Errorf("report: semantic-capable non-default target %q requires its own target page", entry.Target.ID)
		}
	}

	switch defaultEntry.SemanticState {
	case ProgramSemanticAvailable:
		if analysisTarget == nil || cubeMapView == nil || coreMapView != nil ||
			activityEntrypointView != nil || integrationUsageView != nil || activityPathView != nil ||
			jsTSSurfaceCatalogView != nil || crossSurfacePathView != nil {
			return fmt.Errorf("report: available Go semantic target requires exact analysis target and cube map view")
		}
	case ProgramSemanticProgramAvailable:
		if cubeMapView != nil || coreMapView == nil ||
			activityEntrypointView == nil || integrationUsageView == nil || activityPathView == nil {
			return fmt.Errorf("report: ProgramIndex semantic target requires exact core map, activity entrypoint, integration usage, and activity path authority and no legacy CubeMap authority")
		}
		if defaultEntry.Target.Language == "go" {
			if analysisTarget == nil {
				return fmt.Errorf("report: Go ProgramIndex target page requires its exact outer analysis target")
			}
			if err := validateCubeMapProgramTarget(*analysisTarget, defaultEntry.Target); err != nil {
				return fmt.Errorf("report: Go ProgramIndex target page: %w", err)
			}
		} else if analysisTarget != nil {
			return fmt.Errorf("report: non-Go ProgramIndex target cannot carry a Go analysis target")
		}
		isJSTS := defaultEntry.Target.Language == "javascript" || defaultEntry.Target.Language == "typescript"
		if isJSTS {
			if jsTSSurfaceCatalogView == nil || crossSurfacePathView == nil {
				return fmt.Errorf("report: JavaScript/TypeScript semantic target requires exact surface catalog and cross-surface path authority")
			}
		} else if jsTSSurfaceCatalogView != nil || crossSurfacePathView != nil {
			return fmt.Errorf("report: non-JavaScript/TypeScript target cannot carry JavaScript/TypeScript surface authority")
		}
	case ProgramSemanticStructuralOnly:
		if analysisTarget != nil || cubeMapView != nil || coreMapView != nil ||
			activityEntrypointView != nil || integrationUsageView != nil || activityPathView != nil ||
			jsTSSurfaceCatalogView != nil || crossSurfacePathView != nil {
			return fmt.Errorf("report: structural-only target cannot carry semantic authority")
		}
	default:
		return fmt.Errorf("report: default program target has invalid semantic state")
	}
	return nil
}

type jstsSemanticPresentation struct {
	surfaceCatalog    *JSTSSurfaceCatalogView
	crossSurfacePaths *CrossSurfacePathView
}
