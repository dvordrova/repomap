package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/studymap"
)

const legacyReadingPackReviewPromptVersion = "repository-reading-pack-review-json-v1"

const maxV32ReplayArtifactBytes = 32 << 20

func replaySavedStudyMapV32(runDir string) (studyMapStatus, error) {
	recordPath := filepath.Join(runDir, studymap.RecordFile)
	if err := os.Remove(recordPath); err != nil && !os.IsNotExist(err) {
		return studyMapStatus{}, err
	}
	var status studyMapStatus
	if err := readV32ReplayJSON(filepath.Join(runDir, studymap.StatusFile), &status); err != nil {
		return studyMapStatus{}, err
	}
	status.LocalReplay = true
	status.Selected = 0
	var attempt studyMapAttempt
	fail := func(cause error) (studyMapStatus, error) {
		_ = os.Remove(recordPath)
		status.State = "failed"
		status.FailureReason = semanticDiscoveryReason(cause.Error())
		if err := writeGoldenJSON(filepath.Join(runDir, studymap.StatusFile), status); err != nil {
			return status, fmt.Errorf("%w; save replay failure status: %v", cause, err)
		}
		return status, cause
	}
	var bundle studymap.Bundle
	if err := readV32ReplayJSON(filepath.Join(runDir, studymap.BundleFile), &bundle); err != nil {
		return fail(err)
	}
	if err := bundle.Validate(); err != nil {
		return fail(err)
	}
	bundleSHA, err := studymap.BundleHash(bundle)
	if err != nil {
		return fail(err)
	}
	if err := readV32ReplayJSON(filepath.Join(runDir, studymap.AttemptFile), &attempt); err != nil {
		return fail(err)
	}
	if attempt.Version != 2 || attempt.BundleSHA256 != bundleSHA ||
		(attempt.PromptVersion != "repository-study-map-split-v2" &&
			attempt.PromptVersion != "repository-study-map-v31-replay-plus-reviews-v2") {
		return fail(fmt.Errorf("v32 replay: Study Map attempt bundle hash mismatch"))
	}
	brief, directions, err := loadBoundStudyMapInputs(runDir, bundle, bundleSHA)
	if err != nil {
		return fail(err)
	}
	reviews, summaries, preparationIssues, err := loadBoundStudyMapReviews(
		runDir,
		bundle,
		directions,
		bundleSHA,
	)
	if err != nil {
		return fail(err)
	}
	record, reduction, buildErr := studymap.BuildReviewedRecord(bundle, brief, directions, reviews)
	reduction.Issues = append(reduction.Issues, preparationIssues...)
	reviewArtifact := studyMapReviewArtifact{
		Version: studyMapReviewArtifactVersion,
		Reviews: reviews, Reduction: reduction, Attempts: summaries,
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapReviewsFile), reviewArtifact); err != nil {
		return fail(err)
	}
	if buildErr != nil {
		return fail(buildErr)
	}
	status.State = "published"
	status.FailureReason = ""
	status.RepositoryType = record.RepositoryType
	status.Candidates = reduction.Proposed
	status.Validated = reduction.Reviewed
	status.Selected = len(record.Directions)
	status.Metrics = aggregateStudyMapMetrics(status.Stages, nil)
	status.ProviderLatencyMillis = status.Metrics.LatencyMillis
	status.LocalReplay = true
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.StatusFile), status); err != nil {
		return status, err
	}
	if err := writeGoldenJSON(recordPath, record); err != nil {
		return fail(err)
	}
	return status, nil
}

func loadBoundStudyMapInputs(
	runDir string,
	bundle studymap.Bundle,
	bundleSHA string,
) (studymap.BriefShapeProposal, studymap.DirectionProposal, error) {
	briefAttemptPath := filepath.Join(runDir, studyMapBriefShapeAttempt)
	directionAttemptPath := filepath.Join(runDir, studyMapDirectionsAttempt)
	briefAttemptExists, err := v32ReplayArtifactExists(briefAttemptPath)
	if err != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, err
	}
	directionAttemptExists, err := v32ReplayArtifactExists(directionAttemptPath)
	if err != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, err
	}
	if briefAttemptExists != directionAttemptExists {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{},
			fmt.Errorf("v32 replay: incomplete split Study input attempts")
	}
	if !briefAttemptExists {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{},
			fmt.Errorf("v32 replay: typed split Study input attempts are required")
	}

	var brief studymap.BriefShapeProposal
	var directions studymap.DirectionProposal
	var briefAttempt studyMapV32StageAttempt
	if err := readV32ReplayJSON(briefAttemptPath, &briefAttempt); err != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, err
	}
	var directionAttempt studyMapV32StageAttempt
	if err := readV32ReplayJSON(directionAttemptPath, &directionAttempt); err != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, err
	}
	if briefAttempt.Version != 1 || directionAttempt.Version != 1 ||
		briefAttempt.PromptVersion != semanticdiscovery.StudyBriefPromptVersion ||
		directionAttempt.PromptVersion != semanticdiscovery.StudyCandidatesPromptVersion ||
		briefAttempt.BundleSHA256 != bundleSHA || directionAttempt.BundleSHA256 != bundleSHA ||
		briefAttempt.ValidationState != "accepted" || directionAttempt.ValidationState != "accepted" {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{},
			fmt.Errorf("v32 replay: split Study inputs are not accepted for this bundle")
	}
	briefCatalog, catalogErr := studymap.BuildBriefShapeReferenceCatalog(bundle)
	if catalogErr != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, catalogErr
	}
	var briefDiagnostics studymap.BriefShapeReferenceDiagnostics
	brief, briefDiagnostics, err = studymap.DecodeAndResolveBriefShapeProposal(
		briefAttempt.Response,
		briefCatalog,
	)
	if err != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, err
	}
	if (briefAttempt.BriefDiagnostics == nil &&
		(briefDiagnostics.ShapeReceived != 0 || len(briefDiagnostics.Issues) != 0)) ||
		(briefAttempt.BriefDiagnostics != nil &&
			!reflect.DeepEqual(*briefAttempt.BriefDiagnostics, briefDiagnostics)) {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{},
			fmt.Errorf("v32 replay: Brief diagnostics do not match the typed response")
	}
	directions, err = studymap.DecodeDirectionProposal(directionAttempt.Response)
	if err != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, err
	}
	directions, err = studymap.NormalizeDirectionProposal(directions)
	if err != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, err
	}

	briefRaw, err := readV32ReplayRaw(filepath.Join(runDir, studyMapBriefShapeFile))
	if err != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, err
	}
	projectedBrief, err := studymap.DecodeBriefShapeProposal(briefRaw)
	if err != nil || !equalV32Projection(projectedBrief, brief) {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{},
			fmt.Errorf("v32 replay: Brief projection does not match its saved source attempt")
	}
	directionsRaw, err := readV32ReplayRaw(filepath.Join(runDir, studyMapDirectionsFile))
	if err != nil {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{}, err
	}
	projectedDirections, err := studymap.DecodeNormalizedDirectionProposal(directionsRaw)
	if err != nil || !equalV32Projection(projectedDirections, directions) {
		return studymap.BriefShapeProposal{}, studymap.DirectionProposal{},
			fmt.Errorf("v32 replay: direction projection does not match its saved source attempt")
	}
	return brief, directions, nil
}

func equalV32Projection(left, right any) bool {
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftRaw, rightRaw)
}

func loadBoundStudyMapReviews(
	runDir string,
	bundle studymap.Bundle,
	directions studymap.DirectionProposal,
	bundleSHA string,
) ([]studymap.ReviewProposal, []studyMapReviewSummary, []studymap.ReviewIssue, error) {
	reviews := make([]studymap.ReviewProposal, 0, len(directions.Directions))
	summaries := make([]studyMapReviewSummary, 0, len(directions.Directions))
	issues := make([]studymap.ReviewIssue, 0)
	for _, direction := range directions.Directions {
		attemptPath := filepath.Join(runDir, studyMapReviewAttemptsDir, direction.DirectionID+".json")
		var attempt studyMapReviewAttempt
		if err := readV32ReplayJSON(attemptPath, &attempt); err != nil {
			return nil, nil, nil, err
		}
		if attempt.Version != 1 ||
			!supportedReadingPackReviewPromptVersion(attempt.PromptVersion) ||
			attempt.BundleSHA256 != bundleSHA || attempt.DirectionID != direction.DirectionID {
			return nil, nil, nil, fmt.Errorf("v32 replay: reading review attempt binding mismatch")
		}
		summaries = append(summaries, studyMapReviewSummary{
			DirectionID: attempt.DirectionID, ValidationState: attempt.ValidationState,
			IssueCode: attempt.IssueCode, FailureReason: attempt.FailureReason, Metrics: attempt.Metrics,
		})
		if attempt.ValidationState != "accepted" {
			code := attempt.IssueCode
			if code == "" {
				code = "review_rejected"
			}
			issues = append(issues, studymap.ReviewIssue{
				DirectionID: direction.DirectionID, Code: code, Detail: attempt.FailureReason,
			})
			continue
		}
		expectedBundle, err := studymap.BuildReviewBundle(bundle, direction)
		if err != nil {
			return nil, nil, nil, err
		}
		if attempt.Bundle == nil || !reflect.DeepEqual(*attempt.Bundle, expectedBundle) {
			return nil, nil, nil, fmt.Errorf("v32 replay: reading review bundle binding mismatch")
		}
		proposal, err := studymap.DecodeReviewProposal(attempt.Response)
		if err != nil {
			return nil, nil, nil, err
		}
		reviews = append(reviews, proposal)
	}
	return reviews, summaries, issues, nil
}

func supportedReadingPackReviewPromptVersion(version string) bool {
	return version == semanticdiscovery.ReadingPackReviewPromptVersion ||
		version == legacyReadingPackReviewPromptVersion
}

func v32ReplayArtifactExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func replaySavedPavedPathsV32(runDir string) (pavedPathStatus, error) {
	recordPath := filepath.Join(runDir, pavedpath.RecordFile)
	if err := os.Remove(recordPath); err != nil && !os.IsNotExist(err) {
		return pavedPathStatus{}, err
	}
	var status pavedPathStatus
	if err := readV32ReplayJSON(filepath.Join(runDir, pavedpath.StatusFile), &status); err != nil {
		return pavedPathStatus{}, err
	}
	status.LocalReplay = true
	status.Paths = 0
	var attempt pavedPathAttempt
	fail := func(cause error) (pavedPathStatus, error) {
		_ = os.Remove(recordPath)
		status.State = "failed"
		status.Failure = semanticDiscoveryReason(cause.Error())
		if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.StatusFile), status); err != nil {
			return status, fmt.Errorf("%w; save replay failure status: %v", cause, err)
		}
		return status, cause
	}
	var bundle pavedpath.Bundle
	if err := readV32ReplayJSON(filepath.Join(runDir, pavedpath.BundleFile), &bundle); err != nil {
		return fail(err)
	}
	if err := bundle.Validate(); err != nil {
		return fail(err)
	}
	bundleSHA, err := pavedpath.BundleHash(bundle)
	if err != nil {
		return fail(err)
	}
	if err := readV32ReplayJSON(filepath.Join(runDir, pavedpath.AttemptFile), &attempt); err != nil {
		return fail(err)
	}
	if attempt.BundleSHA256 != bundleSHA || attempt.PromptVersion != pavedpath.PromptVersion {
		return fail(fmt.Errorf("v32 replay: Paved Path attempt bundle hash mismatch"))
	}
	proposal, err := pavedpath.DecodeProposal(attempt.Response)
	if err != nil {
		return fail(err)
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return fail(err)
	}
	var editorEvidenceIDs []string
	allowedStudyIDs := reportStudyDirectionIDs(data)
	switch attempt.Version {
	case 1:
		// Historical v3.2 evaluation attempts predate explicit scope fields and
		// may have sent the full collected bundle. When a legacy attempt did save
		// an editor scope, preserve and revalidate it.
		if len(allowedStudyIDs) == 0 {
			allowedStudyIDs, err = legacyPavedStudyDirectionIDs(runDir)
			if err != nil {
				return fail(err)
			}
		}
		if len(attempt.EditorEvidenceIDs) > 0 {
			editorEvidenceIDs = pavedPathEvidenceIDs(selectPavedPathEditorBundle(
				bundle,
				legacyPavedPathEditorEvidenceLimit,
			).Evidence)
			if !slices.Equal(editorEvidenceIDs, attempt.EditorEvidenceIDs) {
				return fail(fmt.Errorf("v32 replay: legacy Paved Path editor evidence scope mismatch"))
			}
		}
	case pavedPathAttemptVersion:
		if len(attempt.EditorEvidenceIDs) == 0 && len(bundle.Evidence) > 0 {
			return fail(fmt.Errorf("v32 replay: Paved Path editor evidence scope is missing"))
		}
		editorEvidenceIDs = pavedPathEvidenceIDs(selectPavedPathEditorBundle(
			bundle,
			pavedPathEditorEvidenceLimit,
		).Evidence)
		if !slices.Equal(editorEvidenceIDs, attempt.EditorEvidenceIDs) {
			return fail(fmt.Errorf("v32 replay: Paved Path editor evidence scope mismatch"))
		}
		canonicalStudyIDs := sortedV32IDs(reportStudyDirectionIDs(data))
		if !slices.Equal(attempt.StudyIDs, canonicalStudyIDs) {
			cause := fmt.Errorf("v32 replay: Paved Path Study scope does not match the reviewed Study Map")
			return replayPavedPathLandmarksForStaleStudyScope(
				runDir,
				bundle,
				canonicalStudyIDs,
				status,
				attempt,
				cause,
			)
		}
		allowedStudyIDs = canonicalStudyIDs
	default:
		return fail(fmt.Errorf("v32 replay: unsupported Paved Path attempt version"))
	}
	record, err := pavedpath.BuildRecordScoped(
		bundle,
		proposal,
		allowedStudyIDs,
		editorEvidenceIDs,
	)
	if err != nil {
		return fail(err)
	}
	status.State = "published"
	if len(record.Paths) == 0 {
		status.State = "landmarks"
	}
	status.Failure = ""
	status.Paths = len(record.Paths)
	status.Landmarks = len(record.Landmarks)
	status.Metrics.Status = "accepted"
	status.LocalReplay = true
	if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.StatusFile), status); err != nil {
		return status, err
	}
	if err := writeGoldenJSON(recordPath, record); err != nil {
		return fail(err)
	}
	return status, nil
}

func replayPavedPathLandmarksForStaleStudyScope(
	runDir string,
	bundle pavedpath.Bundle,
	studyIDs []string,
	status pavedPathStatus,
	attempt pavedPathAttempt,
	cause error,
) (pavedPathStatus, error) {
	landmarkStatus, landmarkErr := publishPavedPathLandmarks(
		runDir,
		bundle,
		studyIDs,
		status,
		attempt,
		cause,
	)
	if landmarkStatus.State != "landmarks" {
		return landmarkStatus, landmarkErr
	}
	landmarkStatus.LocalReplay = true
	if err := writeGoldenJSON(filepath.Join(runDir, pavedpath.StatusFile), landmarkStatus); err != nil {
		return landmarkStatus, fmt.Errorf("%w; save replay landmarks status: %v", cause, err)
	}
	return landmarkStatus, nil
}

// legacyPavedStudyDirectionIDs reconstructs only the exact canonical Study
// IDs that were available to a v1 Paved Path attempt. Early evaluation runs
// did not persist that small editor scope in the Paved attempt itself. Their
// accepted, hash-bound source Study attempt is the remaining authority; model
// prose and links are never repaired during replay.
func legacyPavedStudyDirectionIDs(runDir string) ([]string, error) {
	var bundle studymap.Bundle
	if err := readV32ReplayJSON(filepath.Join(runDir, studymap.BundleFile), &bundle); err != nil {
		return nil, err
	}
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	bundleSHA, err := studymap.BundleHash(bundle)
	if err != nil {
		return nil, err
	}

	// The aggregate v2 attempt contains the exact reviewed set that preceded
	// legacy Paved editing. Prefer it over the wider v3.1 source proposal.
	reviewedPath := filepath.Join(runDir, studymap.AttemptFile)
	if exists, existsErr := v32ReplayArtifactExists(reviewedPath); existsErr != nil {
		return nil, existsErr
	} else if exists {
		var reviewed studyMapAttempt
		if err := readV32ReplayJSON(reviewedPath, &reviewed); err != nil {
			return nil, err
		}
		if reviewed.Version == 2 && reviewed.BundleSHA256 == bundleSHA &&
			reviewed.ValidationState == "accepted" &&
			(reviewed.PromptVersion == "repository-study-map-split-v2" ||
				reviewed.PromptVersion == "repository-study-map-v31-replay-plus-reviews-v2") {
			proposal, decodeErr := studymap.DecodeProposal(reviewed.Response)
			if decodeErr != nil {
				return nil, decodeErr
			}
			record, buildErr := studymap.BuildRecord(bundle, proposal)
			if buildErr != nil {
				return nil, buildErr
			}
			ids := make([]string, 0, len(record.Directions))
			for _, direction := range record.Directions {
				ids = append(ids, direction.ID)
			}
			return sortedV32IDs(ids), nil
		}
	}

	sourcePath := filepath.Join(runDir, studyMapSourceAttemptFile)
	exists, err := v32ReplayArtifactExists(sourcePath)
	if err != nil || !exists {
		return nil, err
	}
	var attempt studyMapAttempt
	if err := readV32ReplayJSON(sourcePath, &attempt); err != nil {
		return nil, err
	}
	if attempt.Version != 1 || attempt.PromptVersion != semanticdiscovery.StudyMapPromptVersion ||
		attempt.BundleSHA256 != bundleSHA || attempt.ValidationState != "accepted" {
		return nil, fmt.Errorf("v32 replay: legacy Paved Path Study scope is not bound to an accepted source attempt")
	}
	proposal, err := studymap.DecodeProposal(attempt.Response)
	if err != nil {
		return nil, err
	}
	record, err := studymap.BuildRecord(bundle, proposal)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(record.Directions))
	for _, direction := range record.Directions {
		ids = append(ids, direction.ID)
	}
	return sortedV32IDs(ids), nil
}

func readV32ReplayJSON(path string, target any) error {
	raw, err := readV32ReplayRaw(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("v32 replay: decode %s: %w", filepath.Base(path), err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("v32 replay: %s contains trailing JSON", filepath.Base(path))
	}
	return nil
}

func readV32ReplayRaw(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("v32 replay: inspect %s: %w", filepath.Base(path), err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxV32ReplayArtifactBytes {
		return nil, fmt.Errorf("v32 replay: %s is not a bounded regular file", filepath.Base(path))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("v32 replay: read %s: %w", filepath.Base(path), err)
	}
	return raw, nil
}
