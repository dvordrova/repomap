// Package causalpipeline composes one bounded question-scoped causal slice
// from an accepted source episode. It does not inspect source, Git, providers,
// or target repositories.
package causalpipeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	maxEpisodeBytes  = 256 << 10
	maxQuestionBytes = 32 << 10
	maxOutputBytes   = 64 << 10

	maxEpisodeItems = 64
	maxSelectedIDs  = 24
	maxStatements   = 16
	maxRelations    = 16
	maxIDBytes      = 96
	maxTitleBytes   = 512
	maxTextBytes    = 2048

	episodeKind     = "source-episode-microexperiment"
	questionKind    = "source-episode-question-spec"
	causalSliceKind = "source-episode-causal-slice"
	artifactVersion = "1"
)

// Output contains the two deterministic experiment artifacts.
type Output struct {
	CausalSliceJSON       []byte
	WhereToChangeMarkdown []byte
}

type episodeInput struct {
	ArtifactKind    string             `json:"artifact_kind"`
	ArtifactVersion string             `json:"artifact_version"`
	EpisodeID       string             `json:"episode_id"`
	Repository      repositoryInput    `json:"repository"`
	Question        string             `json:"question"`
	Anchors         []anchorInput      `json:"anchors"`
	Facts           []factInput        `json:"facts"`
	Claims          []claimInput       `json:"claims"`
	Flow            *flowInput         `json:"flow"`
	Uncertainties   []uncertaintyInput `json:"uncertainties"`
}

type repositoryInput struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
	WebBase  string `json:"web_base"`
}

type anchorInput struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	URL       string `json:"url"`
}

type factInput struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Statement string `json:"statement"`
}

type claimInput struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Strength  string `json:"strength"`
	Title     string `json:"title"`
	Statement string `json:"statement"`
}

type flowInput struct {
	ID    string          `json:"id"`
	State string          `json:"state"`
	Nodes []flowNodeInput `json:"nodes"`
}

type flowNodeInput struct {
	ID      string `json:"id"`
	ClaimID string `json:"claim_id"`
}

type uncertaintyInput struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Statement string `json:"statement"`
}

type questionSpec struct {
	ArtifactKind    string              `json:"artifact_kind"`
	ArtifactVersion string              `json:"artifact_version"`
	QuestionID      string              `json:"question_id"`
	EpisodeID       string              `json:"episode_id"`
	FlowID          string              `json:"flow_id,omitempty"`
	Title           string              `json:"title"`
	Question        string              `json:"question"`
	Evidence        evidenceSelection   `json:"evidence"`
	Statements      []authoredStatement `json:"design_statements"`
	Relations       []authoredRelation  `json:"relations,omitempty"`
}

type evidenceSelection struct {
	ClaimIDs       []string `json:"claim_ids"`
	FactIDs        []string `json:"fact_ids"`
	FlowNodeIDs    []string `json:"flow_node_ids,omitempty"`
	AnchorIDs      []string `json:"anchor_ids"`
	UncertaintyIDs []string `json:"uncertainty_ids,omitempty"`
}

type authoredStatement struct {
	ID             string   `json:"id"`
	Role           string   `json:"role"`
	State          string   `json:"state"`
	Title          string   `json:"title"`
	Statement      string   `json:"statement"`
	ClaimIDs       []string `json:"claim_ids,omitempty"`
	FactIDs        []string `json:"fact_ids,omitempty"`
	FlowNodeIDs    []string `json:"flow_node_ids,omitempty"`
	AnchorIDs      []string `json:"anchor_ids,omitempty"`
	UncertaintyIDs []string `json:"uncertainty_ids,omitempty"`
}

type authoredRelation struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	FromID    string `json:"from_id"`
	ToID      string `json:"to_id"`
	Statement string `json:"statement"`
}

type causalSlice struct {
	ArtifactKind    string              `json:"artifact_kind"`
	ArtifactVersion string              `json:"artifact_version"`
	QuestionID      string              `json:"question_id"`
	Episode         episodeRef          `json:"episode"`
	Flow            *flowRef            `json:"flow,omitempty"`
	Title           string              `json:"title"`
	Question        string              `json:"question"`
	Evidence        evidenceSlice       `json:"evidence"`
	Statements      []authoredStatement `json:"design_statements"`
	Relations       []authoredRelation  `json:"relations,omitempty"`
}

type episodeRef struct {
	ID                 string `json:"id"`
	RepositoryName     string `json:"repository_name"`
	RepositoryRevision string `json:"repository_revision"`
	SourceQuestion     string `json:"source_question"`
}

type flowRef struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

type evidenceSlice struct {
	Claims        []claimInput       `json:"claims"`
	Facts         []factInput        `json:"facts"`
	FlowNodes     []flowNodeInput    `json:"flow_nodes,omitempty"`
	Anchors       []anchorInput      `json:"anchors"`
	Uncertainties []uncertaintyInput `json:"uncertainties,omitempty"`
}

type episodeIndex struct {
	claims        map[string]claimInput
	facts         map[string]factInput
	flowNodes     map[string]flowNodeInput
	anchors       map[string]anchorInput
	uncertainties map[string]uncertaintyInput
}

// Compose validates a bounded question spec against one accepted episode and
// renders both a neutral causal slice and a human-readable change answer.
func Compose(episodeJSON, questionJSON []byte) (Output, error) {
	if len(episodeJSON) > maxEpisodeBytes {
		return Output{}, fmt.Errorf("causal pipeline: episode input exceeds %d bytes", maxEpisodeBytes)
	}
	if len(questionJSON) > maxQuestionBytes {
		return Output{}, fmt.Errorf("causal pipeline: question input exceeds %d bytes", maxQuestionBytes)
	}

	var episode episodeInput
	if err := json.Unmarshal(episodeJSON, &episode); err != nil {
		return Output{}, fmt.Errorf("causal pipeline: decode episode: %w", err)
	}
	var spec questionSpec
	if err := decodeStrict(questionJSON, &spec); err != nil {
		return Output{}, fmt.Errorf("causal pipeline: decode question: %w", err)
	}
	if err := validateEnvelope(episode, spec); err != nil {
		return Output{}, err
	}

	index, err := indexEpisode(episode)
	if err != nil {
		return Output{}, err
	}
	evidence, selected, err := resolveEvidence(spec.Evidence, index)
	if err != nil {
		return Output{}, err
	}
	if len(evidence.FlowNodes) > 0 && spec.FlowID == "" {
		return Output{}, errors.New("causal pipeline: selected flow nodes require flow_id")
	}
	if err := validateAuthoredContent(spec, selected); err != nil {
		return Output{}, err
	}

	var flow *flowRef
	if spec.FlowID != "" {
		flow = &flowRef{ID: episode.Flow.ID, State: episode.Flow.State}
	}
	slice := causalSlice{
		ArtifactKind:    causalSliceKind,
		ArtifactVersion: artifactVersion,
		QuestionID:      spec.QuestionID,
		Episode: episodeRef{
			ID:                 episode.EpisodeID,
			RepositoryName:     episode.Repository.Name,
			RepositoryRevision: episode.Repository.Revision,
			SourceQuestion:     episode.Question,
		},
		Flow:       flow,
		Title:      spec.Title,
		Question:   spec.Question,
		Evidence:   evidence,
		Statements: spec.Statements,
		Relations:  spec.Relations,
	}

	rawSlice, err := json.MarshalIndent(slice, "", "  ")
	if err != nil {
		return Output{}, fmt.Errorf("causal pipeline: encode causal slice: %w", err)
	}
	rawSlice = append(rawSlice, '\n')
	markdown := renderMarkdown(slice)
	if len(rawSlice) > maxOutputBytes {
		return Output{}, fmt.Errorf("causal pipeline: causal slice exceeds %d bytes", maxOutputBytes)
	}
	if len(markdown) > maxOutputBytes {
		return Output{}, fmt.Errorf("causal pipeline: markdown exceeds %d bytes", maxOutputBytes)
	}
	return Output{CausalSliceJSON: rawSlice, WhereToChangeMarkdown: markdown}, nil
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return fmt.Errorf("trailing data: %w", err)
	}
	return nil
}

func validateEnvelope(episode episodeInput, spec questionSpec) error {
	if episode.ArtifactKind != episodeKind || episode.ArtifactVersion != artifactVersion {
		return errors.New("causal pipeline: unsupported episode artifact")
	}
	if spec.ArtifactKind != questionKind || spec.ArtifactVersion != artifactVersion {
		return errors.New("causal pipeline: unsupported question artifact")
	}
	if spec.EpisodeID != episode.EpisodeID {
		return errors.New("causal pipeline: question does not reference the supplied episode")
	}
	if spec.FlowID != "" {
		if episode.Flow == nil || spec.FlowID != episode.Flow.ID {
			return errors.New("causal pipeline: question does not reference the supplied episode flow")
		}
	}
	if err := validateText("question_id", spec.QuestionID, maxIDBytes); err != nil {
		return err
	}
	if err := validateText("episode_id", spec.EpisodeID, maxIDBytes); err != nil {
		return err
	}
	if spec.FlowID != "" {
		if err := validateText("flow_id", spec.FlowID, maxIDBytes); err != nil {
			return err
		}
	}
	if err := validateText("title", spec.Title, maxTitleBytes); err != nil {
		return err
	}
	if err := validateText("question", spec.Question, maxTextBytes); err != nil {
		return err
	}
	if len(spec.Statements) == 0 || len(spec.Statements) > maxStatements {
		return fmt.Errorf("causal pipeline: design_statements must contain 1-%d items", maxStatements)
	}
	if len(spec.Relations) > maxRelations {
		return fmt.Errorf("causal pipeline: relations contains more than %d items", maxRelations)
	}
	return nil
}

func indexEpisode(episode episodeInput) (episodeIndex, error) {
	counts := []struct {
		name  string
		count int
	}{
		{"claims", len(episode.Claims)},
		{"facts", len(episode.Facts)},
		{"anchors", len(episode.Anchors)},
		{"uncertainties", len(episode.Uncertainties)},
	}
	if episode.Flow != nil {
		counts = append(counts, struct {
			name  string
			count int
		}{"flow_nodes", len(episode.Flow.Nodes)})
	}
	for _, item := range counts {
		if item.count > maxEpisodeItems {
			return episodeIndex{}, fmt.Errorf("causal pipeline: episode %s exceeds %d items", item.name, maxEpisodeItems)
		}
	}

	index := episodeIndex{
		claims:        make(map[string]claimInput, len(episode.Claims)),
		facts:         make(map[string]factInput, len(episode.Facts)),
		flowNodes:     make(map[string]flowNodeInput),
		anchors:       make(map[string]anchorInput, len(episode.Anchors)),
		uncertainties: make(map[string]uncertaintyInput, len(episode.Uncertainties)),
	}
	if episode.Flow != nil {
		index.flowNodes = make(map[string]flowNodeInput, len(episode.Flow.Nodes))
	}
	if err := addUnique("claim", episode.Claims, index.claims, func(item claimInput) string { return item.ID }); err != nil {
		return episodeIndex{}, err
	}
	if err := addUnique("fact", episode.Facts, index.facts, func(item factInput) string { return item.ID }); err != nil {
		return episodeIndex{}, err
	}
	if episode.Flow != nil {
		if err := addUnique("flow node", episode.Flow.Nodes, index.flowNodes, func(item flowNodeInput) string { return item.ID }); err != nil {
			return episodeIndex{}, err
		}
	}
	if err := addUnique("anchor", episode.Anchors, index.anchors, func(item anchorInput) string { return item.ID }); err != nil {
		return episodeIndex{}, err
	}
	if err := addUnique("uncertainty", episode.Uncertainties, index.uncertainties, func(item uncertaintyInput) string { return item.ID }); err != nil {
		return episodeIndex{}, err
	}
	return index, nil
}

func addUnique[T any](kind string, items []T, target map[string]T, id func(T) string) error {
	for _, item := range items {
		key := id(item)
		if key == "" {
			return fmt.Errorf("causal pipeline: episode contains empty %s ID", kind)
		}
		if _, exists := target[key]; exists {
			return fmt.Errorf("causal pipeline: episode contains duplicate %s ID %q", kind, key)
		}
		target[key] = item
	}
	return nil
}

type selectedIDs map[string]struct{}

type selectedEvidence struct {
	claims        selectedIDs
	facts         selectedIDs
	flowNodes     selectedIDs
	anchors       selectedIDs
	uncertainties selectedIDs
}

func resolveEvidence(selection evidenceSelection, index episodeIndex) (evidenceSlice, selectedEvidence, error) {
	selected := selectedEvidence{}
	var result evidenceSlice
	var err error
	if result.Claims, selected.claims, err = resolve("claim_ids", selection.ClaimIDs, index.claims, true); err != nil {
		return evidenceSlice{}, selectedEvidence{}, err
	}
	if result.Facts, selected.facts, err = resolve("fact_ids", selection.FactIDs, index.facts, true); err != nil {
		return evidenceSlice{}, selectedEvidence{}, err
	}
	if result.FlowNodes, selected.flowNodes, err = resolve("flow_node_ids", selection.FlowNodeIDs, index.flowNodes, false); err != nil {
		return evidenceSlice{}, selectedEvidence{}, err
	}
	if result.Anchors, selected.anchors, err = resolve("anchor_ids", selection.AnchorIDs, index.anchors, true); err != nil {
		return evidenceSlice{}, selectedEvidence{}, err
	}
	if result.Uncertainties, selected.uncertainties, err = resolve("uncertainty_ids", selection.UncertaintyIDs, index.uncertainties, false); err != nil {
		return evidenceSlice{}, selectedEvidence{}, err
	}
	if len(result.FlowNodes) > 0 {
		for _, node := range result.FlowNodes {
			if _, ok := selected.claims[node.ClaimID]; !ok {
				return evidenceSlice{}, selectedEvidence{}, fmt.Errorf("causal pipeline: selected flow node %q references unselected claim %q", node.ID, node.ClaimID)
			}
		}
	}
	return result, selected, nil
}

func resolve[T any](name string, ids []string, index map[string]T, required bool) ([]T, selectedIDs, error) {
	if (required && len(ids) == 0) || len(ids) > maxSelectedIDs {
		return nil, nil, fmt.Errorf("causal pipeline: %s must contain %d-%d IDs", name, boolInt(required), maxSelectedIDs)
	}
	result := make([]T, 0, len(ids))
	selected := make(selectedIDs, len(ids))
	for _, id := range ids {
		if err := validateText(name, id, maxIDBytes); err != nil {
			return nil, nil, err
		}
		if _, duplicate := selected[id]; duplicate {
			return nil, nil, fmt.Errorf("causal pipeline: %s contains duplicate %q", name, id)
		}
		item, ok := index[id]
		if !ok {
			return nil, nil, fmt.Errorf("causal pipeline: %s references unknown ID %q", name, id)
		}
		selected[id] = struct{}{}
		result = append(result, item)
	}
	return result, selected, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func validateAuthoredContent(spec questionSpec, selected selectedEvidence) error {
	endpoints := make(selectedIDs, len(selected.claims)+len(selected.facts)+len(selected.flowNodes)+len(selected.anchors)+len(selected.uncertainties)+len(spec.Statements))
	for _, source := range []selectedIDs{selected.claims, selected.facts, selected.flowNodes, selected.anchors, selected.uncertainties} {
		for id := range source {
			if _, duplicate := endpoints[id]; duplicate {
				return fmt.Errorf("causal pipeline: selected evidence ID %q is ambiguous across categories", id)
			}
			endpoints[id] = struct{}{}
		}
	}

	statementIDs := make(selectedIDs, len(spec.Statements))
	for i, statement := range spec.Statements {
		where := fmt.Sprintf("design_statements[%d]", i)
		if err := validateText(where+".id", statement.ID, maxIDBytes); err != nil {
			return err
		}
		if _, duplicate := endpoints[statement.ID]; duplicate {
			return fmt.Errorf("causal pipeline: authored statement ID %q collides with another node", statement.ID)
		}
		statementIDs[statement.ID] = struct{}{}
		endpoints[statement.ID] = struct{}{}
		if !validRole(statement.Role) {
			return fmt.Errorf("causal pipeline: %s.role = %q", where, statement.Role)
		}
		requiredState := "inferred"
		if statement.Role == "limit" {
			requiredState = "unknown"
		}
		if statement.State != requiredState {
			return fmt.Errorf("causal pipeline: %s.state must be %s for role %q", where, requiredState, statement.Role)
		}
		if err := validateText(where+".title", statement.Title, maxTitleBytes); err != nil {
			return err
		}
		if err := validateText(where+".statement", statement.Statement, maxTextBytes); err != nil {
			return err
		}
		referenceCount := len(statement.ClaimIDs) + len(statement.FactIDs) + len(statement.FlowNodeIDs) +
			len(statement.AnchorIDs) + len(statement.UncertaintyIDs)
		if referenceCount == 0 || referenceCount > maxSelectedIDs {
			return fmt.Errorf("causal pipeline: %s must cite 1-%d selected IDs", where, maxSelectedIDs)
		}
		if err := requireSelected(where+".claim_ids", statement.ClaimIDs, selected.claims); err != nil {
			return err
		}
		if err := requireSelected(where+".fact_ids", statement.FactIDs, selected.facts); err != nil {
			return err
		}
		if err := requireSelected(where+".flow_node_ids", statement.FlowNodeIDs, selected.flowNodes); err != nil {
			return err
		}
		if err := requireSelected(where+".anchor_ids", statement.AnchorIDs, selected.anchors); err != nil {
			return err
		}
		if err := requireSelected(where+".uncertainty_ids", statement.UncertaintyIDs, selected.uncertainties); err != nil {
			return err
		}
	}

	relationIDs := make(selectedIDs, len(spec.Relations))
	for i, relation := range spec.Relations {
		where := fmt.Sprintf("relations[%d]", i)
		if err := validateText(where+".id", relation.ID, maxIDBytes); err != nil {
			return err
		}
		if _, duplicate := endpoints[relation.ID]; duplicate {
			return fmt.Errorf("causal pipeline: relation ID %q collides with a causal node", relation.ID)
		}
		if _, duplicate := relationIDs[relation.ID]; duplicate {
			return fmt.Errorf("causal pipeline: duplicate relation ID %q", relation.ID)
		}
		relationIDs[relation.ID] = struct{}{}
		if relation.State != "inferred" && relation.State != "unknown" {
			return fmt.Errorf("causal pipeline: %s.state must be inferred or unknown", where)
		}
		if err := validateText(where+".from_id", relation.FromID, maxIDBytes); err != nil {
			return err
		}
		if err := validateText(where+".to_id", relation.ToID, maxIDBytes); err != nil {
			return err
		}
		if _, ok := endpoints[relation.FromID]; !ok {
			return fmt.Errorf("causal pipeline: %s.from_id references unselected ID %q", where, relation.FromID)
		}
		if _, ok := endpoints[relation.ToID]; !ok {
			return fmt.Errorf("causal pipeline: %s.to_id references unselected ID %q", where, relation.ToID)
		}
		if err := validateText(where+".statement", relation.Statement, maxTextBytes); err != nil {
			return err
		}
	}
	return nil
}

func requireSelected(name string, ids []string, selected selectedIDs) error {
	if len(ids) > maxSelectedIDs {
		return fmt.Errorf("causal pipeline: %s contains more than %d IDs", name, maxSelectedIDs)
	}
	seen := make(selectedIDs, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("causal pipeline: %s contains duplicate %q", name, id)
		}
		seen[id] = struct{}{}
		if _, ok := selected[id]; !ok {
			return fmt.Errorf("causal pipeline: %s references unselected ID %q", name, id)
		}
	}
	return nil
}

func validRole(role string) bool {
	switch role {
	case "answer", "change", "effect", "limit", "check":
		return true
	default:
		return false
	}
}

func validateText(name, value string, limit int) error {
	if len(value) > limit {
		return fmt.Errorf("causal pipeline: %s exceeds %d bytes", name, limit)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("causal pipeline: %s is empty", name)
	}
	return nil
}

func renderMarkdown(slice causalSlice) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n", slice.Title)
	fmt.Fprintf(&out, "Question: **%s**\n\n", slice.Question)
	fmt.Fprintf(&out, "Accepted input: `%s` at `%s@%s`.\n\n", slice.Episode.ID, slice.Episode.RepositoryName, slice.Episode.RepositoryRevision)

	renderStatements(&out, slice.Statements, "answer", "Short answer")

	out.WriteString("## What the accepted episode establishes\n\n")
	for _, claim := range slice.Evidence.Claims {
		fmt.Fprintf(&out, "- **%s** — %s. %s\n", strings.ToUpper(claim.State), claim.Title, claim.Statement)
	}
	out.WriteByte('\n')

	if len(slice.Evidence.FlowNodes) > 0 {
		out.WriteString("## Causal context\n\n")
		claimTitles := make(map[string]string, len(slice.Evidence.Claims))
		for _, claim := range slice.Evidence.Claims {
			claimTitles[claim.ID] = claim.Title
		}
		for _, node := range slice.Evidence.FlowNodes {
			fmt.Fprintf(&out, "- %s\n", claimTitles[node.ClaimID])
		}
		out.WriteByte('\n')
	}

	renderStatements(&out, slice.Statements, "change", "Where to change")
	renderStatements(&out, slice.Statements, "effect", "What behavior changes")

	renderStatements(&out, slice.Statements, "limit", "What remains unknown")
	renderStatements(&out, slice.Statements, "check", "Smallest useful proof")

	out.WriteString("## Source navigation — context, not automatic edit points\n\n")
	out.WriteString("These accepted locations explain the selected behavior. Treat them as navigation unless the change section explicitly names one as an edit point.\n\n")
	for _, anchor := range slice.Evidence.Anchors {
		if anchor.URL != "" {
			fmt.Fprintf(&out, "- [`%s:%d`](%s)\n", anchor.Path, anchor.StartLine, anchor.URL)
		} else {
			fmt.Fprintf(&out, "- `%s:%d`\n", anchor.Path, anchor.StartLine)
		}
	}
	return []byte(out.String())
}

func renderStatements(out *strings.Builder, statements []authoredStatement, role, heading string) {
	var selected []authoredStatement
	for _, statement := range statements {
		if statement.Role == role {
			selected = append(selected, statement)
		}
	}
	if len(selected) == 0 {
		return
	}
	fmt.Fprintf(out, "## %s — %s\n\n", heading, strings.ToUpper(selected[0].State))
	for i, statement := range selected {
		if role == "change" || role == "check" {
			fmt.Fprintf(out, "%d. **%s.** %s\n", i+1, statement.Title, statement.Statement)
		} else {
			fmt.Fprintf(out, "- **%s** — %s\n", statement.Title, statement.Statement)
		}
	}
	out.WriteByte('\n')
}
