package semanticdiscovery

import (
	"fmt"
	"sort"
	"strings"
)

const leafSystemPrefix = `You analyze one independently bounded semantic opportunity. The supplied scope contains only an opaque candidate ID, an artifact kind, and the complete direct local facts for this leaf. No prose from another model stage is supplied. Return valid JSON only. Facts and opaque IDs remain authoritative. Do not create or quote repository paths, file names, symbols, flows, relations, evidence, focus IDs, runtime order, or IDs.`

const leafUserPrefix = `Return exactly this JSON shape:
{
  "version": 1,
  "task_id": "exact supplied task id",
  "candidate_id": "exact supplied candidate id",
  "status": "usable or insufficient_evidence",
  "observations": [
    {"text": "one atomic directly supported observation", "support_ids": ["exact fact ids"]}
  ],
  "candidate_connection": {
    "candidate_id": "exact supplied candidate id",
    "relation": "needs_combination",
    "explanation": "why synthesis is still needed",
    "support_ids": ["union of ids used by all leaf content"]
  },
  "contradictions": [
    {"explanation": "one conflict between independent facts", "support_ids": ["exact fact ids from at least two source groups"]}
  ],
  "missing_evidence": [
    {"explanation": "what the facts do not establish", "support_ids": ["exact fact ids"], "missing_capabilities": ["one exact capability value"]}
  ]
}

Rules:
- Do not decide whether the complete artifact is supported. That verdict belongs to fan-in.
- artifact_kind is an editorial task category, not evidence. Derive every observation and gap directly from the supplied facts.
- usable requires at least one directly supported observation. insufficient_evidence requires at least one meaningful missing_evidence item but may still return supported atomic observations; the status never decides the global artifact verdict.
- Return at most 8 observations, 6 contradictions, and 8 missing_evidence items. Prefer fewer atomic items over exhaustive paraphrase.
- Every observation must overlap semantically with its support fact text. If the cited facts are static-only, either state only that the saved item exists/is listed or move the behavioral conclusion to missing_evidence with the behavior capability. Do not add an unrelated behavior-capable fact merely to authorize an action verb.
- Ordering words require sequence-capable support. When a capability is absent or uncertain, abstain from that observation and record the exact missing capability.
- Missing capability values use only this closed vocabulary: static, behavior, sequence, limitation, entry, direct_call, branch, data_read, data_write, data_transformation, output_effect, error_path, test_evidence, ownership, lifecycle.
- A contradiction needs facts from at least two source groups.
- candidate_connection is required and its support_ids must exactly equal the union used by observations, contradictions, and missing_evidence.
- Do not mention paths, file names, symbols, source locations, dotted identifiers, slash-separated names, flags, or code-formatted tokens in prose. Paraphrase them as roles such as the analyzer, saved bundle, dependency, or report stage.`

const leafTaskMarker = "\n\nVariable canonical leaf task JSON with direct saved facts:\n"

type leafPromptPayload struct {
	Version   int                  `json:"version"`
	TaskID    string               `json:"task_id"`
	Candidate promptCandidateScope `json:"candidate"`
	Facts     []Fact               `json:"facts"`
}

func PlanLeafTasks(bundle Bundle, candidates []OpportunityCandidate) ([]LeafTask, error) {
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 || len(candidates) > MaxSelectedCandidates {
		return nil, fmt.Errorf("semantic discovery: leaf plan needs between 1 and %d candidates", MaxSelectedCandidates)
	}
	known := factIndex(bundle)
	seen := make(map[string]struct{}, len(candidates))
	tasks := make([]LeafTask, 0, len(candidates))
	for index, candidate := range candidates {
		if err := validateOpportunityCandidate(candidate, known); err != nil {
			return nil, fmt.Errorf("semantic discovery: leaf candidate %d: %w", index, err)
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			return nil, fmt.Errorf("semantic discovery: duplicate leaf candidate %q", candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
		facts, err := factsForIDs(candidateFactIDs(candidate), known)
		if err != nil {
			return nil, err
		}
		task := LeafTask{
			Version:   LeafTaskVersion,
			ID:        stableID("semantic-leaf", candidate.ID),
			Candidate: candidate,
			Facts:     facts,
		}
		if err := task.Validate(); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks, nil
}

func (task LeafTask) Validate() error {
	if task.Version != LeafTaskVersion {
		return fmt.Errorf("semantic discovery: unsupported leaf task version %d", task.Version)
	}
	if err := validateOpaque("leaf task id", task.ID); err != nil {
		return err
	}
	if task.ID != stableID("semantic-leaf", task.Candidate.ID) {
		return fmt.Errorf("semantic discovery: leaf task id does not match candidate")
	}
	known := make(map[string]Fact, len(task.Facts))
	for index, fact := range task.Facts {
		if err := validateFact(fact); err != nil {
			return fmt.Errorf("semantic discovery: leaf facts[%d]: %w", index, err)
		}
		if _, duplicate := known[fact.ID]; duplicate {
			return fmt.Errorf("semantic discovery: duplicate leaf fact id %q", fact.ID)
		}
		known[fact.ID] = fact
	}
	if err := validateOpportunityCandidate(task.Candidate, known); err != nil {
		return err
	}
	if len(task.Facts) != len(candidateFactIDs(task.Candidate)) {
		return fmt.Errorf("semantic discovery: leaf facts do not exactly match candidate support")
	}
	for _, id := range candidateFactIDs(task.Candidate) {
		if _, exists := known[id]; !exists {
			return fmt.Errorf("semantic discovery: leaf omits candidate support id %q", id)
		}
	}
	return nil
}

func LeafTaskHash(task LeafTask) (string, []byte, error) {
	if err := task.Validate(); err != nil {
		return "", nil, err
	}
	canonical := canonicalLeafTask(task)
	return hashJSON("leaf task", canonical)
}

func BuildLeafPrompt(task LeafTask) (Prompt, error) {
	if _, _, err := LeafTaskHash(task); err != nil {
		return Prompt{}, err
	}
	payload := leafPromptPayload{
		Version: task.Version,
		TaskID:  task.ID,
		Candidate: promptCandidateScope{
			ID: task.Candidate.ID, Kind: task.Candidate.Kind,
		},
		Facts: modelFacts(task.Facts),
	}
	_, encoded, err := hashJSON("leaf prompt input", payload)
	if err != nil {
		return Prompt{}, err
	}
	return Prompt{
		Version:         LeafPromptVersion,
		System:          leafSystemPrefix,
		User:            leafUserPrefix + leafTaskMarker + string(encoded),
		ThinkingProfile: ThinkingHigh,
		ProgressLabel: fmt.Sprintf(
			"semantic leaf %s %s",
			task.Candidate.Kind,
			shortProgressID(task.ID),
		),
	}, nil
}

func shortProgressID(id string) string {
	const suffixBytes = 8
	if len(id) <= suffixBytes {
		return id
	}
	return id[len(id)-suffixBytes:]
}

func ParseLeafArtifact(raw []byte) (LeafArtifact, error) {
	var artifact LeafArtifact
	if err := decodeStrict(raw, &artifact, maxProposalBytes); err != nil {
		return LeafArtifact{}, fmt.Errorf("semantic discovery: invalid leaf artifact json: %w", err)
	}
	return artifact, nil
}

func NormalizeLeafArtifact(artifact LeafArtifact) LeafArtifact {
	result := artifact
	result.Observations = append([]LeafObservation(nil), artifact.Observations...)
	for index := range result.Observations {
		result.Observations[index].Text = normalizeText(result.Observations[index].Text)
		result.Observations[index].SupportIDs = sortedStringsPreservingInvalid(
			result.Observations[index].SupportIDs,
		)
	}
	result.CandidateConnection.Explanation = normalizeText(result.CandidateConnection.Explanation)
	result.CandidateConnection.SupportIDs = sortedStringsPreservingInvalid(
		result.CandidateConnection.SupportIDs,
	)
	result.Contradictions = append([]LeafContradiction(nil), artifact.Contradictions...)
	for index := range result.Contradictions {
		result.Contradictions[index].Explanation = normalizeText(result.Contradictions[index].Explanation)
		result.Contradictions[index].SupportIDs = sortedStringsPreservingInvalid(
			result.Contradictions[index].SupportIDs,
		)
	}
	result.MissingEvidence = append([]LeafMissingEvidence(nil), artifact.MissingEvidence...)
	for index := range result.MissingEvidence {
		result.MissingEvidence[index].Explanation = normalizeText(result.MissingEvidence[index].Explanation)
		result.MissingEvidence[index].SupportIDs = sortedStringsPreservingInvalid(
			result.MissingEvidence[index].SupportIDs,
		)
		result.MissingEvidence[index].MissingCapabilities = sortedCapabilitiesPreservingInvalid(
			result.MissingEvidence[index].MissingCapabilities,
		)
	}
	return result
}

// ProjectTrustedFactStatement converts a locally validated fact statement into
// prose that is safe to copy into a model-authored leaf field. Repository
// references stay represented by their lexical parts, while path and symbol
// separators are removed. Untrusted model output must still pass the strict
// leaf validators and must not use this projection.
func ProjectTrustedFactStatement(statement string) string {
	return normalizeText(repositoryReferencePattern.ReplaceAllStringFunc(
		statement,
		func(reference string) string {
			suffix := ""
			if strings.HasSuffix(reference, ".") {
				reference = strings.TrimSuffix(reference, ".")
				suffix = "."
			}
			return strings.NewReplacer(".", " ", "/", " ", `\`, " ").Replace(reference) + suffix
		},
	))
}

func ValidateLeafArtifact(task LeafTask, artifact LeafArtifact) error {
	if err := task.Validate(); err != nil {
		return err
	}
	if artifact.Version != LeafArtifactVersion {
		return fmt.Errorf("semantic discovery: unsupported leaf artifact version %d", artifact.Version)
	}
	if artifact.TaskID != task.ID || artifact.CandidateID != task.Candidate.ID {
		return fmt.Errorf("semantic discovery: leaf artifact identity does not match task")
	}
	if len(artifact.Observations) > maxObservationsPerLeaf ||
		len(artifact.Contradictions) > maxContradictionsPerLeaf ||
		len(artifact.MissingEvidence) > maxMissingEvidencePerLeaf {
		return fmt.Errorf("semantic discovery: leaf artifact exceeds item limits")
	}
	switch artifact.Status {
	case LeafStatusUsable:
		if len(artifact.Observations) == 0 {
			return fmt.Errorf("semantic discovery: usable leaf has no observations")
		}
	case LeafStatusInsufficientEvidence:
		if len(artifact.MissingEvidence) == 0 {
			return fmt.Errorf("semantic discovery: insufficient leaf must contain missing evidence")
		}
	default:
		return fmt.Errorf("semantic discovery: unsupported leaf status %q", artifact.Status)
	}

	known := make(map[string]Fact, len(task.Facts))
	for _, fact := range task.Facts {
		known[fact.ID] = fact
	}
	usedSupport := make(map[string]struct{})
	for index, observation := range artifact.Observations {
		if err := validateLeafObservation(known, index, observation); err != nil {
			return err
		}
		addIDs(usedSupport, observation.SupportIDs)
	}
	for index, contradiction := range artifact.Contradictions {
		if err := validateLeafContradiction(known, index, contradiction); err != nil {
			return err
		}
		addIDs(usedSupport, contradiction.SupportIDs)
	}
	for index, missing := range artifact.MissingEvidence {
		if err := validateLeafMissingEvidence(known, index, missing); err != nil {
			return err
		}
		addIDs(usedSupport, missing.SupportIDs)
	}

	connection := artifact.CandidateConnection
	if connection.CandidateID != task.Candidate.ID || connection.Relation != connectionNeedsCombination {
		return fmt.Errorf("semantic discovery: leaf candidate connection is invalid")
	}
	if err := validateModelText("leaf candidate connection", connection.Explanation, maxModelTextBytes, true); err != nil {
		return err
	}
	if err := validateIDList("leaf candidate connection support ids", connection.SupportIDs, true); err != nil {
		return err
	}
	if _, err := factsForIDs(connection.SupportIDs, known); err != nil {
		return err
	}
	if !equalStringSets(connection.SupportIDs, sortedSet(usedSupport)) {
		return fmt.Errorf("semantic discovery: leaf candidate connection support does not match leaf content")
	}
	return nil
}

func validateLeafObservation(
	known map[string]Fact,
	index int,
	observation LeafObservation,
) error {
	if err := validateModelText("leaf observation", observation.Text, maxModelTextBytes, true); err != nil {
		return fmt.Errorf("semantic discovery: observations[%d]: %w", index, err)
	}
	if err := validateIDList("leaf observation support ids", observation.SupportIDs, true); err != nil {
		return err
	}
	facts, err := factsForIDs(observation.SupportIDs, known)
	if err != nil {
		return err
	}
	return validateSemanticSupport(fmt.Sprintf("observations[%d]", index), observation.Text, facts)
}

func validateLeafContradiction(
	known map[string]Fact,
	index int,
	contradiction LeafContradiction,
) error {
	if err := validateModelText("leaf contradiction", contradiction.Explanation, maxModelTextBytes, true); err != nil {
		return fmt.Errorf("semantic discovery: contradictions[%d]: %w", index, err)
	}
	if err := validateIDList("leaf contradiction support ids", contradiction.SupportIDs, true); err != nil {
		return err
	}
	facts, err := factsForIDs(contradiction.SupportIDs, known)
	if err != nil {
		return err
	}
	if sourceGroupCount(facts) < 2 {
		return fmt.Errorf("semantic discovery: contradiction %d needs two source groups", index)
	}
	return validateSemanticSupport(
		fmt.Sprintf("contradictions[%d]", index),
		contradiction.Explanation,
		facts,
	)
}

func validateLeafMissingEvidence(
	known map[string]Fact,
	index int,
	missing LeafMissingEvidence,
) error {
	if err := validateModelText("leaf missing evidence", missing.Explanation, maxModelTextBytes, true); err != nil {
		return fmt.Errorf("semantic discovery: missing_evidence[%d]: %w", index, err)
	}
	if err := validateIDList("leaf missing evidence support ids", missing.SupportIDs, true); err != nil {
		return err
	}
	facts, err := factsForIDs(missing.SupportIDs, known)
	if err != nil {
		return err
	}
	if !hasBoundedLexicalOverlap(missing.Explanation, facts) {
		return fmt.Errorf("semantic discovery: missing_evidence[%d] is unrelated to its support", index)
	}
	if len(missing.MissingCapabilities) == 0 {
		return fmt.Errorf("semantic discovery: missing_evidence[%d] has no missing capabilities", index)
	}
	seenMissingCapabilities := make(map[Capability]struct{}, len(missing.MissingCapabilities))
	for _, capability := range missing.MissingCapabilities {
		if !validCapability(capability) {
			return fmt.Errorf("semantic discovery: missing_evidence[%d] has invalid capability %q", index, capability)
		}
		if _, duplicate := seenMissingCapabilities[capability]; duplicate {
			return fmt.Errorf("semantic discovery: missing_evidence[%d] repeats capability %q", index, capability)
		}
		seenMissingCapabilities[capability] = struct{}{}
		if factsSupportCapability(facts, capability) {
			return fmt.Errorf("semantic discovery: missing_evidence[%d] declares capability %q that is already present", index, capability)
		}
	}
	return nil
}

func canonicalLeafTask(task LeafTask) LeafTask {
	result := task
	result.Candidate = canonicalOpportunityCandidate(task.Candidate)
	result.Facts = make([]Fact, len(task.Facts))
	for index, fact := range task.Facts {
		result.Facts[index] = canonicalFact(fact)
	}
	sort.Slice(result.Facts, func(i, j int) bool { return result.Facts[i].ID < result.Facts[j].ID })
	return result
}

func sortedCapabilitiesPreservingInvalid(values []Capability) []Capability {
	result := append([]Capability(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func addIDs(destination map[string]struct{}, ids []string) {
	for _, id := range ids {
		destination[id] = struct{}{}
	}
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func equalStringSets(left, right []string) bool {
	left = sortedUnique(left)
	right = sortedUnique(right)
	return strings.Join(left, "\x00") == strings.Join(right, "\x00")
}
