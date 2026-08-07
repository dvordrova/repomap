package tasklens

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/secretscan"
)

const SynthesisThinkingProfile = "high"

type SynthesisPrompt struct {
	Version         string
	ThinkingProfile string
	System          string
	User            string
}

// BuildSynthesisPrompt creates the single compact semantic editing request.
// The provider sees only PromptBundle: bounded excerpts, exact opaque IDs, and
// no checkout basename, raw file tree, or global edge list.
func BuildSynthesisPrompt(bundle Bundle) (SynthesisPrompt, error) {
	if err := bundle.Validate(); err != nil {
		return SynthesisPrompt{}, err
	}
	projected, err := json.Marshal(bundle.PromptBundle())
	if err != nil {
		return SynthesisPrompt{}, fmt.Errorf("task lens: encode synthesis bundle: %w", err)
	}
	if kind, found := secretscan.Detect(string(projected)); found {
		return SynthesisPrompt{}, fmt.Errorf("task lens: synthesis bundle rejected because %s was detected", kind)
	}
	system := `You are a senior software engineer editing one bounded repository investigation pack. Use only the supplied task and local evidence bundle. The task describes a symptom or desired outcome; it is not repository truth. Return one JSON object only. Do not propose a patch and do not claim to have executed code.`
	user := `Organize the bounded evidence into one task-first investigation proposal.

Return exactly this JSON shape (all fields are required unless marked optional):
{
  "task_interpretation": {
    "restatement": "concise task restatement",
    "task_kind": "bug | feature | extension | configuration | operational | compatibility | unknown"
  },
  "likely_areas": [
    {"label": "1-3 area label", "why": "bounded reason", "target_ids": ["anchor-id"]}
  ],
  "anchors": [
    {"anchor_id": "exact supplied anchor id", "why": "task relevance"}
  ],
  "evidence_joins": [
    {"left_anchor_id": "selected anchor id", "right_anchor_id": "selected anchor id", "relation_id": "supplied relation id only for locally_observed, otherwise omit", "support_type": "locally_observed | document_supported | model_hypothesis | unresolved", "support_ids": ["exact evidence ids"], "explanation": "bounded connection", "scope_non_guarantees": "what this evidence does not prove"}
  ],
  "working_hypothesis": [
    {"status": "supported | plausible | unresolved", "text": "one clause", "support_ids": ["exact evidence ids"], "relation_ids": ["exact supplied relation ids"]}
  ],
  "reproduce_or_observe": [
    {"text": "grounded step or exact missing evidence", "authority": "task_provided | repository_document | repository_test_or_example | repository_observation | missing_evidence", "evidence_ids": ["exact evidence ids, empty only for missing_evidence"]}
  ],
  "verify": {
    "steps": [
      {"text": "grounded verification or exact missing evidence", "authority": "task_provided | repository_document | repository_test_or_example | repository_observation | missing_evidence", "evidence_ids": ["exact evidence ids, empty only for missing_evidence"]}
    ]
  },
  "next_probes": [
    {"action": "inspect_symbol | resolve_reference | compare_config_copies | inspect_fixture | inspect_sibling_implementation | search_task_terms", "anchor_ids": ["one or two selected anchor ids; empty only for search_task_terms"], "text": "one concrete bounded action"}
  ]
}

Semantic decisions:
- Select 3-8 anchors when that many are supplied. A sparse partial bundle may supply only 1-2; select from the supplied anchors and do not invent replacements. A zero-anchor bundle is a local insufficient-evidence exit and is never sent for synthesis. Select only exact supplied opaque IDs. Do not invent paths, symbols, IDs, commands, endpoints, configuration values, or tests.
- Every selected anchor must have an exact task-relevance reason: a meaningful supplied task term in its evidence, a required or supporting role witness, membership in the strong local component containing decisive_relation_id, or an exact verification_frontier item. Generic auxiliary anchors have no lane; do not select them unless the bundle supplies nothing else.
- Stay compact: at most 6 evidence joins, 3 hypothesis clauses, 4 reproduction/observation steps, and 4 verification steps. Keep each prose field to one or two short sentences.
- likely_areas.target_ids and next_probes.anchor_ids must name selected anchors.
- Preserve fact types. task_provided supports only what the task says. document_claim is not runtime proof. model inference is not locally observed.
- A locally_observed join must select one supplied local relation ID by relation_id; the backend restores its endpoints, relation_kind, complete evidence set and scope from the local relation record. A non-empty subset of that relation's evidence IDs may also be supplied in support_ids. Allowed supplied relation kinds are direct_call, field_copy, field_read, field_write, error_created, error_mapped, error_exposed, value_transformed, type_name_generated, config_applied, script_invokes, test_exercises, fixture_records, documented_uses, shared_state_alias, and scope_unknown. These describe exact retained syntax only; they do not prove runtime reachability, order, or callee behavior.
- document_supported is only for evidence whose kind is exactly document_claim. A source-to-test comparison without a supplied matching local relation is not document-supported; label it model_hypothesis. A model_hypothesis explanation must name both endpoint symbols or their full supplied paths and cite each endpoint's exact evidence. Unresolved must stay visibly unresolved.
- Mark a hypothesis supported only for facts directly shown by repository evidence. Any runtime sequence or causal claim needs a supplied local relation; otherwise label it plausible or unresolved.
- A plausible hypothesis must state the concrete gap and name every cited anchor. Do not emit a generic statement that two anchors may be related.
- When a hypothesis includes relation_ids, the backend completes its support_ids from the exact local relation evidence; supply the relation_ids and any additional support_ids you choose, all from the selected anchors' evidence.
- Each anchor's source_scope is authoritative. A complete_enclosing_symbol, complete_document_section, or complete_file may support bounded absence only inside that declared scope. matched_fragments and partial_window never support an absence claim. Do not turn task_matches_outside_window or any truncated scope into a complete-file conclusion.
- The supplied verification_frontier is immutable and distinguishes exact_existing_test, exact_generated_fixture, exact_example, documented_command, proposed_test_location, and missing_evidence. Reproduce/observe and verify may use only task-provided steps, the exact frontier, repository docs, exact repository source/configuration observations, or missing_evidence. task_provided must cite the exact evidence whose kind is task_provided. Every repository evidence ID used by guidance must belong to one of the selected anchors. repository_observation must cite repository_fact evidence. Never invent a command. proposed_test_location and missing_evidence are not historical test evidence and must not use repository_test_or_example authority.
- Use exact full paths only if a path appears in allowed_paths. Prefer talking through anchor IDs and symbols.
- Return 1-3 next probes, never a vague request to read the codebase.
- The retained subset is bounded. Do not claim omitted files or relations are absent.

The backend restores mechanically derivable details: anchor roles, the exact observable effect for verification, missing-evidence presentation, and the complete evidence closure of cited relations.

Bounded task evidence JSON:
` + string(projected)
	return SynthesisPrompt{
		Version: PromptVersion, ThinkingProfile: SynthesisThinkingProfile,
		System: system, User: user,
	}, nil
}

// LocalProposal supplies a deterministic useful partial result when the task
// is offline or the one semantic editing attempt is rejected. It deliberately
// avoids causal synthesis beyond exact retained relations.
func LocalProposal(bundle Bundle) (Proposal, error) {
	if err := bundle.Validate(); err != nil {
		return Proposal{}, err
	}
	selected := localProposalAnchors(bundle)
	visibleCount := len(selected)
	proposal := Proposal{
		Version: ProposalVersion,
		Interpretation: ProposedInterpretation{
			Restatement: localRestatement(bundle.Task.Text), Kind: bundle.KindHint,
			Observable: bundle.ObservableHint,
		},
		Verify: ProposedVerification{
			Effect: localVerificationEffectForBundle(bundle),
		},
	}
	if visibleCount == 0 {
		proposal.Hypothesis = []ProposedClause{{
			Status: HypothesisUnresolved,
			Text:   "No tracked source or document anchor matched the bounded exact-term retrieval; no repository mechanism is asserted.",
		}}
		if TaskProvidesConcreteReproductionOrObservation(bundle.Task.Text) {
			proposal.ReproduceOrObserve = []ProposedGuidance{{
				Text:        "Use only the reproduction or observation supplied by the task.",
				Authority:   AuthorityTaskProvided,
				EvidenceIDs: []string{bundle.Task.EvidenceID},
			}}
		} else {
			proposal.ReproduceOrObserve = []ProposedGuidance{{
				Text:      "No concrete reproduction or repository observation was retained.",
				Authority: AuthorityMissing,
			}}
		}
		proposal.Verify.Steps = []ProposedGuidance{{
			Text:      "No repository-owned verification anchor was retained.",
			Authority: AuthorityMissing,
		}}
		proposal.NextProbes = []ProposedProbe{{
			Action:    ProbeSearchTaskTerms,
			AnchorIDs: []string{},
			Text:      "Search one exact task term within the bounded tracked repository.",
		}}
		return proposal, nil
	}
	selectedIDs := make(map[string]struct{}, len(selected))
	for _, anchor := range selected {
		selectedIDs[anchor.ID] = struct{}{}
		role := preferredContractRole(anchor, bundle.RoleContract)
		proposal.Anchors = append(proposal.Anchors, ProposedAnchor{
			AnchorID: anchor.ID, Role: role,
			Why: fmt.Sprintf("This exact retained %s excerpt contains task-relevant terms.", sourceKind(anchor.Path)),
		})
	}

	type areaGroup struct {
		label string
		ids   []string
	}
	groups := make(map[string]*areaGroup)
	var groupOrder []string
	for _, anchor := range selected {
		directory := path.Dir(anchor.Path)
		if directory == "." {
			directory = "repository root"
		}
		group := groups[directory]
		if group == nil {
			group = &areaGroup{label: directory}
			groups[directory] = group
			groupOrder = append(groupOrder, directory)
		}
		group.ids = append(group.ids, anchor.ID)
	}
	for _, key := range groupOrder {
		group := groups[key]
		proposal.Areas = append(proposal.Areas, ProposedArea{
			Label: group.label, Why: "Contains retained anchors with exact task-term overlap.",
			TargetIDs: group.ids,
		})
		if len(proposal.Areas) == 3 {
			break
		}
	}

	localRelations := rankedLocalProposalRelations(bundle, selectedIDs)
	for _, relation := range localRelations {
		proposal.Joins = append(proposal.Joins, ProposedJoin{
			LeftID: relation.LeftID, RightID: relation.RightID, RelationID: relation.ID,
			Kind: relation.Kind, SupportType: SupportLocallyObserved,
			SupportIDs:  append([]string(nil), relation.EvidenceIDs...),
			Explanation: "The retained excerpts contain the exact identifier or task term recorded by the local relation.",
			Scope:       relation.Scope,
		})
		if len(proposal.Joins) == 3 {
			break
		}
	}
	if len(localRelations) > 0 {
		relation := localRelations[0]
		proposal.Hypothesis = []ProposedClause{{
			Status: HypothesisSupported, Text: localRelationExplanation(relation.Kind),
			SupportIDs:  append([]string(nil), relation.EvidenceIDs...),
			RelationIDs: []string{relation.ID},
		}}
	} else {
		firstEvidence := selected[0].EvidenceIDs
		proposal.Hypothesis = []ProposedClause{{
			Status:     HypothesisPlausible,
			Text:       plausibleClauseText(firstEvidence, newBundleIndex(bundle)),
			SupportIDs: append([]string(nil), firstEvidence...),
		}}
	}
	if TaskProvidesConcreteReproductionOrObservation(bundle.Task.Text) {
		proposal.ReproduceOrObserve = []ProposedGuidance{{
			Text:      "Use the reproduction or observation already supplied by the task.",
			Authority: AuthorityTaskProvided, EvidenceIDs: []string{bundle.Task.EvidenceID},
		}}
	} else if authority, evidenceID := firstLocalObservationEvidence(selected, bundle); evidenceID != "" {
		proposal.ReproduceOrObserve = []ProposedGuidance{{
			Text:      "Observe the exact retained repository evidence without treating it as executed behavior.",
			Authority: authority, EvidenceIDs: []string{evidenceID},
		}}
	} else {
		proposal.ReproduceOrObserve = []ProposedGuidance{{
			Text:      "No concrete reproduction or repository observation was retained.",
			Authority: AuthorityMissing,
		}}
	}
	proposal.Verify.Steps = []ProposedGuidance{localFrontierVerification(bundle, selectedIDs)}
	proposal.NextProbes = []ProposedProbe{{
		Action: ProbeInspectSymbol, AnchorIDs: []string{selected[0].ID},
		Text: "Inspect this exact symbol and resolve one task-relevant reference beyond the retained excerpt.",
	}}
	if len(selected) > 1 && len(proposal.NextProbes) < MaxNextProbes {
		proposal.NextProbes = append(proposal.NextProbes, ProposedProbe{
			Action: ProbeResolveReference, AnchorIDs: []string{selected[0].ID, selected[1].ID},
			Text: "Resolve whether these two exact anchors share a direct reference beyond the locally recorded evidence.",
		})
	}
	return proposal, nil
}

func preferredContractRole(anchor Anchor, contract RoleContract) AnchorRole {
	for _, group := range [][]RoleRequirement{contract.Key, contract.Supporting, contract.Optional} {
		for _, requirement := range group {
			if slices.Contains(anchor.RoleHints, requirement.Role) {
				return requirement.Role
			}
		}
	}
	if len(anchor.RoleHints) > 0 {
		return anchor.RoleHints[0]
	}
	return RoleRepresentativeImplementation
}

func rankedLocalProposalRelations(bundle Bundle, selected map[string]struct{}) []Relation {
	ranked := make([]Relation, 0, len(bundle.Relations))
	anchors := make(map[string]Anchor, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		anchors[anchor.ID] = anchor
	}
	for _, relation := range bundle.Relations {
		if _, ok := selected[relation.LeftID]; !ok {
			continue
		}
		if _, ok := selected[relation.RightID]; !ok {
			continue
		}
		ranked = append(ranked, relation)
	}
	score := func(relation Relation) int {
		kindBonus := 0
		if relation.ID == bundle.DecisiveRelationID {
			kindBonus += 10_000
		}
		switch relation.Kind {
		case relationKindDirectCall:
			kindBonus += 400
		case relationKindTestReference:
			kindBonus += 350
		case relationKindExactIdentifier:
			kindBonus += 250
		}
		return kindBonus + localProposalAnchorScore(anchors[relation.LeftID], bundle.Terms, nil) +
			localProposalAnchorScore(anchors[relation.RightID], bundle.Terms, nil)
	}
	sort.Slice(ranked, func(i, j int) bool {
		leftDecisive := ranked[i].ID == bundle.DecisiveRelationID
		rightDecisive := ranked[j].ID == bundle.DecisiveRelationID
		if leftDecisive != rightDecisive {
			return leftDecisive
		}
		left, right := score(ranked[i]), score(ranked[j])
		if left != right {
			return left > right
		}
		return ranked[i].ID < ranked[j].ID
	})
	return ranked
}

// TaskProvidesConcreteReproductionOrObservation reports whether the task text
// itself contains an actionable observation rather than only a desired plan.
// It is deterministic reducer input; task kind alone is never enough.
func TaskProvidesConcreteReproductionOrObservation(task string) bool {
	lower := strings.ToLower(strings.Join(strings.Fields(task), " "))
	for _, marker := range []string{
		"minimal reproduction:", "minimal reproduction uses", "minimal reproduction constructs",
		"minimal reproduction calls", "minimal reproduction sends", "minimal reproduction sets",
		"minimal reproduction passes", "minimal reproduction creates", "steps to reproduce:",
		"reproduction:", "reproduce by ", "triggered by", "trigger when", "fails when",
		"panics when", "panic when", "fails after", "panics after", "fails with", "panics with",
		"actual behavior", "observed behavior", "current behavior", "currently fails",
		"observed stack", "does not", "doesn't", "is ignored", "wrong status", "unexpected status",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func firstLocalObservationEvidence(selected []Anchor, bundle Bundle) (GuidanceAuthority, string) {
	evidence := make(map[string]Evidence, len(bundle.Evidence))
	for _, item := range bundle.Evidence {
		evidence[item.ID] = item
	}
	for _, anchor := range selected {
		for _, id := range anchor.EvidenceIDs {
			item, ok := evidence[id]
			if !ok {
				continue
			}
			if item.Kind == EvidenceDocumentClaim {
				return AuthorityRepositoryDocument, id
			}
			if item.Kind == EvidenceRepositoryFact {
				if IsTestOrExamplePath(anchor.Path) {
					return AuthorityRepositoryTest, id
				}
				return AuthorityRepositoryObservation, id
			}
		}
	}
	return AuthorityMissing, ""
}

func localProposalAnchors(bundle Bundle) []Anchor {
	relationBonus := make(map[string]int)
	for _, relation := range bundle.Relations {
		bonus := 10
		switch relation.Kind {
		case relationKindDirectCall:
			bonus = 200
		case relationKindTestReference:
			bonus = 180
		case relationKindExactIdentifier:
			bonus = 100
		}
		relationBonus[relation.LeftID] += bonus
		relationBonus[relation.RightID] += bonus
	}
	ranked := append([]Anchor(nil), bundle.Anchors...)
	sort.Slice(ranked, func(i, j int) bool {
		left := localProposalAnchorScore(ranked[i], bundle.Terms, relationBonus)
		right := localProposalAnchorScore(ranked[j], bundle.Terms, relationBonus)
		if left != right {
			return left > right
		}
		if ranked[i].Path != ranked[j].Path {
			return ranked[i].Path < ranked[j].Path
		}
		return ranked[i].StartLine < ranked[j].StartLine
	})
	relevantIDs := taskRelevantBundleAnchorIDs(bundle)
	relevant := make([]Anchor, 0, len(ranked))
	for _, anchor := range ranked {
		if _, ok := relevantIDs[anchor.ID]; ok {
			relevant = append(relevant, anchor)
		}
	}
	minimumVisible := min(PreferredMinVisibleAnchors, len(bundle.Anchors))
	if len(relevant) < minimumVisible {
		for _, anchor := range ranked {
			if _, ok := relevantIDs[anchor.ID]; ok {
				continue
			}
			relevant = append(relevant, anchor)
			if len(relevant) == minimumVisible {
				break
			}
		}
	}
	if len(relevant) <= MaxVisibleAnchors {
		return relevant
	}
	selected := make([]Anchor, 0, MaxVisibleAnchors)
	chosen := make(map[string]struct{})
	perFile := make(map[string]int)
	add := func(anchor Anchor, perFileLimit int) {
		if len(selected) >= MaxVisibleAnchors || perFile[anchor.Path] >= perFileLimit {
			return
		}
		if _, exists := chosen[anchor.ID]; exists {
			return
		}
		chosen[anchor.ID] = struct{}{}
		perFile[anchor.Path]++
		selected = append(selected, anchor)
	}
	for _, requirement := range bundle.RoleContract.Key {
		added := 0
		for _, anchor := range relevant {
			if !slices.Contains(anchor.RoleHints, requirement.Role) {
				continue
			}
			before := len(selected)
			add(anchor, 4)
			if len(selected) > before {
				added++
			}
			if added >= requirement.MinimumAnchors {
				break
			}
		}
	}
	// Reserve both decisive endpoints before optional verification and ranking
	// fill. Their relation is the local-complete mechanism, so file diversity
	// preferences must not crowd either endpoint out of the visible pack.
	for _, relation := range bundle.Relations {
		if relation.ID != bundle.DecisiveRelationID {
			continue
		}
		for _, anchor := range relevant {
			if anchor.ID == relation.LeftID || anchor.ID == relation.RightID {
				add(anchor, MaxVisibleAnchors)
			}
		}
		break
	}
	for _, item := range bundle.Verification.allItems() {
		for _, anchor := range relevant {
			if anchor.ID == item.AnchorID {
				add(anchor, 4)
				break
			}
		}
	}
	for _, anchor := range relevant {
		add(anchor, 1)
		if len(selected) >= 6 {
			break
		}
	}
	for _, anchor := range relevant {
		add(anchor, 3)
	}
	return selected
}

// taskRelevantBundleAnchorIDs returns the only four locally authoritative
// reasons an anchor may be visible in a sufficient investigation pack. The
// model may organize this evidence, but it cannot make a generic auxiliary
// anchor task-relevant by writing a persuasive explanation for it.
func taskRelevantBundleAnchorIDs(bundle Bundle) map[string]struct{} {
	relevant := make(map[string]struct{}, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		if anchorHasMeaningfulTaskGrounding(anchor, bundle.Terms) {
			relevant[anchor.ID] = struct{}{}
		}
	}
	for _, relation := range bundle.Relations {
		if relation.ID != bundle.DecisiveRelationID {
			continue
		}
		for anchorID := range decisiveStrongComponent(
			relation,
			bundle.Anchors,
			bundle.Relations,
		) {
			relevant[anchorID] = struct{}{}
		}
		break
	}
	for _, item := range bundle.Verification.allItems() {
		if item.AnchorID != "" && isExactVerificationAuthority(item.Authority) {
			relevant[item.AnchorID] = struct{}{}
		}
	}
	available := make(map[string]struct{}, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		available[anchor.ID] = struct{}{}
	}
	addRoleCoverageWitnesses(
		relevant,
		available,
		bundle.RoleCoverage.Key,
		bundle.RoleCoverage.Supporting,
	)
	return relevant
}

func anchorHasMeaningfulTaskGrounding(anchor Anchor, terms []Term) bool {
	for _, term := range terms {
		if term.Weight < 8 || genericVisibleAnchorTerm(term.Normalized) {
			continue
		}
		if anchorContainsExact(anchor, term.Normalized) {
			return true
		}
	}
	return false
}

func genericVisibleAnchorTerm(term string) bool {
	if genericGrepTerm(term) {
		return true
	}
	switch term {
	case "config", "configuration", "option", "options", "setting", "settings",
		"test", "tests", "example", "examples":
		return true
	default:
		return false
	}
}

func addRoleCoverageWitnesses(
	relevant map[string]struct{},
	available map[string]struct{},
	groups ...[]RoleCoverageItem,
) {
	for _, group := range groups {
		for _, item := range group {
			candidates := make([]string, 0, len(item.AnchorIDs))
			for _, anchorID := range item.AnchorIDs {
				if _, exists := available[anchorID]; exists {
					candidates = append(candidates, anchorID)
				}
			}
			sort.Slice(candidates, func(i, j int) bool {
				_, leftRelevant := relevant[candidates[i]]
				_, rightRelevant := relevant[candidates[j]]
				if leftRelevant != rightRelevant {
					return leftRelevant
				}
				return candidates[i] < candidates[j]
			})
			for index := 0; index < min(item.MinimumAnchors, len(candidates)); index++ {
				relevant[candidates[index]] = struct{}{}
			}
		}
	}
}

func localFrontierVerification(bundle Bundle, selected map[string]struct{}) ProposedGuidance {
	for _, item := range bundle.Verification.allItems() {
		if item.AnchorID != "" {
			if _, ok := selected[item.AnchorID]; !ok {
				continue
			}
		}
		switch item.Authority {
		case VerificationExactExistingTest, VerificationExactGeneratedFixture, VerificationExactExample:
			return ProposedGuidance{
				Text:      "Use the exact retained repository test, example, or generated fixture as the verification anchor.",
				Authority: AuthorityRepositoryTest, EvidenceIDs: append([]string(nil), item.EvidenceIDs...),
			}
		case VerificationDocumentedCommand:
			authority := AuthorityRepositoryObservation
			for _, evidenceID := range item.EvidenceIDs {
				for _, evidence := range bundle.Evidence {
					if evidence.ID == evidenceID && evidence.Kind == EvidenceDocumentClaim {
						authority = AuthorityRepositoryDocument
					}
				}
			}
			return ProposedGuidance{
				Text:      "Use the exact retained repository-owned documented command or expected effect.",
				Authority: authority, EvidenceIDs: append([]string(nil), item.EvidenceIDs...),
			}
		}
	}
	return ProposedGuidance{
		Text:      "No exact repository-owned verification anchor or effect was retained; obtain that evidence before choosing a command.",
		Authority: AuthorityMissing,
	}
}

func localProposalAnchorScore(anchor Anchor, terms []Term, relationBonus map[string]int) int {
	score := anchor.Score + relationBonus[anchor.ID]
	lowerSymbol := strings.ToLower(anchor.Symbol + " " + anchor.Section)
	lowerPath := strings.ToLower(anchor.Path)
	for _, term := range terms {
		if term.Weight < 8 {
			continue
		}
		if strings.Contains(lowerSymbol, term.Normalized) {
			score += term.Weight * 12
		}
		if strings.Contains(lowerPath, term.Normalized) {
			score += term.Weight * 6
		}
	}
	for _, role := range anchor.RoleHints {
		switch role {
		case RoleConfigurationCopy, RoleErrorMapping, RoleIntegrationBoundary:
			score += 80
		case RoleConfigurationSource, RoleStateMutation, RoleVerificationAnchor:
			score += 40
		}
	}
	return score
}

func firstTestEvidence(anchors []Anchor, terms []Term, relations []Relation) string {
	selected := make(map[string]Anchor, len(anchors))
	for _, anchor := range anchors {
		selected[anchor.ID] = anchor
	}
	relationBonus := make(map[string]int)
	for _, relation := range relations {
		left, leftOK := selected[relation.LeftID]
		right, rightOK := selected[relation.RightID]
		if !leftOK || !rightOK || isTestPath(left.Path) == isTestPath(right.Path) {
			continue
		}
		testID := relation.LeftID
		if !isTestPath(left.Path) {
			testID = relation.RightID
		}
		switch relation.Kind {
		case relationKindDirectCall:
			relationBonus[testID] += 200
		case relationKindTestReference:
			relationBonus[testID] += 180
		case relationKindExactIdentifier:
			relationBonus[testID] += 100
		case relationKindSharedTaskTerm:
			relationBonus[testID] += 10
		}
	}
	bestEvidence, bestPath := "", ""
	bestScore := -1
	for _, anchor := range anchors {
		if !isTestPath(anchor.Path) || len(anchor.EvidenceIDs) == 0 {
			continue
		}
		score := anchor.Score + relationBonus[anchor.ID]
		for _, term := range terms {
			if term.Weight < 8 {
				continue
			}
			if containsExactTerm(anchor.Symbol, term.Normalized) ||
				containsExactTerm(anchor.Section, term.Normalized) {
				score += term.Weight * 8
			} else if strings.Contains(strings.ToLower(anchor.Symbol), term.Normalized) {
				// Ranking recognizes an exact task identifier embedded in a
				// conventional TestFoo name; authority remains the exact excerpt.
				score += term.Weight * 12
			}
			if strings.Contains(strings.ToLower(anchor.Path), term.Normalized) {
				score += term.Weight * 8
			}
		}
		if score > bestScore || score == bestScore && (bestPath == "" || anchor.Path < bestPath) {
			bestEvidence, bestPath, bestScore = anchor.EvidenceIDs[0], anchor.Path, score
		}
	}
	return bestEvidence
}

func sourceKind(filePath string) string {
	if isDocumentPath(filePath) {
		return "document"
	}
	return "source"
}

func localRestatement(task string) string {
	value := strings.Join(strings.Fields(task), " ")
	if separator := strings.IndexAny(value, ".!?"); separator >= 0 && separator < 600 {
		value = value[:separator+1]
	}
	if len(value) > 700 {
		value = truncateUTF8(value, 700)
	}
	return value
}

// StablePromptJSON is used by freeze manifests and regressions to bind the
// exact semantic prompt independently of provider envelope fields.
func StablePromptJSON(bundle Bundle) ([]byte, error) {
	prompt, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		return nil, err
	}
	value := struct {
		Version         string `json:"version"`
		ThinkingProfile string `json:"thinking_profile"`
		System          string `json:"system"`
		User            string `json:"user"`
	}{prompt.Version, prompt.ThinkingProfile, prompt.System, prompt.User}
	return json.Marshal(value)
}

func sortedAnchorIDs(anchors []Anchor) []string {
	result := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		result = append(result, anchor.ID)
	}
	sort.Strings(result)
	return result
}
