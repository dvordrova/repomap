package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
)

const (
	SemanticSearchIndexVersion = 5

	maxSemanticSearchItems         = 1024
	maxSemanticSearchSuggestions   = 8
	minSemanticSearchSuggestions   = 5
	maxSemanticSearchIDBytes       = 128
	maxSemanticSearchTitleBytes    = 256
	maxSemanticSearchQuestionBytes = 512
	maxSemanticSearchSummaryBytes  = 1024
	maxSemanticSearchAliases       = 16
	maxSemanticSearchAliasBytes    = 256
	maxSemanticSearchTargetIDBytes = 256
)

// SemanticSearchIndex is a bounded, presentation-only projection of an
// already coherent report. Suggestions contain item IDs rather than copies so
// every suggested action is validated by the same target contract as Items.
type SemanticSearchIndex struct {
	Version     int                  `json:"version"`
	Items       []SemanticSearchItem `json:"items"`
	Suggestions []string             `json:"suggestions,omitempty"`
	Truncated   bool                 `json:"truncated,omitempty"`
}

type SemanticSearchKind string

const (
	SemanticSearchKindMap               SemanticSearchKind = "map"
	SemanticSearchKindSubsystem         SemanticSearchKind = "subsystem"
	SemanticSearchKindComponent         SemanticSearchKind = "component"
	SemanticSearchKindMember            SemanticSearchKind = "member"
	SemanticSearchKindFlow              SemanticSearchKind = "flow"
	SemanticSearchKindFlowStep          SemanticSearchKind = "flow_step"
	SemanticSearchKindSurface           SemanticSearchKind = "surface"
	SemanticSearchKindGuidedTour        SemanticSearchKind = "guided_tour"
	SemanticSearchKindGuidedStep        SemanticSearchKind = "guided_step"
	SemanticSearchKindDirection         SemanticSearchKind = "direction"
	SemanticSearchKindDomainTerm        SemanticSearchKind = "domain_term"
	SemanticSearchKindWarning           SemanticSearchKind = "warning"
	SemanticSearchKindUnknown           SemanticSearchKind = "unknown"
	SemanticSearchKindLocation          SemanticSearchKind = "location"
	SemanticSearchKindAnchor            SemanticSearchKind = "behavior_anchor"
	SemanticSearchKindMechanism         SemanticSearchKind = "mechanism"
	SemanticSearchKindDependencyUsage   SemanticSearchKind = "dependency_usage"
	SemanticSearchKindRepositoryPattern SemanticSearchKind = "repository_pattern"
	SemanticSearchKindContributionGuide SemanticSearchKind = "contribution_guide"
	SemanticSearchKindGoLearning        SemanticSearchKind = "go_learning"
	SemanticSearchKindRepositoryStory   SemanticSearchKind = "repository_story"
	SemanticSearchKindStudyDirection    SemanticSearchKind = "study_direction"
	SemanticSearchKindPavedPath         SemanticSearchKind = "paved_path"
)

type SemanticSearchStability string

const (
	SemanticSearchStabilityExact       SemanticSearchStability = "exact"
	SemanticSearchStabilityRunStable   SemanticSearchStability = "run_stable"
	SemanticSearchStabilityProvisional SemanticSearchStability = "provisional"
	SemanticSearchStabilityScoped      SemanticSearchStability = "scoped"
)

type SemanticSearchItem struct {
	ID        string                  `json:"id"`
	Kind      SemanticSearchKind      `json:"kind"`
	Title     string                  `json:"title"`
	Summary   string                  `json:"summary,omitempty"`
	Question  string                  `json:"question,omitempty"`
	Aliases   []string                `json:"aliases,omitempty"`
	Stability SemanticSearchStability `json:"stability"`
	Target    SemanticSearchTarget    `json:"target"`
}

type SemanticSearchTargetKind string

const (
	SemanticSearchTargetMap        SemanticSearchTargetKind = "map"
	SemanticSearchTargetComponent  SemanticSearchTargetKind = "component"
	SemanticSearchTargetFlow       SemanticSearchTargetKind = "flow"
	SemanticSearchTargetFlowStep   SemanticSearchTargetKind = "flow_step"
	SemanticSearchTargetSurface    SemanticSearchTargetKind = "surface"
	SemanticSearchTargetGuidedStep SemanticSearchTargetKind = "guided_step"
	SemanticSearchTargetLocation   SemanticSearchTargetKind = "location"
	SemanticSearchTargetArtifact   SemanticSearchTargetKind = "semantic_artifact"
	SemanticSearchTargetStudy      SemanticSearchTargetKind = "study_direction"
	SemanticSearchTargetPavedPath  SemanticSearchTargetKind = "paved_path"
)

// SemanticSearchTarget is a closed navigation contract. Exactly the fields
// required by Kind may be populated; Validate rejects mixed or dangling
// targets rather than repairing them from display text.
type SemanticSearchTarget struct {
	Kind        SemanticSearchTargetKind `json:"kind"`
	ComponentID componentmap.ComponentID `json:"component_id,omitempty"`
	FlowID      componentmap.FlowID      `json:"flow_id,omitempty"`
	StepID      string                   `json:"step_id,omitempty"`
	SurfaceID   string                   `json:"surface_id,omitempty"`
	CandidateID string                   `json:"candidate_id,omitempty"`
	ArtifactID  string                   `json:"artifact_id,omitempty"`
	DirectionID string                   `json:"direction_id,omitempty"`
	PavedPathID string                   `json:"paved_path_id,omitempty"`
	StepIndex   *int                     `json:"step_index,omitempty"`
	Location    *evidence.Location       `json:"location,omitempty"`
}

// BuildSemanticSearchIndex derives a deterministic, bounded index without
// reading repository files or introducing new architecture facts.
func BuildSemanticSearchIndex(data *ReportData) (SemanticSearchIndex, error) {
	if data == nil {
		return SemanticSearchIndex{}, fmt.Errorf("semantic search: report data is required")
	}

	builder := newSemanticSearchBuilder(data)
	if data.RepositoryGuide == nil || data.RepositoryGuide.ArchitectureUseful {
		builder.addMap()
		builder.addCanvas()
	}
	builder.addUserMechanisms()
	builder.addStudyDirections()
	builder.addPavedPaths()
	builder.addOpenableLocations()

	index := builder.finish()
	if err := index.Validate(data); err != nil {
		return SemanticSearchIndex{}, err
	}
	return index, nil
}

func attachSemanticSearchIndex(data *ReportData) error {
	if data == nil {
		return fmt.Errorf("semantic search: report data is required")
	}
	data.SemanticSearch = nil
	if data.SemanticSearchDisabled {
		return nil
	}
	index, err := BuildSemanticSearchIndex(data)
	if err != nil {
		return err
	}
	data.SemanticSearch = &index
	return nil
}

// Validate checks both the bounded wire shape and every exact navigation
// target against the report from which the index is served.
func (index SemanticSearchIndex) Validate(data *ReportData) error {
	if data == nil {
		return fmt.Errorf("semantic search: report data is required")
	}
	if index.Version != SemanticSearchIndexVersion {
		return fmt.Errorf("semantic search: unsupported index version %d", index.Version)
	}
	if len(index.Items) > maxSemanticSearchItems {
		return fmt.Errorf("semantic search: more than %d items", maxSemanticSearchItems)
	}

	itemByID := make(map[string]SemanticSearchItem, len(index.Items))
	suggestible := 0
	for itemIndex, item := range index.Items {
		if err := validateSemanticSearchItem(item, data); err != nil {
			return fmt.Errorf("semantic search: item %d: %w", itemIndex, err)
		}
		if _, duplicate := itemByID[item.ID]; duplicate {
			return fmt.Errorf("semantic search: duplicate item id %q", item.ID)
		}
		itemByID[item.ID] = item
		if item.Target.directlyActionable() && usefulSemanticSearchSuggestion(item) {
			suggestible++
		}
	}

	if len(index.Suggestions) > maxSemanticSearchSuggestions {
		return fmt.Errorf("semantic search: more than %d suggestions", maxSemanticSearchSuggestions)
	}
	if suggestible >= minSemanticSearchSuggestions && len(index.Suggestions) < minSemanticSearchSuggestions {
		return fmt.Errorf("semantic search: fewer than %d suggestions with %d suggestible items", minSemanticSearchSuggestions, suggestible)
	}
	seenSuggestions := make(map[string]struct{}, len(index.Suggestions))
	for suggestionIndex, itemID := range index.Suggestions {
		item, ok := itemByID[itemID]
		if !ok {
			return fmt.Errorf("semantic search: suggestion %d references unknown item %q", suggestionIndex, itemID)
		}
		if !item.Target.directlyActionable() || !usefulSemanticSearchSuggestion(item) {
			return fmt.Errorf("semantic search: suggestion %d is not a suggestible semantic item", suggestionIndex)
		}
		if _, duplicate := seenSuggestions[itemID]; duplicate {
			return fmt.Errorf("semantic search: duplicate suggestion %q", itemID)
		}
		seenSuggestions[itemID] = struct{}{}
	}
	return nil
}

type semanticSearchCandidate struct {
	item     SemanticSearchItem
	priority int
}

type semanticSearchBuilder struct {
	data          *ReportData
	candidates    map[string]semanticSearchCandidate
	components    map[componentmap.ComponentID]struct{}
	flows         map[componentmap.FlowID]ArchitectureFlow
	surfaces      map[string]struct{}
	openablePaths map[string]struct{}
	sourcePaths   map[string][]SourceSnippet
	provisional   map[string]struct{}
}

func newSemanticSearchBuilder(data *ReportData) *semanticSearchBuilder {
	builder := &semanticSearchBuilder{
		data:          data,
		candidates:    make(map[string]semanticSearchCandidate),
		components:    make(map[componentmap.ComponentID]struct{}),
		flows:         make(map[componentmap.FlowID]ArchitectureFlow),
		surfaces:      make(map[string]struct{}),
		openablePaths: make(map[string]struct{}, len(data.OpenablePaths)),
		sourcePaths:   make(map[string][]SourceSnippet),
		provisional:   make(map[string]struct{}),
	}
	for _, path := range data.OpenablePaths {
		if strings.TrimSpace(path) != "" {
			builder.openablePaths[path] = struct{}{}
		}
	}
	for _, snippet := range data.UserSources {
		builder.addSourceSnippet(snippet)
	}
	for _, mechanism := range data.UserMechanisms {
		for _, step := range mechanism.Steps {
			for _, snippet := range step.Sources {
				builder.addSourceSnippet(snippet)
			}
		}
	}
	if data.StudyMap != nil {
		for _, area := range data.StudyMap.Shape {
			if area.Source != nil {
				builder.addSourceSnippet(*area.Source)
			}
		}
		for _, direction := range data.StudyMap.Directions {
			for _, area := range direction.Areas {
				if area.Source != nil {
					builder.addSourceSnippet(*area.Source)
				}
			}
			for _, reading := range direction.ReadingAnchors {
				builder.addSourceSnippet(reading.Source)
			}
		}
	}
	if data.ArchitectureCanvas != nil {
		for _, component := range data.ArchitectureCanvas.Components {
			builder.components[component.ID] = struct{}{}
		}
		for _, flow := range data.ArchitectureCanvas.Flows {
			builder.flows[flow.ID] = flow
		}
		for _, surface := range data.ArchitectureCanvas.Surfaces {
			builder.surfaces[surface.ID] = struct{}{}
		}
	}
	if data.DiscoveredSurfaces != nil {
		for _, trigger := range data.DiscoveredSurfaces.Triggers {
			if trigger.ProvisionalID {
				builder.provisional[trigger.ID] = struct{}{}
			}
		}
	}
	return builder
}

func (builder *semanticSearchBuilder) addMap() {
	title := builder.data.RepoName
	summary := builder.data.ProjectGuess
	aliases := []string{
		builder.data.ProjectGuess,
		"main components",
		"главные компоненты",
		"architecture map",
		"карта архитектуры",
	}
	if canvas := builder.data.ArchitectureCanvas; canvas != nil {
		if strings.TrimSpace(canvas.Title) != "" {
			title = canvas.Title
		}
		aliases = append(aliases, builder.data.RepoName)
	}
	if strings.TrimSpace(title) == "" {
		title = "Repository map"
	}
	builder.add(SemanticSearchItem{
		ID:        semanticSearchID(SemanticSearchKindMap, "map"),
		Kind:      SemanticSearchKindMap,
		Title:     title,
		Summary:   summary,
		Aliases:   aliases,
		Stability: SemanticSearchStabilityRunStable,
		Target:    SemanticSearchTarget{Kind: SemanticSearchTargetMap},
	}, 0)
}

func (builder *semanticSearchBuilder) addCanvas() {
	canvas := builder.data.ArchitectureCanvas
	if canvas == nil {
		return
	}
	for _, subsystem := range canvas.Subsystems {
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindSubsystem, string(subsystem.ID)),
			Kind:      SemanticSearchKindSubsystem,
			Title:     subsystem.Name,
			Summary:   subsystem.Description,
			Aliases:   []string{string(subsystem.Category)},
			Stability: SemanticSearchStabilityExact,
			Target:    SemanticSearchTarget{Kind: SemanticSearchTargetMap},
		}, 70)
	}
	for _, component := range canvas.Components {
		aliases := make([]string, 0, len(component.Members)+1)
		for _, member := range component.Members {
			aliases = append(aliases, member.Name)
		}
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindComponent, string(component.ID)),
			Kind:      SemanticSearchKindComponent,
			Title:     component.Name,
			Summary:   component.Description,
			Aliases:   aliases,
			Stability: SemanticSearchStabilityExact,
			Target: SemanticSearchTarget{
				Kind:        SemanticSearchTargetComponent,
				ComponentID: component.ID,
			},
		}, 20)
		for _, member := range component.Members {
			builder.add(SemanticSearchItem{
				ID: semanticSearchID(
					SemanticSearchKindMember,
					"member",
					string(component.ID),
					string(member.ID.Kind),
					member.ID.Value,
				),
				Kind:      SemanticSearchKindMember,
				Title:     member.Name,
				Summary:   component.Name,
				Aliases:   []string{string(member.ID.Kind)},
				Stability: SemanticSearchStabilityExact,
				Target: SemanticSearchTarget{
					Kind:        SemanticSearchTargetComponent,
					ComponentID: component.ID,
				},
			}, 55)
		}
	}
	for _, flow := range canvas.Flows {
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindFlow, string(flow.ID)),
			Kind:      SemanticSearchKindFlow,
			Title:     flow.Name,
			Summary:   firstSemanticSearchText(flow.MentalModel, flow.Goal, flow.WhyInspect),
			Aliases:   []string{flow.Trigger, flow.Scope, flow.Goal, flow.Command},
			Stability: SemanticSearchStabilityExact,
			Target: SemanticSearchTarget{
				Kind:   SemanticSearchTargetFlow,
				FlowID: flow.ID,
			},
		}, 30)
		for _, step := range flow.Steps {
			builder.add(SemanticSearchItem{
				ID:        semanticSearchID(SemanticSearchKindFlowStep, string(flow.ID), step.ID),
				Kind:      SemanticSearchKindFlowStep,
				Title:     step.Label,
				Summary:   step.QualifiedName,
				Aliases:   []string{string(step.Kind), step.QualifiedName, step.BranchID},
				Stability: SemanticSearchStabilityScoped,
				Target: SemanticSearchTarget{
					Kind:   SemanticSearchTargetFlowStep,
					FlowID: flow.ID,
					StepID: step.ID,
				},
			}, 50)
		}
	}
	for _, surface := range canvas.Surfaces {
		stability := SemanticSearchStabilityExact
		if _, ok := builder.provisional[surface.ID]; ok {
			stability = SemanticSearchStabilityProvisional
		}
		builder.add(SemanticSearchItem{
			ID:      semanticSearchID(SemanticSearchKindSurface, surface.ID),
			Kind:    SemanticSearchKindSurface,
			Title:   surface.Name,
			Summary: firstSemanticSearchText(surface.OwningExecutable, surface.Kind, surface.Category),
			Aliases: []string{
				surface.Kind,
				surface.Category,
				surface.OwningExecutable,
				surface.SurfaceRole,
			},
			Stability: stability,
			Target: SemanticSearchTarget{
				Kind:      SemanticSearchTargetSurface,
				SurfaceID: surface.ID,
			},
		}, 40)
	}
	for _, anchor := range canvas.BehaviorAnchors {
		target := SemanticSearchTarget{Kind: SemanticSearchTargetMap}
		if builder.pathIsOpenable(anchor.Location.Path) {
			location := anchor.Location
			target = SemanticSearchTarget{Kind: SemanticSearchTargetLocation, Location: &location}
		}
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindAnchor, anchor.ID),
			Kind:      SemanticSearchKindAnchor,
			Title:     anchor.Label,
			Aliases:   []string{string(anchor.Kind), anchor.Location.Path},
			Stability: SemanticSearchStabilityExact,
			Target:    target,
		}, 65)
	}
}

func (builder *semanticSearchBuilder) addUserMechanisms() {
	for _, mechanism := range builder.data.UserMechanisms {
		aliases := make([]string, 0, len(mechanism.SearchQueries)+len(mechanism.Steps)+len(mechanism.Files))
		aliases = append(aliases, mechanism.SearchQueries...)
		for _, step := range mechanism.Steps {
			aliases = append(aliases, step.Title, step.Explanation)
		}
		for _, file := range mechanism.Files {
			aliases = append(aliases, file.Path)
		}
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindMechanism, mechanism.ArtifactID),
			Kind:      SemanticSearchKindMechanism,
			Title:     mechanism.Title,
			Summary:   mechanism.Answer,
			Question:  mechanism.Question,
			Aliases:   aliases,
			Stability: SemanticSearchStabilityRunStable,
			Target: SemanticSearchTarget{
				Kind:       SemanticSearchTargetArtifact,
				ArtifactID: mechanism.ArtifactID,
			},
		}, 5)
		for phaseIndex, phase := range mechanism.Phases {
			if len(phase.ImplementationStepIndexes) == 0 {
				continue
			}
			implementationIndex := phase.ImplementationStepIndexes[0]
			phaseAliases := append([]string(nil), mechanism.SearchQueries...)
			phaseAliases = append(phaseAliases, mechanism.Title, mechanism.Question)
			for _, snippet := range phase.Sources {
				phaseAliases = append(phaseAliases, snippet.Path, snippet.EnclosingSymbol)
			}
			builder.add(SemanticSearchItem{
				ID: semanticSearchID(
					SemanticSearchKindMechanism,
					mechanism.ArtifactID,
					"phase",
					strconv.Itoa(phaseIndex),
				),
				Kind:      SemanticSearchKindMechanism,
				Title:     phase.Title,
				Summary:   phase.Explanation,
				Question:  mechanism.Title,
				Aliases:   phaseAliases,
				Stability: SemanticSearchStabilityRunStable,
				Target: SemanticSearchTarget{
					Kind:       SemanticSearchTargetArtifact,
					ArtifactID: mechanism.ArtifactID,
					StepIndex:  &implementationIndex,
				},
			}, 6)
		}
		for index, step := range mechanism.Steps {
			stepIndex := index
			stepAliases := []string{mechanism.Title, mechanism.Question}
			for _, snippet := range step.Sources {
				stepAliases = append(stepAliases, snippet.Path, snippet.EnclosingSymbol)
			}
			builder.add(SemanticSearchItem{
				ID:        semanticSearchID(SemanticSearchKindMechanism, mechanism.ArtifactID, "step", strconv.Itoa(index)),
				Kind:      SemanticSearchKindMechanism,
				Title:     step.Title,
				Summary:   step.Explanation,
				Question:  mechanism.Title,
				Aliases:   stepAliases,
				Stability: SemanticSearchStabilityRunStable,
				Target: SemanticSearchTarget{
					Kind:       SemanticSearchTargetArtifact,
					ArtifactID: mechanism.ArtifactID,
					StepIndex:  &stepIndex,
				},
			}, 6)
		}
	}
}

func (builder *semanticSearchBuilder) addStudyDirections() {
	if builder.data.StudyMap == nil {
		return
	}
	for _, direction := range builder.data.StudyMap.Directions {
		aliases := append([]string(nil), direction.SearchQueries...)
		for _, anchor := range direction.PrincipalAnchors {
			aliases = append(aliases, anchor.Path, anchor.Symbol)
		}
		for _, area := range direction.Areas {
			aliases = append(aliases, area.Name)
		}
		target := SemanticSearchTarget{
			Kind: SemanticSearchTargetStudy, DirectionID: direction.ID,
		}
		if direction.MechanismID != "" {
			target = SemanticSearchTarget{
				Kind: SemanticSearchTargetArtifact, ArtifactID: direction.MechanismID,
			}
		}
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindStudyDirection, direction.ID),
			Kind:      SemanticSearchKindStudyDirection,
			Title:     direction.Question,
			Summary:   direction.LearningOutcome,
			Question:  direction.Question,
			Aliases:   aliases,
			Stability: SemanticSearchStabilityRunStable,
			Target:    target,
		}, 4)
	}
}

func (builder *semanticSearchBuilder) addPavedPaths() {
	if builder.data.Operations == nil {
		return
	}
	for _, pavedPath := range builder.data.Operations.Paths {
		aliases := []string{pavedPath.Goal}
		for _, action := range pavedPath.Actions {
			aliases = append(aliases, action.Instruction, action.Command, action.Endpoint)
		}
		for _, directionID := range pavedPath.RelatedStudyIDs {
			if direction, ok := studyDirectionByID(builder.data, directionID); ok {
				aliases = append(aliases, direction.Question)
			}
		}
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindPavedPath, pavedPath.ID),
			Kind:      SemanticSearchKindPavedPath,
			Title:     pavedPath.Title,
			Summary:   pavedPath.Goal,
			Question:  pavedPath.Title,
			Aliases:   aliases,
			Stability: SemanticSearchStabilityRunStable,
			Target: SemanticSearchTarget{
				Kind: SemanticSearchTargetPavedPath, PavedPathID: pavedPath.ID,
			},
		}, 4)
	}
}

func pavedPathByID(data *ReportData, pavedPathID string) (RepositoryPavedPath, bool) {
	if data == nil || data.Operations == nil {
		return RepositoryPavedPath{}, false
	}
	for _, pavedPath := range data.Operations.Paths {
		if pavedPath.ID == strings.TrimSpace(pavedPathID) {
			return pavedPath, true
		}
	}
	return RepositoryPavedPath{}, false
}

func semanticSearchKindForArtifact(kind string) (SemanticSearchKind, bool) {
	switch kind {
	case string(SemanticSearchKindMechanism):
		return SemanticSearchKindMechanism, true
	case string(SemanticSearchKindDependencyUsage):
		return SemanticSearchKindDependencyUsage, true
	case string(SemanticSearchKindRepositoryPattern):
		return SemanticSearchKindRepositoryPattern, true
	case string(SemanticSearchKindContributionGuide):
		return SemanticSearchKindContributionGuide, true
	case string(SemanticSearchKindGoLearning):
		return SemanticSearchKindGoLearning, true
	case string(SemanticSearchKindRepositoryStory):
		return SemanticSearchKindRepositoryStory, true
	default:
		return "", false
	}
}

func (builder *semanticSearchBuilder) addGuidedTour() {
	tour := builder.data.GuidedTour
	if tour == nil {
		return
	}
	if len(tour.Steps) > 0 {
		stepIndex := 0
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindGuidedTour, tour.CandidateID),
			Kind:      SemanticSearchKindGuidedTour,
			Title:     tour.Title,
			Summary:   tour.Summary,
			Aliases:   []string{tour.CandidateName, tour.Trigger, string(tour.CandidateKind)},
			Stability: SemanticSearchStabilityRunStable,
			Target: SemanticSearchTarget{
				Kind:        SemanticSearchTargetGuidedStep,
				CandidateID: tour.CandidateID,
				StepIndex:   &stepIndex,
			},
		}, 10)
	}
	for index, step := range tour.Steps {
		stepIndex := index
		aliases := []string{tour.Title, tour.CandidateName}
		for _, beat := range step.Beats {
			aliases = append(aliases, beat.Label)
		}
		for _, component := range step.Components {
			aliases = append(aliases, component.Name)
		}
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindGuidedStep, tour.CandidateID, strconv.Itoa(index)),
			Kind:      SemanticSearchKindGuidedStep,
			Title:     step.Title,
			Summary:   step.Explanation,
			Aliases:   aliases,
			Stability: SemanticSearchStabilityScoped,
			Target: SemanticSearchTarget{
				Kind:        SemanticSearchTargetGuidedStep,
				CandidateID: tour.CandidateID,
				StepIndex:   &stepIndex,
			},
		}, 15)
	}
}

func (builder *semanticSearchBuilder) addCandidateDirections() {
	for _, direction := range builder.data.CandidateDirections {
		if direction.Disposition == flowexplain.DirectionRejected {
			continue
		}
		target := SemanticSearchTarget{Kind: SemanticSearchTargetMap}
		flowID := componentmap.FlowID(direction.ID)
		if builder.hasFlow(flowID) {
			target = SemanticSearchTarget{Kind: SemanticSearchTargetFlow, FlowID: flowID}
		}
		aliases := []string{direction.Trigger, direction.LikelyEntrypoint, direction.FlowType}
		aliases = append(aliases, direction.LikelyFiles...)
		aliases = append(aliases, direction.Evidence...)
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindDirection, direction.ID),
			Kind:      SemanticSearchKindDirection,
			Title:     direction.Name,
			Summary:   direction.WhyInteresting,
			Aliases:   aliases,
			Stability: SemanticSearchStabilityRunStable,
			Target:    target,
		}, 60)
	}
}

func (builder *semanticSearchBuilder) addDomainTerms() {
	for _, term := range builder.data.ImportantDomainWords {
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindDomainTerm, term.Word),
			Kind:      SemanticSearchKindDomainTerm,
			Title:     term.Word,
			Summary:   term.Guess,
			Aliases:   term.Evidence,
			Stability: SemanticSearchStabilityRunStable,
			Target:    SemanticSearchTarget{Kind: SemanticSearchTargetMap},
		}, 75)
	}
}

func (builder *semanticSearchBuilder) addFlowWarningsAndUnknowns() {
	for _, flow := range builder.data.Flows {
		target := SemanticSearchTarget{Kind: SemanticSearchTargetMap}
		flowID := componentmap.FlowID(flow.ID)
		if builder.hasFlow(flowID) {
			target = SemanticSearchTarget{Kind: SemanticSearchTargetFlow, FlowID: flowID}
		}
		for _, unknown := range flow.Unknowns {
			builder.add(SemanticSearchItem{
				ID:        semanticSearchID(SemanticSearchKindUnknown, flow.ID, unknown),
				Kind:      SemanticSearchKindUnknown,
				Title:     unknown,
				Summary:   flow.Name,
				Stability: SemanticSearchStabilityRunStable,
				Target:    target,
			}, 90)
		}
		for _, warning := range flow.Warnings {
			builder.add(SemanticSearchItem{
				ID:        semanticSearchID(SemanticSearchKindWarning, flow.ID, warning),
				Kind:      SemanticSearchKindWarning,
				Title:     warning,
				Summary:   flow.Name,
				Stability: SemanticSearchStabilityRunStable,
				Target:    target,
			}, 90)
		}
	}
}

func (builder *semanticSearchBuilder) addReportWarnings() {
	for _, warning := range builder.data.Warnings {
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindWarning, "report", warning),
			Kind:      SemanticSearchKindWarning,
			Title:     warning,
			Stability: SemanticSearchStabilityRunStable,
			Target:    SemanticSearchTarget{Kind: SemanticSearchTargetMap},
		}, 95)
	}
}

func (builder *semanticSearchBuilder) addOpenableLocations() {
	paths := make([]string, 0, len(builder.openablePaths))
	for path := range builder.openablePaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		location := evidence.Location{Path: path}
		builder.add(SemanticSearchItem{
			ID:        semanticSearchID(SemanticSearchKindLocation, path),
			Kind:      SemanticSearchKindLocation,
			Title:     path,
			Stability: SemanticSearchStabilityExact,
			Target: SemanticSearchTarget{
				Kind:     SemanticSearchTargetLocation,
				Location: &location,
			},
		}, 100)
	}
}

func (builder *semanticSearchBuilder) add(item SemanticSearchItem, priority int) {
	item.Title = boundedSemanticSearchText(item.Title, maxSemanticSearchTitleBytes)
	if item.Title == "" {
		return
	}
	item.Summary = boundedSemanticSearchText(item.Summary, maxSemanticSearchSummaryBytes)
	item.Question = boundedSemanticSearchText(item.Question, maxSemanticSearchQuestionBytes)
	item.Aliases = boundedSemanticSearchAliases(item.Aliases, item.Title, item.Summary, item.Question)

	if existing, ok := builder.candidates[item.ID]; ok {
		merged := existing.item
		if merged.Summary == "" {
			merged.Summary = item.Summary
		}
		if merged.Question == "" {
			merged.Question = item.Question
		}
		additionalAliases := append([]string{item.Title, item.Summary}, item.Aliases...)
		merged.Aliases = boundedSemanticSearchAliases(
			append(append([]string(nil), merged.Aliases...), additionalAliases...),
			merged.Title,
			merged.Summary,
			merged.Question,
		)
		if !merged.Target.directlyActionable() && item.Target.directlyActionable() {
			merged.Target = item.Target
		}
		if item.Stability == SemanticSearchStabilityProvisional {
			merged.Stability = item.Stability
		}
		if priority < existing.priority {
			existing.priority = priority
		}
		existing.item = merged
		builder.candidates[item.ID] = existing
		return
	}
	builder.candidates[item.ID] = semanticSearchCandidate{item: item, priority: priority}
}

func (builder *semanticSearchBuilder) finish() SemanticSearchIndex {
	candidates := make([]semanticSearchCandidate, 0, len(builder.candidates))
	for _, candidate := range builder.candidates {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		left, right := candidates[i].item, candidates[j].item
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Title != right.Title {
			return left.Title < right.Title
		}
		return left.ID < right.ID
	})

	index := SemanticSearchIndex{
		Version:   SemanticSearchIndexVersion,
		Truncated: len(candidates) > maxSemanticSearchItems,
	}
	if len(candidates) > maxSemanticSearchItems {
		candidates = candidates[:maxSemanticSearchItems]
	}
	index.Items = make([]SemanticSearchItem, 0, len(candidates))
	for _, candidate := range candidates {
		index.Items = append(index.Items, candidate.item)
	}
	index.Suggestions = semanticSearchSuggestions(index.Items)
	return index
}

func semanticSearchSuggestions(items []SemanticSearchItem) []string {
	kindOrder := []SemanticSearchKind{
		SemanticSearchKindRepositoryStory,
		SemanticSearchKindStudyDirection,
		SemanticSearchKindPavedPath,
		SemanticSearchKindMechanism,
		SemanticSearchKindDependencyUsage,
		SemanticSearchKindRepositoryPattern,
		SemanticSearchKindContributionGuide,
		SemanticSearchKindGoLearning,
		SemanticSearchKindGuidedTour,
		SemanticSearchKindFlow,
		SemanticSearchKindSurface,
		SemanticSearchKindMap,
		SemanticSearchKindDirection,
		SemanticSearchKindDomainTerm,
		SemanticSearchKindComponent,
		SemanticSearchKindGuidedStep,
		SemanticSearchKindFlowStep,
		SemanticSearchKindAnchor,
		SemanticSearchKindSubsystem,
	}
	selected := make([]string, 0, maxSemanticSearchSuggestions)
	seen := make(map[string]struct{}, maxSemanticSearchSuggestions)
	for len(selected) < maxSemanticSearchSuggestions {
		added := false
		for _, kind := range kindOrder {
			for _, item := range items {
				if item.Kind != kind || !item.Target.directlyActionable() || !usefulSemanticSearchSuggestion(item) {
					continue
				}
				if _, ok := seen[item.ID]; ok {
					continue
				}
				selected = append(selected, item.ID)
				seen[item.ID] = struct{}{}
				added = true
				break
			}
			if len(selected) == maxSemanticSearchSuggestions {
				return selected
			}
		}
		if !added {
			break
		}
	}
	if len(selected) < minSemanticSearchSuggestions {
		for _, item := range items {
			if !item.Target.directlyActionable() || !usefulSemanticSearchSuggestion(item) {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			selected = append(selected, item.ID)
			seen[item.ID] = struct{}{}
			if len(selected) == minSemanticSearchSuggestions {
				break
			}
		}
	}
	return selected
}

func usefulSemanticSearchSuggestion(item SemanticSearchItem) bool {
	if item.Kind != SemanticSearchKindComponent {
		return item.Kind != SemanticSearchKindMember &&
			item.Kind != SemanticSearchKindLocation &&
			item.Kind != SemanticSearchKindWarning && item.Kind != SemanticSearchKindUnknown
	}
	title := strings.ToLower(item.Title)
	return !strings.Contains(title, "playground") &&
		!strings.Contains(title, "example") &&
		!strings.Contains(title, "additional responsibilities")
}

func validateSemanticSearchItem(item SemanticSearchItem, data *ReportData) error {
	if item.ID == "" || len(item.ID) > maxSemanticSearchIDBytes {
		return fmt.Errorf("invalid id")
	}
	if !item.Kind.valid() {
		return fmt.Errorf("unsupported kind %q", item.Kind)
	}
	if strings.TrimSpace(item.Title) == "" || len(item.Title) > maxSemanticSearchTitleBytes {
		return fmt.Errorf("invalid title")
	}
	if len(item.Summary) > maxSemanticSearchSummaryBytes {
		return fmt.Errorf("summary exceeds %d bytes", maxSemanticSearchSummaryBytes)
	}
	if len(item.Question) > maxSemanticSearchQuestionBytes {
		return fmt.Errorf("question exceeds %d bytes", maxSemanticSearchQuestionBytes)
	}
	if item.Question != "" && (strings.TrimSpace(item.Question) != item.Question || !utf8.ValidString(item.Question)) {
		return fmt.Errorf("invalid question")
	}
	if !item.Stability.valid() {
		return fmt.Errorf("unsupported stability %q", item.Stability)
	}
	if len(item.Aliases) > maxSemanticSearchAliases {
		return fmt.Errorf("more than %d aliases", maxSemanticSearchAliases)
	}
	seenAliases := make(map[string]struct{}, len(item.Aliases))
	for aliasIndex, alias := range item.Aliases {
		if strings.TrimSpace(alias) == "" || len(alias) > maxSemanticSearchAliasBytes {
			return fmt.Errorf("invalid alias %d", aliasIndex)
		}
		if _, duplicate := seenAliases[alias]; duplicate {
			return fmt.Errorf("duplicate alias %q", alias)
		}
		seenAliases[alias] = struct{}{}
	}
	return validateSemanticSearchTarget(item.Target, data)
}

func validateSemanticSearchTarget(target SemanticSearchTarget, data *ReportData) error {
	if err := validateSemanticSearchTargetIdentifier(string(target.ComponentID)); err != nil {
		return fmt.Errorf("component id: %w", err)
	}
	if err := validateSemanticSearchTargetIdentifier(string(target.FlowID)); err != nil {
		return fmt.Errorf("flow id: %w", err)
	}
	for label, value := range map[string]string{
		"step id":       target.StepID,
		"surface id":    target.SurfaceID,
		"candidate id":  target.CandidateID,
		"artifact id":   target.ArtifactID,
		"direction id":  target.DirectionID,
		"paved path id": target.PavedPathID,
	} {
		if err := validateSemanticSearchTargetIdentifier(value); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}

	canvas := data.ArchitectureCanvas
	switch target.Kind {
	case SemanticSearchTargetMap:
		if target.hasPayload() {
			return fmt.Errorf("map target contains object fields")
		}
	case SemanticSearchTargetComponent:
		if target.ComponentID == "" || target.hasPayloadOtherThan("component") || !canvasHasComponent(canvas, target.ComponentID) {
			return fmt.Errorf("component target is not present on the canvas")
		}
	case SemanticSearchTargetFlow:
		if target.FlowID == "" || target.hasPayloadOtherThan("flow") || !canvasHasFlow(canvas, target.FlowID) {
			return fmt.Errorf("flow target is not present on the canvas")
		}
	case SemanticSearchTargetFlowStep:
		if target.FlowID == "" || target.StepID == "" || target.hasPayloadOtherThan("flow_step") ||
			!canvasHasFlowStep(canvas, target.FlowID, target.StepID) {
			return fmt.Errorf("flow step target is not present on the canvas")
		}
	case SemanticSearchTargetSurface:
		if target.SurfaceID == "" || target.hasPayloadOtherThan("surface") || !canvasHasSurface(canvas, target.SurfaceID) {
			return fmt.Errorf("surface target is not present on the canvas")
		}
	case SemanticSearchTargetGuidedStep:
		if target.CandidateID == "" || target.StepIndex == nil || target.hasPayloadOtherThan("guided_step") ||
			data.GuidedTour == nil || data.GuidedTour.CandidateID != target.CandidateID ||
			*target.StepIndex < 0 || *target.StepIndex >= len(data.GuidedTour.Steps) {
			return fmt.Errorf("guided step target is not present in the saved story")
		}
	case SemanticSearchTargetArtifact:
		if target.ArtifactID == "" || target.hasPayloadOtherThan("semantic_artifact") ||
			!reportHasUserMechanismStep(data, target.ArtifactID, target.StepIndex) {
			return fmt.Errorf("semantic artifact target is not present in the user report")
		}
	case SemanticSearchTargetStudy:
		if target.DirectionID == "" || target.hasPayloadOtherThan("study_direction") {
			return fmt.Errorf("study direction target is incomplete")
		}
		if _, ok := studyDirectionByID(data, target.DirectionID); !ok {
			return fmt.Errorf("study direction target is not present in the user report")
		}
	case SemanticSearchTargetPavedPath:
		if target.PavedPathID == "" || target.hasPayloadOtherThan("paved_path") {
			return fmt.Errorf("paved path target is incomplete")
		}
		if _, ok := pavedPathByID(data, target.PavedPathID); !ok {
			return fmt.Errorf("paved path target is not present in the user report")
		}
	case SemanticSearchTargetLocation:
		if target.Location == nil || target.Location.Path == "" || target.hasPayloadOtherThan("location") {
			return fmt.Errorf("location target is incomplete")
		}
		if !stringSliceContains(data.OpenablePaths, target.Location.Path) {
			return fmt.Errorf("location path %q is not openable", target.Location.Path)
		}
	default:
		return fmt.Errorf("unsupported target kind %q", target.Kind)
	}
	return nil
}

func (target SemanticSearchTarget) directlyActionable() bool {
	switch target.Kind {
	case SemanticSearchTargetMap,
		SemanticSearchTargetComponent,
		SemanticSearchTargetFlow,
		SemanticSearchTargetFlowStep,
		SemanticSearchTargetSurface,
		SemanticSearchTargetGuidedStep,
		SemanticSearchTargetArtifact,
		SemanticSearchTargetStudy,
		SemanticSearchTargetPavedPath,
		SemanticSearchTargetLocation:
		return true
	default:
		return false
	}
}

func (target SemanticSearchTarget) hasPayload() bool {
	return target.ComponentID != "" || target.FlowID != "" || target.StepID != "" ||
		target.SurfaceID != "" || target.CandidateID != "" || target.ArtifactID != "" ||
		target.DirectionID != "" || target.PavedPathID != "" || target.StepIndex != nil || target.Location != nil
}

func (target SemanticSearchTarget) hasPayloadOtherThan(allowed string) bool {
	if allowed != "component" && target.ComponentID != "" {
		return true
	}
	if allowed != "flow" && allowed != "flow_step" && target.FlowID != "" {
		return true
	}
	if allowed != "flow_step" && target.StepID != "" {
		return true
	}
	if allowed != "surface" && target.SurfaceID != "" {
		return true
	}
	if allowed != "guided_step" && target.CandidateID != "" {
		return true
	}
	if allowed != "guided_step" && allowed != "semantic_artifact" && target.StepIndex != nil {
		return true
	}
	if allowed != "semantic_artifact" && target.ArtifactID != "" {
		return true
	}
	if allowed != "study_direction" && target.DirectionID != "" {
		return true
	}
	if allowed != "paved_path" && target.PavedPathID != "" {
		return true
	}
	return allowed != "location" && target.Location != nil
}

func (kind SemanticSearchKind) valid() bool {
	switch kind {
	case SemanticSearchKindMap,
		SemanticSearchKindSubsystem,
		SemanticSearchKindComponent,
		SemanticSearchKindMember,
		SemanticSearchKindFlow,
		SemanticSearchKindFlowStep,
		SemanticSearchKindSurface,
		SemanticSearchKindGuidedTour,
		SemanticSearchKindGuidedStep,
		SemanticSearchKindDirection,
		SemanticSearchKindDomainTerm,
		SemanticSearchKindWarning,
		SemanticSearchKindUnknown,
		SemanticSearchKindLocation,
		SemanticSearchKindAnchor,
		SemanticSearchKindMechanism,
		SemanticSearchKindDependencyUsage,
		SemanticSearchKindRepositoryPattern,
		SemanticSearchKindContributionGuide,
		SemanticSearchKindGoLearning,
		SemanticSearchKindRepositoryStory,
		SemanticSearchKindStudyDirection,
		SemanticSearchKindPavedPath:
		return true
	default:
		return false
	}
}

func (stability SemanticSearchStability) valid() bool {
	switch stability {
	case SemanticSearchStabilityExact,
		SemanticSearchStabilityRunStable,
		SemanticSearchStabilityProvisional,
		SemanticSearchStabilityScoped:
		return true
	default:
		return false
	}
}

func (builder *semanticSearchBuilder) hasComponent(id componentmap.ComponentID) bool {
	_, ok := builder.components[id]
	return ok
}

func (builder *semanticSearchBuilder) hasFlow(id componentmap.FlowID) bool {
	_, ok := builder.flows[id]
	return ok
}

func (builder *semanticSearchBuilder) pathIsOpenable(path string) bool {
	_, ok := builder.openablePaths[path]
	return ok
}

func (builder *semanticSearchBuilder) addSourceSnippet(snippet SourceSnippet) {
	if snippet.Path == "" || len(snippet.Lines) == 0 || !builder.pathIsOpenable(snippet.Path) {
		return
	}
	builder.sourcePaths[snippet.Path] = append(builder.sourcePaths[snippet.Path], snippet)
}

func (builder *semanticSearchBuilder) pathHasSource(path string) bool {
	return len(builder.sourcePaths[path]) > 0
}

func (builder *semanticSearchBuilder) flowOrLocationTarget(
	flowID componentmap.FlowID,
	location *evidence.Location,
) SemanticSearchTarget {
	if builder.hasFlow(flowID) {
		return SemanticSearchTarget{Kind: SemanticSearchTargetFlow, FlowID: flowID}
	}
	if location != nil && builder.pathIsOpenable(location.Path) {
		copy := *location
		return SemanticSearchTarget{Kind: SemanticSearchTargetLocation, Location: &copy}
	}
	return SemanticSearchTarget{Kind: SemanticSearchTargetMap}
}

func semanticSearchID(kind SemanticSearchKind, parts ...string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(kind))
	for _, part := range parts {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(part))
	}
	digest := hash.Sum(nil)
	return string(kind) + ":" + hex.EncodeToString(digest)
}

func firstSemanticSearchText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func boundedSemanticSearchText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value)
}

func boundedSemanticSearchAliases(values []string, excluded ...string) []string {
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, value := range excluded {
		value = boundedSemanticSearchText(value, maxSemanticSearchAliasBytes)
		if value != "" {
			excludedSet[value] = struct{}{}
		}
	}
	unique := make(map[string]struct{}, len(values))
	aliases := make([]string, 0, min(len(values), maxSemanticSearchAliases))
	for _, value := range values {
		value = boundedSemanticSearchText(value, maxSemanticSearchAliasBytes)
		if value == "" {
			continue
		}
		if _, skip := excludedSet[value]; skip {
			continue
		}
		if _, duplicate := unique[value]; duplicate {
			continue
		}
		unique[value] = struct{}{}
		aliases = append(aliases, value)
	}
	sort.Strings(aliases)
	if len(aliases) > maxSemanticSearchAliases {
		aliases = aliases[:maxSemanticSearchAliases]
	}
	return aliases
}

func validateSemanticSearchTargetIdentifier(value string) error {
	if len(value) > maxSemanticSearchTargetIDBytes {
		return fmt.Errorf("exceeds %d bytes", maxSemanticSearchTargetIDBytes)
	}
	return nil
}

func canvasHasComponent(canvas *ArchitectureCanvas, id componentmap.ComponentID) bool {
	if canvas == nil {
		return false
	}
	for _, component := range canvas.Components {
		if component.ID == id {
			return true
		}
	}
	return false
}

func canvasHasFlow(canvas *ArchitectureCanvas, id componentmap.FlowID) bool {
	if canvas == nil {
		return false
	}
	for _, flow := range canvas.Flows {
		if flow.ID == id {
			return true
		}
	}
	return false
}

func canvasHasFlowStep(canvas *ArchitectureCanvas, flowID componentmap.FlowID, stepID string) bool {
	if canvas == nil {
		return false
	}
	for _, flow := range canvas.Flows {
		if flow.ID != flowID {
			continue
		}
		for _, step := range flow.Steps {
			if step.ID == stepID {
				return true
			}
		}
		return false
	}
	return false
}

func canvasHasSurface(canvas *ArchitectureCanvas, id string) bool {
	if canvas == nil {
		return false
	}
	for _, surface := range canvas.Surfaces {
		if surface.ID == id {
			return true
		}
	}
	return false
}

func reportHasUserMechanism(data *ReportData, id string) bool {
	if data == nil {
		return false
	}
	for _, mechanism := range data.UserMechanisms {
		if mechanism.ArtifactID == id {
			return true
		}
	}
	return false
}

func reportHasUserMechanismStep(data *ReportData, id string, stepIndex *int) bool {
	if data == nil {
		return false
	}
	for _, mechanism := range data.UserMechanisms {
		if mechanism.ArtifactID != id {
			continue
		}
		return stepIndex == nil || *stepIndex >= 0 && *stepIndex < len(mechanism.Steps)
	}
	return false
}

func reportHasSourceSnippet(data *ReportData, sourcePath string) bool {
	if data == nil {
		return false
	}
	for _, snippet := range data.UserSources {
		if snippet.Path == sourcePath && len(snippet.Lines) > 0 {
			return true
		}
	}
	for _, mechanism := range data.UserMechanisms {
		for _, step := range mechanism.Steps {
			for _, snippet := range step.Sources {
				if snippet.Path == sourcePath && len(snippet.Lines) > 0 {
					return true
				}
			}
		}
	}
	if data.StudyMap != nil {
		for _, area := range data.StudyMap.Shape {
			if area.Source != nil && area.Source.Path == sourcePath && len(area.Source.Lines) > 0 {
				return true
			}
		}
		for _, direction := range data.StudyMap.Directions {
			for _, reading := range direction.ReadingAnchors {
				if reading.Source.Path == sourcePath && len(reading.Source.Lines) > 0 {
					return true
				}
			}
		}
	}
	return false
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
