package guidedtour

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	LeafTaskVersion      = 1
	LeafArtifactVersion  = 3
	FanInArtifactVersion = 2
	MaxLeafTasks         = 6

	LeafPromptVersion  = "guided-tour-leaf-json-v7"
	FanInPromptVersion = "guided-tour-fan-in-json-v7"

	maxCandidateLeafTasks  = 3
	maxComponentLeafTasks  = 3
	maxLeafObservations    = 6
	maxLeafMissingEvidence = 6
	maxLeafTaskBytes       = 1 << 20
	maxLeafArtifactBytes   = 32 << 10
	maxFanInArtifactBytes  = 96 << 10
	// The provider request ceiling is 256 KiB. Keep enough headroom for the
	// stable instructions and for escaping this JSON inside the outer chat JSON.
	maxFanInPayloadBytes = 96 << 10
)

const (
	leafPromptSystem     = `You edit one independently bounded guided-tour leaf. The supplied task contains the direct local beats, components, gaps, and evidence for this leaf. Local facts and opaque IDs remain authoritative. Return valid JSON only. Never create or quote a repository path, file name, symbol, relation, transition, evidence, certainty, runtime order, or ID.`
	leafPromptUserPrefix = `Explain only atomic observations supported by this leaf's direct local facts and return exactly this JSON shape:
{
  "version": 3,
  "task_id": "the exact supplied task id",
  "candidate_id": "the exact supplied candidate id",
  "observations": [
    {"explanation": "one atomic locally supported observation", "support_ids": ["exact beat id"]}
  ],
  "candidate_connection": {
    "candidate_id": "the exact supplied candidate id",
    "target_id": "one exact component or beat id from this task",
    "relation": "needs_combination",
    "explanation": "why fan-in must combine this local result",
    "support_ids": ["exact beat ids used by observations"]
  },
  "missing_evidence": [
    {"explanation": "what the supplied facts do not establish", "beat_ids": ["exact beat ids"], "gap_ids": ["exact gap ids"]}
  ]
}

Rules:
- observations contains zero to six affirmative direct facts. Each explanation must be supported by a non-empty exact subset of this task's beat IDs in support_ids. Never put a negation, limitation, unknown, modal, weak, ambiguous, inconclusive, or insufficient-evidence statement in observations; put it in missing_evidence.
- Do not issue a global supported or insufficient verdict. That decision belongs only to fan-in.
- candidate_connection is required. candidate_id must exactly match the task, target_id must be an exact local component or beat ID, relation must be needs_combination, and support_ids must exactly equal the union of observation support_ids.
- missing_evidence contains zero to six meaningful limitations. Every item needs a non-empty exact combination of beat_ids and gap_ids from this task.
- At least one observation or one missing_evidence item is required. A missing-only result is useful and must not be upgraded into a supported observation.
- Never pretend that labels, file names, component descriptions, or other weak facts prove behavior.
- For suggested_direction, observations may state only affirmative non-behavioral static facts. Never narrate execution or behavioral transitions.
- Connection and missing-evidence explanations must remain hedged; they describe combination needs or facts not established.
- Do not repeat an ID and do not emit any repository-bearing field.
- Do not mention paths, file names, symbols, or source locations in prose. The local reducer removes any path-like token before an artifact can be accepted, and final story prose has the same prohibition.
- Static evidence is not observed runtime behavior. Gaps remain unresolved.`
	leafPromptTaskMarker     = "\n\nVariable canonical leaf task JSON with direct local evidence:\n"
	fanInPromptPayloadMarker = "\n\nValidated leaf artifacts plus independent compact LOCAL fact index JSON:\n"

	neutralSavedTraceName       = "Saved trace facts"
	neutralDirectionName        = "Suggested direction facts"
	neutralCandidateTrigger     = "Use the exact supplied beats and evidence"
	neutralCandidateSummary     = "No semantic candidate summary is supplied"
	neutralComponentName        = "Referenced component"
	neutralComponentDescription = "Exact component membership only; behavior is not established"
	neutralDirectionFileDetail  = "Exact bounded file evidence; behavior and runtime order are not established"
	neutralDirectionGapLabel    = "Runtime order not established"
	neutralDirectionGapDetail   = "Runtime order is not established by supplied exact evidence"
)

var leafObservationLimitationPattern = regexp.MustCompile(
	`(?i)\b(` +
		`missing|lacks?|unclear|unresolved|uncertain|incomplete|limitation|fails?|absent|` +
		`unsupported|undemonstrated|without|inadequate(?:ly)?|weak(?:er|ly)?|ambiguous|` +
		`inconclusive|partial(?:ly)?|limited|tentative(?:ly)?|provisional(?:ly)?|` +
		`possible|possibly|potential(?:ly)?|probable|probably|likely|unlikely|apparently|` +
		`appears?|seems?|suggests?|indicates?|implies?|presum(?:e[sd]?|ed|ably)|` +
		`assum(?:e[sd]?|ed|ption)|infer(?:s|red|ence)?|estimat(?:e[sd]?|ed)|` +
		`may|might|could|would|should|can|must|will|shall|ought|perhaps|maybe` +
		`)\b`,
)

type LeafKind string

const (
	LeafFlow      LeafKind = "flow"
	LeafMechanism LeafKind = "mechanism"
	LeafComponent LeafKind = "component"
)

type LeafConnectionRelation string

const LeafConnectionNeedsCombination LeafConnectionRelation = "needs_combination"

type FanInVerdict string

const (
	FanInVerdictSupported            FanInVerdict = "supported"
	FanInVerdictMixed                FanInVerdict = "mixed"
	FanInVerdictInsufficientEvidence FanInVerdict = "insufficient_evidence"
)

// LeafTask is one independently cacheable, locally projected editorial task.
// Candidate and Components contain the complete repository-bearing input.
type LeafTask struct {
	Version          int         `json:"version"`
	ID               string      `json:"id"`
	Kind             LeafKind    `json:"kind"`
	CandidateID      string      `json:"candidate_id"`
	FocusComponentID string      `json:"focus_component_id,omitempty"`
	Candidate        Candidate   `json:"candidate"`
	Components       []Component `json:"components"`
}

// LeafArtifact is the exact leaf response contract. It has no place for a
// model-produced repository reference.
type LeafArtifact struct {
	Version             int                     `json:"version"`
	TaskID              string                  `json:"task_id"`
	CandidateID         string                  `json:"candidate_id"`
	Observations        []LeafObservation       `json:"observations"`
	CandidateConnection LeafCandidateConnection `json:"candidate_connection"`
	MissingEvidence     []LeafMissingEvidence   `json:"missing_evidence"`
}

type LeafObservation struct {
	Explanation string   `json:"explanation"`
	SupportIDs  []string `json:"support_ids"`
}

type LeafCandidateConnection struct {
	CandidateID string                 `json:"candidate_id"`
	TargetID    string                 `json:"target_id"`
	Relation    LeafConnectionRelation `json:"relation"`
	Explanation string                 `json:"explanation"`
	SupportIDs  []string               `json:"support_ids"`
}

type LeafMissingEvidence struct {
	Explanation string   `json:"explanation"`
	BeatIDs     []string `json:"beat_ids"`
	GapIDs      []string `json:"gap_ids"`
}

type LeafResult struct {
	Task     LeafTask     `json:"task"`
	Artifact LeafArtifact `json:"artifact"`
}

type FanInArtifact struct {
	Version     int                `json:"version"`
	Verdict     FanInVerdict       `json:"verdict"`
	Explanation string             `json:"explanation"`
	Proposal    *Proposal          `json:"proposal"`
	StepSupport []FanInStepSupport `json:"step_support"`
}

type FanInStepSupport struct {
	StepIndex int                   `json:"step_index"`
	Refs      []FanInObservationRef `json:"refs"`
}

type FanInObservationRef struct {
	TaskID           string `json:"task_id"`
	ObservationIndex int    `json:"observation_index"`
}

// PlanLeafTasks builds a deterministic prefix-bounded fan-out plan. Candidate
// leaves always precede component leaves.
func PlanLeafTasks(bundle Bundle, maxTasks int) ([]LeafTask, error) {
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	if maxTasks <= 0 {
		return nil, fmt.Errorf("guided tour: leaf task limit must be positive")
	}
	limit := min(maxTasks, MaxLeafTasks)
	canonical := canonicalBundle(bundle)
	components := componentIndex(canonical.Components)
	tasks := make([]LeafTask, 0, limit)

	candidateCount := 0
	for _, candidate := range canonical.Candidates {
		if len(tasks) == limit || candidateCount == maxCandidateLeafTasks {
			break
		}
		kind := LeafMechanism
		if candidate.Kind == CandidateSavedTrace {
			kind = LeafFlow
		}
		task := newLeafTask(kind, candidate, "", components)
		if err := task.Validate(); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
		candidateCount++
	}

	componentCount := 0
	for _, candidate := range canonical.Candidates {
		if len(tasks) == limit || componentCount == maxComponentLeafTasks {
			break
		}
		for _, componentID := range referencedComponentIDs(candidate.Beats) {
			if len(tasks) == limit || componentCount == maxComponentLeafTasks {
				break
			}
			projected := candidate
			projected.Beats = make([]Beat, 0, len(candidate.Beats))
			for _, beat := range candidate.Beats {
				if containsString(beat.ComponentIDs, componentID) {
					projected.Beats = append(projected.Beats, canonicalBeat(beat))
				}
			}
			projected.Gaps = []Gap{}
			task := newLeafTask(LeafComponent, projected, componentID, components)
			if err := task.Validate(); err != nil {
				return nil, err
			}
			tasks = append(tasks, task)
			componentCount++
		}
	}
	return tasks, nil
}

func (task LeafTask) Validate() error {
	if task.Version != LeafTaskVersion {
		return fmt.Errorf("guided tour: unsupported leaf task version %d", task.Version)
	}
	if err := validateOpaque("leaf task id", task.ID); err != nil {
		return fmt.Errorf("guided tour: %w", err)
	}
	if err := validateOpaque("leaf candidate id", task.CandidateID); err != nil {
		return fmt.Errorf("guided tour: %w", err)
	}
	if task.Candidate.ID != task.CandidateID {
		return fmt.Errorf("guided tour: leaf candidate id does not match projected candidate")
	}
	if task.ID != leafTaskID(task.Kind, task.CandidateID, task.FocusComponentID) {
		return fmt.Errorf("guided tour: leaf task id does not match its local identity")
	}

	components := make(map[string]Component, len(task.Components))
	for index, component := range task.Components {
		if err := validateComponent(component); err != nil {
			return fmt.Errorf("guided tour: leaf components[%d]: %w", index, err)
		}
		if _, duplicate := components[component.ID]; duplicate {
			return fmt.Errorf("guided tour: duplicate leaf component id %q", component.ID)
		}
		components[component.ID] = component
	}
	if err := validateProjectedCandidate(task.Candidate, components); err != nil {
		return err
	}
	expectedComponents := referencedComponentIDs(task.Candidate.Beats)
	actualComponents := make([]string, 0, len(task.Components))
	for _, component := range task.Components {
		actualComponents = append(actualComponents, component.ID)
	}
	sort.Strings(actualComponents)
	if !equalStrings(actualComponents, expectedComponents) {
		return fmt.Errorf("guided tour: leaf components do not exactly match projected beats")
	}

	switch task.Kind {
	case LeafFlow:
		if task.FocusComponentID != "" || task.Candidate.Kind != CandidateSavedTrace || len(task.Candidate.Beats) < 3 {
			return fmt.Errorf("guided tour: flow leaf is inconsistent with its saved trace")
		}
	case LeafMechanism:
		if task.FocusComponentID != "" || task.Candidate.Kind != CandidateSuggestedDirection || len(task.Candidate.Beats) < 3 {
			return fmt.Errorf("guided tour: mechanism leaf is inconsistent with its suggested direction")
		}
	case LeafComponent:
		if err := validateOpaque("focus component id", task.FocusComponentID); err != nil {
			return fmt.Errorf("guided tour: %w", err)
		}
		if _, exists := components[task.FocusComponentID]; !exists {
			return fmt.Errorf("guided tour: component leaf focus is not in projected components")
		}
		if len(task.Candidate.Gaps) != 0 {
			return fmt.Errorf("guided tour: component leaf cannot infer candidate gaps")
		}
		for _, beat := range task.Candidate.Beats {
			if !containsString(beat.ComponentIDs, task.FocusComponentID) {
				return fmt.Errorf("guided tour: component leaf contains an unrelated beat")
			}
		}
	default:
		return fmt.Errorf("guided tour: unsupported leaf kind %q", task.Kind)
	}
	return nil
}

// LeafTaskHash returns the canonical JSON used for independent task cache
// identity together with its SHA-256 digest.
func LeafTaskHash(task LeafTask) (string, []byte, error) {
	if err := task.Validate(); err != nil {
		return "", nil, err
	}
	canonical := canonicalLeafTask(task)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", nil, fmt.Errorf("guided tour: encode canonical leaf task: %w", err)
	}
	if len(encoded) > maxLeafTaskBytes {
		return "", nil, fmt.Errorf("guided tour: canonical leaf task is too large")
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), encoded, nil
}

func BuildLeafPrompt(task LeafTask) (Prompt, error) {
	_, encoded, err := LeafTaskHash(task)
	if err != nil {
		return Prompt{}, err
	}
	return Prompt{
		Version: LeafPromptVersion,
		System:  leafPromptSystem,
		User:    leafPromptUserPrefix + leafPromptTaskMarker + string(encoded),
	}, nil
}

func ParseLeafArtifact(raw []byte) (LeafArtifact, error) {
	if len(raw) == 0 || len(raw) > maxLeafArtifactBytes {
		return LeafArtifact{}, fmt.Errorf("guided tour: leaf artifact is empty or too large")
	}
	var artifact LeafArtifact
	if err := decodeStrictJSON(raw, &artifact); err != nil {
		return LeafArtifact{}, fmt.Errorf("guided tour: invalid leaf artifact json: %w", err)
	}
	return artifact, nil
}

// NormalizeLeafArtifact removes repository-bearing path tokens from untrusted
// model prose before validation or persistence. Opaque support IDs and the
// response shape are unchanged.
func NormalizeLeafArtifact(artifact LeafArtifact) LeafArtifact {
	result := artifact
	result.Observations = append([]LeafObservation{}, artifact.Observations...)
	for index := range result.Observations {
		result.Observations[index].SupportIDs = append(
			[]string{},
			artifact.Observations[index].SupportIDs...,
		)
		result.Observations[index].Explanation = normalizeLeafProse(
			artifact.Observations[index].Explanation,
		)
	}
	result.CandidateConnection.SupportIDs = append(
		[]string{},
		artifact.CandidateConnection.SupportIDs...,
	)
	result.CandidateConnection.Explanation = normalizeLeafProse(
		artifact.CandidateConnection.Explanation,
	)
	result.MissingEvidence = append([]LeafMissingEvidence{}, artifact.MissingEvidence...)
	for index := range result.MissingEvidence {
		result.MissingEvidence[index].BeatIDs = append(
			[]string{},
			artifact.MissingEvidence[index].BeatIDs...,
		)
		result.MissingEvidence[index].GapIDs = append(
			[]string{},
			artifact.MissingEvidence[index].GapIDs...,
		)
		result.MissingEvidence[index].Explanation = normalizeLeafProse(
			artifact.MissingEvidence[index].Explanation,
		)
	}
	return result
}

func normalizeLeafProse(value string) string {
	value = repositoryReferencePattern.ReplaceAllString(
		value,
		"the supplied repository reference",
	)
	value = strings.NewReplacer(`/`, " ", `\`, " ").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func ValidateLeafArtifact(task LeafTask, artifact LeafArtifact) error {
	if err := task.Validate(); err != nil {
		return err
	}
	if artifact.Version != LeafArtifactVersion {
		return fmt.Errorf("guided tour: unsupported leaf artifact version %d", artifact.Version)
	}
	if artifact.TaskID != task.ID {
		return fmt.Errorf("guided tour: leaf artifact task id does not match")
	}
	if artifact.CandidateID != task.CandidateID {
		return fmt.Errorf("guided tour: leaf artifact candidate id does not match")
	}
	if len(artifact.Observations) > maxLeafObservations {
		return fmt.Errorf("guided tour: leaf artifact has too many observations")
	}
	if len(artifact.MissingEvidence) > maxLeafMissingEvidence {
		return fmt.Errorf("guided tour: leaf artifact has too many missing-evidence items")
	}
	if len(artifact.Observations) == 0 && len(artifact.MissingEvidence) == 0 {
		return fmt.Errorf("guided tour: leaf artifact has no usable partial result")
	}
	knownBeats := make(map[string]struct{}, len(task.Candidate.Beats))
	for _, beat := range task.Candidate.Beats {
		knownBeats[beat.ID] = struct{}{}
	}
	knownGaps := make(map[string]struct{}, len(task.Candidate.Gaps))
	for _, gap := range task.Candidate.Gaps {
		knownGaps[gap.ID] = struct{}{}
	}
	knownTargets := make(map[string]struct{}, len(knownBeats)+len(task.Components))
	for id := range knownBeats {
		knownTargets[id] = struct{}{}
	}
	for _, component := range task.Components {
		knownTargets[component.ID] = struct{}{}
	}

	observationSupport := make(map[string]struct{})
	for index, observation := range artifact.Observations {
		field := fmt.Sprintf("leaf observations[%d] explanation", index)
		if err := validateModelProse(field, fmt.Sprintf("leaf.observations[%d].explanation", index), observation.Explanation, maxProposalExplainBytes); err != nil {
			return err
		}
		if editorialNegationPattern.MatchString(observation.Explanation) ||
			leafObservationLimitationPattern.MatchString(observation.Explanation) {
			return fmt.Errorf(
				"guided tour: %s must be an affirmative direct fact; limitations belong in missing_evidence",
				field,
			)
		}
		if task.Candidate.Kind == CandidateSuggestedDirection {
			if err := validateLeafHedgedProse(field, observation.Explanation); err != nil {
				return err
			}
		}
		if len(observation.SupportIDs) == 0 {
			return fmt.Errorf("guided tour: leaf observations[%d] has no support ids", index)
		}
		if err := validateIDList("leaf observation support ids", observation.SupportIDs); err != nil {
			return fmt.Errorf("guided tour: leaf observations[%d]: %w", index, err)
		}
		for _, id := range observation.SupportIDs {
			if _, exists := knownBeats[id]; !exists {
				return fmt.Errorf(
					"guided tour: leaf observations[%d] references unknown support id %q",
					index,
					id,
				)
			}
			observationSupport[id] = struct{}{}
		}
	}

	connection := artifact.CandidateConnection
	if connection.CandidateID != task.CandidateID {
		return fmt.Errorf("guided tour: leaf candidate connection candidate id does not match")
	}
	if err := validateOpaque("leaf candidate connection target id", connection.TargetID); err != nil {
		return fmt.Errorf("guided tour: %w", err)
	}
	if _, exists := knownTargets[connection.TargetID]; !exists {
		return fmt.Errorf(
			"guided tour: leaf candidate connection references unknown target id %q",
			connection.TargetID,
		)
	}
	if connection.Relation != LeafConnectionNeedsCombination {
		return fmt.Errorf("guided tour: leaf candidate connection relation must be needs_combination")
	}
	if err := validateModelProse(
		"leaf candidate connection explanation",
		"leaf.candidate_connection.explanation",
		connection.Explanation,
		maxProposalExplainBytes,
	); err != nil {
		return err
	}
	if err := validateIDList("leaf candidate connection support ids", connection.SupportIDs); err != nil {
		return fmt.Errorf("guided tour: %w", err)
	}
	for _, id := range connection.SupportIDs {
		if _, exists := knownBeats[id]; !exists {
			return fmt.Errorf(
				"guided tour: leaf candidate connection references unknown support id %q",
				id,
			)
		}
	}
	if !equalStrings(sortedStrings(connection.SupportIDs), sortedSet(observationSupport)) {
		return fmt.Errorf(
			"guided tour: leaf candidate connection support ids do not match observation support",
		)
	}

	for index, missing := range artifact.MissingEvidence {
		field := fmt.Sprintf("leaf missing_evidence[%d] explanation", index)
		if err := validateModelProse(field, fmt.Sprintf("leaf.missing_evidence[%d].explanation", index), missing.Explanation, maxProposalExplainBytes); err != nil {
			return err
		}
		if err := validateLeafHedgedProse(field, missing.Explanation); err != nil {
			return err
		}
		if len(missing.BeatIDs) == 0 && len(missing.GapIDs) == 0 {
			return fmt.Errorf("guided tour: leaf missing_evidence[%d] has no exact ids", index)
		}
		if err := validateIDList("leaf missing-evidence beat ids", missing.BeatIDs); err != nil {
			return fmt.Errorf("guided tour: leaf missing_evidence[%d]: %w", index, err)
		}
		for _, id := range missing.BeatIDs {
			if _, exists := knownBeats[id]; !exists {
				return fmt.Errorf(
					"guided tour: leaf missing_evidence[%d] references unknown beat id %q",
					index,
					id,
				)
			}
		}
		if err := validateIDList("leaf missing-evidence gap ids", missing.GapIDs); err != nil {
			return fmt.Errorf("guided tour: leaf missing_evidence[%d]: %w", index, err)
		}
		for _, id := range missing.GapIDs {
			if _, exists := knownGaps[id]; !exists {
				return fmt.Errorf(
					"guided tour: leaf missing_evidence[%d] references unknown gap id %q",
					index,
					id,
				)
			}
		}
	}
	return nil
}

func validateLeafHedgedProse(field, value string) error {
	for _, sentence := range editorialSentenceBoundaryPattern.Split(value, -1) {
		for _, clause := range editorialClauseBoundaryPattern.Split(sentence, -1) {
			if editorialBehaviorAssertionPattern.MatchString(clause) &&
				!editorialNegationPattern.MatchString(clause) {
				return fmt.Errorf(
					"guided tour: %s contains an unsupported behavioral assertion",
					field,
				)
			}
		}
	}
	return nil
}

// BuildFanInPrompt accepts any non-empty subset of independently valid leaf
// results. Missing-evidence-only leaves remain useful fan-in input, but only
// atomic observation support can authorize a final story proposal.
func BuildFanInPrompt(bundle Bundle, results []LeafResult) (Prompt, error) {
	validated, consideredCandidates, observationSupport, err := validateFanInResults(bundle, results)
	if err != nil {
		return Prompt{}, err
	}
	storyCandidates := make(map[string]struct{})
	for candidateID, support := range observationSupport {
		if len(support) > 0 {
			storyCandidates[candidateID] = struct{}{}
		}
	}

	payload := fanInPayload{
		Version:                3,
		BundleVersion:          bundle.Version,
		RepoName:               bundle.RepoName,
		CanvasVersion:          bundle.CanvasVersion,
		ConsideredCandidateIDs: sortedSet(consideredCandidates),
		StoryCandidateIDs:      sortedSet(storyCandidates),
		ObservationSupport:     buildFanInObservationSupport(observationSupport),
		Leaves:                 make([]fanInLeaf, 0, len(validated)),
		LocalFacts: buildLocalFactIndex(
			canonicalBundle(bundle),
			consideredCandidates,
		),
	}
	for _, result := range validated {
		payload.Leaves = append(payload.Leaves, fanInLeaf{
			TaskID: result.Task.ID, Kind: result.Task.Kind,
			CandidateID: result.Task.CandidateID, FocusComponentID: result.Task.FocusComponentID,
			Artifact: result.Artifact,
		})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Prompt{}, fmt.Errorf("guided tour: encode fan-in payload: %w", err)
	}
	if len(encoded) > maxFanInPayloadBytes {
		return Prompt{}, fmt.Errorf("guided tour: fan-in payload is too large")
	}

	system := `You make the only global evidence verdict for one guided onboarding story from independently validated atomic leaf artifacts and a compact LOCAL fact index. Leaf prose is editorial input only; LOCAL facts and opaque IDs remain authoritative. Return valid JSON only. Never create or quote repository paths, file names, symbols, relations, transitions, evidence, certainty, runtime order, or IDs.`
	user := `Return exactly this FanInArtifact JSON shape:
{
  "version": 2,
  "verdict": "supported or mixed or insufficient_evidence",
  "explanation": "concise global evidence assessment",
  "proposal": {
    "version": 1,
    "candidate_id": "one exact id from story_candidate_ids",
    "title": "concise title without paths or file names",
    "summary": "concise evidence-aware summary without paths or file names",
    "steps": [
      {"title": "concise title", "explanation": "concise explanation", "beat_ids": ["exact observation-supported beat ids"]}
    ],
    "gap_summary": [
      {"explanation": "why LOCAL gaps remain unresolved", "gap_ids": ["exact LOCAL gap ids"]}
    ]
  },
  "step_support": [
    {"step_index": 0, "refs": [{"task_id": "exact selected-candidate leaf task id", "observation_index": 0}]}
  ]
}

Rules:
- Only this fan-in response decides the global verdict. Leaf artifacts never do.
- The proposal field is mandatory. supported and mixed require a non-null proposal. insufficient_evidence requires an explicit null proposal and an empty step_support array.
- A proposal candidate_id must be in story_candidate_ids. Missing-evidence-only leaves reach fan-in but cannot authorize a story.
- Every proposal beat_id must occur in observation_support for the selected candidate. Return 3 to 6 non-empty steps and reference at least 3 distinct supported beats.
- supported and mixed require exactly one step_support entry per proposal step, in matching zero-based step_index order. Every refs item must identify an exact observation from a validated leaf for the selected candidate. Each step's beat_ids must be a subset of the union of its referenced observations' support_ids.
- Use only candidate, beat, and gap IDs in the LOCAL fact index for the selected candidate.
- Reference each beat or gap at most once. Do not add repository-bearing fields.
- Preserve nondecreasing sequence for saved_trace. Editorial order is never runtime order.
- Atomic observations may help teaching, but candidate connections always need combination and must not be upgraded into proof.
- supported is forbidden when any validated leaf for the selected candidate contains missing_evidence.
- mixed must include every exact gap_id cited by selected-candidate missing_evidence. If missing_evidence has no cited canonical gap_id that can be shown, return insufficient_evidence.
- Every known gap for the selected candidate must appear exactly once in proposal.gap_summary. Use insufficient_evidence when no honest proposal can be built.
- Do not mention .go file names, slash paths, symbols, or source locations in prose.
- For suggested_direction, write editorial inspection guidance or explicitly hedged static observations. Do not narrate execution or assert behavioral transitions; locally enforced proposal validation will reject them.

` + fanInPromptPayloadMarker + string(encoded)
	return Prompt{Version: FanInPromptVersion, System: system, User: user}, nil
}

func ParseFanInArtifact(raw []byte) (FanInArtifact, error) {
	if len(raw) == 0 || len(raw) > maxFanInArtifactBytes {
		return FanInArtifact{}, fmt.Errorf("guided tour: fan-in artifact is empty or too large")
	}
	var wire struct {
		Version     int                `json:"version"`
		Verdict     FanInVerdict       `json:"verdict"`
		Explanation string             `json:"explanation"`
		Proposal    json.RawMessage    `json:"proposal"`
		StepSupport []FanInStepSupport `json:"step_support"`
	}
	if err := decodeStrictJSON(raw, &wire); err != nil {
		return FanInArtifact{}, fmt.Errorf("guided tour: invalid fan-in artifact json: %w", err)
	}
	if wire.Proposal == nil {
		return FanInArtifact{}, fmt.Errorf("guided tour: fan-in artifact omits required proposal field")
	}
	artifact := FanInArtifact{
		Version: wire.Version, Verdict: wire.Verdict, Explanation: wire.Explanation,
		StepSupport: wire.StepSupport,
	}
	if bytes.Equal(bytes.TrimSpace(wire.Proposal), []byte("null")) {
		if wire.Verdict != FanInVerdictInsufficientEvidence {
			return FanInArtifact{}, fmt.Errorf(
				"guided tour: explicit null proposal is only valid for insufficient_evidence",
			)
		}
		return artifact, nil
	}
	var proposal Proposal
	if err := decodeStrictJSON(wire.Proposal, &proposal); err != nil {
		return FanInArtifact{}, fmt.Errorf("guided tour: invalid fan-in proposal json: %w", err)
	}
	artifact.Proposal = &proposal
	return artifact, nil
}

func ValidateFanInArtifact(bundle Bundle, results []LeafResult, artifact FanInArtifact) error {
	validatedResults, _, observationSupport, err := validateFanInResults(bundle, results)
	if err != nil {
		return err
	}
	if artifact.Version != FanInArtifactVersion {
		return fmt.Errorf("guided tour: unsupported fan-in artifact version %d", artifact.Version)
	}
	if err := validateModelProse(
		"fan-in explanation",
		"fan_in.explanation",
		artifact.Explanation,
		maxProposalExplainBytes,
	); err != nil {
		return err
	}
	switch artifact.Verdict {
	case FanInVerdictSupported, FanInVerdictMixed:
		if artifact.Proposal == nil {
			return fmt.Errorf("guided tour: %s fan-in verdict requires a proposal", artifact.Verdict)
		}
		if err := validateFanInProposalSupport(bundle, observationSupport, *artifact.Proposal); err != nil {
			return err
		}
		if err := validateFanInMissingEvidence(
			validatedResults,
			artifact.Verdict,
			*artifact.Proposal,
		); err != nil {
			return err
		}
		return validateFanInStepSupport(validatedResults, *artifact.Proposal, artifact.StepSupport)
	case FanInVerdictInsufficientEvidence:
		if artifact.Proposal != nil {
			return fmt.Errorf("guided tour: insufficient_evidence fan-in verdict requires a null proposal")
		}
		if len(artifact.StepSupport) != 0 {
			return fmt.Errorf("guided tour: insufficient_evidence fan-in verdict requires empty step_support")
		}
		return validateLeafHedgedProse("fan-in explanation", artifact.Explanation)
	default:
		return fmt.Errorf("guided tour: unsupported fan-in verdict %q", artifact.Verdict)
	}
}

func validateFanInMissingEvidence(
	results []LeafResult,
	verdict FanInVerdict,
	proposal Proposal,
) error {
	hasMissingEvidence := false
	citedGapIDs := make(map[string]struct{})
	for _, result := range results {
		if result.Task.CandidateID != proposal.CandidateID {
			continue
		}
		for _, missing := range result.Artifact.MissingEvidence {
			hasMissingEvidence = true
			addStrings(citedGapIDs, missing.GapIDs)
		}
	}
	if !hasMissingEvidence {
		return nil
	}
	if verdict == FanInVerdictSupported {
		return fmt.Errorf(
			"guided tour: supported fan-in verdict cannot discard selected-candidate missing evidence",
		)
	}
	if len(citedGapIDs) == 0 {
		return fmt.Errorf(
			"guided tour: mixed fan-in verdict cannot show missing evidence without a cited canonical gap id",
		)
	}

	shownGapIDs := make(map[string]struct{})
	for _, summary := range proposal.GapSummary {
		addStrings(shownGapIDs, summary.GapIDs)
	}
	for _, id := range sortedSet(citedGapIDs) {
		if _, shown := shownGapIDs[id]; !shown {
			return fmt.Errorf(
				"guided tour: mixed fan-in proposal omits missing-evidence gap id %q",
				id,
			)
		}
	}
	return nil
}

func validateFanInStepSupport(
	results []LeafResult,
	proposal Proposal,
	lineage []FanInStepSupport,
) error {
	if len(lineage) != len(proposal.Steps) {
		return fmt.Errorf("guided tour: fan-in step_support must contain one entry per proposal step")
	}
	tasks := make(map[string]LeafResult, len(results))
	for _, result := range results {
		tasks[result.Task.ID] = result
	}
	for stepIndex, support := range lineage {
		if support.StepIndex != stepIndex {
			return fmt.Errorf("guided tour: fan-in step_support[%d] has mismatched step_index", stepIndex)
		}
		if len(support.Refs) == 0 {
			return fmt.Errorf("guided tour: fan-in step_support[%d] has no observation refs", stepIndex)
		}
		availableBeats := make(map[string]struct{})
		seenRefs := make(map[string]struct{}, len(support.Refs))
		for refIndex, ref := range support.Refs {
			if err := validateOpaque("fan-in observation ref task id", ref.TaskID); err != nil {
				return fmt.Errorf("guided tour: fan-in step_support[%d] refs[%d]: %w", stepIndex, refIndex, err)
			}
			result, exists := tasks[ref.TaskID]
			if !exists {
				return fmt.Errorf(
					"guided tour: fan-in step_support[%d] refs[%d] references unknown task id %q",
					stepIndex,
					refIndex,
					ref.TaskID,
				)
			}
			if result.Task.CandidateID != proposal.CandidateID {
				return fmt.Errorf(
					"guided tour: fan-in step_support[%d] refs[%d] belongs to another candidate",
					stepIndex,
					refIndex,
				)
			}
			if ref.ObservationIndex < 0 || ref.ObservationIndex >= len(result.Artifact.Observations) {
				return fmt.Errorf(
					"guided tour: fan-in step_support[%d] refs[%d] has unknown observation_index",
					stepIndex,
					refIndex,
				)
			}
			key := fmt.Sprintf("%s:%d", ref.TaskID, ref.ObservationIndex)
			if _, duplicate := seenRefs[key]; duplicate {
				return fmt.Errorf("guided tour: fan-in step_support[%d] repeats observation ref %q", stepIndex, key)
			}
			seenRefs[key] = struct{}{}
			addStrings(availableBeats, result.Artifact.Observations[ref.ObservationIndex].SupportIDs)
		}
		for _, beatID := range proposal.Steps[stepIndex].BeatIDs {
			if _, supported := availableBeats[beatID]; !supported {
				return fmt.Errorf(
					"guided tour: fan-in step_support[%d] does not support proposal beat id %q",
					stepIndex,
					beatID,
				)
			}
		}
	}
	return nil
}

// ValidateFanInProposal requires every selected beat to be supported by a
// validated atomic observation for the selected candidate.
func ValidateFanInProposal(bundle Bundle, results []LeafResult, proposal Proposal) error {
	_, _, observationSupport, err := validateFanInResults(bundle, results)
	if err != nil {
		return err
	}
	return validateFanInProposalSupport(bundle, observationSupport, proposal)
}

func validateFanInProposalSupport(
	bundle Bundle,
	observationSupport map[string]map[string]struct{},
	proposal Proposal,
) error {
	if err := ValidateProposal(bundle, proposal); err != nil {
		return err
	}
	supported := observationSupport[proposal.CandidateID]
	if len(supported) == 0 {
		return fmt.Errorf("guided tour: fan-in proposal candidate id %q has no atomic observations", proposal.CandidateID)
	}
	for stepIndex, step := range proposal.Steps {
		for _, id := range step.BeatIDs {
			if _, exists := supported[id]; !exists {
				return fmt.Errorf(
					"guided tour: fan-in proposal steps[%d] beat id %q is not supported by a validated atomic observation",
					stepIndex,
					id,
				)
			}
		}
	}
	return nil
}

func validateFanInResults(
	bundle Bundle,
	results []LeafResult,
) ([]LeafResult, map[string]struct{}, map[string]map[string]struct{}, error) {
	if err := bundle.Validate(); err != nil {
		return nil, nil, nil, err
	}
	if len(results) == 0 {
		return nil, nil, nil, fmt.Errorf("guided tour: fan-in needs at least one valid leaf result")
	}
	planned, err := PlanLeafTasks(bundle, MaxLeafTasks)
	if err != nil {
		return nil, nil, nil, err
	}
	plannedHashes := make(map[string]string, len(planned))
	for _, task := range planned {
		hash, _, hashErr := LeafTaskHash(task)
		if hashErr != nil {
			return nil, nil, nil, hashErr
		}
		plannedHashes[task.ID] = hash
	}

	validated := make([]LeafResult, len(results))
	seenTasks := make(map[string]struct{}, len(results))
	consideredCandidates := make(map[string]struct{})
	observationSupport := make(map[string]map[string]struct{})
	for index, result := range results {
		hash, _, hashErr := LeafTaskHash(result.Task)
		if hashErr != nil {
			return nil, nil, nil, fmt.Errorf("guided tour: fan-in result[%d]: %w", index, hashErr)
		}
		plannedHash, exists := plannedHashes[result.Task.ID]
		if !exists || plannedHash != hash {
			return nil, nil, nil, fmt.Errorf(
				"guided tour: fan-in result[%d] is not an exact planned leaf",
				index,
			)
		}
		if _, duplicate := seenTasks[result.Task.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf(
				"guided tour: fan-in repeats leaf task id %q",
				result.Task.ID,
			)
		}
		if err := ValidateLeafArtifact(result.Task, result.Artifact); err != nil {
			return nil, nil, nil, fmt.Errorf("guided tour: fan-in result[%d]: %w", index, err)
		}
		seenTasks[result.Task.ID] = struct{}{}
		consideredCandidates[result.Task.CandidateID] = struct{}{}
		if observationSupport[result.Task.CandidateID] == nil {
			observationSupport[result.Task.CandidateID] = make(map[string]struct{})
		}
		for _, observation := range result.Artifact.Observations {
			addStrings(observationSupport[result.Task.CandidateID], observation.SupportIDs)
		}
		validated[index] = LeafResult{
			Task:     canonicalLeafTask(result.Task),
			Artifact: canonicalLeafArtifact(result.Artifact),
		}
	}
	sort.Slice(validated, func(i, j int) bool {
		return validated[i].Task.ID < validated[j].Task.ID
	})
	return validated, consideredCandidates, observationSupport, nil
}

func newLeafTask(
	kind LeafKind,
	candidate Candidate,
	focusComponentID string,
	components map[string]Component,
) LeafTask {
	candidate = projectLeafCandidate(candidate)
	ids := referencedComponentIDs(candidate.Beats)
	return LeafTask{
		Version: LeafTaskVersion, ID: leafTaskID(kind, candidate.ID, focusComponentID), Kind: kind,
		CandidateID: candidate.ID, FocusComponentID: focusComponentID, Candidate: candidate,
		Components: projectLeafComponents(ids, components),
	}
}

func leafTaskID(kind LeafKind, candidateID, focusComponentID string) string {
	identity := struct {
		Version          int      `json:"version"`
		Kind             LeafKind `json:"kind"`
		CandidateID      string   `json:"candidate_id"`
		FocusComponentID string   `json:"focus_component_id,omitempty"`
	}{
		Version: LeafTaskVersion, Kind: kind,
		CandidateID: candidateID, FocusComponentID: focusComponentID,
	}
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return "leaf-" + hex.EncodeToString(digest[:10])
}

func validateProjectedCandidate(candidate Candidate, components map[string]Component) error {
	if err := validateOpaque("projected candidate id", candidate.ID); err != nil {
		return fmt.Errorf("guided tour: %w", err)
	}
	if err := validateText("projected candidate name", candidate.Name, maxNameBytes, true); err != nil {
		return err
	}
	if err := validateText("projected candidate trigger", candidate.Trigger, maxSummaryBytes, true); err != nil {
		return err
	}
	if err := validateText("projected candidate summary", candidate.Summary, maxSummaryBytes, true); err != nil {
		return err
	}
	switch candidate.Kind {
	case CandidateSavedTrace:
		if candidate.OrderingBasis != OrderingTrace {
			return fmt.Errorf("guided tour: projected saved trace must use trace_order")
		}
	case CandidateSuggestedDirection:
		if candidate.OrderingBasis != OrderingEditorial {
			return fmt.Errorf("guided tour: projected suggested direction must use editorial ordering")
		}
	default:
		return fmt.Errorf("guided tour: unsupported projected candidate kind %q", candidate.Kind)
	}
	if len(candidate.Beats) == 0 {
		return fmt.Errorf("guided tour: projected candidate has no beats")
	}
	knownIDs := make(map[string]string, len(candidate.Beats)+len(candidate.Gaps))
	knownEvidence := make(map[string]EvidenceRef)
	for index, beat := range candidate.Beats {
		if err := validateBeat(beat, components, knownEvidence); err != nil {
			return fmt.Errorf("guided tour: projected beats[%d]: %w", index, err)
		}
		if previous, duplicate := knownIDs[beat.ID]; duplicate {
			return fmt.Errorf("guided tour: duplicate projected id %q in %s and beat", beat.ID, previous)
		}
		knownIDs[beat.ID] = "beat"
	}
	for index, gap := range candidate.Gaps {
		if err := validateGap(gap, knownEvidence); err != nil {
			return fmt.Errorf("guided tour: projected gaps[%d]: %w", index, err)
		}
		if previous, duplicate := knownIDs[gap.ID]; duplicate {
			return fmt.Errorf("guided tour: duplicate projected id %q in %s and gap", gap.ID, previous)
		}
		knownIDs[gap.ID] = "gap"
	}
	return nil
}

func canonicalLeafTask(task LeafTask) LeafTask {
	result := task
	result.Candidate = canonicalCandidate(task.Candidate)
	result.Components = append([]Component{}, task.Components...)
	sort.Slice(result.Components, func(i, j int) bool {
		return result.Components[i].ID < result.Components[j].ID
	})
	return result
}

func projectLeafCandidate(candidate Candidate) Candidate {
	result := canonicalCandidate(candidate)
	switch result.Kind {
	case CandidateSavedTrace:
		result.Name = neutralSavedTraceName
	case CandidateSuggestedDirection:
		result.Name = neutralDirectionName
		for index := range result.Beats {
			if result.Beats[index].Kind == "file" {
				result.Beats[index].Detail = neutralDirectionFileDetail
			}
		}
		for index := range result.Gaps {
			result.Gaps[index].Label = neutralDirectionGapLabel
			result.Gaps[index].Detail = neutralDirectionGapDetail
		}
	}
	result.Trigger = neutralCandidateTrigger
	result.Summary = neutralCandidateSummary
	return result
}

func projectLeafComponents(ids []string, components map[string]Component) []Component {
	result := componentsForIDs(ids, components)
	for index := range result {
		result[index].Name = neutralComponentName
		result[index].Description = neutralComponentDescription
	}
	return result
}

func canonicalLeafArtifact(artifact LeafArtifact) LeafArtifact {
	result := artifact
	result.Observations = append([]LeafObservation{}, artifact.Observations...)
	for index := range result.Observations {
		result.Observations[index].SupportIDs = sortedStrings(result.Observations[index].SupportIDs)
	}
	result.CandidateConnection.SupportIDs = sortedStrings(artifact.CandidateConnection.SupportIDs)
	result.MissingEvidence = append([]LeafMissingEvidence{}, artifact.MissingEvidence...)
	for index := range result.MissingEvidence {
		result.MissingEvidence[index].BeatIDs = sortedStrings(result.MissingEvidence[index].BeatIDs)
		result.MissingEvidence[index].GapIDs = sortedStrings(result.MissingEvidence[index].GapIDs)
	}
	return result
}

func componentIndex(components []Component) map[string]Component {
	result := make(map[string]Component, len(components))
	for _, component := range components {
		result[component.ID] = component
	}
	return result
}

func referencedComponentIDs(beats []Beat) []string {
	set := make(map[string]struct{})
	for _, beat := range beats {
		addStrings(set, beat.ComponentIDs)
	}
	return sortedSet(set)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type fanInPayload struct {
	Version                int                     `json:"version"`
	BundleVersion          int                     `json:"bundle_version"`
	RepoName               string                  `json:"repo_name"`
	CanvasVersion          int                     `json:"canvas_version"`
	ConsideredCandidateIDs []string                `json:"considered_candidate_ids"`
	StoryCandidateIDs      []string                `json:"story_candidate_ids"`
	ObservationSupport     []fanInCandidateSupport `json:"observation_support"`
	Leaves                 []fanInLeaf             `json:"validated_leaves"`
	LocalFacts             localFactIndex          `json:"local_fact_index"`
}

type fanInCandidateSupport struct {
	CandidateID string   `json:"candidate_id"`
	SupportIDs  []string `json:"support_ids"`
}

type fanInLeaf struct {
	TaskID           string       `json:"task_id"`
	Kind             LeafKind     `json:"kind"`
	CandidateID      string       `json:"candidate_id"`
	FocusComponentID string       `json:"focus_component_id,omitempty"`
	Artifact         LeafArtifact `json:"artifact"`
}

func buildFanInObservationSupport(
	observationSupport map[string]map[string]struct{},
) []fanInCandidateSupport {
	candidateIDs := make([]string, 0, len(observationSupport))
	for candidateID, support := range observationSupport {
		if len(support) > 0 {
			candidateIDs = append(candidateIDs, candidateID)
		}
	}
	sort.Strings(candidateIDs)
	result := make([]fanInCandidateSupport, 0, len(candidateIDs))
	for _, candidateID := range candidateIDs {
		result = append(result, fanInCandidateSupport{
			CandidateID: candidateID,
			SupportIDs:  sortedSet(observationSupport[candidateID]),
		})
	}
	return result
}

type localFactIndex struct {
	Candidates []localCandidateFact `json:"candidates"`
	Components []Component          `json:"components"`
}

type localCandidateFact struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Kind          CandidateKind   `json:"kind"`
	Trigger       string          `json:"trigger"`
	OrderingBasis OrderingBasis   `json:"ordering_basis"`
	Beats         []localBeatFact `json:"beats"`
	Gaps          []localGapFact  `json:"gaps"`
}

type localBeatFact struct {
	ID           string        `json:"id"`
	Kind         string        `json:"kind"`
	Label        string        `json:"label"`
	Detail       string        `json:"detail"`
	Sequence     int           `json:"sequence"`
	ComponentIDs []string      `json:"component_ids"`
	SurfaceIDs   []string      `json:"surface_ids"`
	FlowID       string        `json:"flow_id"`
	FlowStepIDs  []string      `json:"flow_step_ids"`
	Evidence     []EvidenceRef `json:"evidence"`
}

type localGapFact struct {
	ID       string        `json:"id"`
	Label    string        `json:"label"`
	Detail   string        `json:"detail"`
	Evidence []EvidenceRef `json:"evidence"`
}

func buildLocalFactIndex(bundle Bundle, selected map[string]struct{}) localFactIndex {
	index := localFactIndex{Candidates: []localCandidateFact{}, Components: []Component{}}
	componentIDs := make(map[string]struct{})
	for _, candidate := range bundle.Candidates {
		if _, included := selected[candidate.ID]; !included {
			continue
		}
		candidate = projectLeafCandidate(candidate)
		fact := localCandidateFact{
			ID: candidate.ID, Name: candidate.Name, Kind: candidate.Kind,
			Trigger: candidate.Trigger, OrderingBasis: candidate.OrderingBasis,
			Beats: []localBeatFact{}, Gaps: []localGapFact{},
		}
		for _, beat := range candidate.Beats {
			fact.Beats = append(fact.Beats, localBeatFact{
				ID: beat.ID, Kind: beat.Kind, Label: beat.Label, Detail: beat.Detail,
				Sequence: beat.Sequence, ComponentIDs: sortedStrings(beat.ComponentIDs),
				SurfaceIDs: sortedStrings(beat.SurfaceIDs), FlowID: beat.FlowID,
				FlowStepIDs: sortedStrings(beat.FlowStepIDs),
				Evidence:    append([]EvidenceRef{}, beat.Evidence...),
			})
			addStrings(componentIDs, beat.ComponentIDs)
		}
		for _, gap := range candidate.Gaps {
			fact.Gaps = append(fact.Gaps, localGapFact{
				ID: gap.ID, Label: gap.Label, Detail: gap.Detail,
				Evidence: append([]EvidenceRef{}, gap.Evidence...),
			})
		}
		index.Candidates = append(index.Candidates, fact)
	}
	components := componentIndex(bundle.Components)
	index.Components = projectLeafComponents(sortedSet(componentIDs), components)
	return index
}
