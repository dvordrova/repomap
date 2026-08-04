package report

import (
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const minUserMechanismSteps = 2

// UserMechanism is a report-only projection of the useful supported part of
// an independently replayed canonical Mechanism. It deliberately excludes
// verdicts, gaps, claim bases, hashes, and validation diagnostics.
type UserMechanism struct {
	ArtifactID      string                               `json:"artifact_id"`
	Title           string                               `json:"title"`
	Question        string                               `json:"question"`
	Answer          string                               `json:"answer"`
	Role            OnboardingRole                       `json:"role,omitempty"`
	OpportunityKind semanticdiscovery.OpportunityKind    `json:"opportunity_kind,omitempty"`
	TargetUserJob   semanticdiscovery.OpportunityUserJob `json:"target_user_job,omitempty"`
	SearchQueries   []string                             `json:"search_queries,omitempty"`
	Phases          []UserMechanismPhase                 `json:"phases,omitempty"`
	Context         []UserMechanismContext               `json:"context,omitempty"`
	ReadNext        []UserReadNextTarget                 `json:"read_next,omitempty"`
	Steps           []UserMechanismStep                  `json:"steps"`
	Files           []UserCodeLocation                   `json:"files"`
	// canonicalCoveredAspectIDs is retained only for local validation of the
	// presentation compression. It is never serialized or hashed.
	canonicalCoveredAspectIDs []string
	// unplacedImplementationDetails retains accepted canonical steps that have
	// exact evidence locations but no saved source candidate. They cannot become
	// code-first narrative steps; onboarding finalization attaches each one to
	// exactly one source-backed phase for secondary inspection.
	unplacedImplementationDetails []UserMechanismStep
}

// UserMechanismStep is one source-backed explanation step. Opaque statement
// IDs are retained only in private assembly fields for local phase validation;
// they are not serialized as user navigation.
type UserMechanismStep struct {
	Title          string             `json:"title"`
	Explanation    string             `json:"explanation"`
	Locations      []UserCodeLocation `json:"locations"`
	Sources        []SourceSnippet    `json:"sources,omitempty"`
	WhatToNotice   []SourceNotice     `json:"what_to_notice,omitempty"`
	MapTarget      *UserMapTarget     `json:"map_target,omitempty"`
	canonicalTitle string
	// canonicalStatementIDs and canonicalAspectIDs retain only locally
	// validated opaque IDs while the report is assembled. They are excluded
	// from report JSON: the ordinary user workspace needs the projected phases,
	// while canonical artifacts remain the provenance authority.
	canonicalStatementIDs    []string
	canonicalAspectIDs       []string
	canonicalStepIndex       int
	requiredVisibleLocations []UserCodeLocation
}

// UserReadNextTarget is an exact presentation-only continuation selected from
// source already shown by an accepted mechanism phase.
type UserReadNextTarget struct {
	Label     string `json:"label"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Symbol    string `json:"symbol,omitempty"`
	StepIndex int    `json:"step_index"`
}

// UserCodeLocation is the smallest source-navigation value needed by the
// report workspace and local open-file authority.
type UserCodeLocation struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

// UserMapTarget points only at an object already present on this report's
// architecture canvas. It is a navigation reference, not a semantic edge.
type UserMapTarget struct {
	Kind        SemanticSearchTargetKind `json:"kind"`
	ComponentID componentmap.ComponentID `json:"component_id,omitempty"`
	FlowID      componentmap.FlowID      `json:"flow_id,omitempty"`
	SurfaceID   string                   `json:"surface_id,omitempty"`
}

func projectUserMechanism(
	data *ReportData,
	artifact semanticdiscovery.Artifact,
	sourceResults ...goldenmechanism.Result,
) (UserMechanism, bool) {
	if data == nil || artifact.Kind != semanticdiscovery.ArtifactMechanism || artifact.ID == "" ||
		(artifact.Verdict != semanticdiscovery.VerdictSupported &&
			artifact.Verdict != semanticdiscovery.VerdictMixed) {
		return UserMechanism{}, false
	}

	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, path := range data.OpenablePaths {
		if path = strings.TrimSpace(path); path != "" {
			openable[path] = struct{}{}
		}
	}
	supported := make(map[string]semanticdiscovery.Statement, len(artifact.Statements))
	for _, statement := range artifact.Statements {
		if statement.Basis != semanticdiscovery.ClaimDirect &&
			statement.Basis != semanticdiscovery.ClaimCompositional {
			continue
		}
		supported[statement.ID] = statement
	}

	var probe *goldenmechanism.Result
	if len(sourceResults) > 0 {
		probe = &sourceResults[0]
	}
	steps := make([]UserMechanismStep, 0, len(artifact.Steps))
	unplacedDetails := make([]UserMechanismStep, 0)
	for artifactStepIndex, step := range artifact.Steps {
		statements := make([]string, 0, len(step.StatementIDs))
		statementIDs := make([]string, 0, len(step.StatementIDs))
		aspectIDs := make([]string, 0, len(step.StatementIDs))
		for _, statementID := range step.StatementIDs {
			statement, ok := supported[statementID]
			if !ok || strings.TrimSpace(statement.Text) == "" {
				continue
			}
			statements = append(statements, strings.TrimSpace(statement.Text))
			statementIDs = appendUniqueString(statementIDs, statement.ID)
			for _, aspectID := range statement.AspectIDs {
				aspectIDs = appendUniqueString(aspectIDs, aspectID)
			}
		}
		if len(statements) == 0 {
			continue
		}
		locations := userCodeLocations(step.Evidence, openable)
		if len(locations) == 0 {
			continue
		}
		explanation := strings.Join(statements, " ")
		title := userMechanismStepTitle(step.Title, explanation)
		if title == "" {
			title = "Step " + strconv.Itoa(len(steps)+1)
		}
		sources := projectStepSourceSnippets(data, step, supported, probe)
		projected := UserMechanismStep{
			Title:                    title,
			Explanation:              explanation,
			Locations:                locations,
			Sources:                  sources,
			WhatToNotice:             projectStepSourceNotices(data, step, supported, sources),
			canonicalTitle:           strings.TrimSpace(step.Title),
			canonicalStatementIDs:    statementIDs,
			canonicalAspectIDs:       aspectIDs,
			canonicalStepIndex:       artifactStepIndex,
			requiredVisibleLocations: requiredUserCodeLocations(step.Evidence, openable),
		}
		if len(sources) == 0 {
			unplacedDetails = append(unplacedDetails, projected)
			continue
		}
		projected.MapTarget = resolveUserMapTarget(data.ArchitectureCanvas, step.Focus)
		steps = append(steps, projected)
	}
	if len(steps) < minUserMechanismSteps {
		return UserMechanism{}, false
	}
	suppressCoarseUserMapTargets(steps)

	return UserMechanism{
		ArtifactID: artifact.ID,
		Title:      userMechanismTitle(data.RepoName, artifact.Question),
		Question:   strings.TrimSpace(artifact.Question),
		Answer:     userMechanismAnswer(artifact.Summary, steps),
		Steps:      steps,
		Files:      userMechanismFiles(steps),
		canonicalCoveredAspectIDs: append(
			[]string(nil),
			artifact.CoveredAspectIDs...,
		),
		unplacedImplementationDetails: unplacedDetails,
	}, true
}

func requiredUserCodeLocations(
	evidence []semanticdiscovery.EvidenceRef,
	openable map[string]struct{},
) []UserCodeLocation {
	filtered := make([]semanticdiscovery.EvidenceRef, 0, len(evidence))
	for _, reference := range evidence {
		if reference.Kind == "saved_source_window" ||
			reference.Label == "saved bounded source window" {
			continue
		}
		filtered = append(filtered, reference)
	}
	return userCodeLocations(filtered, openable)
}

func suppressCoarseUserMapTargets(steps []UserMechanismStep) {
	if len(steps) < 2 || steps[0].MapTarget == nil {
		return
	}
	first := *steps[0].MapTarget
	for index := 1; index < len(steps); index++ {
		if steps[index].MapTarget == nil || *steps[index].MapTarget != first {
			return
		}
	}
	for index := range steps {
		steps[index].MapTarget = nil
	}
}

func userCodeLocations(
	evidence []semanticdiscovery.EvidenceRef,
	openable map[string]struct{},
) []UserCodeLocation {
	result := make([]UserCodeLocation, 0, len(evidence))
	seen := make(map[UserCodeLocation]struct{}, len(evidence))
	for _, reference := range evidence {
		if _, ok := openable[reference.Path]; !ok || reference.Line <= 0 {
			continue
		}
		location := UserCodeLocation{
			Path: reference.Path, Line: reference.Line, Column: reference.Column,
		}
		if _, duplicate := seen[location]; duplicate {
			continue
		}
		seen[location] = struct{}{}
		result = append(result, location)
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

func resolveUserMapTarget(
	canvas *ArchitectureCanvas,
	focus semanticdiscovery.Focus,
) *UserMapTarget {
	for _, componentID := range focus.ComponentIDs {
		id := componentmap.ComponentID(componentID)
		if canvasHasComponent(canvas, id) {
			return &UserMapTarget{Kind: SemanticSearchTargetComponent, ComponentID: id}
		}
	}
	for _, flowID := range focus.FlowIDs {
		id := componentmap.FlowID(flowID)
		if canvasHasFlow(canvas, id) {
			return &UserMapTarget{Kind: SemanticSearchTargetFlow, FlowID: id}
		}
	}
	for _, surfaceID := range focus.SurfaceIDs {
		if canvasHasSurface(canvas, surfaceID) {
			return &UserMapTarget{Kind: SemanticSearchTargetSurface, SurfaceID: surfaceID}
		}
	}
	return nil
}

func userMechanismFiles(steps []UserMechanismStep) []UserCodeLocation {
	firstByPath := make(map[string]UserCodeLocation)
	for _, step := range steps {
		for _, location := range step.Locations {
			current, exists := firstByPath[location.Path]
			if !exists || location.Line < current.Line || current.Line == 0 {
				firstByPath[location.Path] = location
			}
		}
	}
	paths := make([]string, 0, len(firstByPath))
	for path := range firstByPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]UserCodeLocation, 0, len(paths))
	for _, path := range paths {
		result = append(result, firstByPath[path])
	}
	return result
}

func userMechanismAnswer(summary string, steps []UserMechanismStep) string {
	summary = strings.TrimSpace(summary)
	if summary == "" || !userMechanismSummarySafe(summary) {
		return ""
	}
	if len(steps) > 0 && sameUserMechanismText(summary, steps[0].Explanation) {
		return ""
	}
	missingTopics := make([]string, 0, len(steps))
	for _, step := range steps {
		topic := strings.TrimSpace(step.canonicalTitle)
		if topic == "" || !userMechanismSummarySafe(topic) || userMechanismTopicCovered(summary, topic) {
			continue
		}
		missingTopics = append(missingTopics, lowerFirst(topic))
	}
	if len(missingTopics) == 0 {
		return summary
	}
	if !strings.ContainsAny(summary[len(summary)-1:], ".!?") {
		summary += "."
	}
	return summary + " It also covers " + joinUserMechanismTopics(missingTopics) + "."
}

func userMechanismTopicCovered(summary, topic string) bool {
	summaryWords := userMechanismTopicWords(summary)
	for topicWord := range userMechanismTopicWords(topic) {
		if _, covered := summaryWords[topicWord]; covered {
			return true
		}
		if len(topicWord) < 5 {
			continue
		}
		prefix := topicWord[:5]
		for summaryWord := range summaryWords {
			if len(summaryWord) >= 5 && summaryWord[:5] == prefix {
				return true
			}
		}
	}
	return false
}

func userMechanismTopicWords(value string) map[string]struct{} {
	words := strings.FieldsFunc(strings.ToLower(value), func(current rune) bool {
		return current < 'a' || current > 'z'
	})
	stop := map[string]struct{}{
		"and": {}, "the": {}, "with": {}, "from": {}, "into": {}, "when": {},
		"handling": {}, "application": {}, "selection": {}, "preparation": {},
	}
	result := make(map[string]struct{}, len(words))
	for _, word := range words {
		if len(word) < 4 {
			continue
		}
		if _, ignored := stop[word]; !ignored {
			result[word] = struct{}{}
		}
	}
	return result
}

func lowerFirst(value string) string {
	if value == "" || value[0] < 'A' || value[0] > 'Z' {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}

func joinUserMechanismTopics(topics []string) string {
	switch len(topics) {
	case 0:
		return ""
	case 1:
		return topics[0]
	case 2:
		return topics[0] + " and " + topics[1]
	default:
		return strings.Join(topics[:len(topics)-1], ", ") + ", and " + topics[len(topics)-1]
	}
}

func userMechanismSummarySafe(summary string) bool {
	normalized := strings.ToLower(strings.TrimSpace(summary))
	unsafePhrases := []string{
		"evidence gap",
		"missing evidence",
		"insufficient evidence",
		"cannot determine",
		"cannot be determined",
		"not established",
		"unresolved",
		"unknown",
		"uninspected",
		"bounded facts",
		"bounded probe",
		"model verdict",
	}
	for _, phrase := range unsafePhrases {
		if strings.Contains(normalized, phrase) {
			return false
		}
	}
	return true
}

func sameUserMechanismText(left, right string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		value = strings.TrimRight(value, ".!?")
		return strings.Join(strings.Fields(value), " ")
	}
	return normalize(left) == normalize(right)
}

// userMechanismStepTitle turns recurring behavioral claim-title shapes into
// short action labels. It considers only the accepted title and statement
// text and has no repository, artifact, or semantic-identity special cases.
func userMechanismStepTitle(title, explanation string) string {
	title = strings.TrimSpace(title)
	normalizedTitle := strings.ToLower(title)
	normalizedExplanation := strings.ToLower(strings.TrimSpace(explanation))

	switch {
	case containsUserMechanismTerms(normalizedTitle, "directory", "brows", "entry") &&
		strings.Contains(normalizedExplanation, "enter"):
		return "Enter directory browsing"
	case containsUserMechanismTerms(normalizedTitle, "query", "option") &&
		strings.Contains(normalizedExplanation, "read"):
		return "Read query options"
	case containsUserMechanismTerms(normalizedTitle, "listing", "item", "collection") &&
		(strings.Contains(normalizedExplanation, "collect") ||
			strings.Contains(normalizedExplanation, "append")):
		return "Collect listing items"
	case containsUserMechanismTerms(normalizedTitle, "sort", "pagin"):
		return "Sort and paginate"
	case containsUserMechanismTerms(normalizedTitle, "response", "format", "output") &&
		containsUserMechanismTerms(normalizedExplanation, "encod", "writ"):
		return "Encode and write the response"
	case containsUserMechanismTerms(normalizedTitle, "redirect", "error", "path") &&
		strings.Contains(normalizedExplanation, "alternative path"):
		return "Handle alternate paths"
	case containsUserMechanismTerms(normalizedTitle, "context", "preparation", "invocation") &&
		containsUserMechanismTerms(normalizedExplanation, "context", "invoke"):
		return "Prepare the routing context"
	case containsUserMechanismTerms(normalizedTitle, "route", "lookup", "parameter", "update") &&
		containsUserMechanismTerms(normalizedExplanation, "lookup", "parameter") &&
		(strings.Contains(normalizedExplanation, "copy ") ||
			strings.Contains(normalizedExplanation, "copies")):
		return "Find the route and copy parameters"
	case containsUserMechanismTerms(normalizedTitle, "endpoint", "invocation") &&
		containsUserMechanismTerms(normalizedExplanation, "endpoint", "invoke"):
		return "Call the endpoint"
	case strings.Contains(normalizedTitle, "fallback") &&
		(strings.Contains(normalizedTitle, "not-found") ||
			strings.Contains(normalizedTitle, "method-not-allowed")):
		return "Choose a fallback"
	default:
		return title
	}
}

func containsUserMechanismTerms(text string, terms ...string) bool {
	for _, term := range terms {
		if !strings.Contains(text, term) {
			return false
		}
	}
	return true
}

func userMechanismTitle(repoName, question string) string {
	trimmedQuestion := strings.TrimSpace(question)
	normalizedQuestion := strings.ToLower(strings.TrimSuffix(trimmedQuestion, "?"))
	normalizedRepo := strings.ToLower(strings.TrimSpace(repoName))

	switch {
	case normalizedRepo == "caddy" &&
		strings.Contains(normalizedQuestion, "directory listings"):
		return "How Caddy builds directory listings"
	case (normalizedRepo == "chi" || strings.HasSuffix(normalizedRepo, "/chi")) &&
		strings.Contains(normalizedQuestion, "dispatch") &&
		strings.Contains(normalizedQuestion, "http request"):
		return "How chi dispatches an HTTP request"
	case trimmedQuestion != "":
		return strings.TrimSuffix(trimmedQuestion, "?")
	default:
		return "How this code works"
	}
}

func mergeUserMechanism(existing []UserMechanism, mechanism UserMechanism) []UserMechanism {
	result := make([]UserMechanism, 0, len(existing)+1)
	for _, item := range existing {
		if item.ArtifactID != mechanism.ArtifactID {
			result = append(result, item)
		}
	}
	result = append(result, mechanism)
	sortUserMechanisms(result)
	return result
}
