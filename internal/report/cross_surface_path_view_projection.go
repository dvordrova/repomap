package report

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	CrossSurfacePathViewVersion  = 1
	MaxCrossSurfacePathViewBytes = 32 << 20
)

type CrossSurfaceStepKind string

type CrossSurfacePathAuthority string

const (
	CrossSurfacePageRoute           CrossSurfaceStepKind = "page_route"
	CrossSurfaceRenderTarget        CrossSurfaceStepKind = "render_target"
	CrossSurfaceMutationSite        CrossSurfaceStepKind = "mutation_site"
	CrossSurfaceProgramCall         CrossSurfaceStepKind = "program_call"
	CrossSurfaceClientHTTPUse       CrossSurfaceStepKind = "client_http_use"
	CrossSurfaceHTTPMethodPathMatch CrossSurfaceStepKind = "http_method_path_match"
	CrossSurfaceServerRoute         CrossSurfaceStepKind = "server_route"
	CrossSurfaceMiddleware          CrossSurfaceStepKind = "middleware"
	CrossSurfaceHandlerFactory      CrossSurfaceStepKind = "handler_factory"
	CrossSurfaceHandler             CrossSurfaceStepKind = "handler"
	CrossSurfaceContractValidation  CrossSurfaceStepKind = "contract_validation"
	CrossSurfaceStorageCall         CrossSurfaceStepKind = "storage_call"
	CrossSurfaceResourceBoundary    CrossSurfaceStepKind = "resource_boundary"
)

const (
	CrossSurfaceExactStatic        CrossSurfacePathAuthority = "exact_static"
	CrossSurfaceResolvedIndirect   CrossSurfacePathAuthority = "resolved_indirect"
	CrossSurfacePossible           CrossSurfacePathAuthority = "possible"
	CrossSurfaceUnresolvedFrontier CrossSurfacePathAuthority = "unresolved_frontier"
)

// CrossSurfacePathView is the complete producer-owned path inventory for one
// JS/TS project. It preserves the explicit HTTP compatibility joint as its own
// step kind; it never promotes method/path equality into a ProgramIndex call.
type CrossSurfacePathView struct {
	Version            int                      `json:"version"`
	ProgramTargetID    string                   `json:"program_target_id"`
	ProgramIndexSHA256 string                   `json:"program_index_sha256"`
	JSTSProjectSHA256  string                   `json:"js_ts_project_sha256,omitempty"`
	Facts              []JSTSFactView           `json:"facts"`
	Paths              []CrossSurfacePath       `json:"paths"`
	Coverage           CrossSurfacePathCoverage `json:"coverage"`
}

type CrossSurfacePath struct {
	PathID   string                 `json:"path_id"`
	Name     string                 `json:"name"`
	Outcome  string                 `json:"outcome"`
	Steps    []CrossSurfacePathStep `json:"steps"`
	Frontier string                 `json:"frontier,omitempty"`
}

type CrossSurfacePathStep struct {
	Ordinal    int                       `json:"ordinal"`
	Kind       CrossSurfaceStepKind      `json:"kind"`
	Label      string                    `json:"label"`
	SourceRef  string                    `json:"source_ref"`
	TargetRefs []string                  `json:"target_refs"`
	Resolution programindex.Resolution   `json:"resolution"`
	Authority  CrossSurfacePathAuthority `json:"authority"`
	Location   programindex.Location     `json:"location"`
}

type CrossSurfacePathCoverage struct {
	RoutesObserved          int `json:"routes_observed"`
	HTTPUsesObserved        int `json:"http_uses_observed"`
	PathsProjected          int `json:"paths_projected"`
	StepsProjected          int `json:"steps_projected"`
	ExactSteps              int `json:"exact_steps"`
	AlternativeSteps        int `json:"alternative_steps"`
	UnresolvedSteps         int `json:"unresolved_steps"`
	ExactStaticSteps        int `json:"exact_static_steps"`
	ResolvedIndirectSteps   int `json:"resolved_indirect_steps"`
	PossibleSteps           int `json:"possible_steps"`
	UnresolvedFrontierSteps int `json:"unresolved_frontier_steps"`
	Frontiers               int `json:"frontiers"`
}

func NewCrossSurfacePathView(
	result jstsproject.Result,
	index programindex.Index,
) (*CrossSurfacePathView, error) {
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("cross-surface path view: producer authority: %w", err)
	}
	if err := validateJSTSProgramIndexBinding(result, index); err != nil {
		return nil, fmt.Errorf("cross-surface path view: %w", err)
	}
	directory, err := jstsFactDirectory(result)
	if err != nil {
		return nil, fmt.Errorf("cross-surface path view: fact directory: %w", err)
	}
	view := &CrossSurfacePathView{
		Version: CrossSurfacePathViewVersion, ProgramTargetID: index.Target.ID,
		ProgramIndexSHA256: index.SHA256, JSTSProjectSHA256: result.SHA256,
		Facts: []JSTSFactView{}, Paths: []CrossSurfacePath{},
		Coverage: CrossSurfacePathCoverage{
			RoutesObserved: len(result.Routes), HTTPUsesObserved: len(result.HTTPUses),
			PathsProjected: len(result.ProductPaths),
		},
	}
	referenced := make(map[string]struct{})
	for _, path := range result.ProductPaths {
		projected := CrossSurfacePath{
			PathID: path.Ref, Name: path.Name, Outcome: path.Outcome,
			Steps: []CrossSurfacePathStep{}, Frontier: path.Frontier,
		}
		if path.Frontier != "" {
			view.Coverage.Frontiers++
		}
		for _, step := range path.Steps {
			resolution := programindex.Resolution(step.Resolution)
			projected.Steps = append(projected.Steps, CrossSurfacePathStep{
				Ordinal: step.Ordinal, Kind: CrossSurfaceStepKind(step.Kind), Label: step.Label,
				SourceRef: step.SourceRef, TargetRefs: append([]string{}, step.TargetRefs...),
				Resolution: resolution, Authority: CrossSurfacePathAuthority(step.Authority),
				Location: jstsLocation(step.Location),
			})
			referenced[step.SourceRef] = struct{}{}
			for _, ref := range step.TargetRefs {
				referenced[ref] = struct{}{}
			}
			view.Coverage.StepsProjected++
			switch resolution {
			case programindex.ResolutionExact:
				view.Coverage.ExactSteps++
			case programindex.ResolutionAlternatives:
				view.Coverage.AlternativeSteps++
			case programindex.ResolutionUnresolved:
				view.Coverage.UnresolvedSteps++
			}
			incrementCrossSurfaceAuthority(&view.Coverage, CrossSurfacePathAuthority(step.Authority))
		}
		view.Paths = append(view.Paths, projected)
	}
	for ref := range referenced {
		fact, ok := directory[ref]
		if !ok {
			return nil, fmt.Errorf("cross-surface path cites unknown fact ref %q", ref)
		}
		view.Facts = append(view.Facts, cloneJSTSFact(fact))
	}
	sort.Slice(view.Facts, func(i, j int) bool { return view.Facts[i].Ref < view.Facts[j].Ref })
	if err := view.Validate(); err != nil {
		return nil, fmt.Errorf("cross-surface path view: invalid projection: %w", err)
	}
	return view, nil
}

func (view CrossSurfacePathView) ValidateAgainst(result jstsproject.Result, index programindex.Index) error {
	if err := view.Validate(); err != nil {
		return err
	}
	expected, err := NewCrossSurfacePathView(result, index)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(view, *expected) {
		return fmt.Errorf("cross-surface path view: projection does not match exact producer authority")
	}
	return nil
}

// ValidateSurfaceJoins proves that every surface identity retained by a path
// is the exact catalog surface and that every published path explicitly cites
// at least one such surface. The browser never recovers this join from names.
func (view CrossSurfacePathView) ValidateSurfaceJoins(catalog *JSTSSurfaceCatalogView) error {
	if err := view.Validate(); err != nil {
		return err
	}
	if catalog == nil {
		return fmt.Errorf("cross-surface path view: surface catalog is missing")
	}
	if err := catalog.Validate(); err != nil {
		return fmt.Errorf("cross-surface path view: surface catalog: %w", err)
	}
	if view.ProgramTargetID != catalog.ProgramTargetID ||
		view.ProgramIndexSHA256 != catalog.ProgramIndexSHA256 ||
		view.JSTSProjectSHA256 != catalog.JSTSProjectSHA256 {
		return fmt.Errorf("cross-surface path view: surface catalog authority mismatch")
	}
	surfaces := make(map[string]JSTSSurfaceCatalogSurface, len(catalog.Surfaces))
	for _, surface := range catalog.Surfaces {
		surfaces[surface.SurfaceID] = surface
	}
	facts := make(map[string]JSTSFactView, len(view.Facts))
	for _, fact := range view.Facts {
		facts[fact.Ref] = fact
		if fact.Category != "surface" {
			continue
		}
		surface, ok := surfaces[fact.Ref]
		if !ok || fact.Kind != string(surface.Kind) || fact.Label != surface.Name ||
			fact.Location == nil || *fact.Location != surface.Location {
			return fmt.Errorf("cross-surface path view: surface fact %q does not match catalog", fact.Ref)
		}
	}
	for _, path := range view.Paths {
		citesBrowser := false
		citesServer := false
		cite := func(ref string) {
			if facts[ref].Category != "surface" {
				return
			}
			surface := surfaces[ref]
			if surface.Role != jstsproject.SurfaceProduct {
				return
			}
			switch surface.Kind {
			case jstsproject.SurfaceBrowser:
				citesBrowser = true
			case jstsproject.SurfaceServer:
				citesServer = true
			}
		}
		for _, step := range path.Steps {
			cite(step.SourceRef)
			for _, ref := range step.TargetRefs {
				cite(ref)
			}
		}
		if !citesBrowser || !citesServer {
			return fmt.Errorf("cross-surface path view: path %q must cite one exact browser product surface and one exact server product surface", path.PathID)
		}
	}
	return nil
}

func (view CrossSurfacePathView) Validate() error {
	if view.Version != CrossSurfacePathViewVersion ||
		!validProgramViewText(view.ProgramTargetID) ||
		!validCubeMapViewSHA256(view.ProgramIndexSHA256) ||
		!validCubeMapViewSHA256(view.JSTSProjectSHA256) ||
		view.Facts == nil || view.Paths == nil {
		return fmt.Errorf("cross-surface path view: invalid identity or collection shape")
	}
	facts, err := validateJSTSFactViews(view.Facts)
	if err != nil {
		return fmt.Errorf("cross-surface path view: %w", err)
	}
	referenced := make(map[string]struct{})
	coverage := CrossSurfacePathCoverage{
		RoutesObserved:   view.Coverage.RoutesObserved,
		HTTPUsesObserved: view.Coverage.HTTPUsesObserved,
		PathsProjected:   len(view.Paths),
	}
	if coverage.RoutesObserved < 0 || coverage.HTTPUsesObserved < 0 {
		return fmt.Errorf("cross-surface path view: observed coverage is invalid")
	}
	previousID := ""
	for position, path := range view.Paths {
		if !validProgramViewText(path.PathID) || !validProgramViewText(path.Name) ||
			!validProgramViewText(path.Outcome) || path.Steps == nil || len(path.Steps) == 0 ||
			(path.Frontier != "" && !validProgramViewText(path.Frontier)) {
			return fmt.Errorf("cross-surface path view: path %d is invalid", position)
		}
		if previousID != "" && previousID >= path.PathID {
			return fmt.Errorf("cross-surface path view: paths are not canonical")
		}
		previousID = path.PathID
		if path.Frontier != "" {
			coverage.Frontiers++
		}
		for stepPosition, step := range path.Steps {
			if step.Ordinal != stepPosition+1 || !validCrossSurfaceStepKind(step.Kind) ||
				!validProgramViewText(step.Label) || !validProgramViewText(step.SourceRef) ||
				!step.Resolution.Valid() || !validCrossSurfaceAuthority(step.Authority) ||
				!validCrossSurfaceAuthorityResolution(step.Authority, step.Resolution) ||
				!validJSTSSourceLocation(&step.Location, false) ||
				step.TargetRefs == nil {
				return fmt.Errorf("cross-surface path view: path %q step %d is invalid", path.PathID, stepPosition)
			}
			if _, ok := facts[step.SourceRef]; !ok {
				return fmt.Errorf("cross-surface path view: step cites unknown source fact %q", step.SourceRef)
			}
			referenced[step.SourceRef] = struct{}{}
			if err := validateCanonicalJSTSRefs(step.TargetRefs); err != nil {
				return fmt.Errorf("cross-surface path view: target refs: %w", err)
			}
			for _, ref := range step.TargetRefs {
				if _, ok := facts[ref]; !ok {
					return fmt.Errorf("cross-surface path view: step cites unknown target fact %q", ref)
				}
				referenced[ref] = struct{}{}
			}
			coverage.StepsProjected++
			switch step.Resolution {
			case programindex.ResolutionExact:
				coverage.ExactSteps++
			case programindex.ResolutionAlternatives:
				coverage.AlternativeSteps++
			case programindex.ResolutionUnresolved:
				coverage.UnresolvedSteps++
			}
			incrementCrossSurfaceAuthority(&coverage, step.Authority)
		}
	}
	if len(referenced) != len(view.Facts) {
		return fmt.Errorf("cross-surface path view: fact dictionary contains dead entries")
	}
	if !reflect.DeepEqual(view.Coverage, coverage) {
		return fmt.Errorf("cross-surface path view: coverage does not match complete path inventory")
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return fmt.Errorf("cross-surface path view: encode bound check: %w", err)
	}
	if len(encoded) > MaxCrossSurfacePathViewBytes {
		return fmt.Errorf("cross-surface path view: exact projection requires %d bytes; limit is %d", len(encoded), MaxCrossSurfacePathViewBytes)
	}
	return nil
}

func validCrossSurfaceAuthority(value CrossSurfacePathAuthority) bool {
	switch value {
	case CrossSurfaceExactStatic, CrossSurfaceResolvedIndirect, CrossSurfacePossible,
		CrossSurfaceUnresolvedFrontier:
		return true
	default:
		return false
	}
}

func validCrossSurfaceAuthorityResolution(
	authority CrossSurfacePathAuthority,
	resolution programindex.Resolution,
) bool {
	switch authority {
	case CrossSurfaceExactStatic, CrossSurfaceResolvedIndirect:
		return resolution == programindex.ResolutionExact
	case CrossSurfacePossible:
		return resolution == programindex.ResolutionAlternatives
	case CrossSurfaceUnresolvedFrontier:
		return resolution == programindex.ResolutionUnresolved
	default:
		return false
	}
}

func incrementCrossSurfaceAuthority(coverage *CrossSurfacePathCoverage, value CrossSurfacePathAuthority) {
	switch value {
	case CrossSurfaceExactStatic:
		coverage.ExactStaticSteps++
	case CrossSurfaceResolvedIndirect:
		coverage.ResolvedIndirectSteps++
	case CrossSurfacePossible:
		coverage.PossibleSteps++
	case CrossSurfaceUnresolvedFrontier:
		coverage.UnresolvedFrontierSteps++
	}
}

func validCrossSurfaceStepKind(value CrossSurfaceStepKind) bool {
	switch value {
	case CrossSurfacePageRoute, CrossSurfaceRenderTarget, CrossSurfaceMutationSite,
		CrossSurfaceProgramCall, CrossSurfaceClientHTTPUse, CrossSurfaceHTTPMethodPathMatch,
		CrossSurfaceServerRoute, CrossSurfaceMiddleware, CrossSurfaceHandlerFactory,
		CrossSurfaceHandler, CrossSurfaceContractValidation, CrossSurfaceStorageCall,
		CrossSurfaceResourceBoundary:
		return true
	default:
		return false
	}
}
