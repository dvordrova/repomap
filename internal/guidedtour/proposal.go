package guidedtour

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/modelresearch"
)

const ValidationRulePathLikeReference = "path_like_reference"

// ValidationIssue exposes only a closed field/rule pair for safe run
// diagnostics. The original validation error remains the user-facing detail.
type ValidationIssue struct {
	Field string
	Rule  string
	err   error
}

func (issue *ValidationIssue) Error() string {
	return issue.err.Error()
}

func (issue *ValidationIssue) Unwrap() error {
	return issue.err
}

// ValidationIssueDetails returns bounded diagnostics for known proposal
// validation rules without exposing model prose or repository paths.
func ValidationIssueDetails(err error) (field, rule string, ok bool) {
	var issue *ValidationIssue
	if !errors.As(err, &issue) {
		return "", "", false
	}
	return issue.Field, issue.Rule, true
}

const (
	maxProposalBytes           = modelresearch.ProviderResponseByteLimit
	maxProposalTitleBytes      = 256
	maxProposalSummaryBytes    = 2 << 10
	maxProposalExplainBytes    = 4 << 10
	minProposalSteps           = 3
	maxProposalSteps           = 6
	minReferencedProposalBeats = 3
)

var repositoryReferencePattern = regexp.MustCompile(
	`(?i)(?:` +
		`[[:alnum:]_@.+-]+(?:[\\/][[:alnum:]_@.+-]+)+|` +
		`\b[[:alnum:]_@+-][[:alnum:]_@.+-]*\.[[:alnum:]_+-]{1,16}\b|` +
		`\b(?:readme|makefile|dockerfile)\b` +
		`)`,
)

var editorialBehaviorAssertionPattern = regexp.MustCompile(
	`(?i)\b(` +
		`execution[[:space:]]+(begins?|starts?)|executes?|parses?|initiates?|invokes?|` +
		`calls?|dispatches?|routes|forwards|enqueues|schedules|reads|persists|processes|responds|` +
		`hand(?:s|ed|ing)?[[:space:]]+(?:off|over)|transfers?|delegates?|publishes?|` +
		`emits?|queues?|pushes?|pulls?|delivers?|submits?|propagates?|registers?|launches?|` +
		`spawns?|awaits?|waits?|selects?|consumes?|stores?|fetches?|retrieves?|connects?|` +
		`opens?|closes?|transforms?|converts?|orchestrates?|runs|produces?|generates?|` +
		`assembles?|initiali[sz]es?|serves?|handles?|defines?|builds?|creates?|writes?|` +
		`loads?|returns?|passes?|sends?|receives?|triggers?|flows?|coordinates?|combines?` +
		`)\b`,
)

var editorialNegationPattern = regexp.MustCompile(
	`(?i)\b(not|no|never|cannot|can't|does[[:space:]]+not|do[[:space:]]+not|is[[:space:]]+not|are[[:space:]]+not|unproven|unknown|insufficient|doesn't|don't)\b`,
)

var editorialSentenceBoundaryPattern = regexp.MustCompile(`[.!?;\n]+`)

var editorialClauseBoundaryPattern = regexp.MustCompile(
	`(?i)(?:,|:|[()–—]|\b(?:` +
		`and|but|yet|while|whereas|although|however|then|because|since|though|as|when|` +
		`once|after|before|until|unless|if|therefore|thus|which|that` +
		`)\b)`,
)

// ParseProposal strictly decodes the model response shape. Semantic and
// reference validation is performed by ValidateProposal.
func ParseProposal(raw []byte) (Proposal, error) {
	if len(raw) == 0 {
		return Proposal{}, fmt.Errorf("guided tour: proposal is empty or too large")
	}
	if len(raw) > maxProposalBytes {
		return Proposal{}, &modelresearch.ResourceLimitError{
			Stage: "guided_tour", Kind: modelresearch.ResourceLimitResponseBytes,
			Limit: maxProposalBytes, Observed: len(raw), ObservedKnown: true,
		}
	}
	var proposal Proposal
	if err := decodeStrictJSON(raw, &proposal); err != nil {
		return Proposal{}, fmt.Errorf("guided tour: invalid proposal json: %w", err)
	}
	return proposal, nil
}

// ValidateProposal checks one exact candidate selection and all model-supplied
// prose and opaque references. It never substitutes another candidate.
func ValidateProposal(bundle Bundle, proposal Proposal) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	candidate, exists := findCandidate(bundle, proposal.CandidateID)
	if !exists {
		return fmt.Errorf("guided tour: proposal references unknown candidate id %q", proposal.CandidateID)
	}
	if err := validateProposalShape(proposal, proposal.Title == candidate.Name); err != nil {
		return err
	}
	beats := make(map[string]Beat, len(candidate.Beats))
	for _, beat := range candidate.Beats {
		beats[beat.ID] = beat
	}
	seenBeats := make(map[string]struct{})
	previousSequence := 0
	hasPreviousSequence := false
	for stepIndex, step := range proposal.Steps {
		for _, id := range step.BeatIDs {
			beat, known := beats[id]
			if !known {
				return fmt.Errorf(
					"guided tour: proposal steps[%d] references unknown beat id %q",
					stepIndex,
					id,
				)
			}
			if _, duplicate := seenBeats[id]; duplicate {
				return fmt.Errorf("guided tour: proposal repeats beat id %q", id)
			}
			seenBeats[id] = struct{}{}
			if candidate.Kind == CandidateSavedTrace && hasPreviousSequence && beat.Sequence < previousSequence {
				return fmt.Errorf("guided tour: saved trace proposal decreases beat sequence at %q", id)
			}
			previousSequence = beat.Sequence
			hasPreviousSequence = true
		}
	}
	if len(seenBeats) < minReferencedProposalBeats {
		return fmt.Errorf("guided tour: proposal must reference at least %d distinct beats", minReferencedProposalBeats)
	}

	gaps := make(map[string]Gap, len(candidate.Gaps))
	for _, gap := range candidate.Gaps {
		gaps[gap.ID] = gap
	}
	seenGaps := make(map[string]struct{})
	for summaryIndex, summary := range proposal.GapSummary {
		for _, id := range summary.GapIDs {
			if _, known := gaps[id]; !known {
				return fmt.Errorf(
					"guided tour: proposal gap_summary[%d] references unknown gap id %q",
					summaryIndex,
					id,
				)
			}
			if _, duplicate := seenGaps[id]; duplicate {
				return fmt.Errorf("guided tour: proposal repeats gap id %q", id)
			}
			seenGaps[id] = struct{}{}
		}
	}
	missingGapIDs := make([]string, 0, len(gaps)-len(seenGaps))
	for id := range gaps {
		if _, included := seenGaps[id]; !included {
			missingGapIDs = append(missingGapIDs, id)
		}
	}
	if len(missingGapIDs) > 0 {
		sort.Strings(missingGapIDs)
		return fmt.Errorf(
			"guided tour: proposal omits known candidate gap ids %q",
			strings.Join(missingGapIDs, ", "),
		)
	}
	return nil
}

// UnsupportedBehaviorClaimCount reports a diagnostic count of unqualified
// behavioral clauses in a suggested-direction proposal. It is not publication
// authority: suggested-direction prose is explicitly hypothetical, while
// repository references and opaque IDs remain locally validated. Saved traces
// and unknown candidates return zero.
func UnsupportedBehaviorClaimCount(bundle Bundle, proposal Proposal) int {
	candidate, exists := findCandidate(bundle, proposal.CandidateID)
	if !exists || candidate.Kind != CandidateSuggestedDirection {
		return 0
	}

	count := 0
	for _, value := range proposalBehaviorProse(proposal) {
		for _, sentence := range editorialSentenceBoundaryPattern.Split(value, -1) {
			for _, clause := range editorialClauseBoundaryPattern.Split(sentence, -1) {
				if editorialBehaviorAssertionPattern.MatchString(clause) &&
					!editorialNegationPattern.MatchString(clause) {
					count++
				}
			}
		}
	}
	return count
}

func proposalBehaviorProse(proposal Proposal) []string {
	values := []string{proposal.Title, proposal.Summary}
	for _, step := range proposal.Steps {
		values = append(values, step.Title, step.Explanation)
	}
	for _, gap := range proposal.GapSummary {
		values = append(values, gap.Explanation)
	}
	return values
}

// MaterializeStory derives every repository-bearing story field from the
// selected candidate's referenced beats and gaps.
func MaterializeStory(bundle Bundle, proposal Proposal) (Story, error) {
	if err := ValidateProposal(bundle, proposal); err != nil {
		return Story{}, err
	}
	bundle = canonicalBundle(bundle)
	candidate, _ := findCandidate(bundle, proposal.CandidateID)
	beats := make(map[string]Beat, len(candidate.Beats))
	for _, beat := range candidate.Beats {
		beats[beat.ID] = beat
	}
	gaps := make(map[string]Gap, len(candidate.Gaps))
	for _, gap := range candidate.Gaps {
		gaps[gap.ID] = gap
	}
	components := make(map[string]Component, len(bundle.Components))
	for _, component := range bundle.Components {
		components[component.ID] = component
	}

	story := Story{
		Version:       StoryVersion,
		CandidateID:   candidate.ID,
		CandidateName: candidate.Name,
		CandidateKind: candidate.Kind,
		Trigger:       candidate.Trigger,
		OrderingBasis: candidate.OrderingBasis,
		Title:         proposal.Title,
		Summary:       proposal.Summary,
		Steps:         make([]StoryStep, 0, len(proposal.Steps)),
		GapSummary:    make([]StoryGapSummary, 0, len(proposal.GapSummary)),
		Components:    []Component{},
	}
	storyComponentIDs := make(map[string]struct{})
	for _, proposed := range proposal.Steps {
		step := materializeStep(proposed, beats, components)
		story.Steps = append(story.Steps, step)
		for _, id := range step.ComponentIDs {
			storyComponentIDs[id] = struct{}{}
		}
	}
	story.Components = componentsForIDs(sortedSet(storyComponentIDs), components)
	for _, proposed := range proposal.GapSummary {
		story.GapSummary = append(story.GapSummary, materializeGapSummary(proposed, gaps))
	}
	return story, nil
}

func validateProposalShape(proposal Proposal, allowRepositoryBearingTitle bool) error {
	if proposal.Version != ProposalVersion {
		return fmt.Errorf("guided tour: unsupported proposal version %d", proposal.Version)
	}
	if err := validateOpaque("proposal candidate id", proposal.CandidateID); err != nil {
		return fmt.Errorf("guided tour: %w", err)
	}
	if allowRepositoryBearingTitle {
		if err := validateText("proposal title", proposal.Title, maxProposalTitleBytes, true); err != nil {
			return err
		}
	} else {
		if err := validateModelProse("proposal title", "proposal.title", proposal.Title, maxProposalTitleBytes); err != nil {
			return err
		}
	}
	if err := validateModelProse("proposal summary", "proposal.summary", proposal.Summary, maxProposalSummaryBytes); err != nil {
		return err
	}
	if len(proposal.Steps) < minProposalSteps || len(proposal.Steps) > maxProposalSteps {
		return fmt.Errorf(
			"guided tour: proposal must contain between %d and %d steps",
			minProposalSteps,
			maxProposalSteps,
		)
	}
	for index, step := range proposal.Steps {
		if err := validateModelProse("step title", fmt.Sprintf("steps[%d].title", index), step.Title, maxProposalTitleBytes); err != nil {
			return fmt.Errorf("guided tour: proposal steps[%d]: %w", index, err)
		}
		if err := validateModelProse("step explanation", fmt.Sprintf("steps[%d].explanation", index), step.Explanation, maxProposalExplainBytes); err != nil {
			return fmt.Errorf("guided tour: proposal steps[%d]: %w", index, err)
		}
		if len(step.BeatIDs) == 0 {
			return fmt.Errorf("guided tour: proposal steps[%d] has no beat ids", index)
		}
		if err := validateIDList("proposal beat ids", step.BeatIDs); err != nil {
			return fmt.Errorf("guided tour: proposal steps[%d]: %w", index, err)
		}
	}
	for index, summary := range proposal.GapSummary {
		if err := validateModelProse("gap explanation", fmt.Sprintf("gap_summary[%d].explanation", index), summary.Explanation, maxProposalExplainBytes); err != nil {
			return fmt.Errorf("guided tour: proposal gap_summary[%d]: %w", index, err)
		}
		if len(summary.GapIDs) == 0 {
			return fmt.Errorf("guided tour: proposal gap_summary[%d] has no gap ids", index)
		}
		if err := validateIDList("proposal gap ids", summary.GapIDs); err != nil {
			return fmt.Errorf("guided tour: proposal gap_summary[%d]: %w", index, err)
		}
	}
	return nil
}

func validateModelProse(field, diagnosticField, value string, limit int) error {
	if err := validateText(field, value, limit, true); err != nil {
		return err
	}
	if repositoryReferencePattern.MatchString(value) || strings.ContainsAny(value, `/\`) {
		return &ValidationIssue{
			Field: diagnosticField,
			Rule:  ValidationRulePathLikeReference,
			err:   fmt.Errorf("guided tour: %s contains a path-like reference", field),
		}
	}
	return nil
}

func findCandidate(bundle Bundle, id string) (Candidate, bool) {
	for _, candidate := range bundle.Candidates {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return Candidate{}, false
}

func materializeStep(
	proposed ProposedStep,
	knownBeats map[string]Beat,
	knownComponents map[string]Component,
) StoryStep {
	step := StoryStep{
		Title:        proposed.Title,
		Explanation:  proposed.Explanation,
		BeatIDs:      append([]string{}, proposed.BeatIDs...),
		Beats:        make([]Beat, 0, len(proposed.BeatIDs)),
		ComponentIDs: []string{},
		Components:   []Component{},
		SurfaceIDs:   []string{},
		FlowIDs:      []string{},
		FlowStepIDs:  []string{},
		Evidence:     []EvidenceRef{},
	}
	componentIDs := make(map[string]struct{})
	surfaceIDs := make(map[string]struct{})
	flowIDs := make(map[string]struct{})
	flowStepIDs := make(map[string]struct{})
	evidenceByID := make(map[string]EvidenceRef)
	for _, id := range proposed.BeatIDs {
		beat := cloneBeat(knownBeats[id])
		step.Beats = append(step.Beats, beat)
		addStrings(componentIDs, beat.ComponentIDs)
		addStrings(surfaceIDs, beat.SurfaceIDs)
		if beat.FlowID != "" {
			flowIDs[beat.FlowID] = struct{}{}
		}
		addStrings(flowStepIDs, beat.FlowStepIDs)
		addEvidence(evidenceByID, beat.Evidence)
	}
	step.ComponentIDs = sortedSet(componentIDs)
	step.Components = componentsForIDs(step.ComponentIDs, knownComponents)
	step.SurfaceIDs = sortedSet(surfaceIDs)
	step.FlowIDs = sortedSet(flowIDs)
	step.FlowStepIDs = sortedSet(flowStepIDs)
	step.Evidence = sortedEvidence(evidenceByID)
	return step
}

func materializeGapSummary(
	proposed ProposedGapSummary,
	knownGaps map[string]Gap,
) StoryGapSummary {
	result := StoryGapSummary{
		Explanation: proposed.Explanation,
		GapIDs:      append([]string{}, proposed.GapIDs...),
		Gaps:        make([]Gap, 0, len(proposed.GapIDs)),
		Evidence:    []EvidenceRef{},
	}
	evidenceByID := make(map[string]EvidenceRef)
	for _, id := range proposed.GapIDs {
		gap := cloneGap(knownGaps[id])
		result.Gaps = append(result.Gaps, gap)
		addEvidence(evidenceByID, gap.Evidence)
	}
	result.Evidence = sortedEvidence(evidenceByID)
	return result
}

func cloneBeat(beat Beat) Beat {
	result := beat
	result.ComponentIDs = append([]string{}, beat.ComponentIDs...)
	result.SurfaceIDs = append([]string{}, beat.SurfaceIDs...)
	result.FlowStepIDs = append([]string{}, beat.FlowStepIDs...)
	result.Evidence = make([]EvidenceRef, len(beat.Evidence))
	for index, reference := range beat.Evidence {
		result.Evidence[index] = cloneEvidenceRef(reference)
	}
	return result
}

func cloneGap(gap Gap) Gap {
	result := gap
	result.Evidence = make([]EvidenceRef, len(gap.Evidence))
	for index, reference := range gap.Evidence {
		result.Evidence[index] = cloneEvidenceRef(reference)
	}
	return result
}

func componentsForIDs(ids []string, known map[string]Component) []Component {
	result := make([]Component, 0, len(ids))
	for _, id := range ids {
		result = append(result, known[id])
	}
	return result
}

func addStrings(destination map[string]struct{}, values []string) {
	for _, value := range values {
		destination[value] = struct{}{}
	}
}

func addEvidence(destination map[string]EvidenceRef, values []EvidenceRef) {
	for _, value := range values {
		destination[value.ID] = cloneEvidenceRef(value)
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

func sortedEvidence(values map[string]EvidenceRef) []EvidenceRef {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]EvidenceRef, 0, len(ids))
	for _, id := range ids {
		result = append(result, cloneEvidenceRef(values[id]))
	}
	return result
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing json value")
		}
		return err
	}
	return nil
}
