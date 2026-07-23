package tasklens

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// RetrievalStage identifies the bounded stage that admitted a candidate to
// the retained frontier. Initial candidates that are never admitted remain
// initial; completion and verification values bind the exact later decision.
type RetrievalStage string

const (
	RetrievalStageInitial      RetrievalStage = "initial"
	RetrievalStageCompletion1  RetrievalStage = "completion_expansion_1"
	RetrievalStageCompletion2  RetrievalStage = "completion_expansion_2"
	RetrievalStageVerification RetrievalStage = "verification_probe"
)

// RetrievalScoreKind preserves the separate scoring components required by
// the completion contract.
type RetrievalScoreKind string

const (
	RetrievalScoreDirectTaskTerm       RetrievalScoreKind = "direct_task_term_match"
	RetrievalScoreMissingRole          RetrievalScoreKind = "missing_role_fit"
	RetrievalScoreExactRelation        RetrievalScoreKind = "exact_relation_to_selected_anchor"
	RetrievalScoreProductionRelevance  RetrievalScoreKind = "production_relevance"
	RetrievalScoreTestFixtureRelevance RetrievalScoreKind = "test_fixture_relevance"
	RetrievalScoreScopeCompleteness    RetrievalScoreKind = "source_scope_completeness"
	RetrievalScoreRepositoryRole       RetrievalScoreKind = "repository_role"
	RetrievalScoreDistance             RetrievalScoreKind = "distance_from_selected_anchor"
	RetrievalScoreAdjacentPenalty      RetrievalScoreKind = "duplicate_adjacent_only_penalty"
	RetrievalScoreExampleOnlyPenalty   RetrievalScoreKind = "example_test_only_penalty"
)

// RetrievalTaskTerm records the exact deterministic term extraction result.
type RetrievalTaskTerm struct {
	Text       string `json:"text"`
	Normalized string `json:"normalized"`
	Found      bool   `json:"found"`
	Weight     int    `json:"weight"`
}

type RetrievalScoreComponent struct {
	Kind   RetrievalScoreKind `json:"kind"`
	Value  int                `json:"value"`
	Detail string             `json:"detail,omitempty"`
}

// RetrievalCandidate is recorded before ranking or reservation chooses it.
type RetrievalCandidate struct {
	ID              string                    `json:"id"`
	Stage           RetrievalStage            `json:"stage"`
	DiscoveryOrder  int                       `json:"discovery_order"`
	Path            string                    `json:"path"`
	Symbol          string                    `json:"symbol,omitempty"`
	Roles           []AnchorRole              `json:"roles"`
	Score           int                       `json:"score"`
	ScoreComponents []RetrievalScoreComponent `json:"score_components"`
}

// RetrievalRelationship captures exact local candidate evidence before prose.
type RetrievalRelationship struct {
	ID            string       `json:"id"`
	LeftID        string       `json:"left_candidate_id"`
	RightID       string       `json:"right_candidate_id"`
	Kind          RelationKind `json:"kind"`
	SupportType   SupportType  `json:"support_type"`
	EvidenceIDs   []string     `json:"evidence_ids"`
	Scope         string       `json:"scope"`
	NonGuarantees string       `json:"non_guarantees"`
}

type RetrievalSelection struct {
	CandidateID string `json:"candidate_id"`
	AnchorID    string `json:"anchor_id"`
	Rank        int    `json:"rank"`
	Reason      string `json:"reason"`
}

type RetrievalDrop struct {
	CandidateID string `json:"candidate_id"`
	Reason      string `json:"reason"`
}

type RetrievalSourceScope struct {
	AnchorID string      `json:"anchor_id"`
	Scope    SourceScope `json:"scope"`
}

// RetrievalLimit records both a bound event and whether it caused evidence loss.
type RetrievalLimit struct {
	Name       string `json:"name"`
	Limit      int    `json:"limit"`
	Observed   int    `json:"observed"`
	Applied    bool   `json:"applied"`
	CausedLoss bool   `json:"caused_loss"`
	LossReason string `json:"loss_reason,omitempty"`
}

type GoldAnchorDisposition string

const (
	GoldAnchorPresentBeforeRanking         GoldAnchorDisposition = "present_before_ranking"
	GoldAnchorDroppedDuringRanking         GoldAnchorDisposition = "dropped_during_ranking"
	GoldAnchorNeverGenerated               GoldAnchorDisposition = "never_generated"
	GoldAnchorClippedDuringSourceRetention GoldAnchorDisposition = "clipped_during_source_retention"
)

// GoldAnchorAssessment is an evaluation-only label added after retrieval. It
// must never be consumed by the collector or scorer.
type GoldAnchorAssessment struct {
	Disposition GoldAnchorDisposition `json:"disposition"`
	CandidateID string                `json:"candidate_id,omitempty"`
	AnchorID    string                `json:"anchor_id,omitempty"`
	Detail      string                `json:"detail"`
}

// RetrievalTrace is a replayable local account of retrieval, ranking,
// completeness, role coverage, verification, and actual bound events.
type RetrievalTrace struct {
	Version                 int                     `json:"version"`
	TaskKind                TaskKind                `json:"task_kind"`
	TaskProfile             TaskProfile             `json:"task_profile"`
	TaskTerms               []RetrievalTaskTerm     `json:"task_terms"`
	CandidatesBeforeRanking []RetrievalCandidate    `json:"candidates_before_ranking"`
	Relationships           []RetrievalRelationship `json:"relationships"`
	SelectedAnchors         []RetrievalSelection    `json:"selected_anchors"`
	DroppedAnchors          []RetrievalDrop         `json:"dropped_anchors"`
	SourceScopes            []RetrievalSourceScope  `json:"source_scopes"`
	RoleCoverage            RoleCoverage            `json:"role_coverage"`
	VerificationFrontier    VerificationFrontier    `json:"verification_frontier"`
	Budgets                 Budgets                 `json:"budgets"`
	Limits                  []RetrievalLimit        `json:"limits"`
	GoldAssessment          *GoldAnchorAssessment   `json:"gold_assessment,omitempty"`
}

// Validate checks trace referential integrity and deterministic projections.
func (t RetrievalTrace) Validate() error {
	if t.Version != RetrievalTraceVersion || !validTaskKind(t.TaskKind) ||
		!validTaskProfile(t.TaskProfile) {
		return fmt.Errorf("task lens: invalid retrieval trace header")
	}
	if err := validateTraceTerms(t.TaskTerms); err != nil {
		return err
	}
	candidates, err := validateTraceCandidates(t.CandidatesBeforeRanking)
	if err != nil {
		return err
	}
	if err := validateTraceRelationships(t.Relationships, candidates); err != nil {
		return err
	}
	selected, outcomes, err := validateTraceSelections(
		t.SelectedAnchors,
		t.DroppedAnchors,
		candidates,
	)
	if err != nil {
		return err
	}
	if len(outcomes) != len(candidates) {
		return fmt.Errorf("task lens: every retrieval candidate must have an exact outcome")
	}
	if err := validateTraceScopes(t.SourceScopes, selected); err != nil {
		return err
	}
	if err := t.RoleCoverage.Validate(); err != nil {
		return err
	}
	if t.RoleCoverage.Profile != t.TaskProfile {
		return fmt.Errorf("task lens: trace role profile does not match task profile")
	}
	if err := validateCoverageAnchorIDs(t.RoleCoverage, selected); err != nil {
		return err
	}
	if err := t.VerificationFrontier.Validate(); err != nil {
		return err
	}
	if err := validateFrontierAnchorIDs(t.VerificationFrontier, selected); err != nil {
		return err
	}
	if err := validateBudget(t.Budgets); err != nil {
		return err
	}
	if err := validateTraceLimits(t.Limits); err != nil {
		return err
	}
	if t.GoldAssessment != nil {
		if err := validateGoldAssessment(*t.GoldAssessment, candidates, selected); err != nil {
			return err
		}
	}
	return nil
}

// ValidateAgainstBundle binds the independently saved trace to every
// canonical retrieval projection that can be replayed from the bundle. Gold
// labels are deliberately rejected here: production traces are immutable
// inputs to later development-only evaluation.
func (t RetrievalTrace) ValidateAgainstBundle(bundle Bundle) error {
	if err := t.Validate(); err != nil {
		return err
	}
	if t.GoldAssessment != nil {
		return fmt.Errorf("task lens: production retrieval trace contains gold assessment")
	}
	if t.TaskKind != bundle.KindHint || t.TaskProfile != bundle.Profile ||
		!reflect.DeepEqual(t.RoleCoverage, bundle.RoleCoverage) ||
		!reflect.DeepEqual(t.VerificationFrontier, bundle.Verification) ||
		t.Budgets != bundle.Budgets {
		return fmt.Errorf("task lens: retrieval trace contract projection differs from bundle")
	}
	if len(t.TaskTerms) != len(bundle.Terms) {
		return fmt.Errorf("task lens: retrieval trace task terms differ from bundle")
	}
	for index, item := range t.TaskTerms {
		term := bundle.Terms[index]
		if item.Text != term.Text || item.Normalized != term.Normalized ||
			item.Found != term.Found || item.Weight != term.Weight {
			return fmt.Errorf("task lens: retrieval trace task terms differ from bundle")
		}
	}

	candidates := make(map[string]RetrievalCandidate, len(t.CandidatesBeforeRanking))
	for _, candidate := range t.CandidatesBeforeRanking {
		candidates[candidate.ID] = candidate
	}
	if len(t.SelectedAnchors) != len(bundle.Anchors) || len(t.SourceScopes) != len(bundle.Anchors) {
		return fmt.Errorf("task lens: retrieval trace selected anchors differ from bundle")
	}
	scopes := make(map[string]SourceScope, len(t.SourceScopes))
	for _, item := range t.SourceScopes {
		scopes[item.AnchorID] = item.Scope
	}
	selections := append([]RetrievalSelection(nil), t.SelectedAnchors...)
	sort.Slice(selections, func(i, j int) bool { return selections[i].Rank < selections[j].Rank })
	for index, anchor := range bundle.Anchors {
		selection := selections[index]
		candidate, ok := candidates[selection.CandidateID]
		if !ok || selection.AnchorID != anchor.ID || selection.CandidateID != anchor.ID ||
			candidate.Path != anchor.Path || candidate.Symbol != anchor.Symbol ||
			!reflect.DeepEqual(candidate.Roles, anchor.RoleHints) ||
			!reflect.DeepEqual(scopes[anchor.ID], anchor.Scope) {
			return fmt.Errorf("task lens: retrieval trace selected anchor projection differs from bundle")
		}
	}

	if len(t.Relationships) != len(bundle.Relations) {
		return fmt.Errorf("task lens: retrieval trace relationships differ from bundle")
	}
	relationships := make(map[string]RetrievalRelationship, len(t.Relationships))
	for _, relationship := range t.Relationships {
		relationships[relationship.ID] = relationship
	}
	for _, relation := range bundle.Relations {
		item, ok := relationships[relation.ID]
		if !ok || item.LeftID != relation.LeftID || item.RightID != relation.RightID ||
			item.Kind != RelationKind(relation.Kind) || item.SupportType != relation.SupportType ||
			!reflect.DeepEqual(item.EvidenceIDs, relation.EvidenceIDs) ||
			item.NonGuarantees != relation.Scope {
			return fmt.Errorf("task lens: retrieval trace relationships differ from bundle")
		}
	}
	return nil
}

// RenderRetrievalTraceMarkdown renders a canonical, stable human projection.
// It sorts set-like trace fields without mutating the JSON authority.
func RenderRetrievalTraceMarkdown(trace RetrievalTrace) (string, error) {
	if err := trace.Validate(); err != nil {
		return "", err
	}
	var builder strings.Builder
	builder.WriteString("# Task Lens retrieval trace\n\n")
	fmt.Fprintf(&builder, "- Version: `%d`\n", trace.Version)
	fmt.Fprintf(&builder, "- Task kind: `%s`\n", trace.TaskKind)
	fmt.Fprintf(&builder, "- Task profile: `%s`\n", trace.TaskProfile)
	if trace.GoldAssessment == nil {
		builder.WriteString("- Gold assessment: not applied\n")
	} else {
		fmt.Fprintf(
			&builder,
			"- Gold assessment: `%s` — %s\n",
			trace.GoldAssessment.Disposition,
			markdownCell(trace.GoldAssessment.Detail),
		)
	}

	renderTraceTerms(&builder, trace.TaskTerms)
	renderTraceCandidates(&builder, trace.CandidatesBeforeRanking)
	renderTraceRelationships(&builder, trace.Relationships)
	renderTraceSelections(&builder, trace.SelectedAnchors)
	renderTraceDrops(&builder, trace.DroppedAnchors)
	renderTraceScopes(&builder, trace.SourceScopes)
	renderRoleCoverage(&builder, trace.RoleCoverage)
	renderVerificationFrontier(&builder, trace.VerificationFrontier)
	renderBudgets(&builder, trace.Budgets)
	renderTraceLimits(&builder, trace.Limits)
	return builder.String(), nil
}

// Markdown is the method form of RenderRetrievalTraceMarkdown.
func (t RetrievalTrace) Markdown() (string, error) {
	return RenderRetrievalTraceMarkdown(t)
}

func validateTraceTerms(terms []RetrievalTaskTerm) error {
	seen := make(map[string]struct{}, len(terms))
	for _, term := range terms {
		if !validText(term.Text, 512, true) || !validText(term.Normalized, 512, true) ||
			term.Weight < 0 {
			return fmt.Errorf("task lens: invalid retrieval task term")
		}
		if _, duplicate := seen[term.Normalized]; duplicate {
			return fmt.Errorf("task lens: duplicate normalized retrieval task term")
		}
		seen[term.Normalized] = struct{}{}
	}
	return nil
}

func validateTraceCandidates(candidates []RetrievalCandidate) (map[string]RetrievalCandidate, error) {
	result := make(map[string]RetrievalCandidate, len(candidates))
	discoveryOrders := make(map[int]struct{}, len(candidates))
	for _, candidate := range candidates {
		if !validOpaque(candidate.ID) || !validRetrievalStage(candidate.Stage) ||
			candidate.DiscoveryOrder <= 0 || !validPath(candidate.Path) ||
			!validText(candidate.Symbol, 256, false) || len(candidate.ScoreComponents) == 0 {
			return nil, fmt.Errorf("task lens: invalid retrieval candidate")
		}
		if _, duplicate := result[candidate.ID]; duplicate {
			return nil, fmt.Errorf("task lens: duplicate retrieval candidate")
		}
		if _, duplicate := discoveryOrders[candidate.DiscoveryOrder]; duplicate {
			return nil, fmt.Errorf("task lens: duplicate retrieval discovery order")
		}
		discoveryOrders[candidate.DiscoveryOrder] = struct{}{}
		roles := make(map[AnchorRole]struct{}, len(candidate.Roles))
		for _, role := range candidate.Roles {
			if !validContractAnchorRole(role) {
				return nil, fmt.Errorf("task lens: invalid retrieval candidate role")
			}
			if _, duplicate := roles[role]; duplicate {
				return nil, fmt.Errorf("task lens: duplicate retrieval candidate role")
			}
			roles[role] = struct{}{}
		}
		componentKinds := make(map[RetrievalScoreKind]struct{}, len(candidate.ScoreComponents))
		score := 0
		for _, component := range candidate.ScoreComponents {
			if !validRetrievalScoreKind(component.Kind) ||
				!validText(component.Detail, 1024, false) {
				return nil, fmt.Errorf("task lens: invalid retrieval score component")
			}
			if _, duplicate := componentKinds[component.Kind]; duplicate {
				return nil, fmt.Errorf("task lens: duplicate retrieval score component")
			}
			componentKinds[component.Kind] = struct{}{}
			score += component.Value
		}
		if candidate.Score != score {
			return nil, fmt.Errorf("task lens: retrieval score does not match components")
		}
		result[candidate.ID] = candidate
	}
	return result, nil
}

func validateTraceRelationships(
	relationships []RetrievalRelationship,
	candidates map[string]RetrievalCandidate,
) error {
	seen := make(map[string]struct{}, len(relationships))
	for _, relationship := range relationships {
		if !validOpaque(relationship.ID) || relationship.LeftID == relationship.RightID ||
			!validRelationKind(relationship.Kind) || !validSupportType(relationship.SupportType) ||
			!validText(relationship.Scope, 1024, true) ||
			!validText(relationship.NonGuarantees, 1024, true) ||
			!isUniqueOpaque(relationship.EvidenceIDs) || len(relationship.EvidenceIDs) == 0 {
			return fmt.Errorf("task lens: invalid retrieval relationship")
		}
		if _, ok := candidates[relationship.LeftID]; !ok {
			return fmt.Errorf("task lens: retrieval relationship has unknown left candidate")
		}
		if _, ok := candidates[relationship.RightID]; !ok {
			return fmt.Errorf("task lens: retrieval relationship has unknown right candidate")
		}
		if _, duplicate := seen[relationship.ID]; duplicate {
			return fmt.Errorf("task lens: duplicate retrieval relationship")
		}
		seen[relationship.ID] = struct{}{}
	}
	return nil
}

func validateTraceSelections(
	selections []RetrievalSelection,
	drops []RetrievalDrop,
	candidates map[string]RetrievalCandidate,
) (map[string]struct{}, map[string]struct{}, error) {
	selectedAnchors := make(map[string]struct{}, len(selections))
	outcomes := make(map[string]struct{}, len(selections)+len(drops))
	ranks := make([]int, 0, len(selections))
	for _, selection := range selections {
		if _, ok := candidates[selection.CandidateID]; !ok || !validOpaque(selection.AnchorID) ||
			selection.Rank <= 0 || !validText(selection.Reason, 1024, true) {
			return nil, nil, fmt.Errorf("task lens: invalid retrieval selection")
		}
		if _, duplicate := outcomes[selection.CandidateID]; duplicate {
			return nil, nil, fmt.Errorf("task lens: duplicate retrieval candidate outcome")
		}
		if _, duplicate := selectedAnchors[selection.AnchorID]; duplicate {
			return nil, nil, fmt.Errorf("task lens: duplicate selected anchor id")
		}
		outcomes[selection.CandidateID] = struct{}{}
		selectedAnchors[selection.AnchorID] = struct{}{}
		ranks = append(ranks, selection.Rank)
	}
	sort.Ints(ranks)
	for index, rank := range ranks {
		if rank != index+1 {
			return nil, nil, fmt.Errorf("task lens: selected anchor ranks are not contiguous")
		}
	}
	for _, drop := range drops {
		if _, ok := candidates[drop.CandidateID]; !ok || !validText(drop.Reason, 1024, true) {
			return nil, nil, fmt.Errorf("task lens: invalid dropped retrieval candidate")
		}
		if _, duplicate := outcomes[drop.CandidateID]; duplicate {
			return nil, nil, fmt.Errorf("task lens: duplicate retrieval candidate outcome")
		}
		outcomes[drop.CandidateID] = struct{}{}
	}
	return selectedAnchors, outcomes, nil
}

func validateTraceScopes(scopes []RetrievalSourceScope, selected map[string]struct{}) error {
	seen := make(map[string]struct{}, len(scopes))
	for _, item := range scopes {
		if _, ok := selected[item.AnchorID]; !ok {
			return fmt.Errorf("task lens: source scope has unknown selected anchor")
		}
		if _, duplicate := seen[item.AnchorID]; duplicate {
			return fmt.Errorf("task lens: duplicate retrieval source scope")
		}
		if err := item.Scope.Validate(); err != nil {
			return err
		}
		seen[item.AnchorID] = struct{}{}
	}
	if len(seen) != len(selected) {
		return fmt.Errorf("task lens: every selected anchor must have source scope")
	}
	return nil
}

func validateCoverageAnchorIDs(coverage RoleCoverage, selected map[string]struct{}) error {
	groups := [][]RoleCoverageItem{coverage.Key, coverage.Supporting, coverage.Optional}
	for _, group := range groups {
		for _, item := range group {
			for _, anchorID := range item.AnchorIDs {
				if _, ok := selected[anchorID]; !ok {
					return fmt.Errorf("task lens: role coverage references unselected anchor")
				}
			}
		}
	}
	return nil
}

func validateFrontierAnchorIDs(frontier VerificationFrontier, selected map[string]struct{}) error {
	if frontier.DecisiveAnchorID != "" {
		if _, ok := selected[frontier.DecisiveAnchorID]; !ok {
			return fmt.Errorf("task lens: verification frontier has unselected decisive anchor")
		}
	}
	for _, item := range frontier.allItems() {
		if item.AnchorID == "" {
			continue
		}
		if _, ok := selected[item.AnchorID]; !ok {
			return fmt.Errorf("task lens: verification frontier references unselected anchor")
		}
	}
	return nil
}

func validateTraceLimits(limits []RetrievalLimit) error {
	seen := make(map[string]struct{}, len(limits))
	for _, limit := range limits {
		if !validText(limit.Name, 128, true) || limit.Limit < 0 || limit.Observed < 0 ||
			!validText(limit.LossReason, 1024, false) {
			return fmt.Errorf("task lens: invalid retrieval limit")
		}
		if _, duplicate := seen[limit.Name]; duplicate {
			return fmt.Errorf("task lens: duplicate retrieval limit")
		}
		seen[limit.Name] = struct{}{}
		if limit.CausedLoss && (!limit.Applied || limit.LossReason == "") {
			return fmt.Errorf("task lens: loss-causing limit lacks exact reason")
		}
		if !limit.CausedLoss && limit.LossReason != "" {
			return fmt.Errorf("task lens: non-loss limit cannot carry a loss reason")
		}
	}
	return nil
}

func validateGoldAssessment(
	assessment GoldAnchorAssessment,
	candidates map[string]RetrievalCandidate,
	selected map[string]struct{},
) error {
	if !validGoldDisposition(assessment.Disposition) ||
		!validText(assessment.Detail, 1024, true) {
		return fmt.Errorf("task lens: invalid gold anchor assessment")
	}
	if assessment.CandidateID != "" {
		if _, ok := candidates[assessment.CandidateID]; !ok {
			return fmt.Errorf("task lens: gold assessment references unknown candidate")
		}
	}
	if assessment.AnchorID != "" {
		if _, ok := selected[assessment.AnchorID]; !ok {
			return fmt.Errorf("task lens: gold assessment references unknown selected anchor")
		}
	}
	switch assessment.Disposition {
	case GoldAnchorPresentBeforeRanking, GoldAnchorDroppedDuringRanking:
		if assessment.CandidateID == "" {
			return fmt.Errorf("task lens: gold ranking assessment lacks candidate id")
		}
	case GoldAnchorClippedDuringSourceRetention:
		if assessment.CandidateID == "" || assessment.AnchorID == "" {
			return fmt.Errorf("task lens: clipped gold assessment lacks retained identities")
		}
	case GoldAnchorNeverGenerated:
		if assessment.CandidateID != "" || assessment.AnchorID != "" {
			return fmt.Errorf("task lens: never-generated gold assessment claims local identity")
		}
	}
	return nil
}

func validRetrievalStage(stage RetrievalStage) bool {
	switch stage {
	case RetrievalStageInitial,
		RetrievalStageCompletion1,
		RetrievalStageCompletion2,
		RetrievalStageVerification:
		return true
	default:
		return false
	}
}

func validRetrievalScoreKind(kind RetrievalScoreKind) bool {
	switch kind {
	case RetrievalScoreDirectTaskTerm,
		RetrievalScoreMissingRole,
		RetrievalScoreExactRelation,
		RetrievalScoreProductionRelevance,
		RetrievalScoreTestFixtureRelevance,
		RetrievalScoreScopeCompleteness,
		RetrievalScoreRepositoryRole,
		RetrievalScoreDistance,
		RetrievalScoreAdjacentPenalty,
		RetrievalScoreExampleOnlyPenalty:
		return true
	default:
		return false
	}
}

func validGoldDisposition(disposition GoldAnchorDisposition) bool {
	switch disposition {
	case GoldAnchorPresentBeforeRanking,
		GoldAnchorDroppedDuringRanking,
		GoldAnchorNeverGenerated,
		GoldAnchorClippedDuringSourceRetention:
		return true
	default:
		return false
	}
}

func renderTraceTerms(builder *strings.Builder, terms []RetrievalTaskTerm) {
	items := append([]RetrievalTaskTerm(nil), terms...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Normalized != items[j].Normalized {
			return items[i].Normalized < items[j].Normalized
		}
		return items[i].Text < items[j].Text
	})
	builder.WriteString("\n## Extracted task terms\n\n")
	if len(items) == 0 {
		builder.WriteString("None.\n")
		return
	}
	builder.WriteString("| Term | Normalized | Found | Weight |\n| --- | --- | --- | ---: |\n")
	for _, item := range items {
		fmt.Fprintf(
			builder,
			"| %s | %s | %t | %d |\n",
			markdownCell(item.Text),
			markdownCell(item.Normalized),
			item.Found,
			item.Weight,
		)
	}
}

func renderTraceCandidates(builder *strings.Builder, candidates []RetrievalCandidate) {
	items := append([]RetrievalCandidate(nil), candidates...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].DiscoveryOrder != items[j].DiscoveryOrder {
			return items[i].DiscoveryOrder < items[j].DiscoveryOrder
		}
		return items[i].ID < items[j].ID
	})
	builder.WriteString("\n## Candidates before ranking\n\n")
	if len(items) == 0 {
		builder.WriteString("None.\n")
		return
	}
	builder.WriteString("| Order | Stage | Candidate | Path / symbol | Roles | Score | Components |\n")
	builder.WriteString("| ---: | --- | --- | --- | --- | ---: | --- |\n")
	for _, item := range items {
		roles := make([]string, 0, len(item.Roles))
		for _, role := range item.Roles {
			roles = append(roles, string(role))
		}
		sort.Strings(roles)
		location := item.Path
		if item.Symbol != "" {
			location += " / " + item.Symbol
		}
		fmt.Fprintf(
			builder,
			"| %d | %s | %s | %s | %s | %d | %s |\n",
			item.DiscoveryOrder,
			item.Stage,
			item.ID,
			markdownCell(location),
			markdownCell(strings.Join(roles, ", ")),
			item.Score,
			markdownCell(formatScoreComponents(item.ScoreComponents)),
		)
	}
}

func renderTraceRelationships(builder *strings.Builder, relationships []RetrievalRelationship) {
	items := append([]RetrievalRelationship(nil), relationships...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	builder.WriteString("\n## Exact local relationships\n\n")
	if len(items) == 0 {
		builder.WriteString("None.\n")
		return
	}
	builder.WriteString("| ID | Left → right | Kind | Support | Evidence | Scope / non-guarantees |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, item := range items {
		evidenceIDs := append([]string(nil), item.EvidenceIDs...)
		sort.Strings(evidenceIDs)
		fmt.Fprintf(
			builder,
			"| %s | %s → %s | %s | %s | %s | %s / %s |\n",
			item.ID,
			item.LeftID,
			item.RightID,
			item.Kind,
			item.SupportType,
			markdownCell(strings.Join(evidenceIDs, ", ")),
			markdownCell(item.Scope),
			markdownCell(item.NonGuarantees),
		)
	}
}

func renderTraceSelections(builder *strings.Builder, selections []RetrievalSelection) {
	items := append([]RetrievalSelection(nil), selections...)
	sort.Slice(items, func(i, j int) bool { return items[i].Rank < items[j].Rank })
	builder.WriteString("\n## Selected anchors\n\n")
	if len(items) == 0 {
		builder.WriteString("None.\n")
		return
	}
	builder.WriteString("| Rank | Candidate | Anchor | Reason |\n| ---: | --- | --- | --- |\n")
	for _, item := range items {
		fmt.Fprintf(
			builder,
			"| %d | %s | %s | %s |\n",
			item.Rank,
			item.CandidateID,
			item.AnchorID,
			markdownCell(item.Reason),
		)
	}
}

func renderTraceDrops(builder *strings.Builder, drops []RetrievalDrop) {
	items := append([]RetrievalDrop(nil), drops...)
	sort.Slice(items, func(i, j int) bool { return items[i].CandidateID < items[j].CandidateID })
	builder.WriteString("\n## Dropped candidates\n\n")
	if len(items) == 0 {
		builder.WriteString("None.\n")
		return
	}
	builder.WriteString("| Candidate | Exact reason |\n| --- | --- |\n")
	for _, item := range items {
		fmt.Fprintf(builder, "| %s | %s |\n", item.CandidateID, markdownCell(item.Reason))
	}
}

func renderTraceScopes(builder *strings.Builder, scopes []RetrievalSourceScope) {
	items := append([]RetrievalSourceScope(nil), scopes...)
	sort.Slice(items, func(i, j int) bool { return items[i].AnchorID < items[j].AnchorID })
	builder.WriteString("\n## Source-scope completeness\n\n")
	if len(items) == 0 {
		builder.WriteString("None.\n")
		return
	}
	builder.WriteString("| Anchor | Kind | Lines | Truncated | Match outside | Negative claims | Basis | Reason |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, item := range items {
		scope := item.Scope
		fmt.Fprintf(
			builder,
			"| %s | %s | %d-%d / %d | %t | %t | %t | %s | %s |\n",
			item.AnchorID,
			scope.ScopeKind,
			scope.ScopeStart,
			scope.ScopeEnd,
			scope.SourceTotalLines,
			scope.Truncated,
			scope.TaskMatchesOutsideWindow,
			scope.NegativeClaimsAllowed,
			scope.NegativeEvidenceBasis,
			markdownCell(scope.TruncationReason),
		)
	}
}

func renderRoleCoverage(builder *strings.Builder, coverage RoleCoverage) {
	builder.WriteString("\n## Role coverage\n\n")
	builder.WriteString("| Tier | Role | Required | Anchors | Represented |\n")
	builder.WriteString("| --- | --- | ---: | --- | --- |\n")
	type tierGroup struct {
		name  string
		items []RoleCoverageItem
	}
	groups := []tierGroup{
		{name: "key", items: coverage.Key},
		{name: "supporting", items: coverage.Supporting},
		{name: "optional", items: coverage.Optional},
	}
	rowCount := 0
	for _, group := range groups {
		items := append([]RoleCoverageItem(nil), group.items...)
		sort.Slice(items, func(i, j int) bool { return items[i].Role < items[j].Role })
		for _, item := range items {
			fmt.Fprintf(
				builder,
				"| %s | %s | %d | %s | %t |\n",
				group.name,
				item.Role,
				item.MinimumAnchors,
				markdownCell(strings.Join(item.AnchorIDs, ", ")),
				item.Represented,
			)
			rowCount++
		}
	}
	if rowCount == 0 {
		builder.WriteString("| — | — | 0 | — | false |\n")
	}
}

func renderVerificationFrontier(builder *strings.Builder, frontier VerificationFrontier) {
	type slottedItem struct {
		slot string
		item VerificationItem
	}
	items := make([]slottedItem, 0, len(frontier.Anchors)+2)
	for _, item := range frontier.Anchors {
		items = append(items, slottedItem{slot: "anchor", item: item})
	}
	if frontier.Fixture != nil {
		items = append(items, slottedItem{slot: "fixture", item: *frontier.Fixture})
	}
	if frontier.CommandOrEffect != nil {
		items = append(items, slottedItem{slot: "command_or_effect", item: *frontier.CommandOrEffect})
	}
	sort.Slice(items, func(i, j int) bool {
		leftRank := verificationAuthorityRank(items[i].item.Authority)
		rightRank := verificationAuthorityRank(items[j].item.Authority)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return items[i].item.ID < items[j].item.ID
	})
	builder.WriteString("\n## Verification frontier\n\n")
	if frontier.DecisiveAnchorID == "" {
		builder.WriteString("Decisive anchor: none.\n\n")
	} else {
		fmt.Fprintf(builder, "Decisive anchor: `%s`.\n\n", frontier.DecisiveAnchorID)
	}
	if len(items) == 0 {
		builder.WriteString("No verification candidates.\n")
		return
	}
	builder.WriteString("| Slot | ID | Authority | Anchor | Path / symbol | Evidence | Text |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, entry := range items {
		item := entry.item
		location := item.Path
		if item.Symbol != "" {
			location += " / " + item.Symbol
		}
		evidenceIDs := append([]string(nil), item.EvidenceIDs...)
		sort.Strings(evidenceIDs)
		fmt.Fprintf(
			builder,
			"| %s | %s | %s | %s | %s | %s | %s |\n",
			entry.slot,
			item.ID,
			item.Authority,
			item.AnchorID,
			markdownCell(location),
			markdownCell(strings.Join(evidenceIDs, ", ")),
			markdownCell(item.Text),
		)
	}
}

func renderBudgets(builder *strings.Builder, budgets Budgets) {
	builder.WriteString("\n## Budget consumption\n\n")
	builder.WriteString("| Budget | Consumed | Bound event |\n| --- | ---: | --- |\n")
	rows := []struct {
		name  string
		value int64
		bound bool
	}{
		{name: "initial_candidates", value: int64(budgets.InitialCandidates), bound: budgets.CandidateLimitBound},
		{name: "retained_anchors", value: int64(budgets.RetainedAnchors), bound: budgets.AnchorLimitBound},
		{name: "read_files", value: int64(budgets.ReadFiles), bound: budgets.FileLimitBound},
		{name: "read_bytes", value: int64(budgets.ReadBytes), bound: budgets.ByteLimitBound},
		{name: "source_scan_bytes", value: int64(budgets.SourceScanBytes), bound: budgets.SourceScanLimitBound},
		{name: "retained_source_bytes", value: int64(budgets.RetainedSourceBytes), bound: budgets.RetainedByteLimitBound},
		{name: "gopls_queries", value: int64(budgets.GoplsQueries)},
		{name: "frontier_expansions", value: int64(budgets.FrontierExpansions)},
		{name: "local_wall_millis", value: budgets.LocalWallMillis, bound: budgets.TimeLimitBound},
	}
	for _, row := range rows {
		fmt.Fprintf(builder, "| %s | %d | %t |\n", row.name, row.value, row.bound)
	}
}

func renderTraceLimits(builder *strings.Builder, limits []RetrievalLimit) {
	items := append([]RetrievalLimit(nil), limits...)
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	builder.WriteString("\n## Limit-caused loss\n\n")
	if len(items) == 0 {
		builder.WriteString("No limit events were recorded.\n")
		return
	}
	builder.WriteString("| Limit | Bound | Observed | Applied | Caused loss | Exact reason |\n")
	builder.WriteString("| --- | ---: | ---: | --- | --- | --- |\n")
	for _, item := range items {
		fmt.Fprintf(
			builder,
			"| %s | %d | %d | %t | %t | %s |\n",
			markdownCell(item.Name),
			item.Limit,
			item.Observed,
			item.Applied,
			item.CausedLoss,
			markdownCell(item.LossReason),
		)
	}
}

func formatScoreComponents(components []RetrievalScoreComponent) string {
	items := append([]RetrievalScoreComponent(nil), components...)
	sort.Slice(items, func(i, j int) bool { return items[i].Kind < items[j].Kind })
	values := make([]string, 0, len(items))
	for _, item := range items {
		value := fmt.Sprintf("%s=%+d", item.Kind, item.Value)
		if item.Detail != "" {
			value += " (" + item.Detail + ")"
		}
		values = append(values, value)
	}
	return strings.Join(values, "; ")
}

func verificationAuthorityRank(authority VerificationAuthority) int {
	for index, candidate := range DefaultVerificationAuthorityOrder() {
		if authority == candidate {
			return index
		}
	}
	return len(DefaultVerificationAuthorityOrder())
}

func markdownCell(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"|", "\\|",
		"\n", "<br>",
	)
	return replacer.Replace(value)
}
