package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/studymap"
)

const (
	maxDecisionTraceArtifactBytes = 4 << 20
	maxDecisionTraceReportBytes   = 32 << 20
	maxDecisionTraceReasonCodes   = 6
	maxDecisionTraceReasonBytes   = 64
)

type decisionStudyDirectionAttempt struct {
	Version              int                                    `json:"version"`
	DirectionDiagnostics *studymap.DirectionProposalDiagnostics `json:"direction_diagnostics,omitempty"`
}

type decisionStudyReviewArtifact struct {
	Version   int                          `json:"version"`
	Attempts  []decisionStudyReviewAttempt `json:"attempts,omitempty"`
	Reduction struct {
		Version  int                  `json:"version"`
		Proposed int                  `json:"proposed"`
		Reviewed int                  `json:"reviewed"`
		Selected int                  `json:"selected"`
		Issues   []decisionStudyIssue `json:"issues,omitempty"`
	} `json:"reduction"`
}

type decisionStudyReviewAttempt struct {
	ValidationState string `json:"validation_state"`
	IssueCode       string `json:"issue_code,omitempty"`
}

type decisionStudyIssue struct {
	Code string `json:"code"`
}

var knownStudyDecisionReasons = map[string]struct{}{
	"decode_candidate":                              {},
	"direction_invalid_after_review":                {},
	"direction_invalid_before_review":               {},
	"duplicate_direction_id":                        {},
	"fewer_than_three_direct_or_supporting_anchors": {},
	"invalid_anchor_selection":                      {},
	"invalid_anchor_count":                          {},
	"duplicate_anchor_ids":                          {},
	"invalid_anchor_id":                             {},
	"invalid_candidate":                             {},
	"invalid_object_ids":                            {},
	"invalid_reading_anchor_count":                  {},
	"invalid_reading_anchors":                       {},
	"invalid_reading_copy":                          {},
	"invalid_search_query":                          {},
	"learning_outcome_scope_broader":                {},
	"model_direction_id":                            {},
	"production_or_operational_anchor_missing":      {},
	"question_scope_broader":                        {},
	"reading_anchor_mismatch":                       {},
	"review_anchor_missing":                         {},
	"review_anchor_unknown":                         {},
	"review_bundle_build_failed":                    {},
	"review_bundle_encode_failed":                   {},
	"review_direction_id_invalid":                   {},
	"review_direction_mismatch":                     {},
	"review_direction_unknown":                      {},
	"review_duplicate":                              {},
	"review_malformed":                              {},
	"review_missing":                                {},
	"review_request_plan_failed":                    {},
	"too_many_search_queries":                       {},
	"unsupported_runtime_order":                     {},
}

// writePublicationTrace projects already-saved bounded stage outcomes into
// ordinary progress output. It deliberately prints only counts and stable
// codes: provider prose, source contents, paths, IDs, and diagnostic details
// remain in their owning artifacts.
func writePublicationTrace(
	writer io.Writer,
	runDir string,
	data *report.ReportData,
	cacheDisabled bool,
	authorityMode string,
	requestedFlows int,
) {
	if writer == nil || data == nil {
		return
	}
	cacheMode := "enabled"
	if cacheDisabled {
		cacheMode = "disabled"
	}
	fmt.Fprintf(writer, "repomap: publication decision summary (bounded counts only): cache=%s\n", cacheMode)
	writeOrientationDecision(writer, data)
	writeFlowExpansionDecision(writer, data, requestedFlows)
	writeResearchDecision(writer, data)
	writeSurfaceDecision(writer, data)
	writeArchitectureDecision(writer, data)
	writeStudyDecision(writer, runDir, data)
	writeFinalPublicationDecision(writer, data, authorityMode)
}

func writeOrientationDecision(writer io.Writer, data *report.ReportData) {
	found := len(data.CandidateDirections)
	accepted := 0
	rejected := 0
	for _, direction := range data.CandidateDirections {
		switch direction.Disposition {
		case "accepted":
			accepted++
		case "rejected":
			rejected++
		}
	}
	if data.Run != nil {
		found = max(found, data.Run.CandidateDirectionCount)
		accepted = max(accepted, data.Run.AcceptedDirectionCount)
		rejected = max(rejected, data.Run.RejectedDirectionCount)
	}
	found = max(found, accepted+rejected)
	pending := max(found-accepted-rejected, 0)
	fmt.Fprintf(
		writer,
		"repomap: decision orientation: found=%d accepted=%d rejected=%d unresolved=%d\n",
		found, accepted, rejected, pending,
	)
}

func writeFlowExpansionDecision(
	writer io.Writer,
	data *report.ReportData,
	requested int,
) {
	eligible := 0
	for _, direction := range data.CandidateDirections {
		if direction.Disposition == "accepted" {
			eligible++
		}
	}
	if data.Run != nil {
		eligible = max(eligible, data.Run.AcceptedDirectionCount)
	}
	expanded := len(data.Flows)
	state := "completed"
	switch {
	case requested <= 0:
		state = "not_requested"
	case eligible == 0:
		state = "no_accepted_direction"
	case expanded == 0:
		state = "no_expansion_published"
	case expanded < min(requested, eligible):
		state = "partial"
	}
	fmt.Fprintf(
		writer,
		"repomap: decision direction expansion: requested=%d eligible=%d expanded=%d not_expanded=%d state=%s\n",
		max(requested, 0),
		eligible,
		expanded,
		max(eligible-expanded, 0),
		state,
	)
}

func writeResearchDecision(writer io.Writer, data *report.ReportData) {
	state := data.ModelResearch
	if state == nil {
		fmt.Fprintln(writer, "repomap: decision targeted research: selected=0 skipped=0 state=not_run")
		return
	}
	if err := state.Validate(); err != nil ||
		len(state.SkippedRounds) > state.Policy.MaxTargetedRounds {
		fmt.Fprintln(writer, "repomap: decision targeted research: state=invalid")
		return
	}
	validatedFindings := 0
	rejectedFindings := 0
	newFacts := 0
	frontiers := 0
	outcomes := make([]string, 0, len(state.Rounds))
	for _, round := range state.Rounds {
		validatedFindings += len(round.ValidatedFindings)
		rejectedFindings += len(round.RejectedFindings)
		newFacts += round.NewGroundedFactsCount
		frontiers += len(round.UnresolvedFrontiers)
		outcomes = append(outcomes, knownResearchOutcome(string(round.Status)))
	}
	skipReasons := make([]string, 0, len(state.SkippedRounds))
	for _, round := range state.SkippedRounds {
		skipReasons = append(skipReasons, knownResearchSkipReason(round.Gate.Reason))
	}
	fmt.Fprintf(
		writer,
		"repomap: decision targeted research: selected=%d skipped=%d validated_findings=%d rejected_findings=%d new_grounded_facts=%d unresolved_frontiers=%d outcomes=%s skip_reasons=%s\n",
		len(state.Rounds),
		len(state.SkippedRounds),
		validatedFindings,
		rejectedFindings,
		newFacts,
		frontiers,
		formatDecisionReasonCounts(outcomes),
		formatDecisionReasonCounts(skipReasons),
	)
}

func writeSurfaceDecision(writer io.Writer, data *report.ReportData) {
	surfaces := data.DiscoveredSurfaces
	genericScheduled := false
	found := 0
	if data.Run != nil {
		genericScheduled = data.Run.SurfaceDiscoveryRan
		found = data.Run.SurfaceDiscoveryCount
	}
	if surfaces == nil {
		fmt.Fprintf(
			writer,
			"repomap: decision surfaces: generic_scheduled=%t found=%d published=0 hidden=%d catalog=unavailable\n",
			genericScheduled,
			found,
			found,
		)
		return
	}
	published := len(surfaces.Triggers)
	found = max(found, surfaces.TotalCount, published)
	hidden := max(found-published, 0)
	fmt.Fprintf(
		writer,
		"repomap: decision surfaces: generic_scheduled=%t found=%d published=%d hidden=%d application=%d unavailable=%d unresolved_handlers=%d packages=%d functions=%d package_diagnostics=%d budgets=%s\n",
		genericScheduled,
		found,
		published,
		hidden,
		surfaces.ApplicationCount,
		surfaces.UnavailableSurfaceCount,
		surfaces.UnresolvedHandlerCount,
		surfaces.PackagesInspected,
		surfaces.FunctionsInspected,
		surfaces.PackageDiagnosticCount,
		formatDecisionReasonCounts(surfaces.BudgetsReached),
	)
}

func writeArchitectureDecision(writer io.Writer, data *report.ReportData) {
	status := data.ArchitectureSynthesis
	canvas := data.ArchitectureCanvas
	if canvas == nil {
		state := "not_run"
		reasons := []string(nil)
		if status != nil {
			state = status.State
			reasons = append(reasons, status.ErrorCode, status.FallbackReason)
		}
		fmt.Fprintf(
			writer,
			"repomap: decision architecture: state=%s published=0 reasons=%s\n",
			safeDecisionCode(state),
			formatDecisionReasonCounts(reasons),
		)
		return
	}
	members := 0
	for _, component := range canvas.Components {
		members += len(component.Members)
	}
	outcome := string(canvas.ValidationOutcome)
	source := string(canvas.ArchitectureSource)
	reasons := []string(nil)
	normalizations := len(canvas.Normalizations)
	if status != nil {
		if status.ProposalRejected {
			outcome = "rejected"
		} else if status.ProposalNormalized {
			outcome = "accepted_normalized"
		} else if status.ProposalAccepted {
			outcome = "accepted"
		}
		if status.ArchitectureSource != "" {
			source = status.ArchitectureSource
		}
		normalizations = status.NormalizationCount
		reasons = append(reasons, status.ErrorCode, status.FallbackReason)
	}
	fmt.Fprintf(
		writer,
		"repomap: decision architecture: anchors=%d members=%d grouped_components=%d groups=%d surfaces=%d outcome=%s source=%s normalizations=%d reasons=%s\n",
		len(canvas.BehaviorAnchors),
		members,
		len(canvas.Components),
		len(canvas.Subsystems),
		len(canvas.Surfaces),
		safeDecisionCode(outcome),
		safeDecisionCode(source),
		normalizations,
		formatDecisionReasonCounts(reasons),
	)
}

func writeStudyDecision(writer io.Writer, runDir string, data *report.ReportData) {
	if data.StudyMap != nil {
		fmt.Fprintf(
			writer,
			"repomap: decision study shape: areas=%d canonical_directions=%d hidden_directions=%d\n",
			len(data.StudyMap.Shape),
			len(data.StudyMap.Directions),
			len(data.StudyMap.HiddenDirections),
		)
	}

	var directionAttempt decisionStudyDirectionAttempt
	if readDecisionTraceArtifact(
		filepath.Join(runDir, studymap.DirectionsAttemptFile),
		&directionAttempt,
	) && directionAttempt.DirectionDiagnostics != nil {
		diagnostics := directionAttempt.DirectionDiagnostics
		if validStudyDirectionAttempt(directionAttempt) {
			fmt.Fprintf(
				writer,
				"repomap: decision study drafts: received=%d accepted=%d rejected=%d reasons=%s\n",
				diagnostics.Received,
				diagnostics.Accepted,
				diagnostics.Rejected,
				formatStudyDecisionReasonCounts(diagnostics.Issues),
			)
		} else {
			fmt.Fprintln(writer, "repomap: decision study drafts: artifact=invalid")
		}
	}

	var reviews decisionStudyReviewArtifact
	reviewAvailable := readDecisionTraceArtifact(
		filepath.Join(runDir, studyMapReviewsFile),
		&reviews,
	)
	if reviewAvailable {
		if validStudyReviewArtifact(reviews) {
			fmt.Fprintf(
				writer,
				"repomap: decision study reviews: proposed=%d reviewed=%d rejected=%d selected=%d reduced_after_review=%d reasons=%s\n",
				reviews.Reduction.Proposed,
				reviews.Reduction.Reviewed,
				reviews.Reduction.Proposed-reviews.Reduction.Reviewed,
				reviews.Reduction.Selected,
				reviews.Reduction.Reviewed-reviews.Reduction.Selected,
				formatStudyReviewReasonCounts(reviews.Reduction.Issues),
			)
			fmt.Fprintf(
				writer,
				"repomap: decision study review attempts: outcomes=%s reasons=%s\n",
				formatStudyReviewAttemptOutcomes(reviews.Attempts),
				formatStudyReviewAttemptReasons(reviews.Attempts),
			)
		} else {
			fmt.Fprintln(writer, "repomap: decision study reviews: artifact=invalid")
		}
	}

	state := "not_run"
	candidates := 0
	selected := 0
	failure := "none"
	if data.StudyPublication != nil {
		state = data.StudyPublication.State
		candidates = data.StudyPublication.Candidates
		selected = data.StudyPublication.Selected
		if state == "failed" {
			failure = studyFailureCode(data.StudyPublication.FailureReason)
		}
	}
	published := 0
	hidden := 0
	projection := "none"
	if data.StudyMap != nil {
		published = len(data.StudyMap.Directions)
		hidden = len(data.StudyMap.HiddenDirections)
		projection = "canonical"
	} else if data.IncompleteStudy != nil {
		published = len(data.IncompleteStudy.Directions)
		projection = "incomplete"
	}
	fmt.Fprintf(
		writer,
		"repomap: decision study publication: state=%s failure=%s candidates=%d selected=%d not_selected=%d published=%d hidden=%d projection=%s\n",
		safeDecisionCode(state),
		failure,
		candidates,
		selected,
		max(candidates-selected, 0),
		published,
		hidden,
		projection,
	)
}

func writeFinalPublicationDecision(
	writer io.Writer,
	data *report.ReportData,
	authorityMode string,
) {
	architectureComponents := 0
	architectureSurfaces := 0
	if data.ArchitectureCanvas != nil {
		architectureComponents = len(data.ArchitectureCanvas.Components)
		architectureSurfaces = len(data.ArchitectureCanvas.Surfaces)
	}
	studyDirections := 0
	if data.StudyMap != nil {
		studyDirections = len(data.StudyMap.Directions)
	} else if data.IncompleteStudy != nil {
		studyDirections = len(data.IncompleteStudy.Directions)
	}
	operations := 0
	landmarks := 0
	if data.Operations != nil {
		operations = len(data.Operations.Paths)
		landmarks = len(data.Operations.Landmarks)
	}
	guidedTour := 0
	if data.GuidedTour != nil {
		guidedTour = 1
	}
	fmt.Fprintf(
		writer,
		"repomap: decision publication: mechanisms=%d topics=%d study=%d guided_tour=%d architecture_components=%d architecture_surfaces=%d operations=%d landmarks=%d source_targets=%d warnings=%d authority=%s\n",
		len(data.UserMechanisms),
		len(data.UserTopics),
		studyDirections,
		guidedTour,
		architectureComponents,
		architectureSurfaces,
		operations,
		landmarks,
		len(data.OpenablePaths),
		len(data.Warnings),
		safeDecisionCode(authorityMode),
	)
}

func validStudyDirectionAttempt(attempt decisionStudyDirectionAttempt) bool {
	diagnostics := attempt.DirectionDiagnostics
	if attempt.Version != 1 || diagnostics == nil ||
		diagnostics.Received <= 0 || diagnostics.Received > studymap.MaxCandidates ||
		diagnostics.Accepted < 0 || diagnostics.Rejected < 0 ||
		diagnostics.Accepted+diagnostics.Rejected != diagnostics.Received ||
		len(diagnostics.Issues) > diagnostics.Rejected ||
		len(diagnostics.Issues) > studymap.MaxCandidates {
		return false
	}
	for _, issue := range diagnostics.Issues {
		if issue.Position < 0 || issue.Position >= diagnostics.Received {
			return false
		}
	}
	return true
}

func validStudyReviewArtifact(artifact decisionStudyReviewArtifact) bool {
	reduction := artifact.Reduction
	return artifact.Version == 1 &&
		reduction.Version == studymap.ReviewReductionVersion &&
		reduction.Proposed >= 0 &&
		reduction.Proposed <= studymap.MaxCandidates &&
		reduction.Reviewed >= 0 &&
		reduction.Reviewed <= reduction.Proposed &&
		reduction.Selected >= 0 &&
		reduction.Selected <= reduction.Reviewed &&
		len(reduction.Issues) <= 2*studymap.MaxCandidates &&
		len(artifact.Attempts) <= reduction.Proposed
}

func formatStudyDecisionReasonCounts(issues []studymap.DirectionProposalIssue) string {
	values := make([]string, 0, min(len(issues), studymap.MaxCandidates))
	for index, issue := range issues {
		if index >= studymap.MaxCandidates {
			break
		}
		values = append(values, knownStudyDecisionReason(issue.Code))
	}
	return formatDecisionReasonCounts(values)
}

func formatStudyReviewReasonCounts(issues []decisionStudyIssue) string {
	const maxReviewIssues = 2 * studymap.MaxCandidates
	values := make([]string, 0, min(len(issues), maxReviewIssues))
	for index, issue := range issues {
		if index >= maxReviewIssues {
			break
		}
		values = append(values, knownStudyDecisionReason(issue.Code))
	}
	return formatDecisionReasonCounts(values)
}

func knownStudyDecisionReason(value string) string {
	if len(value) == 0 || len(value) > maxDecisionTraceReasonBytes {
		return "unknown_code"
	}
	if _, ok := knownStudyDecisionReasons[value]; !ok {
		return "unknown_code"
	}
	return value
}

func formatStudyReviewAttemptOutcomes(attempts []decisionStudyReviewAttempt) string {
	values := make([]string, 0, min(len(attempts), studymap.MaxCandidates))
	for index, attempt := range attempts {
		if index >= studymap.MaxCandidates {
			break
		}
		values = append(values, knownStudyReviewOutcome(attempt.ValidationState))
	}
	return formatDecisionReasonCounts(values)
}

func formatStudyReviewAttemptReasons(attempts []decisionStudyReviewAttempt) string {
	values := make([]string, 0, min(len(attempts), studymap.MaxCandidates))
	for index, attempt := range attempts {
		if index >= studymap.MaxCandidates {
			break
		}
		if attempt.IssueCode == "" {
			continue
		}
		values = append(values, knownStudyDecisionReason(attempt.IssueCode))
	}
	return formatDecisionReasonCounts(values)
}

func knownStudyReviewOutcome(value string) string {
	switch value {
	case "accepted", "rejected", "failed_provider", "canceled":
		return value
	default:
		return "unknown_state"
	}
}

func knownResearchOutcome(value string) string {
	switch modelresearch.RoundStatus(value) {
	case modelresearch.RoundPlanned,
		modelresearch.RoundSkipped,
		modelresearch.RoundCompleted,
		modelresearch.RoundRejected,
		modelresearch.RoundCached,
		modelresearch.RoundNoNewEvidence,
		modelresearch.RoundBudgetExhausted,
		modelresearch.RoundFailed:
		return value
	default:
		return "unknown_state"
	}
}

func knownResearchSkipReason(value string) string {
	switch value {
	case "runtime_only_frontier",
		"unknown_candidate_ids",
		"no_code_bearing_bounded_window",
		"no_new_exact_evidence",
		"no_bounded_local_evidence",
		"targeted_round_limit":
		return value
	default:
		return "unknown_code"
	}
}

func studyFailureCode(value string) string {
	const maxFailureInspectBytes = 4096
	if len(value) > maxFailureInspectBytes {
		value = value[:maxFailureInspectBytes]
	}
	value = strings.ToLower(value)
	for _, match := range []struct {
		fragment string
		code     string
	}{
		{"insufficient code anchors", "insufficient_code_anchors"},
		{"reviewed selection has", "insufficient_reviewed_directions"},
		{"invalid reading copy", "invalid_reading_copy"},
		{"invalid anchor", "invalid_anchor_selection"},
		{"unknown field", "unknown_field"},
		{"unexpected eof", "invalid_json"},
		{"invalid character", "invalid_json"},
		{"decode", "invalid_json"},
		{"provider", "provider_failure"},
		{"context canceled", "canceled"},
		{"deadline exceeded", "deadline_exceeded"},
	} {
		if strings.Contains(value, match.fragment) {
			return match.code
		}
	}
	return "local_stage_failure"
}

func readDecisionTraceArtifact(path string, target any) bool {
	raw, err := readBoundedRegularFile(path, maxDecisionTraceArtifactBytes)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, target) == nil
}

func readPublishedReportData(runDir string, fallback *report.ReportData) *report.ReportData {
	raw, err := readBoundedRegularFile(
		filepath.Join(runDir, "report.json"),
		maxDecisionTraceReportBytes,
	)
	if err != nil {
		return fallback
	}
	var data report.ReportData
	if err := json.Unmarshal(raw, &data); err != nil {
		return fallback
	}
	return &data
}

func formatDecisionReasonCounts(values []string) string {
	counts := make(map[string]int)
	for _, value := range values {
		code := safeDecisionCode(value)
		if code == "none" {
			continue
		}
		counts[code]++
	}
	if len(counts) == 0 {
		return "none"
	}
	codes := make([]string, 0, len(counts))
	for code := range counts {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	parts := make([]string, 0, min(len(codes), maxDecisionTraceReasonCodes)+1)
	other := 0
	for index, code := range codes {
		if index >= maxDecisionTraceReasonCodes {
			other += counts[code]
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", code, counts[code]))
	}
	if other > 0 {
		parts = append(parts, fmt.Sprintf("other=%d", other))
	}
	return strings.Join(parts, ",")
}

func safeDecisionCode(value string) string {
	if len(value) > maxDecisionTraceReasonBytes {
		return "invalid_code"
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "none"
	}
	for _, current := range value {
		if (current >= 'a' && current <= 'z') ||
			(current >= 'A' && current <= 'Z') ||
			(current >= '0' && current <= '9') ||
			current == '_' || current == '-' || current == '.' || current == ':' {
			continue
		}
		return "invalid_code"
	}
	return value
}
