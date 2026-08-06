package report

import (
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	minNarrativePhases = 3
	maxNarrativePhases = 6
	maxThesisAreas     = 7
)

// OnboardingRole is a presentation-only classification of an accepted
// Mechanism. The zero value means that local role selection has not run.
type OnboardingRole string

const (
	OnboardingRoleUnknown            OnboardingRole = ""
	OnboardingRolePrimaryBehavior    OnboardingRole = "primary_behavior"
	OnboardingRoleSecondaryBehavior  OnboardingRole = "secondary_behavior"
	OnboardingRoleExtensionPoint     OnboardingRole = "extension_point"
	OnboardingRoleOperationalSupport OnboardingRole = "operational_support"
	OnboardingRoleErrorDetail        OnboardingRole = "error_detail"
)

// RepositoryThesis is a bounded, presentation-only repository overview. All
// navigation values resolve to objects or paths already present in ReportData.
type RepositoryThesis struct {
	Purpose               string                 `json:"purpose"`
	SystemStory           []string               `json:"system_story,omitempty"`
	Areas                 []RepositoryThesisArea `json:"areas,omitempty"`
	RecommendedArtifactID string                 `json:"recommended_artifact_id,omitempty"`
}

// RepositoryThesisArea is one clickable projection of an existing report
// component, canvas component, or exact repository file.
type RepositoryThesisArea struct {
	Label          string            `json:"label"`
	Responsibility string            `json:"responsibility"`
	Role           componentmap.Role `json:"role,omitempty"`
	CodeLocation   *UserCodeLocation `json:"code_location,omitempty"`
	MapTarget      *UserMapTarget    `json:"map_target,omitempty"`
}

// RepositoryGuide orders existing presentation objects around concrete user
// jobs. It introduces no behavior, relationship, or source claim of its own.
type RepositoryGuide struct {
	Purpose              string                 `json:"purpose"`
	SystemStory          []string               `json:"system_story,omitempty"`
	Areas                []RepositoryThesisArea `json:"areas,omitempty"`
	StartHereArtifactID  string                 `json:"start_here_artifact_id,omitempty"`
	ExtensionArtifactIDs []string               `json:"extension_artifact_ids,omitempty"`
	MorePathArtifactIDs  []string               `json:"more_path_artifact_ids,omitempty"`
	ReadNext             []UserReadNextTarget   `json:"read_next,omitempty"`
	ArchitectureUseful   bool                   `json:"architecture_useful,omitempty"`
}

// UserMechanismContext keeps a primary walkthrough next to existing
// architecture or code-area references without asserting a new edge.
type UserMechanismContext struct {
	Label          string            `json:"label"`
	Responsibility string            `json:"responsibility"`
	CodeLocation   *UserCodeLocation `json:"code_location,omitempty"`
	MapTarget      *UserMapTarget    `json:"map_target,omitempty"`
}

// NarrativeOrderingBasis makes it explicit that phase order is editorial and
// is not evidence of runtime sequence.
type NarrativeOrderingBasis string

const NarrativeOrderingEditorial NarrativeOrderingBasis = "editorial"

// NarrativeCompression is an optional editorial proposal over opaque,
// already accepted statement IDs. Its copy is not semantic authority.
type NarrativeCompression struct {
	ArtifactID    string                      `json:"artifact_id"`
	OrderingBasis NarrativeOrderingBasis      `json:"ordering_basis"`
	Phases        []NarrativeCompressionPhase `json:"phases"`
}

type NarrativeCompressionPhase struct {
	Title              string   `json:"title"`
	Explanation        string   `json:"explanation,omitempty"`
	MemberStatementIDs []string `json:"member_statement_ids"`
}

// UserMechanismPhase is the locally resolved user projection of one proposed
// phase. Exact source values are unioned only from its canonical member steps.
type UserMechanismPhase struct {
	Title                     string              `json:"title"`
	Explanation               string              `json:"explanation"`
	Locations                 []UserCodeLocation  `json:"locations"`
	Sources                   []SourceSnippet     `json:"sources,omitempty"`
	WhatToNotice              []SourceNotice      `json:"what_to_notice,omitempty"`
	MapTarget                 *UserMapTarget      `json:"map_target,omitempty"`
	ImplementationStepIndexes []int               `json:"implementation_step_indexes"`
	ImplementationDetails     []UserMechanismStep `json:"implementation_details,omitempty"`
}

// DeriveMechanismOnboardingRole classifies only an already accepted, visible
// Mechanism projection. Strong extension/error shapes are handled before a
// behavior can qualify as a primary path.
func DeriveMechanismOnboardingRole(data *ReportData, mechanism UserMechanism) OnboardingRole {
	text := userMechanismRoleText(mechanism)
	switch mechanism.OpportunityKind {
	case semanticdiscovery.OpportunityKindExtensionPath:
		return OnboardingRoleExtensionPoint
	case semanticdiscovery.OpportunityKindQuestionPath:
		return OnboardingRoleSecondaryBehavior
	case semanticdiscovery.OpportunityKindMaintenanceBoundary:
		return OnboardingRoleOperationalSupport
	case semanticdiscovery.OpportunityKindCentralBehavior:
		// Continue through the ordinary correctness and usefulness gates below.
	default:
		if mechanismHasExtensionShape(text) {
			return OnboardingRoleExtensionPoint
		}
	}
	meaningfulSteps := 0
	errorSteps := 0
	for _, step := range mechanism.Steps {
		if userMechanismStepIsErrorDetail(step) {
			errorSteps++
			continue
		}
		meaningfulSteps++
	}
	if errorSteps > 0 && errorSteps*2 >= len(mechanism.Steps) {
		return OnboardingRoleErrorDetail
	}
	if meaningfulSteps >= minNarrativePhases && mechanismHasPrimaryPathContract(mechanism) &&
		mechanismPurposeAligned(data, text) {
		return OnboardingRolePrimaryBehavior
	}
	if meaningfulSteps >= minNarrativePhases && mechanismHasInputAndEffect(text) &&
		mechanismPurposeAligned(data, text) && mechanismCoreAreaCoverage(data, mechanism) > 0 {
		return OnboardingRolePrimaryBehavior
	}
	if mechanismHasOperationalShape(text) {
		return OnboardingRoleOperationalSupport
	}
	return OnboardingRoleSecondaryBehavior
}

func mechanismHasPrimaryPathContract(mechanism UserMechanism) bool {
	required := map[string]bool{
		"input-trigger":     false,
		"core-work":         false,
		"observable-effect": false,
	}
	for _, id := range mechanism.canonicalCoveredAspectIDs {
		if _, exists := required[id]; exists {
			required[id] = true
		}
	}
	for _, covered := range required {
		if !covered {
			return false
		}
	}
	return true
}

func sortUserMechanisms(mechanisms []UserMechanism) {
	sort.SliceStable(mechanisms, func(i, j int) bool {
		leftRank := onboardingRoleRank(mechanisms[i].Role)
		rightRank := onboardingRoleRank(mechanisms[j].Role)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if mechanisms[i].Title != mechanisms[j].Title {
			return mechanisms[i].Title < mechanisms[j].Title
		}
		return mechanisms[i].ArtifactID < mechanisms[j].ArtifactID
	})
}

func onboardingRoleRank(role OnboardingRole) int {
	switch role {
	case OnboardingRolePrimaryBehavior:
		return 0
	case OnboardingRoleSecondaryBehavior:
		return 1
	case OnboardingRoleExtensionPoint:
		return 2
	case OnboardingRoleOperationalSupport:
		return 3
	case OnboardingRoleErrorDetail:
		return 4
	default:
		return 5
	}
}

func userMechanismRoleText(mechanism UserMechanism) string {
	parts := []string{mechanism.Title, mechanism.Question, mechanism.Answer}
	for _, step := range mechanism.Steps {
		parts = append(parts, step.Title, step.Explanation)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func mechanismHasExtensionShape(text string) bool {
	return containsAnyOnboardingPhrase(text,
		"registry", "registration", "register ", "registered ", "factory",
		"plugin", "adapter construction", "backend lookup", "implementation selection",
		"scheme-to-factory", "url scheme",
	)
}

func mechanismHasOperationalShape(text string) bool {
	return containsAnyOnboardingPhrase(text,
		"structured logging", "logger middleware", "observability", "metrics",
		"health check", "startup", "shutdown", "maintenance", "migration",
	)
}

func mechanismHasInputAndEffect(text string) bool {
	hasInput := containsAnyOnboardingPhrase(text,
		"request", "query", "command", "input", "entry point", "incoming",
		"receive", "read ", "parse ", "handler",
	)
	hasEffect := containsAnyOnboardingPhrase(text,
		"response", "output", "write", "persist", "store", "replicat", "restore",
		"endpoint", "invoke", "dispatch", "publish", "send ", "result",
	)
	return hasInput && hasEffect
}

func mechanismPurposeAligned(data *ReportData, mechanismText string) bool {
	if data == nil {
		return false
	}
	purpose := strings.TrimSpace(data.DocumentedPurpose + " " + data.ProjectGuess)
	return purpose != "" && userMechanismTopicCovered(mechanismText, purpose)
}

func mechanismCoreAreaCoverage(data *ReportData, mechanism UserMechanism) int {
	if data == nil {
		return 0
	}
	count := 0
	for _, component := range data.Components {
		switch component.Role {
		case componentmap.RoleDomain, componentmap.RoleCoordination,
			componentmap.RoleState, componentmap.RoleBoundary:
		default:
			continue
		}
		matched := false
		for _, file := range mechanism.Files {
			if reportComponentTouchesPath(component, file.Path) {
				matched = true
				break
			}
		}
		if !matched && len(mechanism.Files) >= 2 {
			matched = mechanismComponentTopicAligned(
				userMechanismRoleText(mechanism),
				component.Name+" "+component.ModelPurpose,
			)
		}
		if matched {
			count++
		}
	}
	return count
}

func mechanismComponentTopicAligned(mechanismText, componentText string) bool {
	mechanismTerms := userMechanismTopicWords(mechanismText)
	componentTerms := userMechanismTopicWords(componentText)
	for term := range componentTerms {
		if term == "core" || term == "code" || term == "repository" || len(term) < 6 {
			continue
		}
		if _, exact := mechanismTerms[term]; exact {
			return true
		}
		for mechanismTerm := range mechanismTerms {
			if len(mechanismTerm) >= 5 && mechanismTerm[:5] == term[:5] {
				return true
			}
		}
	}
	return false
}

func reportComponentTouchesPath(component Component, filePath string) bool {
	filePath = cleanSurfacePath(filePath)
	for _, anchor := range component.AnchorGroups {
		anchorPath := cleanSurfacePath(anchor.Path)
		if anchorPath == "" {
			continue
		}
		if anchorPath == filePath {
			return true
		}
		dir := path.Dir(anchorPath)
		if dir != "." && strings.HasPrefix(filePath, strings.TrimSuffix(dir, "/")+"/") {
			return true
		}
	}
	return false
}

func userMechanismStepIsErrorDetail(step UserMechanismStep) bool {
	title := strings.ToLower(strings.TrimSpace(step.Title))
	if containsAnyOnboardingPhrase(title,
		"error propagation", "error return", "error check", "failure", "fallback",
		"not-found", "not found", "method-not-allowed", "method not allowed",
		"unsupported case", "validation error", "reject invalid",
	) {
		return true
	}
	// A substantive action title owns the step's presentation purpose. Error
	// flags or fallback details mentioned inside its explanation remain
	// subordinate instead of reclassifying the whole action.
	if title != "" && !strings.HasPrefix(title, "step ") && title != "implementation" {
		return false
	}
	explanation := strings.ToLower(strings.TrimSpace(step.Explanation))
	return containsAnyOnboardingPhrase(explanation,
		"error propagation", "returns an error", "failure path", "fallback",
		"not-found", "not found", "method-not-allowed", "method not allowed",
		"unsupported case", "validation error", "rejects invalid",
	)
}

func containsAnyOnboardingPhrase(text string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

// FinalizeRepositoryOnboarding derives the complete presentation projection
// after all accepted Mechanisms have been replayed. Invalid or duplicate
// compression proposals fall back to deterministic grouping of canonical
// steps; no proposal can mutate those steps.
func FinalizeRepositoryOnboarding(data *ReportData, compressions []NarrativeCompression) {
	if data == nil {
		return
	}

	preferredArtifactID := data.StartHereArtifactID
	data.StartHereArtifactID = ""
	compressionByArtifact := make(map[string]NarrativeCompression, len(compressions))
	compressionCounts := make(map[string]int, len(compressions))
	for _, compression := range compressions {
		artifactID := strings.TrimSpace(compression.ArtifactID)
		if artifactID == "" {
			continue
		}
		compressionCounts[artifactID]++
		compressionByArtifact[artifactID] = compression
	}

	primaryIndex := -1
	primaryScore := 0
	for index := range data.UserMechanisms {
		mechanism := &data.UserMechanisms[index]
		mechanism.Role = DeriveMechanismOnboardingRole(data, *mechanism)
		mechanism.Phases = nil
		mechanism.Context = nil
		compressionApplied := false
		if compressionCounts[mechanism.ArtifactID] == 1 {
			if phases, ok := ProjectNarrativeCompression(*mechanism, compressionByArtifact[mechanism.ArtifactID]); ok {
				mechanism.Phases = phases
				compressionApplied = true
			}
		}
		if !compressionApplied {
			if fallback, ok := deterministicNarrativeCompression(*mechanism); ok {
				mechanism.Phases, _ = ProjectNarrativeCompression(*mechanism, fallback)
			}
		}
		mechanism.ReadNext = deriveMechanismReadNext(*mechanism)
		if mechanism.OpportunityKind == semanticdiscovery.OpportunityKindExtensionPath &&
			mechanism.Role == OnboardingRoleExtensionPoint && len(mechanism.ReadNext) < 2 {
			mechanism.Role = OnboardingRoleSecondaryBehavior
		}
		if mechanism.Role != OnboardingRolePrimaryBehavior {
			continue
		}
		if !mechanismVisibleForPrimary(*mechanism) {
			mechanism.Role = OnboardingRoleSecondaryBehavior
			continue
		}
		if mechanism.ArtifactID == preferredArtifactID {
			primaryIndex = index
			primaryScore = mechanismPrimaryScore(data, *mechanism)
			continue
		}
		if primaryIndex >= 0 && data.UserMechanisms[primaryIndex].ArtifactID == preferredArtifactID {
			continue
		}
		score := mechanismPrimaryScore(data, *mechanism)
		if primaryIndex < 0 || score > primaryScore ||
			(score == primaryScore && mechanism.ArtifactID < data.UserMechanisms[primaryIndex].ArtifactID) {
			primaryIndex = index
			primaryScore = score
		}
	}
	for index := range data.UserMechanisms {
		if data.UserMechanisms[index].Role != OnboardingRolePrimaryBehavior {
			continue
		}
		if index == primaryIndex {
			data.StartHereArtifactID = data.UserMechanisms[index].ArtifactID
			continue
		}
		data.UserMechanisms[index].Role = OnboardingRoleSecondaryBehavior
	}

	sortUserMechanisms(data.UserMechanisms)
	data.RepositoryThesis = DeriveRepositoryThesis(data)
	if data.RepositoryThesis != nil {
		data.RepositoryThesis.RecommendedArtifactID = data.StartHereArtifactID
		for index := range data.UserMechanisms {
			if data.UserMechanisms[index].ArtifactID != data.StartHereArtifactID {
				continue
			}
			data.UserMechanisms[index].Context = mechanismContext(
				data.UserMechanisms[index],
				data.RepositoryThesis.Areas,
			)
			break
		}
	}
	data.RepositoryGuide = DeriveRepositoryGuide(data)
}

// DeriveRepositoryGuide selects only references to already accepted user
// projections. Its ordering is useful even when no architecture view passes
// the distinct-area usefulness gate.
func DeriveRepositoryGuide(data *ReportData) *RepositoryGuide {
	if data == nil {
		return nil
	}
	guide := &RepositoryGuide{StartHereArtifactID: data.StartHereArtifactID}
	if data.RepositoryThesis != nil {
		guide.Purpose = data.RepositoryThesis.Purpose
		guide.SystemStory = append([]string(nil), data.RepositoryThesis.SystemStory...)
		guide.Areas = append([]RepositoryThesisArea(nil), data.RepositoryThesis.Areas...)
	}
	// Decision 221 A / 229 D3: the guide purpose is the backend-filtered
	// thesis purpose ONLY. Raw README (DocumentedPurpose) is source
	// material that renders as a labeled quote; it is never silently
	// promoted to the primary purpose when the filtered thesis purpose is
	// empty — the frontend shows the neutral local fallback instead.
	for _, mechanism := range data.UserMechanisms {
		switch mechanism.Role {
		case OnboardingRoleExtensionPoint:
			guide.ExtensionArtifactIDs = append(guide.ExtensionArtifactIDs, mechanism.ArtifactID)
		case OnboardingRoleErrorDetail:
			continue
		default:
			if mechanism.ArtifactID != guide.StartHereArtifactID {
				guide.MorePathArtifactIDs = append(guide.MorePathArtifactIDs, mechanism.ArtifactID)
			}
		}
		if mechanism.ArtifactID == guide.StartHereArtifactID {
			guide.ReadNext = append([]UserReadNextTarget(nil), mechanism.ReadNext...)
		}
	}
	guide.ArchitectureUseful = repositoryArchitectureUseful(data, guide.Areas)
	if guide.Purpose == "" && len(guide.Areas) == 0 && len(data.UserMechanisms) == 0 {
		return nil
	}
	return guide
}

func repositoryArchitectureUseful(data *ReportData, areas []RepositoryThesisArea) bool {
	if data == nil || data.ArchitectureCanvas == nil {
		return false
	}
	targets := make(map[string]struct{})
	addTarget := func(target *UserMapTarget) {
		if target == nil {
			return
		}
		key := string(target.Kind) + "\x00" + string(target.ComponentID) + "\x00" +
			string(target.FlowID) + "\x00" + target.SurfaceID
		if key != "\x00\x00\x00" {
			targets[key] = struct{}{}
		}
	}
	for _, area := range areas {
		addTarget(area.MapTarget)
	}
	for _, mechanism := range data.UserMechanisms {
		for _, phase := range mechanism.Phases {
			addTarget(phase.MapTarget)
		}
	}
	return len(targets) >= 2
}

func deriveMechanismReadNext(mechanism UserMechanism) []UserReadNextTarget {
	items := mechanism.Phases
	if len(items) == 0 {
		items = make([]UserMechanismPhase, 0, len(mechanism.Steps))
		for index, step := range mechanism.Steps {
			items = append(items, UserMechanismPhase{
				Title: step.Title, Sources: step.Sources,
				ImplementationStepIndexes: []int{index},
			})
		}
	}
	result := make([]UserReadNextTarget, 0, 5)
	seen := make(map[string]struct{})
	for stepIndex, item := range items {
		for _, source := range item.Sources {
			if len(source.Lines) == 0 || source.Path == "" {
				continue
			}
			key := source.Path + "\x00" + source.EnclosingSymbol
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			line := source.StartLine
			if len(source.HighlightRanges) > 0 {
				line = source.HighlightRanges[0].StartLine
			}
			label := strings.TrimSpace(source.EnclosingSymbol)
			if label == "" {
				label = path.Base(source.Path)
			}
			result = append(result, UserReadNextTarget{
				Label: label, Path: source.Path, Line: line,
				Symbol: source.EnclosingSymbol, StepIndex: stepIndex,
			})
			seen[key] = struct{}{}
			if len(result) == 5 {
				return result
			}
		}
	}
	return result
}

func mechanismVisibleForPrimary(mechanism UserMechanism) bool {
	if len(mechanism.Phases) < minNarrativePhases || len(mechanism.Phases) > maxNarrativePhases ||
		len(mechanism.ReadNext) < 2 || len(mechanism.ReadNext) > 5 {
		return false
	}
	visibleAspects := make(map[string]struct{})
	for _, phase := range mechanism.Phases {
		if len(phase.Sources) == 0 || len(phase.ImplementationStepIndexes) == 0 {
			return false
		}
		for _, source := range phase.Sources {
			if len(source.Lines) == 0 || len(source.Lines) > maxInlineSourceLines ||
				len(source.HighlightRanges) == 0 {
				return false
			}
		}
		for _, stepIndex := range phase.ImplementationStepIndexes {
			if stepIndex < 0 || stepIndex >= len(mechanism.Steps) {
				return false
			}
			step := mechanism.Steps[stepIndex]
			if !userMechanismStepSourceComplete(step) {
				return false
			}
			for _, aspectID := range step.canonicalAspectIDs {
				visibleAspects[aspectID] = struct{}{}
			}
		}
	}
	if mechanism.OpportunityKind == semanticdiscovery.OpportunityKindCentralBehavior ||
		mechanismHasPrimaryPathContract(mechanism) {
		for _, aspectID := range []string{"input-trigger", "core-work", "observable-effect"} {
			if _, ok := visibleAspects[aspectID]; !ok {
				return false
			}
		}
		return true
	}
	return mechanismHasInputAndEffect(userMechanismRoleText(mechanism))
}

func userMechanismStepSourceComplete(step UserMechanismStep) bool {
	if len(step.Sources) == 0 {
		return false
	}
	for _, location := range step.requiredVisibleLocations {
		covered := false
		for _, source := range step.Sources {
			if source.Path != location.Path {
				continue
			}
			for _, sourceRange := range source.HighlightRanges {
				if location.Line >= sourceRange.StartLine && location.Line <= sourceRange.EndLine {
					covered = true
					break
				}
			}
			if covered {
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func deterministicNarrativeCompression(mechanism UserMechanism) (NarrativeCompression, bool) {
	if len(mechanism.Steps) < minNarrativePhases {
		return NarrativeCompression{}, false
	}
	groups := make([][]int, len(mechanism.Steps))
	for index := range mechanism.Steps {
		if len(mechanism.Steps[index].canonicalStatementIDs) == 0 {
			return NarrativeCompression{}, false
		}
		groups[index] = []int{index}
	}

	for len(groups) > minNarrativePhases {
		errorGroup := -1
		for index := len(groups) - 1; index >= 0; index-- {
			if narrativePhaseOnlyErrors(mechanism.Steps, groups[index]) {
				errorGroup = index
				break
			}
		}
		if errorGroup < 0 {
			break
		}
		target := previousNonErrorGroup(mechanism.Steps, groups, errorGroup)
		if target < 0 {
			target = nextNonErrorGroup(mechanism.Steps, groups, errorGroup)
		}
		if target < 0 {
			break
		}
		groups[target] = append(groups[target], groups[errorGroup]...)
		sort.Ints(groups[target])
		groups = append(groups[:errorGroup], groups[errorGroup+1:]...)
	}
	for len(groups) > maxNarrativePhases {
		left := closestNarrativeGroups(mechanism.Steps, groups)
		groups[left] = append(groups[left], groups[left+1]...)
		groups = append(groups[:left+1], groups[left+2:]...)
	}
	if len(groups) < minNarrativePhases || len(groups) > maxNarrativePhases {
		return NarrativeCompression{}, false
	}
	for _, group := range groups {
		if narrativePhaseOnlyErrors(mechanism.Steps, group) {
			return NarrativeCompression{}, false
		}
	}

	compression := NarrativeCompression{
		ArtifactID:    mechanism.ArtifactID,
		OrderingBasis: NarrativeOrderingEditorial,
		Phases:        make([]NarrativeCompressionPhase, 0, len(groups)),
	}
	for _, group := range groups {
		statementIDs := make([]string, 0, len(group))
		for _, stepIndex := range group {
			statementIDs = append(statementIDs, mechanism.Steps[stepIndex].canonicalStatementIDs...)
		}
		compression.Phases = append(compression.Phases, NarrativeCompressionPhase{
			Title:              deterministicNarrativeTitle(mechanism.Steps, group),
			MemberStatementIDs: statementIDs,
		})
	}
	return compression, true
}

func previousNonErrorGroup(steps []UserMechanismStep, groups [][]int, from int) int {
	for index := from - 1; index >= 0; index-- {
		if !narrativePhaseOnlyErrors(steps, groups[index]) {
			return index
		}
	}
	return -1
}

func nextNonErrorGroup(steps []UserMechanismStep, groups [][]int, from int) int {
	for index := from + 1; index < len(groups); index++ {
		if !narrativePhaseOnlyErrors(steps, groups[index]) {
			return index
		}
	}
	return -1
}

func closestNarrativeGroups(steps []UserMechanismStep, groups [][]int) int {
	best := 0
	bestScore := -1
	for index := 0; index+1 < len(groups); index++ {
		score := narrativeGroupAffinity(steps, groups[index], groups[index+1])
		if score > bestScore {
			best = index
			bestScore = score
		}
	}
	return best
}

func narrativeGroupAffinity(steps []UserMechanismStep, left, right []int) int {
	leftPaths := make(map[string]struct{})
	for _, index := range left {
		for _, location := range steps[index].Locations {
			leftPaths[location.Path] = struct{}{}
		}
	}
	score := 0
	for _, index := range right {
		for _, location := range steps[index].Locations {
			if _, ok := leftPaths[location.Path]; ok {
				score++
			}
		}
	}
	return score
}

func deterministicNarrativeTitle(steps []UserMechanismStep, indexes []int) string {
	var titles []string
	for _, index := range indexes {
		if userMechanismStepIsErrorDetail(steps[index]) {
			continue
		}
		title := strings.TrimSpace(steps[index].Title)
		if title != "" {
			titles = append(titles, title)
		}
	}
	if len(titles) == 0 {
		return ""
	}
	if len(titles) == 1 {
		return titles[0]
	}
	combined := titles[0] + " and " + lowerFirst(titles[1])
	if len(combined) <= 160 {
		return combined
	}
	return titles[0]
}

func mechanismPrimaryScore(data *ReportData, mechanism UserMechanism) int {
	score := min(len(mechanism.Steps), maxNarrativePhases)
	if mechanismPurposeAligned(data, userMechanismRoleText(mechanism)) {
		score += 8
	}
	score += 3 * min(mechanismCoreAreaCoverage(data, mechanism), 3)
	if mechanismHasInputAndEffect(userMechanismRoleText(mechanism)) {
		score += 5
	}
	score += min(len(mechanism.Files), 4)
	return score
}

// DeriveRepositoryThesis uses only author-written purpose and existing
// validated report areas. It never infers a new data path or runtime edge.
func DeriveRepositoryThesis(data *ReportData) *RepositoryThesis {
	if data == nil {
		return nil
	}
	documented := documentedPurposeSentences(data.DocumentedPurpose, data.RepoName)
	// Decision 221 A: a README warning, badge, release note, capability list,
	// or quote is never the sole product purpose when a stronger bounded
	// sentence exists. Unsafe leading sentences are skipped deterministically
	// (no repository-specific table).
	documented = skipUnsafePurposeSentences(documented)
	purpose := ""
	var documentedStory []string
	if len(documented) > 0 {
		purpose = documented[0]
		documentedStory = append(documentedStory, documented[1:]...)
	}
	if purpose == "" {
		purpose = repositoryPurpose("", data.ProjectGuess, data.RepoName)
	}
	areas := repositoryThesisAreas(data)
	if purpose == "" && len(areas) == 0 {
		return nil
	}
	return &RepositoryThesis{
		Purpose:     purpose,
		SystemStory: repositorySystemStory(documentedStory, areas),
		Areas:       areas,
	}
}

// skipUnsafePurposeSentences removes leading README sentences that are not
// trustworthy product purpose (Decision 221 A): unstable/release warnings,
// quotes, capability/protocol lists, badges, and build-status notes. The
// remaining sentences are returned in order; an empty result means no
// README sentence is safe as purpose.
func skipUnsafePurposeSentences(sentences []string) []string {
	result := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		lower := strings.ToLower(sentence)
		if purposeSentenceIsUnsafe(lower) {
			continue
		}
		result = append(result, sentence)
	}
	return result
}

func purposeSentenceIsUnsafe(lower string) bool {
	// Unstable/main-branch/release warnings.
	for _, phrase := range []string{
		"main branch may be in an unstable", "unstable or even",
		"**note**", "note:", "caution:", "warning:",
		"not ready for production", "do not use in production",
		"under development", "work in progress", "experimental",
		"beta version", "release note", "changelog",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	// Quoted marketing or joke material.
	if strings.HasPrefix(lower, "\"") || strings.HasPrefix(lower, ">") ||
		strings.HasPrefix(lower, "«") || strings.HasPrefix(lower, "“") {
		return true
	}
	if strings.Contains(lower, "never knew") || strings.Contains(lower, "so _sexy_") {
		return true
	}
	// Capability/protocol/badge lists dominate the sentence.
	if strings.Contains(lower, "supports the three major operating systems") {
		return true
	}
	protocolList := 0
	for _, protocol := range []string{"oauth 2.0", "oidc", "saml", "cas", "mcp", "a2a", "ldap", "smtp", "rest api", "graphql"} {
		if strings.Contains(lower, protocol) {
			protocolList++
		}
	}
	if protocolList >= 2 {
		return true
	}
	// Build-status badges.
	if strings.Contains(lower, "build status") || strings.Contains(lower, "coverage") ||
		strings.Contains(lower, "ci status") || strings.Contains(lower, "[![") {
		return true
	}
	return false
}

func documentedPurposeSentences(value, repoName string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "#>*- "))
		if line == "" || strings.HasPrefix(line, "[![") || strings.HasPrefix(line, "<") ||
			(len(parts) == 0 && sameRepositoryHeading(line, repoName)) {
			continue
		}
		parts = append(parts, line)
	}
	joined := strings.Join(parts, " ")
	if joined == "" {
		return nil
	}

	result := make([]string, 0, 4)
	start := 0
	for _, end := range documentedSentenceEnds(joined) {
		sentence := strings.TrimSpace(joined[start : end+1])
		start = end + 1
		if sentence == "" || len(sentence) > 320 || !userMechanismSummarySafe(sentence) {
			continue
		}
		result = append(result, sentence)
		if len(result) == 4 {
			return result
		}
	}
	if len(result) < 4 {
		last := strings.TrimSpace(joined[start:])
		if last != "" && len(last) <= 320 && userMechanismSummarySafe(last) {
			result = append(result, ensureOnboardingSentence(last))
		}
	}
	return result
}

func repositoryPurpose(documented, fallback, repoName string) string {
	for _, candidate := range []string{documented, fallback} {
		candidate = firstUsefulPurposeSentence(candidate, repoName)
		if candidate != "" && userMechanismSummarySafe(candidate) {
			return ensureOnboardingSentence(candidate)
		}
	}
	return ""
}

func firstUsefulPurposeSentence(value, repoName string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	parts := make([]string, 0, 3)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "#>*- "))
		if line == "" || strings.HasPrefix(line, "[![") || strings.HasPrefix(line, "<") {
			continue
		}
		if len(parts) == 0 && sameRepositoryHeading(line, repoName) {
			continue
		}
		parts = append(parts, line)
		joined := strings.Join(parts, " ")
		if sentenceEnd := firstSentenceEnd(joined); sentenceEnd >= 0 {
			return strings.TrimSpace(joined[:sentenceEnd+1])
		}
		if len(joined) >= 320 || len(parts) == 3 {
			break
		}
	}
	result := strings.TrimSpace(strings.Join(parts, " "))
	if len(result) > 320 {
		result = strings.TrimSpace(result[:320])
	}
	return result
}

func sameRepositoryHeading(value, repoName string) bool {
	normalize := func(text string) string {
		text = strings.ToLower(strings.TrimSpace(text))
		text = strings.TrimSuffix(text, ".")
		return strings.Join(strings.Fields(text), " ")
	}
	value = normalize(value)
	repoName = normalize(repoName)
	if value == "" || repoName == "" {
		return false
	}
	return value == repoName || strings.HasPrefix(repoName, value+"-")
}

func firstSentenceEnd(value string) int {
	ends := documentedSentenceEnds(value)
	if len(ends) > 0 {
		return ends[0]
	}
	return -1
}

// documentedSentenceEnds finds presentation sentence terminators without
// treating punctuation inside inline code, URLs, semantic versions, decimals,
// or repository-relative paths as sentence boundaries. Returned indexes are
// byte offsets so callers can slice the original UTF-8 string exactly.
func documentedSentenceEnds(value string) []int {
	result := make([]int, 0, 4)
	codeDelimiter := 0
	for index := 0; index < len(value); index++ {
		if value[index] == '`' && (index == 0 || value[index-1] != '\\') {
			runEnd := index + 1
			for runEnd < len(value) && value[runEnd] == '`' {
				runEnd++
			}
			runLength := runEnd - index
			switch {
			case codeDelimiter == 0:
				codeDelimiter = runLength
			case codeDelimiter == runLength:
				codeDelimiter = 0
			}
			index = runEnd - 1
			continue
		}
		if codeDelimiter != 0 ||
			(value[index] != '.' && value[index] != '!' && value[index] != '?') {
			continue
		}
		if end, ok := documentedSentenceBoundaryEnd(value, index+1); ok {
			result = append(result, end)
			index = end
		}
	}
	return result
}

func documentedSentenceBoundaryEnd(value string, index int) (int, bool) {
	end := index
	for end < len(value) {
		current, size := utf8.DecodeRuneInString(value[end:])
		switch current {
		case '"', '\'', '”', '’', ')', ']', '}', '*', '_', '~':
			end += size
			continue
		}
		if unicode.IsSpace(current) {
			return end - 1, true
		}
		return 0, false
	}
	return len(value) - 1, true
}

func ensureOnboardingSentence(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value[len(value)-1:], ".!?") {
		return value
	}
	return value + "."
}

func repositoryThesisAreas(data *ReportData) []RepositoryThesisArea {
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, filePath := range data.OpenablePaths {
		if filePath = cleanSurfacePath(filePath); filePath != "" {
			openable[filePath] = struct{}{}
		}
	}

	var owners architectureOwnershipIndex
	if data.ArchitectureCanvas != nil {
		owners = buildArchitectureOwnershipIndex(*data.ArchitectureCanvas, data.RepositoryGraph)
	}
	type rankedComponent struct {
		component Component
		position  int
	}
	ranked := make([]rankedComponent, 0, len(data.Components))
	for index, component := range data.Components {
		ranked = append(ranked, rankedComponent{component: component, position: index})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		leftRank := thesisAreaRoleRank(ranked[i].component.Role)
		rightRank := thesisAreaRoleRank(ranked[j].component.Role)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return ranked[i].position < ranked[j].position
	})

	areas := make([]RepositoryThesisArea, 0, maxThesisAreas)
	seenLabels := make(map[string]struct{})
	seenPaths := make(map[string]struct{})
	seenTargets := make(map[string]struct{})
	for _, candidate := range ranked {
		area, ok := thesisAreaFromReportComponent(candidate.component, openable, owners)
		if !ok || !appendRepositoryThesisArea(&areas, area, seenLabels, seenPaths, seenTargets) {
			continue
		}
		if len(areas) == maxThesisAreas {
			return prunePeripheralThesisAreas(data, areas)
		}
	}

	if data.ArchitectureCanvas != nil {
		for _, subsystem := range data.ArchitectureCanvas.Subsystems {
			area, ok := thesisAreaFromCanvasSubsystem(subsystem, data.ArchitectureCanvas)
			if !ok || !appendRepositoryThesisArea(&areas, area, seenLabels, seenPaths, seenTargets) {
				continue
			}
			if len(areas) == maxThesisAreas {
				return prunePeripheralThesisAreas(data, areas)
			}
		}
	}

	for _, file := range data.FirstFilesToOpen {
		filePath := cleanSurfacePath(file.Path)
		if _, ok := openable[filePath]; !ok {
			continue
		}
		location := UserCodeLocation{Path: filePath}
		area := RepositoryThesisArea{
			Label:          path.Base(filePath),
			Responsibility: safeAreaResponsibility(file.Reason),
			CodeLocation:   &location,
		}
		if !appendRepositoryThesisArea(&areas, area, seenLabels, seenPaths, seenTargets) {
			continue
		}
		if len(areas) == maxThesisAreas {
			break
		}
	}
	return prunePeripheralThesisAreas(data, areas)
}

func prunePeripheralThesisAreas(data *ReportData, areas []RepositoryThesisArea) []RepositoryThesisArea {
	if len(areas) <= 3 || repositoryPurposeNeedsTestAreas(data) {
		return areas
	}
	central := make([]RepositoryThesisArea, 0, len(areas))
	for _, area := range areas {
		text := strings.ToLower(area.Label + " " + area.Responsibility)
		if containsAnyOnboardingPhrase(text,
			"test harness", "testing", "test support", "test fixture", "benchmark", "example programs",
		) {
			continue
		}
		central = append(central, area)
	}
	if len(central) < 3 {
		return areas
	}
	return central
}

func repositoryPurposeNeedsTestAreas(data *ReportData) bool {
	if data == nil {
		return false
	}
	purpose := strings.ToLower(data.DocumentedPurpose + " " + data.ProjectGuess)
	return containsAnyOnboardingPhrase(purpose,
		"test framework", "testing framework", "test runner", "testing tool", "benchmark tool",
	)
}

func thesisAreaRoleRank(role componentmap.Role) int {
	switch role {
	case componentmap.RoleEntry:
		return 0
	case componentmap.RoleDomain, componentmap.RoleCoordination, componentmap.RoleState:
		return 1
	case componentmap.RoleBoundary:
		return 2
	case componentmap.RoleSupport:
		return 3
	default:
		return 4
	}
}

func thesisAreaFromReportComponent(
	component Component,
	openable map[string]struct{},
	owners architectureOwnershipIndex,
) (RepositoryThesisArea, bool) {
	label := strings.TrimSpace(component.Name)
	if label == "" {
		return RepositoryThesisArea{}, false
	}
	var location *UserCodeLocation
	var target *UserMapTarget
	for _, anchor := range component.AnchorGroups {
		anchorPath := cleanSurfacePath(anchor.Path)
		if _, ok := openable[anchorPath]; !ok {
			continue
		}
		resolved := UserCodeLocation{Path: anchorPath}
		for _, candidate := range anchor.Locations {
			if cleanSurfacePath(candidate.Path) == anchorPath && candidate.Line > 0 {
				resolved.Line = candidate.Line
				resolved.Column = candidate.Column
				break
			}
		}
		location = &resolved
		if ids := sortedComponentIDs(ownersForPath(owners.pathOwners, anchorPath)); len(ids) == 1 {
			mapTarget := UserMapTarget{Kind: SemanticSearchTargetComponent, ComponentID: ids[0]}
			target = &mapTarget
		}
		break
	}
	if location == nil && target == nil {
		return RepositoryThesisArea{}, false
	}
	return RepositoryThesisArea{
		Label:          label,
		Responsibility: safeAreaResponsibility(component.ModelPurpose),
		Role:           component.Role,
		CodeLocation:   location,
		MapTarget:      target,
	}, true
}

func thesisAreaFromCanvasSubsystem(
	subsystem ArchitectureSubsystem,
	canvas *ArchitectureCanvas,
) (RepositoryThesisArea, bool) {
	label := strings.TrimSpace(subsystem.Name)
	if label == "" || canvas == nil {
		return RepositoryThesisArea{}, false
	}
	for _, componentID := range subsystem.ComponentIDs {
		if !canvasHasComponent(canvas, componentID) {
			continue
		}
		target := UserMapTarget{Kind: SemanticSearchTargetComponent, ComponentID: componentID}
		return RepositoryThesisArea{
			Label:          label,
			Responsibility: safeAreaResponsibility(subsystem.Description),
			MapTarget:      &target,
		}, true
	}
	return RepositoryThesisArea{}, false
}

func safeAreaResponsibility(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !userMechanismSummarySafe(value) {
		return "Open this area to inspect its implementation."
	}
	return ensureOnboardingSentence(value)
}

func appendRepositoryThesisArea(
	areas *[]RepositoryThesisArea,
	area RepositoryThesisArea,
	seenLabels map[string]struct{},
	seenPaths map[string]struct{},
	seenTargets map[string]struct{},
) bool {
	if len(*areas) >= maxThesisAreas || (area.CodeLocation == nil && area.MapTarget == nil) {
		return false
	}
	labelKey := strings.ToLower(strings.TrimSpace(area.Label))
	if labelKey == "" {
		return false
	}
	if _, duplicate := seenLabels[labelKey]; duplicate {
		return false
	}
	targetKey := ""
	if area.MapTarget != nil {
		targetKey = userMapTargetKey(*area.MapTarget)
		if _, duplicate := seenTargets[targetKey]; duplicate {
			return false
		}
	}
	pathKey := ""
	if area.CodeLocation != nil {
		pathKey = cleanSurfacePath(area.CodeLocation.Path)
		if _, duplicate := seenPaths[pathKey]; duplicate {
			return false
		}
	}
	seenLabels[labelKey] = struct{}{}
	if targetKey != "" {
		seenTargets[targetKey] = struct{}{}
	}
	if pathKey != "" {
		seenPaths[pathKey] = struct{}{}
	}
	*areas = append(*areas, area)
	return true
}

func userMapTargetKey(target UserMapTarget) string {
	return string(target.Kind) + "\x00" + string(target.ComponentID) + "\x00" +
		string(target.FlowID) + "\x00" + target.SurfaceID
}

func repositorySystemStory(seed []string, areas []RepositoryThesisArea) []string {
	story := make([]string, 0, 4)
	for _, sentence := range seed {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" || !userMechanismSummarySafe(sentence) {
			continue
		}
		story = append(story, ensureOnboardingSentence(sentence))
		if len(story) == 3 {
			return story
		}
	}
	if len(areas) == 0 {
		return story
	}
	appendRole := func(roles ...componentmap.Role) {
		if len(story) >= 4 {
			return
		}
		for _, area := range areas {
			matches := false
			for _, role := range roles {
				if area.Role == role {
					matches = true
					break
				}
			}
			if !matches || storyContainsArea(story, area.Label) {
				continue
			}
			story = append(story, areaStorySentence(area))
			return
		}
	}
	appendRole(componentmap.RoleEntry)
	appendRole(componentmap.RoleDomain, componentmap.RoleCoordination, componentmap.RoleState)
	appendRole(componentmap.RoleBoundary)
	for _, area := range areas {
		if len(story) >= 4 {
			break
		}
		if !storyContainsArea(story, area.Label) {
			story = append(story, areaStorySentence(area))
		}
	}
	return story
}

func areaStorySentence(area RepositoryThesisArea) string {
	responsibility := strings.TrimSpace(area.Responsibility)
	responsibility = strings.TrimSuffix(responsibility, ".")
	if responsibility == "" {
		return ensureOnboardingSentence(area.Label)
	}
	return ensureOnboardingSentence(area.Label + ": " + lowerFirst(responsibility))
}

func storyContainsArea(story []string, label string) bool {
	for _, sentence := range story {
		if strings.HasPrefix(sentence, label+":") {
			return true
		}
	}
	return false
}

func mechanismContext(
	mechanism UserMechanism,
	areas []RepositoryThesisArea,
) []UserMechanismContext {
	result := make([]UserMechanismContext, 0, 3)
	for _, area := range areas {
		matched := false
		if area.CodeLocation != nil {
			for _, file := range mechanism.Files {
				if file.Path == area.CodeLocation.Path ||
					(path.Dir(area.CodeLocation.Path) != "." &&
						strings.HasPrefix(file.Path, path.Dir(area.CodeLocation.Path)+"/")) {
					matched = true
					break
				}
			}
		}
		if !matched && !userMechanismTopicCovered(userMechanismRoleText(mechanism), area.Label) {
			continue
		}
		result = append(result, UserMechanismContext{
			Label:          area.Label,
			Responsibility: area.Responsibility,
			CodeLocation:   cloneUserCodeLocation(area.CodeLocation),
			MapTarget:      cloneUserMapTarget(area.MapTarget),
		})
		if len(result) == 3 {
			break
		}
	}
	return result
}

func cloneUserCodeLocation(location *UserCodeLocation) *UserCodeLocation {
	if location == nil {
		return nil
	}
	clone := *location
	return &clone
}

func cloneUserMapTarget(target *UserMapTarget) *UserMapTarget {
	if target == nil {
		return nil
	}
	clone := *target
	return &clone
}

// ProjectNarrativeCompression validates opaque memberships and resolves all
// visible source data from canonical projected steps. Unsupported proposal
// prose is replaced locally with accepted step wording.
func ProjectNarrativeCompression(
	mechanism UserMechanism,
	compression NarrativeCompression,
) ([]UserMechanismPhase, bool) {
	if compression.ArtifactID != mechanism.ArtifactID ||
		compression.OrderingBasis != NarrativeOrderingEditorial ||
		len(compression.Phases) < minNarrativePhases ||
		len(compression.Phases) > maxNarrativePhases {
		return nil, false
	}

	statementOwner := make(map[string]int)
	for stepIndex, step := range mechanism.Steps {
		if len(step.canonicalStatementIDs) == 0 {
			return nil, false
		}
		for _, statementID := range step.canonicalStatementIDs {
			if statementID == "" {
				return nil, false
			}
			if _, duplicate := statementOwner[statementID]; duplicate {
				return nil, false
			}
			statementOwner[statementID] = stepIndex
		}
	}
	if len(statementOwner) == 0 {
		return nil, false
	}
	detailOwner := make(map[string]int)
	for detailIndex, detail := range mechanism.unplacedImplementationDetails {
		if len(detail.canonicalStatementIDs) == 0 || len(detail.Locations) == 0 || len(detail.Sources) != 0 {
			return nil, false
		}
		for _, statementID := range detail.canonicalStatementIDs {
			if statementID == "" {
				return nil, false
			}
			if _, duplicate := statementOwner[statementID]; duplicate {
				return nil, false
			}
			if _, duplicate := detailOwner[statementID]; duplicate {
				return nil, false
			}
			detailOwner[statementID] = detailIndex
		}
	}

	seenStatements := make(map[string]struct{}, len(statementOwner))
	seenDetailStatements := make(map[string]struct{}, len(detailOwner))
	coveredAspects := make(map[string]struct{})
	stepPhase := make(map[int]int, len(mechanism.Steps))
	detailPhase := make(map[int]int, len(mechanism.unplacedImplementationDetails))
	phaseStepIndexes := make([][]int, len(compression.Phases))
	for phaseIndex, phase := range compression.Phases {
		if len(phase.MemberStatementIDs) == 0 {
			return nil, false
		}
		stepSet := make(map[int]struct{})
		for _, statementID := range phase.MemberStatementIDs {
			stepIndex, ok := statementOwner[statementID]
			if !ok {
				detailIndex, detailOK := detailOwner[statementID]
				if !detailOK {
					return nil, false
				}
				if _, duplicate := seenDetailStatements[statementID]; duplicate {
					return nil, false
				}
				seenDetailStatements[statementID] = struct{}{}
				if previous, assigned := detailPhase[detailIndex]; assigned && previous != phaseIndex {
					return nil, false
				}
				detailPhase[detailIndex] = phaseIndex
				continue
			}
			if _, duplicate := seenStatements[statementID]; duplicate {
				return nil, false
			}
			seenStatements[statementID] = struct{}{}
			if previous, assigned := stepPhase[stepIndex]; assigned && previous != phaseIndex {
				return nil, false
			}
			stepPhase[stepIndex] = phaseIndex
			stepSet[stepIndex] = struct{}{}
		}
		for stepIndex := range stepSet {
			for _, statementID := range mechanism.Steps[stepIndex].canonicalStatementIDs {
				if _, included := seenStatements[statementID]; !included {
					return nil, false
				}
			}
			for _, aspectID := range mechanism.Steps[stepIndex].canonicalAspectIDs {
				coveredAspects[aspectID] = struct{}{}
			}
			phaseStepIndexes[phaseIndex] = append(phaseStepIndexes[phaseIndex], stepIndex)
		}
		sort.Ints(phaseStepIndexes[phaseIndex])
		if narrativePhaseOnlyErrors(mechanism.Steps, phaseStepIndexes[phaseIndex]) {
			return nil, false
		}
	}
	if len(seenStatements) != len(statementOwner) || len(stepPhase) != len(mechanism.Steps) {
		return nil, false
	}
	if len(seenDetailStatements) > 0 && len(seenDetailStatements) != len(detailOwner) {
		return nil, false
	}
	for _, detail := range mechanism.unplacedImplementationDetails {
		for _, aspectID := range detail.canonicalAspectIDs {
			coveredAspects[aspectID] = struct{}{}
		}
	}
	for _, aspectID := range mechanism.canonicalCoveredAspectIDs {
		if _, covered := coveredAspects[aspectID]; !covered {
			return nil, false
		}
	}

	result := make([]UserMechanismPhase, 0, len(compression.Phases))
	for phaseIndex, proposal := range compression.Phases {
		steps := phaseStepIndexes[phaseIndex]
		title := narrativePhaseTitle(proposal.Title, mechanism.Steps, steps)
		explanation := narrativePhaseExplanation(mechanism.Steps, steps)
		if title == "" || explanation == "" {
			return nil, false
		}
		sources := narrativePhaseSources(mechanism.Steps, steps)
		if len(sources) == 0 {
			return nil, false
		}
		result = append(result, UserMechanismPhase{
			Title:                     title,
			Explanation:               explanation,
			Locations:                 narrativePhaseLocations(mechanism.Steps, steps),
			Sources:                   sources,
			WhatToNotice:              narrativePhaseNotices(mechanism.Steps, steps),
			MapTarget:                 narrativePhaseMapTarget(mechanism.Steps, steps),
			ImplementationStepIndexes: append([]int(nil), steps...),
		})
	}
	for _, detail := range mechanism.unplacedImplementationDetails {
		phaseIndex := bestNarrativePhaseForDetail(detail, result, phaseStepIndexes, mechanism.Steps)
		result[phaseIndex].ImplementationDetails = append(
			result[phaseIndex].ImplementationDetails,
			publicImplementationDetail(detail),
		)
	}
	return result, true
}

func publicImplementationDetail(detail UserMechanismStep) UserMechanismStep {
	return UserMechanismStep{
		Title:        detail.Title,
		Explanation:  detail.Explanation,
		Locations:    append([]UserCodeLocation(nil), detail.Locations...),
		Sources:      append([]SourceSnippet(nil), detail.Sources...),
		WhatToNotice: append([]SourceNotice(nil), detail.WhatToNotice...),
	}
}

func bestNarrativePhaseForDetail(
	detail UserMechanismStep,
	phases []UserMechanismPhase,
	phaseStepIndexes [][]int,
	steps []UserMechanismStep,
) int {
	bestIndex := 0
	bestScore := -1
	bestLineDistance := int(^uint(0) >> 1)
	bestStepDistance := int(^uint(0) >> 1)
	for phaseIndex, phase := range phases {
		titleAffinity := narrativeTextAffinity(detail.Title, phase.Title)
		semanticAffinity := narrativeTextAffinity(
			detail.Title+" "+detail.Explanation,
			phase.Title+" "+phase.Explanation,
		)
		sharedPath, lineDistance := narrativeLocationAffinity(detail.Locations, phase.Locations)
		stepDistance := narrativeStepDistance(
			detail.canonicalStepIndex,
			phaseStepIndexes[phaseIndex],
			steps,
		)
		score := titleAffinity*100 + semanticAffinity*10
		if sharedPath {
			score += 8
			switch {
			case lineDistance <= 20:
				score += 6
			case lineDistance <= 100:
				score += 4
			case lineDistance <= 500:
				score += 2
			}
		}
		if score > bestScore ||
			(score == bestScore && lineDistance < bestLineDistance) ||
			(score == bestScore && lineDistance == bestLineDistance && stepDistance < bestStepDistance) {
			bestIndex = phaseIndex
			bestScore = score
			bestLineDistance = lineDistance
			bestStepDistance = stepDistance
		}
	}
	return bestIndex
}

func narrativeTextAffinity(left, right string) int {
	leftTerms := onboardingTermSet(left)
	rightTerms := onboardingTermSet(right)
	result := 0
	for leftTerm := range leftTerms {
		if _, exact := rightTerms[leftTerm]; exact {
			result += 2
			continue
		}
		if len(leftTerm) < 5 {
			continue
		}
		prefix := leftTerm[:5]
		for rightTerm := range rightTerms {
			if len(rightTerm) >= 5 && rightTerm[:5] == prefix {
				result++
				break
			}
		}
	}
	return result
}

func narrativeLocationAffinity(
	left []UserCodeLocation,
	right []UserCodeLocation,
) (bool, int) {
	minDistance := int(^uint(0) >> 1)
	sharedPath := false
	for _, leftLocation := range left {
		for _, rightLocation := range right {
			if leftLocation.Path == "" || leftLocation.Path != rightLocation.Path {
				continue
			}
			sharedPath = true
			distance := leftLocation.Line - rightLocation.Line
			if distance < 0 {
				distance = -distance
			}
			if distance < minDistance {
				minDistance = distance
			}
		}
	}
	return sharedPath, minDistance
}

func narrativeStepDistance(
	detailStepIndex int,
	indexes []int,
	steps []UserMechanismStep,
) int {
	minDistance := int(^uint(0) >> 1)
	for _, index := range indexes {
		if index < 0 || index >= len(steps) {
			continue
		}
		distance := detailStepIndex - steps[index].canonicalStepIndex
		if distance < 0 {
			distance = -distance
		}
		if distance < minDistance {
			minDistance = distance
		}
	}
	return minDistance
}

func narrativePhaseOnlyErrors(steps []UserMechanismStep, indexes []int) bool {
	if len(indexes) == 0 {
		return true
	}
	for _, index := range indexes {
		if !userMechanismStepIsErrorDetail(steps[index]) {
			return false
		}
	}
	return true
}

func narrativePhaseTitle(proposed string, steps []UserMechanismStep, indexes []int) string {
	proposed = strings.TrimSpace(proposed)
	if len(proposed) <= 160 && userMechanismSummarySafe(proposed) &&
		narrativeCopySupported(proposed, steps, indexes) {
		return proposed
	}
	for _, index := range indexes {
		if !userMechanismStepIsErrorDetail(steps[index]) {
			return strings.TrimSpace(steps[index].Title)
		}
	}
	return ""
}

func narrativeCopySupported(copy string, steps []UserMechanismStep, indexes []int) bool {
	sourceParts := make([]string, 0, len(indexes)*2)
	for _, index := range indexes {
		sourceParts = append(sourceParts, steps[index].Title, steps[index].Explanation)
	}
	sourceTerms := onboardingTermSet(strings.Join(sourceParts, " "))
	copyTerms := onboardingTermSet(copy)
	if len(copyTerms) == 0 {
		return false
	}
	for term := range copyTerms {
		if _, exact := sourceTerms[term]; exact {
			continue
		}
		matched := false
		if len(term) >= 5 {
			prefix := term[:5]
			for sourceTerm := range sourceTerms {
				if len(sourceTerm) >= 5 && sourceTerm[:5] == prefix {
					matched = true
					break
				}
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func onboardingTermSet(value string) map[string]struct{} {
	ignored := map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "as": {}, "for": {}, "from": {},
		"in": {}, "into": {}, "of": {}, "or": {}, "the": {}, "to": {}, "with": {},
		// Editorial glue may improve an action title without introducing a new
		// repository concept. Domain nouns and effect terms still must match an
		// accepted member step.
		"perform": {}, "performs": {}, "performing": {},
		"handle": {}, "handles": {}, "handling": {},
	}
	result := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(current rune) bool {
		return current < 'a' || current > 'z'
	}) {
		if len(token) < 2 {
			continue
		}
		if _, skip := ignored[token]; !skip {
			result[token] = struct{}{}
		}
	}
	return result
}

func narrativePhaseExplanation(steps []UserMechanismStep, indexes []int) string {
	parts := make([]string, 0, len(indexes))
	seen := make(map[string]struct{}, len(indexes))
	for _, index := range indexes {
		explanation := strings.TrimSpace(steps[index].Explanation)
		if explanation == "" {
			continue
		}
		key := strings.ToLower(strings.Join(strings.Fields(explanation), " "))
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		parts = append(parts, explanation)
	}
	return strings.Join(parts, " ")
}

func narrativePhaseLocations(steps []UserMechanismStep, indexes []int) []UserCodeLocation {
	seen := make(map[UserCodeLocation]struct{})
	var result []UserCodeLocation
	for _, index := range indexes {
		for _, location := range steps[index].Locations {
			if _, duplicate := seen[location]; duplicate {
				continue
			}
			seen[location] = struct{}{}
			result = append(result, location)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		return result[i].Column < result[j].Column
	})
	return result
}

func narrativePhaseSources(steps []UserMechanismStep, indexes []int) []SourceSnippet {
	seen := make(map[string]struct{})
	var result []SourceSnippet
	for _, index := range indexes {
		for _, source := range steps[index].Sources {
			key := source.PresentationSHA256
			if key == "" {
				key = source.Path + "\x00" + strconv.Itoa(source.StartLine) + "\x00" +
					strconv.Itoa(source.EndLine) + "\x00" + source.ContentSHA256
			}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, source)
		}
	}
	return result
}

func narrativePhaseNotices(steps []UserMechanismStep, indexes []int) []SourceNotice {
	seen := make(map[string]struct{})
	var result []SourceNotice
	for _, index := range indexes {
		for _, notice := range steps[index].WhatToNotice {
			var key strings.Builder
			key.WriteString(notice.Path)
			key.WriteByte(0)
			key.WriteString(notice.Text)
			for _, sourceRange := range notice.SupportingRanges {
				key.WriteByte(0)
				key.WriteString(strconv.Itoa(sourceRange.StartLine))
				key.WriteByte(':')
				key.WriteString(strconv.Itoa(sourceRange.EndLine))
			}
			if _, duplicate := seen[key.String()]; duplicate {
				continue
			}
			seen[key.String()] = struct{}{}
			result = append(result, notice)
		}
	}
	return result
}

func narrativePhaseMapTarget(steps []UserMechanismStep, indexes []int) *UserMapTarget {
	if len(indexes) == 0 || steps[indexes[0]].MapTarget == nil {
		return nil
	}
	target := *steps[indexes[0]].MapTarget
	for _, index := range indexes[1:] {
		if steps[index].MapTarget == nil || *steps[index].MapTarget != target {
			return nil
		}
	}
	return &target
}
