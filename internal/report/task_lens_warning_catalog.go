package report

import (
	"fmt"
	"slices"

	"github.com/dvordrova/repomap/internal/tasklens"
)

var taskInvestigationWarningMessageIDs = map[tasklens.WarningCode]string{
	tasklens.WarningAnchorOmittedIrrelevant:        "main.task_lens.warning.anchor_omitted_irrelevant",
	tasklens.WarningAnchorRoleReplaced:             "main.task_lens.warning.anchor_role_replaced",
	tasklens.WarningAnchorExplanationReplaced:      "main.task_lens.warning.anchor_explanation_replaced",
	tasklens.WarningAreaTargetsFiltered:            "main.task_lens.warning.area_targets_filtered",
	tasklens.WarningAreaOmittedWithoutAnchor:       "main.task_lens.warning.area_omitted_without_anchor",
	tasklens.WarningAreasBounded:                   "main.task_lens.warning.areas_bounded",
	tasklens.WarningAreaCopyReplaced:               "main.task_lens.warning.area_copy_replaced",
	tasklens.WarningAreaFallbackAdded:              "main.task_lens.warning.area_fallback_added",
	tasklens.WarningJoinsBounded:                   "main.task_lens.warning.joins_bounded",
	tasklens.WarningJoinRejected:                   "main.task_lens.warning.join_rejected",
	tasklens.WarningHypothesesBounded:              "main.task_lens.warning.hypotheses_bounded",
	tasklens.WarningHypothesisRejected:             "main.task_lens.warning.hypothesis_rejected",
	tasklens.WarningHypothesisSupportCompleted:     "main.task_lens.warning.hypothesis_support_completed",
	tasklens.WarningHypothesisCopyReplaced:         "main.task_lens.warning.hypothesis_copy_replaced",
	tasklens.WarningHypothesisFallbackAdded:        "main.task_lens.warning.hypothesis_fallback_added",
	tasklens.WarningReproductionBounded:            "main.task_lens.warning.reproduction_bounded",
	tasklens.WarningReproductionRejected:           "main.task_lens.warning.reproduction_rejected",
	tasklens.WarningReproductionDuplicate:          "main.task_lens.warning.reproduction_duplicate",
	tasklens.WarningReproductionFallbackAdded:      "main.task_lens.warning.reproduction_fallback_added",
	tasklens.WarningVerificationBounded:            "main.task_lens.warning.verification_bounded",
	tasklens.WarningVerificationOutsideFrontier:    "main.task_lens.warning.verification_outside_frontier",
	tasklens.WarningVerificationRejected:           "main.task_lens.warning.verification_rejected",
	tasklens.WarningVerificationDuplicate:          "main.task_lens.warning.verification_duplicate",
	tasklens.WarningVerificationFallbackAdded:      "main.task_lens.warning.verification_fallback_added",
	tasklens.WarningVerificationAuthorityAdded:     "main.task_lens.warning.verification_authority_added",
	tasklens.WarningVerificationTestAuthorityAdded: "main.task_lens.warning.verification_test_authority_added",
	tasklens.WarningNextProbesBounded:              "main.task_lens.warning.next_probes_bounded",
	tasklens.WarningNextProbeRejected:              "main.task_lens.warning.next_probe_rejected",
	tasklens.WarningNextProbeFallbackAdded:         "main.task_lens.warning.next_probe_fallback_added",
	tasklens.WarningAttemptResponseSize:            "main.task_lens.warning.attempt_response_size",
	tasklens.WarningAttemptResponseSecret:          "main.task_lens.warning.attempt_response_secret",
	tasklens.WarningAttemptProviderFailed:          "main.task_lens.warning.attempt_provider_failed",
	tasklens.WarningAttemptResponseRejected:        "main.task_lens.warning.attempt_response_rejected",
	tasklens.WarningAttemptSparseEvidence:          "main.task_lens.warning.attempt_sparse_evidence",
	tasklens.WarningPackLocalPartial:               "main.task_lens.warning.pack_local_partial",
	tasklens.WarningPackModelPartial:               "main.task_lens.warning.pack_model_partial",
}

func taskInvestigationPresentationWarnings(
	diagnostics []tasklens.WarningDiagnostic,
) []TaskInvestigationPresentationWarning {
	if len(diagnostics) == 0 {
		return nil
	}
	projected := make([]TaskInvestigationPresentationWarning, len(diagnostics))
	for index, diagnostic := range diagnostics {
		projected[index] = TaskInvestigationPresentationWarning{
			MessageID: taskInvestigationWarningMessageIDs[diagnostic.Code],
			Index:     diagnostic.Index,
		}
	}
	return projected
}

// taskInvestigationWarningDiagnostics reconstructs the product-owned warning
// identities from the same typed reducer that produced the saved canonical
// warning strings. It never interprets English prose. Accepted and partially
// rejected responses remain replayable by the Task Lens artifact contract;
// fixed attempt/pack warnings use the producer's closed constant mapping.
func taskInvestigationWarningDiagnostics(
	bundle tasklens.Bundle,
	attempt tasklens.Attempt,
	pack tasklens.Pack,
	status tasklens.Status,
) ([]tasklens.WarningDiagnostic, error) {
	var attemptWarnings []string
	var attemptDiagnostics []tasklens.WarningDiagnostic
	appendEmission := func(emission tasklens.WarningEmission) {
		attemptWarnings = append(attemptWarnings, emission.Raw)
		attemptDiagnostics = append(attemptDiagnostics, emission.Diagnostic)
	}
	if omitted, ok := tasklens.RawResponseOmissionEmission(
		attempt.RawResponseOmittedReason,
	); ok {
		appendEmission(omitted)
	}

	switch attempt.State {
	case "accepted", "accepted_with_rejections":
		proposal, err := tasklens.DecodeProposal([]byte(attempt.RawResponse))
		if err != nil {
			return nil, fmt.Errorf(
				"task investigation warning replay: decode accepted response: %w",
				err,
			)
		}
		_, warnings, diagnostics, err := tasklens.ReduceProposalWithDiagnostics(
			bundle,
			proposal,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"task investigation warning replay: accepted response: %w",
				err,
			)
		}
		attemptWarnings = append(attemptWarnings, warnings...)
		attemptDiagnostics = append(attemptDiagnostics, diagnostics...)
	case "rejected":
		if attempt.RawResponseOmittedReason == "" {
			proposal, decodeErr := tasklens.DecodeProposal([]byte(attempt.RawResponse))
			if decodeErr == nil {
				_, warnings, diagnostics, _ := tasklens.ReduceProposalWithDiagnostics(
					bundle,
					proposal,
				)
				attemptWarnings = append(attemptWarnings, warnings...)
				attemptDiagnostics = append(attemptDiagnostics, diagnostics...)
			}
		}
		attemptWarning, _ := tasklens.AttemptStateWarningEmission(attempt.State)
		appendEmission(attemptWarning)
	case "provider_failed":
		attemptWarning, _ := tasklens.AttemptStateWarningEmission(attempt.State)
		appendEmission(attemptWarning)
	case "skipped_insufficient_evidence":
		attemptWarning, _ := tasklens.AttemptStateWarningEmission(attempt.State)
		appendEmission(attemptWarning)
	case "skipped_offline", "skipped_local_complete":
	default:
		return nil, fmt.Errorf("task investigation warning replay: invalid attempt state")
	}
	if !slices.Equal(attemptWarnings, attempt.Warnings) ||
		len(attemptWarnings) != len(attemptDiagnostics) {
		return nil, fmt.Errorf(
			"task investigation warning replay does not match the saved attempt",
		)
	}

	raw := make([]string, 0, len(attemptWarnings)+len(pack.Warnings))
	diagnostics := make(
		[]tasklens.WarningDiagnostic,
		0,
		len(attemptDiagnostics)+len(pack.Warnings),
	)
	seen := make(map[string]struct{}, cap(raw))
	appendPair := func(message string, diagnostic tasklens.WarningDiagnostic) {
		if _, duplicate := seen[message]; duplicate {
			return
		}
		seen[message] = struct{}{}
		raw = append(raw, message)
		diagnostics = append(diagnostics, diagnostic)
	}
	for index, message := range attemptWarnings {
		appendPair(message, attemptDiagnostics[index])
	}
	if len(pack.Warnings) > 0 {
		emission := tasklens.PartialPackWarningEmission(attempt.State)
		if len(pack.Warnings) != 1 || pack.Warnings[0] != emission.Raw {
			return nil, fmt.Errorf(
				"task investigation pack warning does not match structural state",
			)
		}
		appendPair(emission.Raw, emission.Diagnostic)
	}
	if !slices.Equal(raw, status.Warnings) {
		return nil, fmt.Errorf(
			"task investigation warning replay does not match saved status",
		)
	}
	return diagnostics, nil
}
