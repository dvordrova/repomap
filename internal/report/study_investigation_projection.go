package report

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	StudyInvestigationVersion = 1

	maxStudyInvestigationCards          = 8
	maxStudyInvestigationReadings       = 5
	maxStudyInvestigationMechanisms     = 3
	minStudyInvestigationEdges          = 2
	maxStudyInvestigationEdges          = 8
	maxStudyInvestigationNodeLabelRunes = 96
	maxStudyInvestigationSymbolBytes    = 1024
	maxStudyInvestigationWitnessCount   = 1_000_000_000
)

// StudyInvestigationOutcome is the complete calm product state of one
// report-side Study investigation. Operational stage/status/frontier codes do
// not enter this projection.
type StudyInvestigationOutcome string

const (
	StudyInvestigationOutcomePrepared  StudyInvestigationOutcome = "prepared_investigation"
	StudyInvestigationOutcomeMechanism StudyInvestigationOutcome = "mechanism"
)

// StudyInvestigationInput is a transient report-neutral seam. Its caller must
// restore every field from locally validated mechanism authority before
// invoking the report package. These values are not an artifact schema and
// carry no request-local provider refs or model prose.
type StudyInvestigationInput struct {
	Cards []StudyInvestigationCardInput
}

// StudyInvestigationCardInput binds one restored result to the manifest-
// relative ordinal of an already accepted Study theme. Reading ordinals are
// one-based indexes into that theme's primary Readings.
type StudyInvestigationCardInput struct {
	ThemeOrdinal    int
	Outcome         StudyInvestigationOutcome
	ReadingOrdinals []int
	Mechanisms      []StudyInvestigationMechanismInput
}

// StudyInvestigationMechanismInput is one exact ordered directed path.
// ReadingOrdinals are the exact primary readings tied to a node on this path.
type StudyInvestigationMechanismInput struct {
	ReadingOrdinals []int
	Nodes           []StudyInvestigationNodeInput
	Edges           []StudyInvestigationEdgeInput
}

// StudyInvestigationNodeInput keeps the canonical symbol only as private join
// material. Symbol is never copied into the public investigation; Label is the
// backend-owned bounded display label.
type StudyInvestigationNodeInput struct {
	Label    string
	Symbol   string
	Location evidence.Location
}

// StudyInvestigationEdgeInput uses one-based FromNodeOrdinal/ToNodeOrdinal
// indexes into the mechanism's ordered Nodes. A valid path contains exactly
// edge i: node i+1 -> node i+2.
type StudyInvestigationEdgeInput struct {
	FromNodeOrdinal int
	ToNodeOrdinal   int
	Invocation      surfacediscovery.DirectCallInvocation
	WitnessCount    int
	Callsite        evidence.Location
}

// StudyInvestigation is the bounded public-safe v1 projection attached to one
// StudyThemeCard. IDs exist only inside the current report and derive from
// manifest-relative ordinals, never canonical or request-local identities.
type StudyInvestigation struct {
	Version         int                           `json:"version"`
	ID              string                        `json:"id"`
	Outcome         StudyInvestigationOutcome     `json:"outcome"`
	ReadingOrdinals []int                         `json:"reading_ordinals"`
	Mechanisms      []StudyInvestigationMechanism `json:"mechanisms"`
}

type StudyInvestigationMechanism struct {
	ID              string                   `json:"id"`
	Ordinal         int                      `json:"ordinal"`
	ReadingOrdinals []int                    `json:"reading_ordinals"`
	Nodes           []StudyInvestigationNode `json:"nodes"`
	Edges           []StudyInvestigationEdge `json:"edges"`
}

type StudyInvestigationNode struct {
	ID           string                     `json:"id"`
	Ordinal      int                        `json:"ordinal"`
	Label        string                     `json:"label"`
	Declaration  UserCodeLocation           `json:"declaration"`
	ComponentIDs []componentmap.ComponentID `json:"component_ids"`
}

type StudyInvestigationEdge struct {
	ID           string                                `json:"id"`
	Ordinal      int                                   `json:"ordinal"`
	FromNodeID   string                                `json:"from_node_id"`
	ToNodeID     string                                `json:"to_node_id"`
	Invocation   surfacediscovery.DirectCallInvocation `json:"invocation"`
	WitnessCount int                                   `json:"witness_count"`
	Callsite     UserCodeLocation                      `json:"callsite"`
}

type normalizedStudyInvestigationCard struct {
	themeOrdinal    int
	outcome         StudyInvestigationOutcome
	readingOrdinals []int
	mechanisms      []normalizedStudyInvestigationMechanism
}

type normalizedStudyInvestigationMechanism struct {
	readingOrdinals []int
	nodes           []StudyInvestigationNodeInput
	edges           []StudyInvestigationEdgeInput
	pathKey         string
}

// CollectStudyInvestigationSourceLocations performs the source-authorization
// preflight before final report projection. It validates the complete neutral
// input atomically, then returns every exact declaration and representative
// callsite in deterministic order with duplicates removed.
func CollectStudyInvestigationSourceLocations(
	input StudyInvestigationInput,
) ([]evidence.Location, error) {
	cards, err := normalizeStudyInvestigationInput(input)
	if err != nil {
		return nil, err
	}
	locations := make(map[string]evidence.Location)
	for _, card := range cards {
		for _, mechanism := range card.mechanisms {
			for _, node := range mechanism.nodes {
				locations[studyInvestigationLocationKey(node.Location)] = node.Location
			}
			for _, edge := range mechanism.edges {
				locations[studyInvestigationLocationKey(edge.Callsite)] = edge.Callsite
			}
		}
	}
	result := make([]evidence.Location, 0, len(locations))
	for _, location := range locations {
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
	return result, nil
}

// PrepareAuthorizedStudyInvestigationSourceCoverage explicitly admits the
// validated investigation declarations/callsites into captured source
// authority before ProjectStudyInvestigations is called. Newly needed paths
// force a complete recapture against the already confirmed repository state.
// Caller-owned report and authority values change only after every exact
// source target has been read successfully.
func PrepareAuthorizedStudyInvestigationSourceCoverage(
	ctx context.Context,
	data *ReportData,
	authority *RunAuthority,
	input StudyInvestigationInput,
) error {
	if data == nil || authority == nil {
		return fmt.Errorf("study investigation report: authorized report data is required")
	}
	locations, err := CollectStudyInvestigationSourceLocations(input)
	if err != nil {
		return err
	}

	prepared := *data
	prepared.OpenablePaths = append([]string(nil), data.OpenablePaths...)
	prepared.evidenceLocations = append([]evidence.Location(nil), data.evidenceLocations...)
	prepared.studyInvestigationSourceLocations = append([]evidence.Location(nil), locations...)
	openable := make(map[string]struct{}, len(prepared.OpenablePaths)+len(locations))
	for _, sourcePath := range prepared.OpenablePaths {
		openable[sourcePath] = struct{}{}
	}
	for _, location := range locations {
		if _, exists := openable[location.Path]; !exists {
			prepared.OpenablePaths = append(prepared.OpenablePaths, location.Path)
			openable[location.Path] = struct{}{}
		}
		prepared.evidenceLocations = append(prepared.evidenceLocations, location)
	}
	sort.Strings(prepared.OpenablePaths)
	prepared.OpenablePaths = compactStudyInvestigationStrings(prepared.OpenablePaths)
	if len(prepared.OpenablePaths) > maxManifestOpenablePaths {
		return fmt.Errorf(
			"study investigation report: source path count %d exceeds limit %d",
			len(prepared.OpenablePaths),
			maxManifestOpenablePaths,
		)
	}
	prepared.evidenceLocations = compactStudyInvestigationLocations(prepared.evidenceLocations)

	preparedAuthority := *authority
	preparedAuthority.inputs = append([]freshness.CapturedInput(nil), authority.inputs...)
	if authority.inputs == nil ||
		!studyInvestigationCapturedInputsCover(
			authority.repository.Identity,
			authority.analysisRoot,
			prepared.OpenablePaths,
			authority.inputs,
		) {
		// PrepareAuthorizedSourceCoverage captures the complete current
		// allowlist atomically through the confirmed repository snapshot.
		preparedAuthority.inputs = nil
	}
	if err := PrepareAuthorizedSourceCoverage(ctx, &prepared, &preparedAuthority); err != nil {
		return err
	}

	data.OpenablePaths = prepared.OpenablePaths
	data.evidenceLocations = prepared.evidenceLocations
	data.studyInvestigationSourceLocations = prepared.studyInvestigationSourceLocations
	data.UserSources = prepared.UserSources
	data.CapturedRevision = prepared.CapturedRevision
	authority.inputs = preparedAuthority.inputs
	return nil
}

// ProjectStudyInvestigations attaches validated public-safe investigations to
// existing theme cards. The operation is atomic: every theme/read/source join
// is checked and every projection is built before any card is mutated.
func ProjectStudyInvestigations(
	themes *AtlasStudyThemesProjection,
	canvas *ArchitectureCanvas,
	repositoryGraph *RepositoryGraph,
	openablePaths []string,
	input StudyInvestigationInput,
) error {
	cards, err := normalizeStudyInvestigationInput(input)
	if err != nil {
		return err
	}
	if len(cards) == 0 {
		return nil
	}
	if themes == nil {
		return fmt.Errorf("study investigation report: non-empty input requires Study themes")
	}

	themePositions := make(map[int]int, len(themes.Cards))
	for position, card := range themes.Cards {
		if card.Ordinal <= 0 {
			return fmt.Errorf("study investigation report: invalid theme ordinal %d", card.Ordinal)
		}
		if _, duplicate := themePositions[card.Ordinal]; duplicate {
			return fmt.Errorf("study investigation report: duplicate theme ordinal %d", card.Ordinal)
		}
		themePositions[card.Ordinal] = position
	}
	openable := make(map[string]struct{}, len(openablePaths))
	for _, path := range openablePaths {
		openable[path] = struct{}{}
	}

	type pendingProjection struct {
		position      int
		investigation *StudyInvestigation
	}
	pending := make([]pendingProjection, 0, len(cards))
	for _, card := range cards {
		position, ok := themePositions[card.themeOrdinal]
		if !ok {
			return fmt.Errorf(
				"study investigation report: unknown theme ordinal %d",
				card.themeOrdinal,
			)
		}
		theme := themes.Cards[position]
		if err := validateStudyInvestigationReadingOrdinals(
			card.readingOrdinals,
			len(theme.Readings),
		); err != nil {
			return fmt.Errorf(
				"study investigation report: theme %d: %w",
				card.themeOrdinal,
				err,
			)
		}
		for mechanismIndex, mechanism := range card.mechanisms {
			if err := validateStudyInvestigationReadingOrdinals(
				mechanism.readingOrdinals,
				len(theme.Readings),
			); err != nil {
				return fmt.Errorf(
					"study investigation report: theme %d mechanism %d: %w",
					card.themeOrdinal,
					mechanismIndex+1,
					err,
				)
			}
			for _, node := range mechanism.nodes {
				if _, authorized := openable[node.Location.Path]; !authorized {
					return fmt.Errorf(
						"study investigation report: declaration path %q is not source-authorized",
						node.Location.Path,
					)
				}
			}
			for _, edge := range mechanism.edges {
				if _, authorized := openable[edge.Callsite.Path]; !authorized {
					return fmt.Errorf(
						"study investigation report: callsite path %q is not source-authorized",
						edge.Callsite.Path,
					)
				}
			}
		}
		pending = append(pending, pendingProjection{
			position: position,
			investigation: projectStudyInvestigationCard(
				card,
				canvas,
				repositoryGraph,
			),
		})
	}

	for _, projection := range pending {
		themes.Cards[projection.position].Investigation = projection.investigation
	}
	return nil
}

func projectStudyInvestigationCard(
	card normalizedStudyInvestigationCard,
	canvas *ArchitectureCanvas,
	repositoryGraph *RepositoryGraph,
) *StudyInvestigation {
	investigationID := fmt.Sprintf("study-investigation-%d", card.themeOrdinal)
	result := &StudyInvestigation{
		Version:         StudyInvestigationVersion,
		ID:              investigationID,
		Outcome:         card.outcome,
		ReadingOrdinals: append([]int{}, card.readingOrdinals...),
		Mechanisms:      make([]StudyInvestigationMechanism, 0, len(card.mechanisms)),
	}
	for mechanismPosition, mechanism := range card.mechanisms {
		mechanismOrdinal := mechanismPosition + 1
		mechanismID := fmt.Sprintf("%s-mechanism-%d", investigationID, mechanismOrdinal)
		projected := StudyInvestigationMechanism{
			ID:              mechanismID,
			Ordinal:         mechanismOrdinal,
			ReadingOrdinals: append([]int{}, mechanism.readingOrdinals...),
			Nodes:           make([]StudyInvestigationNode, 0, len(mechanism.nodes)),
			Edges:           make([]StudyInvestigationEdge, 0, len(mechanism.edges)),
		}
		for nodePosition, node := range mechanism.nodes {
			nodeOrdinal := nodePosition + 1
			projected.Nodes = append(projected.Nodes, StudyInvestigationNode{
				ID:          fmt.Sprintf("%s-node-%d", mechanismID, nodeOrdinal),
				Ordinal:     nodeOrdinal,
				Label:       node.Label,
				Declaration: studyInvestigationUserLocation(node.Location),
				ComponentIDs: architectureComponentIDsForExactDeclaration(
					canvas,
					repositoryGraph,
					node.Symbol,
					node.Location,
				),
			})
		}
		for edgePosition, edge := range mechanism.edges {
			edgeOrdinal := edgePosition + 1
			projected.Edges = append(projected.Edges, StudyInvestigationEdge{
				ID:           fmt.Sprintf("%s-edge-%d", mechanismID, edgeOrdinal),
				Ordinal:      edgeOrdinal,
				FromNodeID:   projected.Nodes[edge.FromNodeOrdinal-1].ID,
				ToNodeID:     projected.Nodes[edge.ToNodeOrdinal-1].ID,
				Invocation:   edge.Invocation,
				WitnessCount: edge.WitnessCount,
				Callsite:     studyInvestigationUserLocation(edge.Callsite),
			})
		}
		result.Mechanisms = append(result.Mechanisms, projected)
	}
	return result
}

func normalizeStudyInvestigationInput(
	input StudyInvestigationInput,
) ([]normalizedStudyInvestigationCard, error) {
	if len(input.Cards) > maxStudyInvestigationCards {
		return nil, fmt.Errorf(
			"study investigation report: card count %d exceeds limit %d",
			len(input.Cards),
			maxStudyInvestigationCards,
		)
	}
	result := make([]normalizedStudyInvestigationCard, 0, len(input.Cards))
	seenThemes := make(map[int]struct{}, len(input.Cards))
	for cardPosition, card := range input.Cards {
		if card.ThemeOrdinal <= 0 || card.ThemeOrdinal > maxAtlasStudyReportCoverageCount {
			return nil, fmt.Errorf(
				"study investigation report: card %d has invalid theme ordinal",
				cardPosition+1,
			)
		}
		if _, duplicate := seenThemes[card.ThemeOrdinal]; duplicate {
			return nil, fmt.Errorf(
				"study investigation report: duplicate theme ordinal %d",
				card.ThemeOrdinal,
			)
		}
		seenThemes[card.ThemeOrdinal] = struct{}{}
		if card.Outcome != StudyInvestigationOutcomePrepared && card.Outcome != StudyInvestigationOutcomeMechanism {
			return nil, fmt.Errorf(
				"study investigation report: theme %d has invalid outcome %q",
				card.ThemeOrdinal,
				card.Outcome,
			)
		}
		readingOrdinals, err := normalizeStudyInvestigationReadingOrdinals(card.ReadingOrdinals)
		if err != nil {
			return nil, fmt.Errorf(
				"study investigation report: theme %d: %w",
				card.ThemeOrdinal,
				err,
			)
		}
		if card.Outcome == StudyInvestigationOutcomePrepared && len(card.Mechanisms) != 0 {
			return nil, fmt.Errorf(
				"study investigation report: prepared theme %d contains mechanisms",
				card.ThemeOrdinal,
			)
		}
		if card.Outcome == StudyInvestigationOutcomeMechanism &&
			(len(card.Mechanisms) == 0 || len(card.Mechanisms) > maxStudyInvestigationMechanisms) {
			return nil, fmt.Errorf(
				"study investigation report: mechanism theme %d has invalid mechanism count %d",
				card.ThemeOrdinal,
				len(card.Mechanisms),
			)
		}

		normalized := normalizedStudyInvestigationCard{
			themeOrdinal:    card.ThemeOrdinal,
			outcome:         card.Outcome,
			readingOrdinals: readingOrdinals,
			mechanisms:      make([]normalizedStudyInvestigationMechanism, 0, len(card.Mechanisms)),
		}
		seenPaths := make(map[string]struct{}, len(card.Mechanisms))
		mechanismReadingSet := make(map[int]struct{})
		for mechanismPosition, mechanism := range card.Mechanisms {
			projected, err := normalizeStudyInvestigationMechanism(mechanism)
			if err != nil {
				return nil, fmt.Errorf(
					"study investigation report: theme %d mechanism %d: %w",
					card.ThemeOrdinal,
					mechanismPosition+1,
					err,
				)
			}
			if _, duplicate := seenPaths[projected.pathKey]; duplicate {
				return nil, fmt.Errorf(
					"study investigation report: theme %d contains a duplicate exact path",
					card.ThemeOrdinal,
				)
			}
			seenPaths[projected.pathKey] = struct{}{}
			for _, ordinal := range projected.readingOrdinals {
				mechanismReadingSet[ordinal] = struct{}{}
			}
			normalized.mechanisms = append(normalized.mechanisms, projected)
		}
		if card.Outcome == StudyInvestigationOutcomeMechanism &&
			!studyInvestigationReadingSetEquals(readingOrdinals, mechanismReadingSet) {
			return nil, fmt.Errorf(
				"study investigation report: theme %d reading union does not match its mechanisms",
				card.ThemeOrdinal,
			)
		}
		sort.Slice(normalized.mechanisms, func(i, j int) bool {
			return normalized.mechanisms[i].pathKey < normalized.mechanisms[j].pathKey
		})
		result = append(result, normalized)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].themeOrdinal < result[j].themeOrdinal })
	return result, nil
}

func normalizeStudyInvestigationMechanism(
	input StudyInvestigationMechanismInput,
) (normalizedStudyInvestigationMechanism, error) {
	readingOrdinals, err := normalizeStudyInvestigationReadingOrdinals(input.ReadingOrdinals)
	if err != nil {
		return normalizedStudyInvestigationMechanism{}, err
	}
	if len(readingOrdinals) == 0 {
		return normalizedStudyInvestigationMechanism{}, fmt.Errorf("mechanism has no exact reading tie")
	}
	if len(input.Edges) < minStudyInvestigationEdges || len(input.Edges) > maxStudyInvestigationEdges ||
		len(input.Nodes) != len(input.Edges)+1 {
		return normalizedStudyInvestigationMechanism{}, fmt.Errorf(
			"invalid path shape with %d nodes and %d edges",
			len(input.Nodes),
			len(input.Edges),
		)
	}
	nodes := append([]StudyInvestigationNodeInput(nil), input.Nodes...)
	for position, node := range nodes {
		if !validStudyInvestigationText(node.Label, maxStudyInvestigationNodeLabelRunes) ||
			!validStudyInvestigationSymbol(node.Symbol) ||
			!validGroundingLocation(node.Location) {
			return normalizedStudyInvestigationMechanism{}, fmt.Errorf(
				"node %d is not exact public-safe authority",
				position+1,
			)
		}
	}
	edges := append([]StudyInvestigationEdgeInput(nil), input.Edges...)
	for position, edge := range edges {
		if edge.FromNodeOrdinal != position+1 || edge.ToNodeOrdinal != position+2 ||
			!edge.Invocation.Valid() || edge.WitnessCount <= 0 ||
			edge.WitnessCount > maxStudyInvestigationWitnessCount ||
			!validGroundingLocation(edge.Callsite) {
			return normalizedStudyInvestigationMechanism{}, fmt.Errorf(
				"edge %d is not one exact consecutive direct-static call",
				position+1,
			)
		}
	}
	return normalizedStudyInvestigationMechanism{
		readingOrdinals: readingOrdinals,
		nodes:           nodes,
		edges:           edges,
		pathKey:         studyInvestigationPathKey(nodes, edges),
	}, nil
}

func normalizeStudyInvestigationReadingOrdinals(values []int) ([]int, error) {
	if len(values) > maxStudyInvestigationReadings {
		return nil, fmt.Errorf(
			"reading count %d exceeds limit %d",
			len(values),
			maxStudyInvestigationReadings,
		)
	}
	result := append([]int{}, values...)
	sort.Ints(result)
	for position, ordinal := range result {
		if ordinal <= 0 || (position > 0 && ordinal == result[position-1]) {
			return nil, fmt.Errorf("reading ordinals are not unique positive indexes")
		}
	}
	return result, nil
}

func validateStudyInvestigationReadingOrdinals(values []int, readingCount int) error {
	for _, ordinal := range values {
		if ordinal > readingCount {
			return fmt.Errorf(
				"reading ordinal %d exceeds theme reading count %d",
				ordinal,
				readingCount,
			)
		}
	}
	return nil
}

func studyInvestigationReadingSetEquals(values []int, set map[int]struct{}) bool {
	if len(values) != len(set) {
		return false
	}
	for _, value := range values {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func studyInvestigationPathKey(
	nodes []StudyInvestigationNodeInput,
	edges []StudyInvestigationEdgeInput,
) string {
	var builder strings.Builder
	for _, node := range nodes {
		fmt.Fprintf(
			&builder,
			"n\x00%s\x00%s\x00%d\x00%d\x00",
			node.Symbol,
			node.Location.Path,
			node.Location.Line,
			node.Location.Column,
		)
	}
	for _, edge := range edges {
		fmt.Fprintf(
			&builder,
			"e\x00%d\x00%d\x00%s\x00%d\x00%s\x00%d\x00%d\x00",
			edge.FromNodeOrdinal,
			edge.ToNodeOrdinal,
			edge.Invocation,
			edge.WitnessCount,
			edge.Callsite.Path,
			edge.Callsite.Line,
			edge.Callsite.Column,
		)
	}
	return builder.String()
}

func studyInvestigationLocationKey(location evidence.Location) string {
	return fmt.Sprintf("%s\x00%d\x00%d", location.Path, location.Line, location.Column)
}

func studyInvestigationUserLocation(location evidence.Location) UserCodeLocation {
	return UserCodeLocation{Path: location.Path, Line: location.Line, Column: location.Column}
}

func compactStudyInvestigationStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	result := values[:0]
	for _, value := range values {
		if value == "" || (len(result) > 0 && result[len(result)-1] == value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func compactStudyInvestigationLocations(values []evidence.Location) []evidence.Location {
	if len(values) == 0 {
		return values
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Path != values[j].Path {
			return values[i].Path < values[j].Path
		}
		if values[i].Line != values[j].Line {
			return values[i].Line < values[j].Line
		}
		return values[i].Column < values[j].Column
	})
	result := values[:0]
	previous := ""
	for _, value := range values {
		key := studyInvestigationLocationKey(value)
		if key == previous {
			continue
		}
		previous = key
		result = append(result, value)
	}
	return result
}

func studyInvestigationCapturedInputsCover(
	repositoryRoot string,
	analysisRoot string,
	paths []string,
	inputs []freshness.CapturedInput,
) bool {
	repositoryPaths, err := repositoryRelativeInputPaths(repositoryRoot, analysisRoot, paths)
	if err != nil {
		return false
	}
	captured := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		captured[input.Path] = struct{}{}
	}
	for _, sourcePath := range repositoryPaths {
		if _, ok := captured[sourcePath]; !ok {
			return false
		}
	}
	return true
}

func validStudyInvestigationText(value string, maxRunes int) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > maxRunes {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return false
		}
	}
	return true
}

func validStudyInvestigationSymbol(value string) bool {
	return len(value) <= maxStudyInvestigationSymbolBytes &&
		validStudyInvestigationText(value, maxStudyInvestigationSymbolBytes)
}
