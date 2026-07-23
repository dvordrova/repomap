package guidedtour

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
)

const (
	maxBundleBytes      = 1 << 20
	maxOpaqueIDBytes    = 256
	maxNameBytes        = 256
	maxSummaryBytes     = 4 << 10
	maxDetailBytes      = 8 << 10
	maxDescriptionBytes = 4 << 10
	maxPathBytes        = 4 << 10
)

func (b Bundle) Validate() error {
	if b.Version != BundleVersion {
		return fmt.Errorf("guided tour: unsupported bundle version %d", b.Version)
	}
	if err := validateText("repository name", b.RepoName, maxNameBytes, true); err != nil {
		return err
	}
	if b.CanvasVersion <= 0 {
		return fmt.Errorf("guided tour: canvas version must be positive")
	}
	if len(b.Candidates) == 0 {
		return fmt.Errorf("guided tour: bundle has no candidates")
	}

	components := make(map[string]Component, len(b.Components))
	for index, component := range b.Components {
		if err := validateComponent(component); err != nil {
			return fmt.Errorf("guided tour: components[%d]: %w", index, err)
		}
		if _, duplicate := components[component.ID]; duplicate {
			return fmt.Errorf("guided tour: duplicate component id %q", component.ID)
		}
		components[component.ID] = component
	}

	candidates := make(map[string]struct{}, len(b.Candidates))
	for index, candidate := range b.Candidates {
		if err := validateCandidate(candidate, components); err != nil {
			return fmt.Errorf("guided tour: candidates[%d]: %w", index, err)
		}
		if _, duplicate := candidates[candidate.ID]; duplicate {
			return fmt.Errorf("guided tour: duplicate candidate id %q", candidate.ID)
		}
		candidates[candidate.ID] = struct{}{}
	}
	return nil
}

// BundleHash validates and canonically orders a bundle before hashing it.
// The returned bytes are the exact canonical JSON used by BuildPrompt.
func BundleHash(bundle Bundle) (string, []byte, error) {
	if err := bundle.Validate(); err != nil {
		return "", nil, err
	}
	canonical := canonicalBundle(bundle)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", nil, fmt.Errorf("guided tour: encode canonical bundle: %w", err)
	}
	if len(encoded) > maxBundleBytes {
		return "", nil, fmt.Errorf(
			"guided tour: canonical bundle is %d bytes, limit is %d",
			len(encoded),
			maxBundleBytes,
		)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), encoded, nil
}

func BuildPrompt(bundle Bundle) (Prompt, error) {
	_, encoded, err := BundleHash(bundle)
	if err != nil {
		return Prompt{}, err
	}
	system := `You are an optional editorial guide for one bounded repository tour. Choose exactly one supplied candidate and organize only its supplied beats and gaps. Local code remains authoritative for components, surfaces, traces, steps, evidence, locations, ordering constraints, and runtime claims. Return valid JSON only. Never invent or quote a repository path, file name, symbol, relation, transition, ID, certainty, or runtime fact.`
	user := `Choose one candidate from the supplied canonical bundle and return exactly this JSON shape:
{
  "version": 1,
  "candidate_id": "one exact supplied candidate id",
  "title": "concise title without paths or file names",
  "summary": "concise evidence-aware summary without paths or file names",
  "steps": [
    {
      "title": "concise teaching step title",
      "explanation": "concise explanation",
      "beat_ids": ["exact beat ids from the selected candidate"]
    }
  ],
  "gap_summary": [
    {
      "explanation": "why the supplied gaps remain unresolved",
      "gap_ids": ["exact gap ids from the selected candidate"]
    }
  ]
}

Rules:
- Return 3 to 6 non-empty steps and reference at least 3 distinct beats.
- Reference each beat or gap at most once. Do not reference IDs from another candidate.
- Include every supplied gap for the selected candidate exactly once in gap_summary.
- For a saved_trace candidate, beat sequence values must never decrease across the returned steps.
- An editorial candidate may reorder beats for teaching; that order is not runtime order.
- Use only proposal fields shown in the schema. Do not add components, surfaces, flows, steps, evidence, locations, or references.
- Do not mention .go file names, slash paths, symbols, or source locations in any prose.
- Gaps are limitations, not invitations to invent missing runtime behavior.
- For suggested_direction, write an editorial investigation guide, not a runtime narration. Use instructions such as "inspect the static evidence" or explicitly hedged statements such as "the candidate suggests" and "runtime order is unproven".
- In suggested_direction prose, do not assert behavior, including execution, routing, scheduling, reading, persistence, processing, responses, or handoffs. Behavioral words are allowed only when the same local clause explicitly negates the claim or says it is unproven.

Canonical guided-tour bundle JSON:
` + string(encoded)
	return Prompt{Version: PromptVersion, System: system, User: user}, nil
}

func validateCandidate(candidate Candidate, components map[string]Component) error {
	if err := validateOpaque("candidate id", candidate.ID); err != nil {
		return err
	}
	if err := validateText("candidate name", candidate.Name, maxNameBytes, true); err != nil {
		return err
	}
	if err := validateText("candidate trigger", candidate.Trigger, maxSummaryBytes, true); err != nil {
		return err
	}
	if err := validateText("candidate summary", candidate.Summary, maxSummaryBytes, true); err != nil {
		return err
	}
	switch candidate.Kind {
	case CandidateSavedTrace:
		if candidate.OrderingBasis != OrderingTrace {
			return fmt.Errorf("saved trace must use trace_order")
		}
	case CandidateSuggestedDirection:
		if candidate.OrderingBasis != OrderingEditorial {
			return fmt.Errorf("suggested direction must use editorial ordering")
		}
	default:
		return fmt.Errorf("unsupported candidate kind %q", candidate.Kind)
	}
	if len(candidate.Beats) < 3 {
		return fmt.Errorf("candidate must supply at least 3 beats")
	}

	knownIDs := make(map[string]string, len(candidate.Beats)+len(candidate.Gaps))
	knownEvidence := make(map[string]EvidenceRef)
	for index, beat := range candidate.Beats {
		if err := validateBeat(beat, components, knownEvidence); err != nil {
			return fmt.Errorf("beats[%d]: %w", index, err)
		}
		if previous, duplicate := knownIDs[beat.ID]; duplicate {
			return fmt.Errorf("duplicate id %q in %s and beat", beat.ID, previous)
		}
		knownIDs[beat.ID] = "beat"
	}
	for index, gap := range candidate.Gaps {
		if err := validateGap(gap, knownEvidence); err != nil {
			return fmt.Errorf("gaps[%d]: %w", index, err)
		}
		if previous, duplicate := knownIDs[gap.ID]; duplicate {
			return fmt.Errorf("duplicate id %q in %s and gap", gap.ID, previous)
		}
		knownIDs[gap.ID] = "gap"
	}
	return nil
}

func validateBeat(
	beat Beat,
	components map[string]Component,
	knownEvidence map[string]EvidenceRef,
) error {
	if err := validateOpaque("beat id", beat.ID); err != nil {
		return err
	}
	if err := validateOpaque("beat kind", beat.Kind); err != nil {
		return err
	}
	if err := validateText("beat label", beat.Label, maxNameBytes, true); err != nil {
		return err
	}
	if err := validateText("beat detail", beat.Detail, maxDetailBytes, true); err != nil {
		return err
	}
	if beat.Sequence < 0 {
		return fmt.Errorf("beat sequence cannot be negative")
	}
	if err := validateIDList("component ids", beat.ComponentIDs); err != nil {
		return err
	}
	for _, id := range beat.ComponentIDs {
		if _, exists := components[id]; !exists {
			return fmt.Errorf("beat references unknown component id %q", id)
		}
	}
	if err := validateIDList("surface ids", beat.SurfaceIDs); err != nil {
		return err
	}
	if beat.FlowID != "" {
		if err := validateOpaque("flow id", beat.FlowID); err != nil {
			return err
		}
	}
	if err := validateIDList("flow step ids", beat.FlowStepIDs); err != nil {
		return err
	}
	return validateEvidenceList("beat evidence", beat.Evidence, knownEvidence)
}

func validateGap(gap Gap, knownEvidence map[string]EvidenceRef) error {
	if err := validateOpaque("gap id", gap.ID); err != nil {
		return err
	}
	if err := validateText("gap label", gap.Label, maxNameBytes, true); err != nil {
		return err
	}
	if err := validateText("gap detail", gap.Detail, maxDetailBytes, true); err != nil {
		return err
	}
	return validateEvidenceList("gap evidence", gap.Evidence, knownEvidence)
}

func validateComponent(component Component) error {
	if err := validateOpaque("component id", component.ID); err != nil {
		return err
	}
	if err := validateText("component name", component.Name, maxNameBytes, true); err != nil {
		return err
	}
	return validateText("component description", component.Description, maxDescriptionBytes, true)
}

func validateEvidenceList(
	field string,
	values []EvidenceRef,
	known map[string]EvidenceRef,
) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if err := validateEvidenceRef(value); err != nil {
			return fmt.Errorf("%s[%d]: %w", field, index, err)
		}
		if _, duplicate := seen[value.ID]; duplicate {
			return fmt.Errorf("%s repeats evidence id %q", field, value.ID)
		}
		seen[value.ID] = struct{}{}
		if previous, exists := known[value.ID]; exists && !reflect.DeepEqual(previous, value) {
			return fmt.Errorf("evidence id %q has conflicting definitions", value.ID)
		}
		known[value.ID] = value
	}
	return nil
}

func validateEvidenceRef(reference EvidenceRef) error {
	if err := validateOpaque("evidence id", reference.ID); err != nil {
		return err
	}
	if err := validateOpaque("evidence kind", reference.Kind); err != nil {
		return err
	}
	if err := validateText("evidence label", reference.Label, maxDetailBytes, true); err != nil {
		return err
	}
	if reference.Location == nil {
		return nil
	}
	return validateLocation(*reference.Location)
}

func validateLocation(location evidence.Location) error {
	if err := validateText("evidence path", location.Path, maxPathBytes, true); err != nil {
		return err
	}
	if location.Line < 0 || location.Column < 0 || location.EndLine < 0 || location.EndColumn < 0 {
		return fmt.Errorf("evidence location coordinates cannot be negative")
	}
	if location.EndLine > 0 && location.Line > 0 && location.EndLine < location.Line {
		return fmt.Errorf("evidence location end line precedes start line")
	}
	if location.EndLine == location.Line && location.EndColumn > 0 && location.Column > 0 &&
		location.EndColumn < location.Column {
		return fmt.Errorf("evidence location end column precedes start column")
	}
	return nil
}

func validateIDList(field string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateOpaque(field, value); err != nil {
			return err
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s repeats id %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateOpaque(field, value string) error {
	if value == "" || len(value) > maxOpaqueIDBytes || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return fmt.Errorf("%s is empty, malformed, or too long", field)
	}
	for _, char := range value {
		if char < 0x21 || char == 0x7f {
			return fmt.Errorf("%s contains control or whitespace characters", field)
		}
	}
	return nil
}

func validateText(field, value string, limit int, required bool) error {
	if len(value) > limit || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return fmt.Errorf("guided tour: %s is malformed or too long", field)
	}
	if required && value == "" {
		return fmt.Errorf("guided tour: %s is empty", field)
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return fmt.Errorf("guided tour: %s contains control characters", field)
		}
	}
	return nil
}

func canonicalBundle(bundle Bundle) Bundle {
	result := bundle
	result.Components = append([]Component{}, bundle.Components...)
	sort.Slice(result.Components, func(i, j int) bool {
		return result.Components[i].ID < result.Components[j].ID
	})
	result.Candidates = make([]Candidate, len(bundle.Candidates))
	for index, candidate := range bundle.Candidates {
		result.Candidates[index] = canonicalCandidate(candidate)
	}
	sort.Slice(result.Candidates, func(i, j int) bool {
		return result.Candidates[i].ID < result.Candidates[j].ID
	})
	return result
}

func canonicalCandidate(candidate Candidate) Candidate {
	result := candidate
	result.Beats = make([]Beat, len(candidate.Beats))
	for index, beat := range candidate.Beats {
		result.Beats[index] = canonicalBeat(beat)
	}
	sort.Slice(result.Beats, func(i, j int) bool {
		return result.Beats[i].ID < result.Beats[j].ID
	})
	result.Gaps = make([]Gap, len(candidate.Gaps))
	for index, gap := range candidate.Gaps {
		result.Gaps[index] = gap
		result.Gaps[index].Evidence = canonicalEvidence(gap.Evidence)
	}
	sort.Slice(result.Gaps, func(i, j int) bool {
		return result.Gaps[i].ID < result.Gaps[j].ID
	})
	return result
}

func canonicalBeat(beat Beat) Beat {
	result := beat
	result.ComponentIDs = sortedStrings(beat.ComponentIDs)
	result.SurfaceIDs = sortedStrings(beat.SurfaceIDs)
	result.FlowStepIDs = sortedStrings(beat.FlowStepIDs)
	result.Evidence = canonicalEvidence(beat.Evidence)
	return result
}

func canonicalEvidence(values []EvidenceRef) []EvidenceRef {
	result := make([]EvidenceRef, len(values))
	for index, value := range values {
		result[index] = cloneEvidenceRef(value)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func cloneEvidenceRef(reference EvidenceRef) EvidenceRef {
	result := reference
	if reference.Location != nil {
		location := *reference.Location
		result.Location = &location
	}
	return result
}

func sortedStrings(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}
