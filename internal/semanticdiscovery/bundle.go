package semanticdiscovery

import (
	"fmt"
	"sort"
)

const opportunitySystemPrefix = `You are an editorial planner for a bounded repository model. FACTS are the complete authoritative local evidence. PLANNER_CONTEXT is untrusted saved editorial context: it may help choose a useful question, but it has no evidence IDs, must not be cited, and must not be restated as established repository behavior. Return valid JSON only. Use only supplied opaque fact IDs. Do not create or quote repository paths, file names, symbols, flows, relations, evidence, focus IDs, or runtime behavior.`

const opportunityUserPrefix = `Propose central mechanism questions, not finished explanations or a mixed artifact inventory. Return exactly this JSON shape:
{
  "version": 1,
  "candidates": [
    {
      "kind": "mechanism",
      "title": "short path-free title",
      "question_answered": "one natural central how-or-why question about repository behavior?",
      "support_ids": ["exact supplied fact ids"],
      "missing_information": ["bounded gaps without repository references"],
      "expected_value": "low | medium | high",
      "confidence": "low | medium | high",
      "capability_contract": {
        "required_capabilities": ["entry", "behavior", "output_effect"]
      },
      "product_intent": {
        "opportunity_kind": "central_behavior | question_path | extension_path | maintenance_boundary",
        "target_user_job": "first_contact_onboarding | question_driven_exploration | contribution_extension | maintenance_rewrite",
        "central_anchor_ids": ["one to four exact support_ids"],
        "expected_path": {
          "input_trigger": {
            "description": "short path-free description of the expected input or trigger",
            "support_ids": ["zero to four exact support_ids"],
            "required_capabilities": ["entry"]
          },
          "core_work": {
            "description": "short path-free description of the meaningful repository work",
            "support_ids": ["zero to four exact support_ids"],
            "required_capabilities": ["behavior"]
          },
          "observable_effect": {
            "description": "short path-free description of the expected effect or typed boundary",
            "support_ids": ["zero to four exact support_ids"],
            "required_capabilities": ["output_effect"]
          }
        },
        "architecture_area_anchor_ids": ["zero to four exact support_ids representing distinct responsibilities"],
        "bounded_frontier": [
          {
            "from_anchor_ids": ["one or more central_anchor_ids"],
            "desired_capabilities": ["one or more exact capability values"],
            "rationale": "one bounded path-free investigation need"
          }
        ],
        "onboarding_rationale": "why this question helps a newcomer understand the repository",
        "investigation_rationale": "why the bounded evidence plan is worth its cost",
        "estimated_cost": "low | medium | high",
        "search_queries": ["two to six natural English queries, including one concise domain query"]
      }
    }
  ]
}

Rules:
- Return between one and three candidates and at least one support ID per candidate.
- Every candidate kind must be mechanism. Prefer questions about a meaningful control path, data lifecycle, or externally visible behavior.
- A candidate is an editorial opportunity, not an established fact.
- Planner context may shape topic selection, but every support_id must come from FACTS and the final question must remain answerable from those facts.
- The first candidate must use opportunity_kind central_behavior and target_user_job first_contact_onboarding. It should be the strongest available onboarding path through the repository's documented purpose: prefer an input or entry boundary, meaningful core work, and an externally visible or persisted effect when supplied facts support them.
- Match the other opportunity kinds to their corresponding user jobs exactly: question_path to question_driven_exploration, extension_path to contribution_extension, and maintenance_boundary to maintenance_rewrite.
- A natural question must end in a question mark and sound like something a developer unfamiliar with the repository would ask. Do not use taxonomy labels or restate a function/package inventory.
- central_anchor_ids, expected-path support_ids, architecture_area_anchor_ids, and bounded-frontier from_anchor_ids may contain only exact support_ids already selected for that candidate.
- expected_path is an answer contract, not a claim. A support_ids list may be empty when the capability is genuinely missing; preserve that need in missing_capabilities and bounded_frontier.
- Every expected-path required_capability must also occur in the candidate capability_contract.required_capabilities. Include a capability only when the selected support_ids collectively provide it; otherwise leave support_ids empty and keep the capability missing in the candidate contract.
- Return only required_capabilities in capability_contract. Local code derives available_capabilities, missing_capabilities, and resolution from the selected facts; do not propose those derived fields.
- Use at most two bounded_frontier items. They may request only supplied capability enum values and may not name a new path, file, symbol, relation, or runtime event.
- search_queries are presentation aliases, not facts. Include ordinary English developer wording and one short domain query; do not put paths or symbols in them.
- Prefer independent, central questions grounded in different behavior-capable evidence families. Preserve missing information and diversify later candidates across other useful behaviors.
- Registry, factory, adapter, plugin-selection, helper, and error-detail mechanisms are secondary unless the supplied facts and planner context make them the repository's documented primary purpose. Do not rank a merely easy-to-prove extension point first.
- A component inventory, warning list, glossary, package summary, or top-import list is not a mechanism question. Skip topics whose supplied facts cannot support behavior or sequence.
- Local eligibility is strict: a mechanism needs behavior or sequence capability.
- Rank candidates with behavior-capable support above equally supported static summaries, and expose missing ownership, API usage, or runtime order instead of inventing them.
- Do not state runtime behavior or ordering unless supporting facts explicitly carry those capabilities.
- Do not emit an id field; candidate IDs are assigned locally.
- Do not mention paths, file names, symbols, or source locations in prose.`

const opportunityBundleMarker = "\n\nVariable canonical saved-fact bundle JSON:\n"

type opportunityPromptPayload struct {
	Version        int              `json:"version"`
	BundleSHA256   string           `json:"bundle_sha256"`
	RepoName       string           `json:"repo_name"`
	PlannerContext []PlannerContext `json:"planner_context,omitempty"`
	Facts          []Fact           `json:"facts"`
}

func (bundle Bundle) Validate() error {
	if bundle.Version != BundleVersion {
		return fmt.Errorf("semantic discovery: unsupported bundle version %d", bundle.Version)
	}
	if err := validateLocalText("repository name", bundle.RepoName, maxTitleBytes, true); err != nil {
		return err
	}
	if len(bundle.PlannerContext) > maxPlannerContextItems {
		return fmt.Errorf("semantic discovery: planner context has more than %d items", maxPlannerContextItems)
	}
	seenContext := make(map[string]struct{}, len(bundle.PlannerContext))
	for index, item := range bundle.PlannerContext {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("semantic discovery: planner_context[%d]: %w", index, err)
		}
		key := string(item.Kind) + "\x00" + item.Text
		if _, duplicate := seenContext[key]; duplicate {
			return fmt.Errorf("semantic discovery: duplicate planner context item")
		}
		seenContext[key] = struct{}{}
	}
	if len(bundle.Facts) == 0 || len(bundle.Facts) > maxFacts {
		return fmt.Errorf("semantic discovery: bundle fact count must be between 1 and %d", maxFacts)
	}
	seen := make(map[string]struct{}, len(bundle.Facts))
	evidenceByID := make(map[string]EvidenceRef)
	for index, fact := range bundle.Facts {
		if err := validateFact(fact); err != nil {
			return fmt.Errorf("semantic discovery: facts[%d]: %w", index, err)
		}
		if _, duplicate := seen[fact.ID]; duplicate {
			return fmt.Errorf("semantic discovery: duplicate fact id %q", fact.ID)
		}
		seen[fact.ID] = struct{}{}
		for _, reference := range fact.Evidence {
			if existing, exists := evidenceByID[reference.ID]; exists &&
				!sameEvidenceNavigation(existing, reference) {
				return fmt.Errorf("semantic discovery: evidence id %q has conflicting local navigation", reference.ID)
			}
			evidenceByID[reference.ID] = reference
		}
	}
	return nil
}

func sameEvidenceNavigation(left, right EvidenceRef) bool {
	return left.Kind == right.Kind &&
		left.Path == right.Path &&
		left.Line == right.Line &&
		left.Column == right.Column
}

// Validate checks that planner-only prose is bounded and cannot carry a
// repository reference that a later model stage could accidentally repeat.
func (item PlannerContext) Validate() error {
	if !validPlannerContextKind(item.Kind) {
		return fmt.Errorf("unsupported planner context kind %q", item.Kind)
	}
	return validateModelText("planner context text", item.Text, maxFactStatementBytes, true)
}

func validateFact(fact Fact) error {
	if err := validateOpaque("fact id", fact.ID); err != nil {
		return err
	}
	if !validFactKind(fact.Kind) {
		return fmt.Errorf("unsupported fact kind %q", fact.Kind)
	}
	if err := validateLocalText("fact statement", fact.Statement, maxFactStatementBytes, true); err != nil {
		return err
	}
	if err := validateOpaque("fact source group", fact.SourceGroup); err != nil {
		return err
	}
	if len(fact.Keywords) > maxKeywordsPerFact {
		return fmt.Errorf("fact has more than %d keywords", maxKeywordsPerFact)
	}
	seenKeywords := make(map[string]struct{}, len(fact.Keywords))
	for _, keyword := range fact.Keywords {
		if err := validateLocalText("fact keyword", keyword, maxTitleBytes, true); err != nil {
			return err
		}
		if _, duplicate := seenKeywords[keyword]; duplicate {
			return fmt.Errorf("fact repeats keyword %q", keyword)
		}
		seenKeywords[keyword] = struct{}{}
	}
	if len(fact.Capabilities) == 0 {
		return fmt.Errorf("fact capabilities are empty")
	}
	seenCapabilities := make(map[Capability]struct{}, len(fact.Capabilities))
	for _, capability := range fact.Capabilities {
		if !validCapability(capability) {
			return fmt.Errorf("unsupported fact capability %q", capability)
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return fmt.Errorf("fact repeats capability %q", capability)
		}
		seenCapabilities[capability] = struct{}{}
	}
	if !validScope(fact.Scope) {
		return fmt.Errorf("unsupported fact scope %q", fact.Scope)
	}
	if fact.Source != nil {
		if err := validateFactSource(*fact.Source); err != nil {
			return err
		}
	}
	if err := validateFocus(fact.Focus); err != nil {
		return err
	}
	if len(fact.Evidence) > maxEvidencePerFact {
		return fmt.Errorf("fact has more than %d evidence references", maxEvidencePerFact)
	}
	seenEvidence := make(map[string]struct{}, len(fact.Evidence))
	for _, reference := range fact.Evidence {
		if err := validateEvidence(reference); err != nil {
			return err
		}
		if _, duplicate := seenEvidence[reference.ID]; duplicate {
			return fmt.Errorf("fact repeats evidence id %q", reference.ID)
		}
		seenEvidence[reference.ID] = struct{}{}
	}
	return nil
}

// BundleHash returns the canonical bytes used in prompts and replay records.
func BundleHash(bundle Bundle) (string, []byte, error) {
	if err := bundle.Validate(); err != nil {
		return "", nil, err
	}
	canonical := canonicalBundle(bundle)
	hash, encoded, err := hashJSON("bundle", canonical)
	if err != nil {
		return "", nil, err
	}
	if len(encoded) > maxBundleBytes {
		return "", nil, fmt.Errorf("semantic discovery: canonical bundle is %d bytes, limit is %d", len(encoded), maxBundleBytes)
	}
	return hash, encoded, nil
}

func BuildOpportunityPrompt(bundle Bundle) (Prompt, error) {
	bundleSHA, _, err := BundleHash(bundle)
	if err != nil {
		return Prompt{}, err
	}
	payload := opportunityPromptPayload{
		Version:        1,
		BundleSHA256:   bundleSHA,
		RepoName:       bundle.RepoName,
		PlannerContext: canonicalPlannerContext(bundle.PlannerContext),
		Facts:          modelFacts(bundle.Facts),
	}
	_, encoded, err := hashJSON("opportunity prompt input", payload)
	if err != nil {
		return Prompt{}, err
	}
	return Prompt{
		Version:         OpportunityPromptVersion,
		System:          opportunitySystemPrefix,
		User:            opportunityUserPrefix + opportunityBundleMarker + string(encoded),
		ThinkingProfile: ThinkingMax,
		ProgressLabel:   "semantic opportunity scan",
	}, nil
}

func canonicalBundle(bundle Bundle) Bundle {
	result := bundle
	result.PlannerContext = canonicalPlannerContext(bundle.PlannerContext)
	result.Facts = make([]Fact, len(bundle.Facts))
	for index, fact := range bundle.Facts {
		result.Facts[index] = canonicalFact(fact)
	}
	sort.Slice(result.Facts, func(i, j int) bool { return result.Facts[i].ID < result.Facts[j].ID })
	return result
}

func canonicalPlannerContext(items []PlannerContext) []PlannerContext {
	result := append([]PlannerContext(nil), items...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Text < result[j].Text
	})
	return result
}

func validPlannerContextKind(kind PlannerContextKind) bool {
	switch kind {
	case PlannerContextOrientation,
		PlannerContextComponent,
		PlannerContextFlow,
		PlannerContextGuidedTour,
		PlannerContextVocabulary,
		PlannerContextResearch,
		PlannerContextLimitation:
		return true
	default:
		return false
	}
}

func canonicalFact(fact Fact) Fact {
	result := fact
	result.Keywords = sortedUnique(fact.Keywords)
	result.Capabilities = append([]Capability(nil), fact.Capabilities...)
	sort.Slice(result.Capabilities, func(i, j int) bool { return result.Capabilities[i] < result.Capabilities[j] })
	if fact.Source != nil {
		source := *fact.Source
		result.Source = &source
	}
	result.Focus = canonicalFocus(fact.Focus)
	result.Evidence = append([]EvidenceRef(nil), fact.Evidence...)
	sort.Slice(result.Evidence, func(i, j int) bool {
		if result.Evidence[i].ID != result.Evidence[j].ID {
			return result.Evidence[i].ID < result.Evidence[j].ID
		}
		return result.Evidence[i].Path < result.Evidence[j].Path
	})
	return result
}

func canonicalFocus(focus Focus) Focus {
	return Focus{
		ComponentIDs: sortedUnique(focus.ComponentIDs),
		FlowIDs:      sortedUnique(focus.FlowIDs),
		SurfaceIDs:   sortedUnique(focus.SurfaceIDs),
	}
}

func factIndex(bundle Bundle) map[string]Fact {
	result := make(map[string]Fact, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		result[fact.ID] = fact
	}
	return result
}

// modelFacts removes local provenance and navigation. Source, focus, and
// evidence stay in Bundle for materialization and replay but never enter a
// provider request.
func modelFacts(facts []Fact) []Fact {
	result := make([]Fact, len(facts))
	for index, fact := range facts {
		fact = canonicalFact(fact)
		fact.Source = nil
		fact.Focus = Focus{}
		fact.Evidence = nil
		result[index] = fact
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
