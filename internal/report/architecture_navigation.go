package report

import (
	"fmt"
	"reflect"

	"github.com/dvordrova/repomap/internal/componentmap"
)

// ArchitectureComponentNavigationVersion changes when the persisted
// report-owned navigation shape or its exact derivation rules change.
const ArchitectureComponentNavigationVersion = 1

// ArchitectureComponentNavigationProjection keeps map navigation available
// for every accepted conceptual component while retaining source starts only
// when exact producer-owned symbol ancestry proves them.
type ArchitectureComponentNavigationProjection struct {
	Version    int                               `json:"version"`
	Components []ArchitectureComponentNavigation `json:"components"`
}

// ArchitectureComponentNavigation is one exact component navigation record.
// PackageParticipantIDs remain conceptual participation, never source starts
// or ownership. SymbolSources is deliberately plural and has no primary item.
type ArchitectureComponentNavigation struct {
	ComponentID           componentmap.ComponentID            `json:"component_id"`
	MapTarget             UserMapTarget                       `json:"map_target"`
	PackageParticipantIDs []componentmap.MemberID             `json:"package_participant_ids,omitempty"`
	SymbolSources         []ArchitectureComponentSymbolSource `json:"symbol_sources,omitempty"`
}

// ArchitectureComponentSymbolSource binds a source action to the exact
// producer-owned member identity and declaration, not to a sorted file choice.
type ArchitectureComponentSymbolSource struct {
	MemberID componentmap.MemberID `json:"member_id"`
	Symbol   string                `json:"symbol"`
	Location UserCodeLocation      `json:"location"`
}

// ProjectArchitectureComponentNavigation derives the complete report
// navigation projection from a current Canvas. Canvas component/member order
// is retained; deterministic serialization order is not a primary-source
// selection rule.
func ProjectArchitectureComponentNavigation(
	canvas *ArchitectureCanvas,
	openablePaths []string,
) (*ArchitectureComponentNavigationProjection, error) {
	if canvas == nil {
		return nil, nil
	}
	if canvas.Version != ArchitectureCanvasVersion {
		return nil, fmt.Errorf(
			"architecture component navigation: unsupported canvas version %d",
			canvas.Version,
		)
	}

	openable := make(map[string]struct{}, len(openablePaths))
	for _, sourcePath := range openablePaths {
		openable[sourcePath] = struct{}{}
	}
	known, locators, err := architectureNavigationMemberIndex(canvas)
	if err != nil {
		return nil, err
	}
	projection := &ArchitectureComponentNavigationProjection{
		Version: ArchitectureComponentNavigationVersion,
	}
	componentIDs := make(map[componentmap.ComponentID]struct{}, len(canvas.Components))
	remainderSeen := false
	for _, component := range canvas.Components {
		if component.ID == "" {
			return nil, fmt.Errorf("architecture component navigation: empty component id")
		}
		if _, duplicate := componentIDs[component.ID]; duplicate {
			return nil, fmt.Errorf(
				"architecture component navigation: duplicate component %q",
				component.ID,
			)
		}
		componentIDs[component.ID] = struct{}{}
		if component.ID == canvas.LocalRemainderComponentID && canvas.LocalRemainderComponentID != "" {
			remainderSeen = true
			continue
		}

		entry := ArchitectureComponentNavigation{
			ComponentID: component.ID,
			MapTarget: UserMapTarget{
				Kind:        SemanticSearchTargetComponent,
				ComponentID: component.ID,
			},
		}
		seenPackages := make(map[componentmap.MemberID]struct{})
		seenSymbols := make(map[componentmap.MemberID]struct{})
		for _, member := range component.Members {
			switch member.ID.Kind {
			case componentmap.MemberPackage:
				if _, duplicate := seenPackages[member.ID]; duplicate {
					continue
				}
				seenPackages[member.ID] = struct{}{}
				entry.PackageParticipantIDs = append(entry.PackageParticipantIDs, member.ID)
			case componentmap.MemberSymbol:
				if _, duplicate := seenSymbols[member.ID]; duplicate {
					continue
				}
				seenSymbols[member.ID] = struct{}{}
				source, exact := exactArchitectureComponentSymbolSource(
					component.ID,
					member,
					known,
					locators,
					openable,
				)
				if exact {
					entry.SymbolSources = append(entry.SymbolSources, source)
				}
			}
		}
		projection.Components = append(projection.Components, entry)
	}
	if canvas.LocalRemainderComponentID != "" && !remainderSeen {
		return nil, fmt.Errorf(
			"architecture component navigation: local remainder component %q is absent",
			canvas.LocalRemainderComponentID,
		)
	}
	return projection, nil
}

// ValidateArchitectureComponentNavigation rejects persisted navigation that
// no longer exactly matches its Canvas, typed ancestry, or source locations.
func ValidateArchitectureComponentNavigation(
	canvas *ArchitectureCanvas,
	openablePaths []string,
	projection *ArchitectureComponentNavigationProjection,
) error {
	if canvas == nil {
		if projection != nil {
			return fmt.Errorf("architecture component navigation: projection has no canvas")
		}
		return nil
	}
	expected, err := ProjectArchitectureComponentNavigation(canvas, openablePaths)
	if err != nil {
		return err
	}
	if expected == nil {
		return fmt.Errorf("architecture component navigation: projection is unavailable for a current canvas")
	}
	if projection == nil {
		if len(expected.Components) == 0 {
			return nil
		}
		return fmt.Errorf("architecture component navigation: projection is missing")
	}
	if projection.Version != ArchitectureComponentNavigationVersion {
		return fmt.Errorf(
			"architecture component navigation: unsupported version %d",
			projection.Version,
		)
	}
	if !reflect.DeepEqual(projection, expected) {
		return fmt.Errorf("architecture component navigation: persisted projection does not match exact canvas ancestry")
	}
	return nil
}

func ensureArchitectureComponentNavigation(data *ReportData) error {
	if data == nil {
		return fmt.Errorf("architecture component navigation: report is required")
	}
	// Historical/manual render fixtures are still displayable, but cannot
	// manufacture a current exact navigation projection. Authorized manifest
	// verification independently rejects unsupported Canvas versions.
	if data.ArchitectureComponentNavigation == nil && data.ArchitectureCanvas != nil &&
		data.ArchitectureCanvas.Version != ArchitectureCanvasVersion {
		return nil
	}
	if data.ArchitectureComponentNavigation != nil {
		return ValidateArchitectureComponentNavigation(
			data.ArchitectureCanvas,
			data.OpenablePaths,
			data.ArchitectureComponentNavigation,
		)
	}
	projection, err := ProjectArchitectureComponentNavigation(
		data.ArchitectureCanvas,
		data.OpenablePaths,
	)
	if err != nil {
		return err
	}
	data.ArchitectureComponentNavigation = projection
	return nil
}

func architectureNavigationMemberIndex(
	canvas *ArchitectureCanvas,
) (map[componentmap.MemberID]componentmap.Candidate, map[componentmap.MemberID]ArchitectureStructuralLocator, error) {
	known := make(map[componentmap.MemberID]componentmap.Candidate)
	addKnown := func(candidate componentmap.Candidate) error {
		if candidate.ID.Value == "" {
			return fmt.Errorf("architecture component navigation: empty member id")
		}
		if previous, duplicate := known[candidate.ID]; duplicate {
			if !reflect.DeepEqual(previous, candidate) {
				return fmt.Errorf(
					"architecture component navigation: member %q has conflicting exact records",
					candidate.ID.Value,
				)
			}
			return nil
		}
		known[candidate.ID] = candidate
		return nil
	}
	for _, component := range canvas.Components {
		for _, member := range component.Members {
			if err := addKnown(member); err != nil {
				return nil, nil, err
			}
		}
	}
	locators := make(map[componentmap.MemberID]ArchitectureStructuralLocator, len(canvas.StructuralLocators))
	for _, locator := range canvas.StructuralLocators {
		if locator.Locator.Role != componentmap.CandidateRoleStructuralLocator {
			return nil, nil, fmt.Errorf(
				"architecture component navigation: member %q is not a structural locator",
				locator.Locator.ID.Value,
			)
		}
		if previous, duplicate := locators[locator.Locator.ID]; duplicate {
			if !reflect.DeepEqual(previous, locator) {
				return nil, nil, fmt.Errorf(
					"architecture component navigation: structural locator %q is duplicated",
					locator.Locator.ID.Value,
				)
			}
			continue
		}
		if err := addKnown(locator.Locator); err != nil {
			return nil, nil, err
		}
		locators[locator.Locator.ID] = locator
	}
	return known, locators, nil
}

func exactArchitectureComponentSymbolSource(
	componentID componentmap.ComponentID,
	member componentmap.Candidate,
	known map[componentmap.MemberID]componentmap.Candidate,
	locators map[componentmap.MemberID]ArchitectureStructuralLocator,
	openable map[string]struct{},
) (ArchitectureComponentSymbolSource, bool) {
	if member.ID.Kind != componentmap.MemberSymbol || member.ParentID == nil {
		return ArchitectureComponentSymbolSource{}, false
	}
	seen := map[componentmap.MemberID]struct{}{member.ID: {}}
	parentID := member.ParentID
	for parentID != nil {
		if _, cycle := seen[*parentID]; cycle {
			return ArchitectureComponentSymbolSource{}, false
		}
		seen[*parentID] = struct{}{}
		if locator, saved := locators[*parentID]; saved {
			if locator.Locator.ID.Kind != componentmap.MemberFile ||
				!architectureNavigationComponentParticipates(locator, componentID) {
				return ArchitectureComponentSymbolSource{}, false
			}
			return exactArchitectureSymbolDeclaration(member, locator.Locator, openable)
		}
		parent, exists := known[*parentID]
		if !exists {
			return ArchitectureComponentSymbolSource{}, false
		}
		parentID = parent.ParentID
	}
	return ArchitectureComponentSymbolSource{}, false
}

func architectureNavigationComponentParticipates(
	locator ArchitectureStructuralLocator,
	componentID componentmap.ComponentID,
) bool {
	for _, participantID := range locator.ParticipatingComponentIDs {
		if participantID == componentID {
			return true
		}
	}
	return false
}

func exactArchitectureSymbolDeclaration(
	member componentmap.Candidate,
	locator componentmap.Candidate,
	openable map[string]struct{},
) (ArchitectureComponentSymbolSource, bool) {
	var matched *ArchitectureComponentSymbolSource
	for _, fact := range member.Facts {
		if fact.Kind != componentmap.FactDeclaration || fact.Location == nil ||
			fact.Value == "" || fact.Location.Path == "" || fact.Location.Line <= 0 {
			continue
		}
		if _, allowed := openable[fact.Location.Path]; !allowed {
			continue
		}
		if !architectureLocatorProvesDeclaration(locator, fact.Location.Path) {
			continue
		}
		candidate := ArchitectureComponentSymbolSource{
			MemberID: member.ID,
			Symbol:   fact.Value,
			Location: UserCodeLocation{
				Path: fact.Location.Path, Line: fact.Location.Line, Column: fact.Location.Column,
			},
		}
		if matched == nil {
			copy := candidate
			matched = &copy
			continue
		}
		if *matched != candidate {
			// More than one distinct exact declaration is not permission to pick
			// the first one by fact or serialization order.
			return ArchitectureComponentSymbolSource{}, false
		}
	}
	if matched == nil {
		return ArchitectureComponentSymbolSource{}, false
	}
	return *matched, true
}

func architectureLocatorProvesDeclaration(
	locator componentmap.Candidate,
	sourcePath string,
) bool {
	for _, fact := range locator.Facts {
		if fact.Kind == componentmap.FactRepositoryPath && fact.Location != nil &&
			fact.Value == sourcePath && fact.Location.Path == sourcePath && fact.Location.Line > 0 {
			return true
		}
	}
	return false
}
