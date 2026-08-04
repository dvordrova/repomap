package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/tasklens"
)

const maxTaskInvestigationArtifactBytes = 4 << 20

// TaskInvestigationWorkspace is a presentation-only, task-first projection of
// a validated Task Investigation Pack. Anchor and relation references use
// bounded indexes so opaque reducer/evidence identifiers are not exposed in
// the user-facing report.
type TaskInvestigationWorkspace struct {
	TaskID               string                          `json:"task_id"`
	Repository           string                          `json:"repository"`
	Task                 string                          `json:"task"`
	State                string                          `json:"state"`
	Sufficient           bool                            `json:"sufficient"`
	Locality             tasklens.Locality               `json:"locality"`
	Profile              tasklens.TaskProfile            `json:"task_profile"`
	RoleContract         tasklens.RoleContract           `json:"role_contract"`
	RoleCoverage         tasklens.RoleCoverage           `json:"-"`
	VerificationFrontier tasklens.VerificationFrontier   `json:"-"`
	CheapExit            tasklens.CheapExitDecision      `json:"cheap_exit"`
	Interpretation       TaskInvestigationInterpretation `json:"interpretation"`
	LikelyAreas          []TaskInvestigationArea         `json:"likely_areas"`
	Anchors              []TaskInvestigationAnchor       `json:"anchors"`
	EvidenceJoins        []TaskInvestigationJoin         `json:"evidence_joins,omitempty"`
	WorkingHypothesis    []TaskInvestigationHypothesis   `json:"working_hypothesis"`
	ReproduceOrObserve   []TaskInvestigationGuidance     `json:"reproduce_or_observe"`
	Verify               TaskInvestigationVerification   `json:"verify"`
	NextProbes           []TaskInvestigationProbe        `json:"next_probes,omitempty"`
	StagesSkipped        []string                        `json:"stages_skipped"`
	Budget               tasklens.Budgets                `json:"budget"`
	Provider             tasklens.ProviderMetrics        `json:"provider"`
	CapturedRevision     string                          `json:"captured_revision"`
	Warnings             []string                        `json:"warnings,omitempty"`
	// PresentationWarnings is populated only on the transient HTML render
	// copy. Canonical report.json and Task Lens artifacts retain Warnings
	// byte-for-byte and never persist these catalog addresses.
	PresentationWarnings         []TaskInvestigationPresentationWarning `json:"presentation_warnings,omitempty"`
	warningDiagnostics           []tasklens.WarningDiagnostic
	BundleSHA256                 string `json:"bundle_sha256"`
	AttemptSHA256                string `json:"attempt_sha256"`
	PackSHA256                   string `json:"pack_sha256"`
	StatusSHA256                 string `json:"status_sha256"`
	RetrievalTraceSHA256         string `json:"retrieval_trace_sha256"`
	RetrievalTraceMarkdownSHA256 string `json:"retrieval_trace_markdown_sha256"`
	RepositoryStateSHA256        string `json:"-"`
	// MaterialPaths retains every bounded source that informed the provider
	// bundle, including unselected anchors and manifest-derived module facts.
	// It is authority input, not presentation data.
	MaterialPaths []string `json:"-"`
}

// TaskInvestigationPresentationWarning is a report-local catalog projection
// of a semantic-neutral tasklens warning code. It exists only in the transient
// HTML render copy; canonical report.json retains the legacy raw warning.
type TaskInvestigationPresentationWarning struct {
	MessageID string `json:"message_id"`
	Index     int    `json:"index,omitempty"`
}

type TaskInvestigationInterpretation struct {
	Restatement      string            `json:"restatement"`
	Kind             tasklens.TaskKind `json:"task_kind"`
	Observable       string            `json:"observable_or_outcome"`
	FoundTerms       []string          `json:"repository_terms_found,omitempty"`
	UserProvidedOnly []string          `json:"user_provided_only_terms,omitempty"`
}

type TaskInvestigationArea struct {
	Label         string `json:"label"`
	Why           string `json:"why"`
	AnchorIndexes []int  `json:"anchor_indexes"`
}

type TaskInvestigationAnchor struct {
	Path      string               `json:"path"`
	Symbol    string               `json:"symbol"`
	Section   string               `json:"section,omitempty"`
	Package   string               `json:"package,omitempty"`
	Role      tasklens.AnchorRole  `json:"role"`
	StartLine int                  `json:"start_line"`
	EndLine   int                  `json:"end_line"`
	Scope     tasklens.SourceScope `json:"source_scope"`
	Why       string               `json:"why"`
	Source    SourceSnippet        `json:"source"`
}

type TaskInvestigationJoin struct {
	LeftAnchor           int                  `json:"left_anchor"`
	RightAnchor          int                  `json:"right_anchor"`
	SupportAnchorIndexes []int                `json:"support_anchor_indexes,omitempty"`
	Kind                 string               `json:"kind"`
	Support              tasklens.SupportType `json:"support"`
	Explanation          string               `json:"explanation"`
	Scope                string               `json:"scope_non_guarantees"`
}

type TaskInvestigationHypothesis struct {
	Status               tasklens.HypothesisStatus `json:"status"`
	Text                 string                    `json:"text"`
	SupportAnchorIndexes []int                     `json:"support_anchor_indexes,omitempty"`
}

type TaskInvestigationGuidance struct {
	Text                 string                     `json:"text"`
	Authority            tasklens.GuidanceAuthority `json:"authority"`
	SupportAnchorIndexes []int                      `json:"support_anchor_indexes,omitempty"`
}

type TaskInvestigationVerification struct {
	Effect string                      `json:"effect_to_observe"`
	Steps  []TaskInvestigationGuidance `json:"steps"`
}

type TaskInvestigationProbe struct {
	Action        tasklens.ProbeAction `json:"action"`
	AnchorIndexes []int                `json:"anchor_indexes"`
	Text          string               `json:"text"`
}

func readTaskInvestigation(runDir, sourceRoot string) (*TaskInvestigationWorkspace, string) {
	present := false
	for _, name := range []string{
		tasklens.BundleFile,
		tasklens.AttemptFile,
		tasklens.PackFile,
		tasklens.StatusFile,
		tasklens.TraceJSONFile,
		tasklens.TraceMarkdownFile,
	} {
		if _, err := os.Lstat(filepath.Join(runDir, name)); err == nil {
			present = true
		} else if !os.IsNotExist(err) {
			return nil, "task investigation unavailable: cannot inspect saved artifacts"
		}
	}
	if !present {
		return nil, ""
	}

	packPath := filepath.Join(runDir, tasklens.PackFile)
	bundleRaw, err := readTaskInvestigationArtifact(filepath.Join(runDir, tasklens.BundleFile))
	if err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: %v", err)
	}
	attemptRaw, err := readTaskInvestigationArtifact(filepath.Join(runDir, tasklens.AttemptFile))
	if err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: %v", err)
	}
	packRaw, err := readTaskInvestigationArtifact(packPath)
	if err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: %v", err)
	}
	statusRaw, err := readTaskInvestigationArtifact(filepath.Join(runDir, tasklens.StatusFile))
	if err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: %v", err)
	}
	traceRaw, err := readTaskInvestigationArtifact(filepath.Join(runDir, tasklens.TraceJSONFile))
	if err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: %v", err)
	}
	traceMarkdownRaw, err := readTaskInvestigationArtifact(filepath.Join(runDir, tasklens.TraceMarkdownFile))
	if err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: %v", err)
	}

	var bundle tasklens.Bundle
	if err := decodeTaskInvestigationArtifact(bundleRaw, &bundle); err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: bundle: %v", err)
	}
	if err := bundle.Validate(); err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: bundle: %v", err)
	}
	if sourceRoot != "" {
		if err := tasklens.VerifyBundleSources(sourceRoot, bundle); err != nil {
			return nil, fmt.Sprintf("task investigation unavailable: source replay: %v", err)
		}
	}
	var attempt tasklens.Attempt
	if err := decodeTaskInvestigationArtifact(attemptRaw, &attempt); err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: attempt: %v", err)
	}
	pack, err := tasklens.DecodePack(packRaw)
	if err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: pack: %v", err)
	}
	var status tasklens.Status
	if err := decodeTaskInvestigationArtifact(statusRaw, &status); err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: status: %v", err)
	}
	var trace tasklens.RetrievalTrace
	if err := decodeTaskInvestigationArtifact(traceRaw, &trace); err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: retrieval trace: %v", err)
	}
	if err := trace.ValidateAgainstBundle(bundle); err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: retrieval trace: %v", err)
	}
	expectedTraceMarkdown, err := tasklens.RenderRetrievalTraceMarkdown(trace)
	if err != nil || string(traceMarkdownRaw) != expectedTraceMarkdown {
		return nil, "task investigation unavailable: retrieval trace markdown does not replay"
	}
	if err := validateTaskInvestigationArtifacts(
		bundle,
		attempt,
		pack,
		status,
		attemptRaw,
		packRaw,
		traceRaw,
		traceMarkdownRaw,
	); err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: %v", err)
	}

	workspace, err := projectTaskInvestigation(bundle, attempt, pack, status)
	if err != nil {
		return nil, fmt.Sprintf("task investigation unavailable: %v", err)
	}
	workspace.BundleSHA256 = tasklens.SHA256(bundleRaw)
	workspace.AttemptSHA256 = tasklens.SHA256(attemptRaw)
	workspace.PackSHA256 = tasklens.SHA256(packRaw)
	workspace.StatusSHA256 = tasklens.SHA256(statusRaw)
	workspace.RetrievalTraceSHA256 = tasklens.SHA256(traceRaw)
	workspace.RetrievalTraceMarkdownSHA256 = tasklens.SHA256(traceMarkdownRaw)
	workspace.MaterialPaths = append([]string(nil), bundle.AllowedPaths...)
	return workspace, ""
}

func readTaskInvestigationArtifact(artifactPath string) ([]byte, error) {
	directory, name := filepath.Dir(artifactPath), filepath.Base(artifactPath)
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("cannot open artifact directory")
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s is missing", filepath.Base(artifactPath))
		}
		return nil, fmt.Errorf("cannot inspect %s", filepath.Base(artifactPath))
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxTaskInvestigationArtifactBytes {
		return nil, fmt.Errorf("%s is not a bounded regular file", filepath.Base(artifactPath))
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s", filepath.Base(artifactPath))
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, fmt.Errorf("%s changed while it was opened", filepath.Base(artifactPath))
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxTaskInvestigationArtifactBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s", filepath.Base(artifactPath))
	}
	if len(raw) == 0 || len(raw) > maxTaskInvestigationArtifactBytes {
		return nil, fmt.Errorf("%s is not a bounded regular file", filepath.Base(artifactPath))
	}
	return raw, nil
}

func decodeTaskInvestigationArtifact(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode saved artifact: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err == nil {
		return fmt.Errorf("saved artifact contains multiple json values")
	} else if err != io.EOF {
		return fmt.Errorf("saved artifact has trailing data: %w", err)
	}
	return nil
}

func validateTaskInvestigationArtifacts(
	bundle tasklens.Bundle,
	attempt tasklens.Attempt,
	pack tasklens.Pack,
	status tasklens.Status,
	attemptRaw []byte,
	packRaw []byte,
	traceRaw []byte,
	traceMarkdownRaw []byte,
) error {
	bundleSHA, err := tasklens.BundleHash(bundle)
	if err != nil {
		return err
	}
	if bundle.ID != pack.ID || bundle.Repository != pack.Repository ||
		bundle.Locality != pack.Locality || bundleSHA != pack.BundleSHA256 ||
		bundle.Budgets != pack.Budgets || !slices.Equal(bundle.StagesSkipped, pack.StagesSkipped) {
		return fmt.Errorf("saved pack does not match its bounded bundle")
	}
	if err := tasklens.ValidatePackAgainstBundle(bundle, pack); err != nil {
		return fmt.Errorf("saved pack failed reducer replay: %w", err)
	}
	if err := validateTaskPackAgainstBundle(bundle, pack); err != nil {
		return err
	}
	stablePrompt, err := tasklens.StablePromptJSON(bundle)
	if err != nil {
		return fmt.Errorf("rebuild stable task prompt: %w", err)
	}
	if attempt.Version != tasklens.AttemptVersion ||
		attempt.BundleSHA256 != bundleSHA ||
		attempt.PromptVersion != tasklens.PromptVersion ||
		attempt.PromptSHA256 != tasklens.SHA256(stablePrompt) ||
		!validTaskAttemptState(attempt.State) ||
		attempt.Provider.Calls < 0 || attempt.Provider.Calls > 1 ||
		attempt.Provider.TransportAttempts < attempt.Provider.Calls ||
		attempt.Provider.TransportAttempts > tasklens.MaxModelCalls ||
		attempt.Provider.RequestBytes < 0 || attempt.Provider.ResponseBytes < 0 ||
		attempt.Provider.InputTokens < 0 || attempt.Provider.OutputTokens < 0 ||
		attempt.Provider.PromptCacheHitTokens < 0 || attempt.Provider.PromptCacheMissTokens < 0 ||
		attempt.Provider.LatencyMillis < 0 {
		return fmt.Errorf("saved attempt is invalid")
	}
	expectedState, expectsProvider := taskStatusStateForAttempt(attempt.State)
	if expectsProvider != (attempt.Provider.Calls == 1) ||
		(!expectsProvider && attempt.Provider.TransportAttempts != 0) ||
		(!expectsProvider && attempt.Provider != (tasklens.ProviderMetrics{})) ||
		(expectsProvider && attempt.Provider.RequestBytes == 0) ||
		((attempt.State == "accepted" || attempt.State == "accepted_with_rejections" || attempt.State == "rejected") &&
			attempt.Provider.ResponseBytes == 0) ||
		((attempt.State == "rejected" || attempt.State == "provider_failed") && strings.TrimSpace(attempt.ReductionError) == "") ||
		((attempt.State == "accepted" || attempt.State == "accepted_with_rejections") && attempt.ReductionError != "") ||
		(strings.HasPrefix(attempt.State, "skipped_") &&
			(attempt.ResponseSHA256 != "" || attempt.RawResponse != "" || attempt.RawResponseOmittedReason != "" ||
				attempt.ReductionError != "" || len(attempt.Warnings) != btoi(attempt.State == "skipped_insufficient_evidence"))) ||
		(attempt.State == "skipped_insufficient_evidence" && len(bundle.Anchors) >= tasklens.PreferredMinVisibleAnchors) {
		return fmt.Errorf("saved attempt state does not match provider accounting")
	}
	if err := validateTaskAttemptResponseBinding(bundle, attempt, pack); err != nil {
		return err
	}
	expectedSufficient := TaskInvestigationPackSufficient(pack, attempt.State)
	if status.Version != tasklens.StatusVersion || strings.TrimSpace(status.State) == "" ||
		status.State != expectedState || status.Sufficient != expectedSufficient || status.TaskID != pack.ID ||
		status.BundleSHA256 != bundleSHA || status.AttemptSHA256 != tasklens.SHA256(attemptRaw) ||
		status.PackSHA256 != tasklens.SHA256(packRaw) ||
		status.RetrievalTraceSHA256 != tasklens.SHA256(traceRaw) ||
		status.RetrievalTraceMarkdownSHA256 != tasklens.SHA256(traceMarkdownRaw) ||
		status.CapturedRevision != pack.Repository.Revision || status.TreeHash != pack.Repository.TreeHash ||
		status.Locality != pack.Locality || status.Provider != attempt.Provider ||
		!reflect.DeepEqual(status.CheapExit, bundle.CheapExit) ||
		status.Budgets != pack.Budgets || !slices.Equal(status.StagesSkipped, pack.StagesSkipped) ||
		!slices.Equal(status.Warnings, taskArtifactWarnings(attempt.Warnings, pack.Warnings)) {
		return fmt.Errorf("saved status does not match the task artifacts")
	}
	return nil
}

func validateTaskAttemptResponseBinding(
	bundle tasklens.Bundle,
	attempt tasklens.Attempt,
	pack tasklens.Pack,
) error {
	switch attempt.State {
	case "accepted", "accepted_with_rejections":
		if attempt.Provider.ResponseBytes <= 0 || !validTaskSHA256(attempt.ResponseSHA256) {
			return fmt.Errorf("saved substantive attempt lacks response identity")
		}
		if err := validateStoredResponseIdentity(attempt, true); err != nil {
			return err
		}
		if attempt.RawResponse == "" || attempt.RawResponseOmittedReason != "" {
			return fmt.Errorf("saved accepted response is not available for reducer replay")
		}
		proposal, err := tasklens.DecodeProposal([]byte(attempt.RawResponse))
		if err != nil {
			return fmt.Errorf("saved accepted response does not decode: %w", err)
		}
		reduced, warnings, err := tasklens.ReduceProposal(bundle, proposal)
		if err != nil {
			return fmt.Errorf("saved accepted response does not reduce: %w", err)
		}
		reduced, _ = FinalizeTaskInvestigationPack(reduced, attempt.State)
		if !reflect.DeepEqual(reduced, pack) {
			return fmt.Errorf("saved accepted response does not reproduce its pack")
		}
		if attempt.State == "accepted" && len(warnings) != 0 {
			return fmt.Errorf("saved accepted response has local rejections")
		}
		if attempt.State == "accepted_with_rejections" && len(warnings) == 0 {
			return fmt.Errorf("saved partial response has no local rejection")
		}
		if !slices.Equal(attempt.Warnings, warnings) {
			return fmt.Errorf("saved attempt warnings do not match reducer output")
		}
		return nil

	case "rejected":
		if attempt.Provider.ResponseBytes <= 0 || !validTaskSHA256(attempt.ResponseSHA256) {
			return fmt.Errorf("saved rejected attempt lacks response identity")
		}
		if err := validateStoredResponseIdentity(attempt, true); err != nil {
			return err
		}
		var reductionWarnings []string
		if attempt.RawResponseOmittedReason != "" {
			if attempt.ReductionError != tasklens.RawResponseOmissionReductionError(attempt.RawResponseOmittedReason) {
				return fmt.Errorf("saved omitted response does not reproduce its rejection")
			}
		} else {
			proposal, decodeErr := tasklens.DecodeProposal([]byte(attempt.RawResponse))
			var reductionErr error
			if decodeErr != nil {
				reductionErr = decodeErr
			} else {
				_, reductionWarnings, reductionErr = tasklens.ReduceProposal(bundle, proposal)
			}
			if reductionErr == nil || attempt.ReductionError != reductionErr.Error() {
				return fmt.Errorf("saved rejected response does not reproduce its rejection")
			}
		}
		expectedWarnings := make([]string, 0, len(reductionWarnings)+2)
		if warning := tasklens.RawResponseOmissionWarning(attempt.RawResponseOmittedReason); warning != "" {
			expectedWarnings = append(expectedWarnings, warning)
		}
		expectedWarnings = append(expectedWarnings, reductionWarnings...)
		expectedWarnings = append(expectedWarnings, tasklens.AttemptWarningResponseRejected)
		if !slices.Equal(attempt.Warnings, expectedWarnings) {
			return fmt.Errorf("saved rejected attempt warnings do not match reducer output")
		}
		return validateLocalFallbackPack(bundle, attempt.State, pack)

	case "provider_failed":
		if err := validateStoredResponseIdentity(attempt, attempt.Provider.ResponseBytes > 0); err != nil {
			return err
		}
		if attempt.ReductionError != tasklens.ReductionErrorProviderFailed {
			return fmt.Errorf("saved provider failure has an invalid reduction error")
		}
		expectedWarnings := make([]string, 0, 2)
		if warning := tasklens.RawResponseOmissionWarning(attempt.RawResponseOmittedReason); warning != "" {
			expectedWarnings = append(expectedWarnings, warning)
		}
		expectedWarnings = append(expectedWarnings, tasklens.AttemptWarningProviderFailed)
		if !slices.Equal(attempt.Warnings, expectedWarnings) {
			return fmt.Errorf("saved provider failure warnings are invalid")
		}
		return validateLocalFallbackPack(bundle, attempt.State, pack)

	case "skipped_offline", "skipped_insufficient_evidence", "skipped_local_complete":
		if err := validateStoredResponseIdentity(attempt, false); err != nil {
			return err
		}
		expectedWarnings := []string(nil)
		if attempt.State == "skipped_insufficient_evidence" {
			expectedWarnings = []string{tasklens.AttemptWarningSparseEvidence}
		}
		if !slices.Equal(attempt.Warnings, expectedWarnings) {
			return fmt.Errorf("saved skipped attempt warnings are invalid")
		}
		return validateLocalFallbackPack(bundle, attempt.State, pack)
	default:
		return fmt.Errorf("saved attempt has an invalid state")
	}
}

func validateLocalFallbackPack(bundle tasklens.Bundle, attemptState string, pack tasklens.Pack) error {
	proposal, err := tasklens.LocalProposal(bundle)
	if err != nil {
		return fmt.Errorf("rebuild local proposal: %w", err)
	}
	expected, err := tasklens.BuildPack(bundle, proposal)
	if err != nil {
		return fmt.Errorf("rebuild local pack: %w", err)
	}
	expected, _ = FinalizeTaskInvestigationPack(expected, attemptState)
	if !reflect.DeepEqual(expected, pack) {
		return fmt.Errorf("saved attempt is not paired with its deterministic local fallback pack")
	}
	return nil
}

func validateStoredResponseIdentity(attempt tasklens.Attempt, required bool) error {
	if attempt.RawResponse != "" {
		if len([]byte(attempt.RawResponse)) > tasklens.MaxSavedRawResponseBytes ||
			attempt.RawResponseOmittedReason != "" ||
			attempt.Provider.ResponseBytes != len([]byte(attempt.RawResponse)) ||
			attempt.ResponseSHA256 != tasklens.SHA256([]byte(attempt.RawResponse)) {
			return fmt.Errorf("saved raw response does not match its response identity")
		}
		if _, found := secretscan.Detect(attempt.RawResponse); found {
			return fmt.Errorf("saved raw response contains secret-like content")
		}
		return nil
	}
	if attempt.RawResponseOmittedReason != "" {
		if attempt.RawResponseOmittedReason != tasklens.RawResponseOmittedSize &&
			attempt.RawResponseOmittedReason != tasklens.RawResponseOmittedSecret {
			return fmt.Errorf("saved raw response has an invalid omission reason")
		}
		if !validTaskSHA256(attempt.ResponseSHA256) || attempt.Provider.ResponseBytes <= 0 {
			return fmt.Errorf("saved omitted response lacks response identity")
		}
		if attempt.RawResponseOmittedReason == tasklens.RawResponseOmittedSize &&
			attempt.Provider.ResponseBytes <= tasklens.MaxSavedRawResponseBytes {
			return fmt.Errorf("saved response size omission is inconsistent")
		}
		return nil
	}
	if required || attempt.ResponseSHA256 != "" {
		return fmt.Errorf("saved response identity is incomplete")
	}
	return nil
}

func validTaskSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}

// TaskInvestigationPackSufficient reports whether a validated pack has enough
// grounded content to support the user-facing sufficiency label. A successful
// transport or reducer state alone is deliberately insufficient.
func TaskInvestigationPackSufficient(pack tasklens.Pack, attemptState string) bool {
	if attemptState == "skipped_offline" || attemptState == "skipped_insufficient_evidence" {
		return false
	}
	if pack.Locality == tasklens.LocalityBroadDynamic || len(pack.Anchors) < 2 {
		return false
	}
	if err := pack.RoleCoverage.ValidateAgainst(pack.RoleContract); err != nil {
		return false
	}
	visible := make(map[string]tasklens.InvestigationAnchor, len(pack.Anchors))
	for _, anchor := range pack.Anchors {
		visible[anchor.ID] = anchor
	}
	if !taskInvestigationEveryVisibleAnchorRelevant(pack) {
		return false
	}
	for _, requirement := range pack.RoleContract.Key {
		covered := 0
		for _, item := range pack.RoleCoverage.Key {
			if item.Role != requirement.Role {
				continue
			}
			for _, anchorID := range item.AnchorIDs {
				if _, ok := visible[anchorID]; ok {
					covered++
				}
			}
		}
		if covered < requirement.MinimumAnchors {
			return false
		}
	}

	decisiveJoin := false
	for _, join := range pack.EvidenceJoins {
		if join.RelationID == pack.DecisiveRelationID && len(join.SupportIDs) > 0 &&
			(join.SupportType == tasklens.SupportLocallyObserved || join.SupportType == tasklens.SupportDocument) {
			decisiveJoin = true
			break
		}
	}
	if pack.DecisiveRelationID == "" || !decisiveJoin || !pack.VerificationFrontier.HasExactAnchorOrEffect() {
		return false
	}
	groundedHypothesis := false
	for _, clause := range pack.WorkingHypothesis {
		if clause.Status == tasklens.HypothesisPlausible {
			return false
		}
		if clause.Status == tasklens.HypothesisSupported &&
			slices.Contains(clause.RelationIDs, pack.DecisiveRelationID) && len(clause.SupportIDs) > 0 {
			groundedHypothesis = true
		}
	}
	if !groundedHypothesis || !taskInvestigationHasGroundedGuidance(pack) {
		return false
	}
	frontierVisible := false
	for _, item := range append(append([]tasklens.VerificationItem(nil), pack.VerificationFrontier.Anchors...),
		func() []tasklens.VerificationItem {
			var result []tasklens.VerificationItem
			if pack.VerificationFrontier.Fixture != nil {
				result = append(result, *pack.VerificationFrontier.Fixture)
			}
			if pack.VerificationFrontier.CommandOrEffect != nil {
				result = append(result, *pack.VerificationFrontier.CommandOrEffect)
			}
			return result
		}()...) {
		if item.Authority == tasklens.VerificationProposedTestLocation ||
			item.Authority == tasklens.VerificationMissingEvidence {
			continue
		}
		if item.AnchorID == "" {
			frontierVisible = true
			break
		}
		if _, ok := visible[item.AnchorID]; ok {
			frontierVisible = true
			break
		}
	}
	return frontierVisible && taskInvestigationHasTaskRelevantVerification(pack)
}

func taskInvestigationHasUsefulHypothesis(pack tasklens.Pack) bool {
	evidenceAnchors := make(map[string]string)
	for _, anchor := range pack.Anchors {
		for _, evidenceID := range anchor.EvidenceIDs {
			evidenceAnchors[evidenceID] = anchor.ID
		}
	}
	for _, clause := range pack.WorkingHypothesis {
		if clause.Status == tasklens.HypothesisSupported && len(clause.RelationIDs) > 0 &&
			len(clause.SupportIDs) > 0 {
			return true
		}
		if clause.Status != tasklens.HypothesisPlausible || len(clause.SupportIDs) < 2 ||
			strings.HasPrefix(clause.Text, "A relationship involving the retained evidence for") ||
			strings.HasPrefix(clause.Text, "A relationship in the cited bounded context") {
			continue
		}
		anchors := make(map[string]struct{})
		for _, evidenceID := range clause.SupportIDs {
			if anchorID := evidenceAnchors[evidenceID]; anchorID != "" {
				anchors[anchorID] = struct{}{}
			}
		}
		if len(anchors) >= 2 {
			return true
		}
	}
	return false
}

func taskInvestigationHasTaskRelevantVerification(pack tasklens.Pack) bool {
	visibleAnchors := make(map[string]struct{}, len(pack.Anchors))
	evidenceAnchors := make(map[string]tasklens.InvestigationAnchor)
	for _, anchor := range pack.Anchors {
		visibleAnchors[anchor.ID] = struct{}{}
		for _, evidenceID := range anchor.EvidenceIDs {
			evidenceAnchors[evidenceID] = anchor
		}
	}
	exactFrontierEvidence := make(map[string]struct{})
	for _, item := range taskInvestigationExactVerificationItems(pack.VerificationFrontier) {
		if item.AnchorID != "" {
			if _, visible := visibleAnchors[item.AnchorID]; !visible {
				continue
			}
		}
		for _, evidenceID := range item.EvidenceIDs {
			exactFrontierEvidence[evidenceID] = struct{}{}
		}
	}
	if len(exactFrontierEvidence) == 0 {
		return false
	}
	strongJoinAnchors := make(map[string]struct{})
	for _, join := range pack.EvidenceJoins {
		if join.SupportType == tasklens.SupportDocument ||
			taskInvestigationStrongLocalJoin(join) {
			strongJoinAnchors[join.LeftID] = struct{}{}
			strongJoinAnchors[join.RightID] = struct{}{}
		}
	}
	for _, guidance := range pack.Verify.Steps {
		if guidance.Authority == tasklens.AuthorityMissing || len(guidance.EvidenceIDs) == 0 {
			continue
		}
		// Task prose can state the desired effect, but it cannot by itself prove
		// that a repository-owned verification target was retained.
		if guidance.Authority == tasklens.AuthorityTaskProvided {
			continue
		}
		for _, evidenceID := range guidance.EvidenceIDs {
			if _, exact := exactFrontierEvidence[evidenceID]; !exact {
				continue
			}
			anchor, visible := evidenceAnchors[evidenceID]
			if !visible {
				continue
			}
			if _, joined := strongJoinAnchors[anchor.ID]; joined ||
				taskInvestigationAnchorMatchesFoundTerm(anchor, pack.Interpretation.FoundTerms) {
				return true
			}
		}
	}
	return false
}

func taskInvestigationAnchorMatchesFoundTerm(
	anchor tasklens.InvestigationAnchor,
	foundTerms []string,
) bool {
	parts := []string{anchor.Path, anchor.Symbol, anchor.Section}
	for _, line := range anchor.Excerpt {
		parts = append(parts, line.Text)
	}
	corpus := strings.Join(parts, "\n")
	for _, raw := range foundTerms {
		term := strings.Trim(raw, "`'\".,;:()[]{}<>")
		if len(term) < 4 || taskInvestigationGenericVerificationTerm(term) {
			continue
		}
		if taskInvestigationContainsIdentifierTerm(corpus, term) {
			return true
		}
	}
	return false
}

func taskInvestigationGenericVerificationTerm(term string) bool {
	switch strings.ToLower(strings.TrimSpace(term)) {
	case "test", "tests", "error", "errors", "config", "configuration", "context", "package", "example",
		"examples", "option", "options", "setting", "settings", "write", "read", "request", "response",
		"handler", "result", "value", "exists", "type":
		return true
	default:
		return false
	}
}

func taskInvestigationContainsIdentifierTerm(text, term string) bool {
	textRunes := []rune(strings.ToLower(text))
	termRunes := []rune(strings.ToLower(term))
	if len(termRunes) == 0 || len(termRunes) > len(textRunes) {
		return false
	}
	firstIdentifier := taskInvestigationIdentifierRune(termRunes[0])
	lastIdentifier := taskInvestigationIdentifierRune(termRunes[len(termRunes)-1])
	for start := 0; start+len(termRunes) <= len(textRunes); start++ {
		if !slices.Equal(textRunes[start:start+len(termRunes)], termRunes) {
			continue
		}
		beforeOK := !firstIdentifier || start == 0 ||
			!taskInvestigationIdentifierRune(textRunes[start-1])
		after := start + len(termRunes)
		afterOK := !lastIdentifier || after == len(textRunes) ||
			!taskInvestigationIdentifierRune(textRunes[after])
		if beforeOK && afterOK {
			return true
		}
	}
	return false
}

func taskInvestigationIdentifierRune(value rune) bool {
	return value == '_' || unicode.IsLetter(value) || unicode.IsDigit(value)
}

// taskInvestigationEveryVisibleAnchorRelevant closes the presentation lane:
// model prose cannot make an auxiliary anchor relevant after retrieval. Each
// visible anchor needs at least one exact local reason to be in the pack.
func taskInvestigationEveryVisibleAnchorRelevant(pack tasklens.Pack) bool {
	relevant := make(map[string]struct{}, len(pack.Anchors))
	for _, anchor := range pack.Anchors {
		if taskInvestigationAnchorMatchesFoundTerm(anchor, pack.Interpretation.FoundTerms) {
			relevant[anchor.ID] = struct{}{}
		}
	}
	for anchorID := range taskInvestigationDecisiveStrongComponent(pack) {
		relevant[anchorID] = struct{}{}
	}
	for _, item := range taskInvestigationExactVerificationItems(pack.VerificationFrontier) {
		if item.AnchorID != "" {
			relevant[item.AnchorID] = struct{}{}
		}
	}
	for _, group := range [][]tasklens.RoleCoverageItem{
		pack.RoleCoverage.Key,
		pack.RoleCoverage.Supporting,
	} {
		for _, item := range group {
			for _, anchorID := range item.AnchorIDs {
				relevant[anchorID] = struct{}{}
			}
		}
	}
	for _, anchor := range pack.Anchors {
		if _, ok := relevant[anchor.ID]; !ok {
			return false
		}
	}
	return true
}

func taskInvestigationDecisiveStrongComponent(pack tasklens.Pack) map[string]struct{} {
	var decisive tasklens.EvidenceJoin
	for _, join := range pack.EvidenceJoins {
		if join.RelationID == pack.DecisiveRelationID && taskInvestigationStrongLocalJoin(join) {
			decisive = join
			break
		}
	}
	component := make(map[string]struct{})
	if decisive.RelationID == "" {
		return component
	}
	adjacent := make(map[string][]string)
	for _, join := range pack.EvidenceJoins {
		if !taskInvestigationStrongLocalJoin(join) {
			continue
		}
		adjacent[join.LeftID] = append(adjacent[join.LeftID], join.RightID)
		adjacent[join.RightID] = append(adjacent[join.RightID], join.LeftID)
	}
	queue := []string{decisive.LeftID, decisive.RightID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, seen := component[current]; seen {
			continue
		}
		component[current] = struct{}{}
		queue = append(queue, adjacent[current]...)
	}
	return component
}

func taskInvestigationExactVerificationItems(
	frontier tasklens.VerificationFrontier,
) []tasklens.VerificationItem {
	items := append([]tasklens.VerificationItem(nil), frontier.Anchors...)
	if frontier.Fixture != nil {
		items = append(items, *frontier.Fixture)
	}
	if frontier.CommandOrEffect != nil {
		items = append(items, *frontier.CommandOrEffect)
	}
	result := make([]tasklens.VerificationItem, 0, len(items))
	for _, item := range items {
		switch item.Authority {
		case tasklens.VerificationExactExistingTest,
			tasklens.VerificationExactGeneratedFixture,
			tasklens.VerificationExactExample,
			tasklens.VerificationDocumentedCommand:
			result = append(result, item)
		}
	}
	return result
}

func taskInvestigationHasActionableExtensionEvidence(pack tasklens.Pack) bool {
	if len(pack.LikelyAreas) < 2 {
		return false
	}
	anchors := make(map[string]tasklens.InvestigationAnchor, len(pack.Anchors))
	evidenceAnchors := make(map[string]string)
	starts := make([]string, 0, len(pack.Anchors))
	for _, anchor := range pack.Anchors {
		anchors[anchor.ID] = anchor
		for _, evidenceID := range anchor.EvidenceIDs {
			evidenceAnchors[evidenceID] = anchor.ID
		}
		if anchor.Role == tasklens.RoleIntegrationBoundary {
			starts = append(starts, anchor.ID)
		}
	}
	if len(starts) == 0 {
		return false
	}
	targets := make(map[string]struct{})
	for _, guidance := range pack.Verify.Steps {
		if guidance.Authority == tasklens.AuthorityMissing ||
			guidance.Authority == tasklens.AuthorityTaskProvided {
			continue
		}
		for _, evidenceID := range guidance.EvidenceIDs {
			anchorID, ok := evidenceAnchors[evidenceID]
			if !ok {
				continue
			}
			role := anchors[anchorID].Role
			if role == tasklens.RoleVerificationAnchor || role == tasklens.RoleRepresentativeImplementation {
				targets[anchorID] = struct{}{}
			}
		}
	}
	if len(targets) == 0 {
		return false
	}
	adjacent := make(map[string][]string)
	for _, join := range pack.EvidenceJoins {
		actionable := join.SupportType == tasklens.SupportDocument ||
			join.SupportType == tasklens.SupportModelHypothesis ||
			taskInvestigationStrongLocalJoin(join)
		if !actionable {
			continue
		}
		adjacent[join.LeftID] = append(adjacent[join.LeftID], join.RightID)
		adjacent[join.RightID] = append(adjacent[join.RightID], join.LeftID)
	}
	seen := make(map[string]struct{}, len(anchors))
	queue := append([]string(nil), starts...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, duplicate := seen[current]; duplicate {
			continue
		}
		seen[current] = struct{}{}
		if _, target := targets[current]; target {
			return true
		}
		for _, next := range adjacent[current] {
			if _, visited := seen[next]; !visited {
				queue = append(queue, next)
			}
		}
	}
	return false
}

func taskInvestigationStrongLocalJoin(join tasklens.EvidenceJoin) bool {
	if join.SupportType != tasklens.SupportLocallyObserved {
		return false
	}
	switch join.Kind {
	case string(tasklens.RelationSharedStateAlias), string(tasklens.RelationScopeUnknown):
		return false
	default:
		return true
	}
}

// FinalizeTaskInvestigationPack applies the closed, deterministic warning set
// after content-based sufficiency has been computed. This same function is
// used when writing and replay-validating artifacts.
func FinalizeTaskInvestigationPack(
	pack tasklens.Pack,
	attemptState string,
) (tasklens.Pack, bool) {
	sufficient := TaskInvestigationPackSufficient(pack, attemptState)
	pack.Warnings = nil
	if sufficient {
		return pack, true
	}
	warning := tasklens.PartialPackWarningEmission(attemptState)
	pack.Warnings = []string{warning.Raw}
	return pack, false
}

func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}

func taskInvestigationHasGroundedGuidance(pack tasklens.Pack) bool {
	for _, value := range pack.ReproduceOrObserve {
		if value.Authority == tasklens.AuthorityTaskProvided && !pack.TaskObservationConcrete {
			continue
		}
		if value.Authority != tasklens.AuthorityMissing && len(value.EvidenceIDs) > 0 {
			return true
		}
	}
	return false
}

func validTaskAttemptState(state string) bool {
	_, valid := map[string]struct{}{
		"accepted": {}, "accepted_with_rejections": {}, "provider_failed": {},
		"rejected": {}, "skipped_offline": {}, "skipped_insufficient_evidence": {},
		"skipped_local_complete": {},
	}[state]
	return valid
}

func taskStatusStateForAttempt(attemptState string) (string, bool) {
	switch attemptState {
	case "accepted":
		return "accepted", true
	case "accepted_with_rejections":
		return "accepted_partial", true
	case "provider_failed", "rejected":
		return "partial_local", true
	case "skipped_offline", "skipped_insufficient_evidence":
		return "partial_local", false
	case "skipped_local_complete":
		return "accepted_local_complete", false
	default:
		return "", false
	}
}

func taskArtifactWarnings(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, group := range groups {
		for _, warning := range group {
			if _, duplicate := seen[warning]; duplicate {
				continue
			}
			seen[warning] = struct{}{}
			result = append(result, warning)
		}
	}
	return result
}

func validateTaskPackAgainstBundle(bundle tasklens.Bundle, pack tasklens.Pack) error {
	anchors := make(map[string]tasklens.Anchor, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		anchors[anchor.ID] = anchor
	}
	evidence := make(map[string]tasklens.Evidence, len(bundle.Evidence))
	for _, item := range bundle.Evidence {
		evidence[item.ID] = item
	}
	relations := make(map[string]tasklens.Relation, len(bundle.Relations))
	for _, relation := range bundle.Relations {
		relations[relation.ID] = relation
	}

	foundTerms := make([]string, 0, len(bundle.Terms))
	userTerms := make([]string, 0, len(bundle.Terms))
	for _, term := range bundle.Terms {
		if term.Found {
			foundTerms = append(foundTerms, term.Text)
		} else {
			userTerms = append(userTerms, term.Text)
		}
	}
	if !slices.Equal(foundTerms, pack.Interpretation.FoundTerms) ||
		!slices.Equal(userTerms, pack.Interpretation.UserProvidedOnly) {
		return fmt.Errorf("saved pack term grounding does not match its bundle")
	}

	selected := make(map[string]struct{}, len(pack.Anchors))
	for _, projected := range pack.Anchors {
		anchor, ok := anchors[projected.ID]
		if !ok || anchor.Path != projected.Path || anchor.Symbol != projected.Symbol ||
			anchor.Section != projected.Section || anchor.Package != projected.Package ||
			anchor.StartLine != projected.StartLine || anchor.EndLine != projected.EndLine ||
			anchor.Scope != projected.Scope ||
			!slices.Equal(anchor.Excerpt, projected.Excerpt) ||
			!slices.Equal(anchor.EvidenceIDs, projected.EvidenceIDs) {
			return fmt.Errorf("saved pack anchor does not match its bounded source")
		}
		selected[projected.ID] = struct{}{}
	}
	for _, join := range pack.EvidenceJoins {
		if _, ok := selected[join.LeftID]; !ok {
			return fmt.Errorf("saved pack join is outside selected anchors")
		}
		if _, ok := selected[join.RightID]; !ok {
			return fmt.Errorf("saved pack join is outside selected anchors")
		}
		for _, id := range join.SupportIDs {
			if _, ok := evidence[id]; !ok {
				return fmt.Errorf("saved pack join cites evidence outside its bundle")
			}
		}
		if join.SupportType == tasklens.SupportLocallyObserved {
			relation, ok := relations[join.RelationID]
			if !ok || relation.SupportType != tasklens.SupportLocallyObserved || relation.Kind != join.Kind ||
				!taskInvestigationSameEndpoints(relation.LeftID, relation.RightID, join.LeftID, join.RightID) ||
				len(join.SupportIDs) == 0 || !taskInvestigationSubset(join.SupportIDs, relation.EvidenceIDs) {
				return fmt.Errorf("saved pack local join lacks matching local evidence")
			}
		} else if join.RelationID != "" {
			return fmt.Errorf("saved pack non-local join cites a local relation")
		}
		if join.SupportType == tasklens.SupportDocument &&
			!taskInvestigationHasEvidenceKind(join.SupportIDs, evidence, tasklens.EvidenceDocumentClaim) {
			return fmt.Errorf("saved pack document join lacks document evidence")
		}
		if join.SupportType == tasklens.SupportModelHypothesis && len(join.SupportIDs) == 0 {
			return fmt.Errorf("saved pack model join lacks exact supporting evidence")
		}
	}
	for _, clause := range pack.WorkingHypothesis {
		for _, id := range clause.SupportIDs {
			if _, ok := evidence[id]; !ok {
				return fmt.Errorf("saved pack hypothesis cites evidence outside its bundle")
			}
		}
		for _, id := range clause.RelationIDs {
			if _, ok := relations[id]; !ok {
				return fmt.Errorf("saved pack hypothesis cites a relation outside its bundle")
			}
		}
		if clause.Status == tasklens.HypothesisSupported &&
			!taskInvestigationHasRepositoryEvidence(clause.SupportIDs, evidence) {
			return fmt.Errorf("saved pack supported hypothesis lacks repository evidence")
		}
	}
	for _, guidance := range append(
		append([]tasklens.Guidance(nil), pack.ReproduceOrObserve...),
		pack.Verify.Steps...,
	) {
		for _, id := range guidance.EvidenceIDs {
			if _, ok := evidence[id]; !ok {
				return fmt.Errorf("saved pack guidance cites evidence outside its bundle")
			}
		}
		switch guidance.Authority {
		case tasklens.AuthorityTaskProvided:
			if !taskInvestigationHasEvidenceKind(guidance.EvidenceIDs, evidence, tasklens.EvidenceTaskProvided) {
				return fmt.Errorf("saved pack task guidance lacks task evidence")
			}
		case tasklens.AuthorityRepositoryDocument:
			if !taskInvestigationHasEvidenceKind(guidance.EvidenceIDs, evidence, tasklens.EvidenceDocumentClaim) {
				return fmt.Errorf("saved pack document guidance lacks document evidence")
			}
		case tasklens.AuthorityRepositoryTest:
			if !taskInvestigationHasTestEvidence(guidance.EvidenceIDs, evidence, anchors) {
				return fmt.Errorf("saved pack test guidance lacks test or example evidence")
			}
		case tasklens.AuthorityRepositoryObservation:
			if !taskInvestigationHasEvidenceKind(guidance.EvidenceIDs, evidence, tasklens.EvidenceRepositoryFact) {
				return fmt.Errorf("saved pack repository observation lacks source or configuration evidence")
			}
		case tasklens.AuthorityMissing:
			if len(guidance.EvidenceIDs) != 0 {
				return fmt.Errorf("saved pack missing-evidence guidance cites evidence")
			}
		}
	}
	return nil
}

func taskInvestigationSameEndpoints(leftA, rightA, leftB, rightB string) bool {
	return leftA == leftB && rightA == rightB
}

func taskInvestigationSubset(values, allowed []string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func taskInvestigationHasEvidenceKind(
	ids []string,
	evidence map[string]tasklens.Evidence,
	kind tasklens.EvidenceKind,
) bool {
	for _, id := range ids {
		if evidence[id].Kind == kind {
			return true
		}
	}
	return false
}

func taskInvestigationHasRepositoryEvidence(
	ids []string,
	evidence map[string]tasklens.Evidence,
) bool {
	return taskInvestigationHasEvidenceKind(ids, evidence, tasklens.EvidenceRepositoryFact) ||
		taskInvestigationHasEvidenceKind(ids, evidence, tasklens.EvidenceDocumentClaim)
}

func taskInvestigationHasTestEvidence(
	ids []string,
	evidence map[string]tasklens.Evidence,
	anchors map[string]tasklens.Anchor,
) bool {
	for _, id := range ids {
		item := evidence[id]
		if tasklens.IsTestOrExamplePath(item.Path) {
			return true
		}
	}
	return false
}

func projectTaskInvestigation(
	bundle tasklens.Bundle,
	attempt tasklens.Attempt,
	pack tasklens.Pack,
	status tasklens.Status,
) (*TaskInvestigationWorkspace, error) {
	workspace := &TaskInvestigationWorkspace{
		TaskID: pack.ID, Repository: pack.Repository.Identity,
		Task: bundle.Task.Text, State: status.State,
		Sufficient: status.Sufficient, Locality: pack.Locality,
		Profile: pack.Profile, RoleContract: pack.RoleContract,
		RoleCoverage: pack.RoleCoverage, VerificationFrontier: pack.VerificationFrontier,
		CheapExit: pack.CheapExit,
		Interpretation: TaskInvestigationInterpretation{
			Restatement: pack.Interpretation.Restatement,
			Kind:        pack.Interpretation.Kind,
			Observable:  pack.Interpretation.Observable,
			FoundTerms: append(
				[]string(nil),
				pack.Interpretation.FoundTerms...,
			),
			UserProvidedOnly: append(
				[]string(nil),
				pack.Interpretation.UserProvidedOnly...,
			),
		},
		StagesSkipped:         append([]string(nil), pack.StagesSkipped...),
		Budget:                pack.Budgets,
		Provider:              status.Provider,
		CapturedRevision:      status.CapturedRevision,
		RepositoryStateSHA256: bundle.Repository.StateSHA256,
	}
	// Status is validated above as the exact de-duplicated union of pack and
	// attempt warnings, so it is the single presentation source.
	workspace.Warnings = append(workspace.Warnings, status.Warnings...)
	warningDiagnostics, err := taskInvestigationWarningDiagnostics(
		bundle,
		attempt,
		pack,
		status,
	)
	if err != nil {
		return nil, err
	}
	workspace.warningDiagnostics = warningDiagnostics

	anchorIndexes := make(map[string]int, len(pack.Anchors))
	for index, anchor := range pack.Anchors {
		source, err := taskInvestigationSourceSnippet(anchor, status.CapturedRevision)
		if err != nil {
			return nil, fmt.Errorf("project source anchor %d: %w", index+1, err)
		}
		anchorIndexes[anchor.ID] = index
		workspace.Anchors = append(workspace.Anchors, TaskInvestigationAnchor{
			Path: anchor.Path, Symbol: anchor.Symbol, Section: anchor.Section,
			Package: anchor.Package, Role: anchor.Role,
			StartLine: anchor.StartLine, EndLine: anchor.EndLine,
			Scope: anchor.Scope, Why: anchor.Why, Source: source,
		})
	}
	evidenceByID := make(map[string]tasklens.Evidence, len(bundle.Evidence))
	for _, item := range bundle.Evidence {
		evidenceByID[item.ID] = item
	}
	relationByID := make(map[string]tasklens.Relation, len(bundle.Relations))
	for _, relation := range bundle.Relations {
		relationByID[relation.ID] = relation
	}
	for _, area := range pack.LikelyAreas {
		projected := TaskInvestigationArea{Label: area.Label, Why: area.Why}
		for _, id := range area.TargetIDs {
			index, ok := anchorIndexes[id]
			if !ok {
				return nil, fmt.Errorf("likely area references an unavailable anchor")
			}
			projected.AnchorIndexes = append(projected.AnchorIndexes, index)
		}
		workspace.LikelyAreas = append(workspace.LikelyAreas, projected)
	}
	for _, join := range pack.EvidenceJoins {
		left, leftOK := anchorIndexes[join.LeftID]
		right, rightOK := anchorIndexes[join.RightID]
		if !leftOK || !rightOK {
			return nil, fmt.Errorf("evidence join references an unavailable anchor")
		}
		workspace.EvidenceJoins = append(workspace.EvidenceJoins, TaskInvestigationJoin{
			LeftAnchor: left, RightAnchor: right, Kind: join.Kind,
			Support: join.SupportType, Explanation: join.Explanation, Scope: join.Scope,
			SupportAnchorIndexes: taskInvestigationSupportAnchorIndexes(
				join.SupportIDs,
				[]string{join.RelationID},
				anchorIndexes,
				evidenceByID,
				relationByID,
			),
		})
	}
	for _, clause := range pack.WorkingHypothesis {
		workspace.WorkingHypothesis = append(
			workspace.WorkingHypothesis,
			TaskInvestigationHypothesis{
				Status: clause.Status,
				Text:   clause.Text,
				SupportAnchorIndexes: taskInvestigationSupportAnchorIndexes(
					clause.SupportIDs,
					clause.RelationIDs,
					anchorIndexes,
					evidenceByID,
					relationByID,
				),
			},
		)
	}
	workspace.ReproduceOrObserve = projectTaskInvestigationGuidance(
		pack.ReproduceOrObserve,
		anchorIndexes,
		evidenceByID,
	)
	workspace.Verify = TaskInvestigationVerification{
		Effect: pack.Verify.Effect,
		Steps: projectTaskInvestigationGuidance(
			pack.Verify.Steps,
			anchorIndexes,
			evidenceByID,
		),
	}
	for _, probe := range pack.NextProbes {
		projected := TaskInvestigationProbe{Action: probe.Action, Text: probe.Text}
		for _, id := range probe.AnchorIDs {
			index, ok := anchorIndexes[id]
			if !ok {
				return nil, fmt.Errorf("next probe references an unavailable anchor")
			}
			projected.AnchorIndexes = append(projected.AnchorIndexes, index)
		}
		workspace.NextProbes = append(workspace.NextProbes, projected)
	}
	return workspace, nil
}

func bindTaskInvestigationAuthority(
	workspace *TaskInvestigationWorkspace,
	repository freshness.RepositoryState,
) error {
	if workspace == nil {
		return nil
	}
	stateSHA, err := tasklens.RepositoryStateSHA(repository)
	if err != nil {
		return fmt.Errorf("task investigation authority state: %w", err)
	}
	if workspace.CapturedRevision != repository.Head ||
		workspace.RepositoryStateSHA256 != stateSHA {
		return fmt.Errorf("task investigation artifacts do not match the authorized repository state")
	}
	bindTaskInvestigationRevision(workspace, repository.Head)
	return nil
}

func (workspace *TaskInvestigationWorkspace) RepoName() string {
	if workspace == nil {
		return ""
	}
	return strings.TrimSpace(workspace.Repository)
}

func projectTaskInvestigationGuidance(
	values []tasklens.Guidance,
	anchorIndexes map[string]int,
	evidenceByID map[string]tasklens.Evidence,
) []TaskInvestigationGuidance {
	result := make([]TaskInvestigationGuidance, 0, len(values))
	for _, value := range values {
		result = append(result, TaskInvestigationGuidance{
			Text: value.Text, Authority: value.Authority,
			SupportAnchorIndexes: taskInvestigationSupportAnchorIndexes(
				value.EvidenceIDs,
				nil,
				anchorIndexes,
				evidenceByID,
				nil,
			),
		})
	}
	return result
}

func taskInvestigationSupportAnchorIndexes(
	evidenceIDs []string,
	relationIDs []string,
	anchorIndexes map[string]int,
	evidenceByID map[string]tasklens.Evidence,
	relationByID map[string]tasklens.Relation,
) []int {
	seen := make(map[int]struct{})
	addAnchor := func(anchorID string) {
		index, ok := anchorIndexes[anchorID]
		if ok {
			seen[index] = struct{}{}
		}
	}
	for _, evidenceID := range evidenceIDs {
		addAnchor(evidenceByID[evidenceID].AnchorID)
	}
	for _, relationID := range relationIDs {
		relation, ok := relationByID[relationID]
		if !ok {
			continue
		}
		addAnchor(relation.LeftID)
		addAnchor(relation.RightID)
	}
	result := make([]int, 0, len(seen))
	for index := range seen {
		result = append(result, index)
	}
	slices.Sort(result)
	return result
}

func taskInvestigationSourceSnippet(
	anchor tasklens.InvestigationAnchor,
	revision string,
) (SourceSnippet, error) {
	projected := anchor.Excerpt
	if len(projected) > maxInlineSourceLines {
		head := maxInlineSourceLines / 2
		tail := maxInlineSourceLines - head
		projected = append(append([]tasklens.SourceLine(nil), anchor.Excerpt[:head]...),
			anchor.Excerpt[len(anchor.Excerpt)-tail:]...)
	}
	texts := make([]string, 0, len(projected))
	content := make([]string, 0, len(projected)+1)
	lines := make([]SourceSnippetLine, 0, len(projected))
	highlightLines := make([]int, 0, len(projected))
	previous := 0
	for _, line := range projected {
		texts = append(texts, line.Text)
		gap := previous > 0 && line.Line != previous+1
		if gap {
			content = append(content, omittedSourceLinesMarker)
		}
		content = append(content, line.Text)
		lines = append(lines, SourceSnippetLine{
			Line: line.Line, Text: line.Text, Highlight: line.Highlight, GapBefore: gap,
		})
		if line.Highlight {
			highlightLines = append(highlightLines, line.Line)
		}
		previous = line.Line
	}
	snippet := SourceSnippet{
		Path: anchor.Path, Language: sourceLanguage(anchor.Path),
		EnclosingSymbol: anchor.Symbol,
		StartLine:       anchor.StartLine, EndLine: anchor.EndLine,
		HighlightRanges: sourceHighlightRanges(highlightLines),
		Content:         strings.Join(content, "\n"), Lines: lines,
		ContentSHA256: sourceLinesSHA256(texts), Role: string(anchor.Role),
		Revision: revision, SourceComplete: !anchor.Scope.Truncated,
	}
	snippet.PresentationSHA256 = sourceSnippetPresentationSHA(snippet)
	if err := snippet.Validate(); err != nil {
		return SourceSnippet{}, err
	}
	return snippet, nil
}

func bindTaskInvestigationRevision(workspace *TaskInvestigationWorkspace, revision string) {
	if workspace == nil || strings.TrimSpace(revision) == "" {
		return
	}
	workspace.CapturedRevision = strings.TrimSpace(revision)
	for index := range workspace.Anchors {
		workspace.Anchors[index].Source.Revision = workspace.CapturedRevision
		workspace.Anchors[index].Source.PresentationSHA256 = sourceSnippetPresentationSHA(
			workspace.Anchors[index].Source,
		)
	}
}
