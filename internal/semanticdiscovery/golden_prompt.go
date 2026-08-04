package semanticdiscovery

import "fmt"

const goldenMechanismSystemPrefix = `You edit one bounded evidence-backed mechanism for a human onboarding report. The original question, rubric, aliases, and known editorial gaps define intent but are not repository evidence. Only supplied FACTS and the validated leaf material can support claims. Return valid JSON only. Use exact supplied opaque IDs. Do not invent or quote repository paths, file names, symbols, flows, relations, evidence, focus IDs, runtime order, or IDs.`

const goldenMechanismUserPrefix = `Return the existing fan-in JSON shape:
{
  "version": 1,
  "artifacts": [
    {
      "candidate_id": "the exact supplied candidate id",
      "verdict": "supported | mixed | insufficient_evidence",
      "title": "short title that retains the original mechanism intent",
      "summary": "short direct answer to the original question",
      "claims": [
        {
          "title": "short step title",
          "text": "one supported mechanism step or explicit unresolved gap",
          "basis": "direct | compositional | interpretive | unresolved",
          "support_ids": ["exact supplied fact ids"],
          "observation_refs": [{"task_id": "exact leaf task id", "observation_index": 0}],
          "missing_refs": [{"task_id": "exact leaf task id", "missing_index": 0}]
        }
      ],
      "aliases": ["exact supplied local search aliases"],
      "likely_questions": ["the original question"],
      "related_candidate_ids": []
    }
  ]
}

Rules:
- Return exactly one artifact and keep its candidate_id exact.
- Return between 3 and 7 claims. At least 3 must be resolved direct, compositional, or interpretive mechanism steps.
- Answer the original_question. Do not replace it with an easier topic, debug log, import list, configuration inventory, or generic component summary.
- Retain concrete mechanism vocabulary from original_title, original_question, and local_search_aliases only when the supplied facts support those terms.
- Do not promise a layer, effect, output, or user-visible result unless the cited facts establish it.
- In prose, write ordinary conjunctions rather than slash-separated shorthand; slash-shaped text is reserved for repository-reference rejection.
- Claim order is editorial and does not by itself establish execution order.
- Use temporal or ordering language only when the cited support facts are sequence-capable. Describe a direct call without such support without "then", "before", "after", "next", or "finally".
- A same-function and same-branch local sequence stays conditional and branch-scoped. Do not turn it into a guarantee that the branch is selected, the call succeeds, or a wider runtime, process, component, or distributed order.
- Treat each required_answer_aspect as a rubric item. An aspect is coverable only by a resolved claim citing a fact that has the exact keyword answer_aspect:<aspect id> and all of that aspect's required_capabilities.
- Preserve uncovered rubric items as explicit unresolved claims when the validated leaf contains matching missing evidence. Never imply that an uncovered aspect is established.
- direct, compositional, and interpretive claims require observation_refs. support_ids must exactly equal the union of referenced observations.
- compositional claims need at least two facts from at least two independent source groups.
- Every unresolved claim must use title "Evidence gap" and begin its text with "Evidence gap:". It requires missing_refs, and support_ids must exactly equal the union of referenced observations and missing evidence.
- Return aliases exactly from local_search_aliases. Do not invent additional search aliases.
- Return exactly one likely_questions item: the exact original_question.
- Use facts as evidence, not the original question, rubric labels, aliases, capability contract, or editorial gaps.
- Do not mention paths, file names, symbols, source locations, or create repository objects in prose.

Small positive unresolved example:
{"title":"Evidence gap","text":"Evidence gap: Direct behavior tests are not established by the bounded facts.","basis":"unresolved","support_ids":["exact supplied fact id"],"observation_refs":[],"missing_refs":[{"task_id":"exact leaf task id","missing_index":0}]}`

const goldenMechanismPayloadMarker = "\n\nVariable golden mechanism intent, facts, and validated leaf JSON:\n"

type goldenMechanismPromptPayload struct {
	Version               int                `json:"version"`
	BundleSHA256          string             `json:"bundle_sha256"`
	RepoName              string             `json:"repo_name"`
	CandidateID           string             `json:"candidate_id"`
	ArtifactKind          ArtifactKind       `json:"artifact_kind"`
	OriginalTitle         string             `json:"original_title"`
	OriginalQuestion      string             `json:"original_question"`
	RequiredAnswerAspects []AnswerAspect     `json:"required_answer_aspects"`
	MinCovered            int                `json:"min_covered"`
	MinKeyCovered         int                `json:"min_key_covered"`
	LocalSearchAliases    []string           `json:"local_search_aliases"`
	CapabilityContract    CapabilityContract `json:"capability_contract"`
	KnownEditorialGaps    []string           `json:"known_editorial_gaps,omitempty"`
	Facts                 []Fact             `json:"facts"`
	ValidatedLeaf         fanInPromptLeaf    `json:"validated_leaf"`
}

// BuildGoldenMechanismPrompt builds one intent-retaining synthesis request on
// top of an already validated leaf. Its response is the existing FanInArtifact
// contract and is checked and materialized by the existing fan-in path.
func BuildGoldenMechanismPrompt(bundle Bundle, result LeafResult) (Prompt, error) {
	validated, _, err := validateLeafResults(bundle, []LeafResult{result})
	if err != nil {
		return Prompt{}, err
	}
	result = validated[0]
	candidate := result.Task.Candidate
	if candidate.IntentContract == nil || candidate.CapabilityContract == nil {
		return Prompt{}, fmt.Errorf(
			"semantic discovery: golden mechanism needs capability and intent contracts",
		)
	}
	bundleHash, _, err := BundleHash(bundle)
	if err != nil {
		return Prompt{}, err
	}
	intent := candidate.IntentContract
	payload := goldenMechanismPromptPayload{
		Version:               1,
		BundleSHA256:          bundleHash,
		RepoName:              bundle.RepoName,
		CandidateID:           candidate.ID,
		ArtifactKind:          candidate.Kind,
		OriginalTitle:         candidate.Title,
		OriginalQuestion:      candidate.QuestionAnswered,
		RequiredAnswerAspects: append([]AnswerAspect(nil), intent.RequiredAnswerAspects...),
		MinCovered:            intent.MinCovered,
		MinKeyCovered:         intent.MinKeyCovered,
		LocalSearchAliases:    append([]string(nil), intent.LocalSearchAliases...),
		CapabilityContract:    *candidate.CapabilityContract,
		KnownEditorialGaps:    append([]string(nil), candidate.MissingInformation...),
		Facts:                 modelFacts(goldenMechanismLeafFacts(result)),
		ValidatedLeaf: fanInPromptLeaf{
			TaskID:      result.Task.ID,
			CandidateID: candidate.ID,
			Artifact:    NormalizeLeafArtifact(result.Artifact),
		},
	}
	_, encoded, err := hashJSON("golden mechanism input", payload)
	if err != nil {
		return Prompt{}, err
	}
	if len(encoded) > maxRecordBytes {
		return Prompt{}, fmt.Errorf("semantic discovery: golden mechanism payload is too large")
	}
	return Prompt{
		Version:         GoldenMechanismPromptVersion,
		System:          goldenMechanismSystemPrefix,
		User:            goldenMechanismUserPrefix + goldenMechanismPayloadMarker + string(encoded),
		ThinkingProfile: ThinkingMax,
		ProgressLabel:   "golden mechanism synthesis",
	}, nil
}

// goldenMechanismLeafFacts keeps the one-call editor focused on facts that
// survived local leaf validation. The candidate's original support still
// binds its identity and seed provenance, but unrelated historical support
// cannot distract synthesis or become claim material.
func goldenMechanismLeafFacts(result LeafResult) []Fact {
	used := make(map[string]struct{})
	for _, observation := range result.Artifact.Observations {
		addIDs(used, observation.SupportIDs)
	}
	for _, contradiction := range result.Artifact.Contradictions {
		addIDs(used, contradiction.SupportIDs)
	}
	for _, missing := range result.Artifact.MissingEvidence {
		addIDs(used, missing.SupportIDs)
	}
	facts := make(map[string]Fact, len(result.Task.Facts))
	for _, fact := range result.Task.Facts {
		facts[fact.ID] = fact
	}
	return factsForKnownIDs(sortedSet(used), facts)
}
