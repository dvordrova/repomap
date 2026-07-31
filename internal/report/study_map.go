package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/studymap"
)

const (
	maxSavedStudyMapBytes                 = 4 << 20
	maxSavedStudyStatusBytes              = 64 << 10
	maxStudyDocumentPresentationReadBytes = 16 << 10
	maxStudyDocumentPresentationExcerpt   = 1200
)

// StudyPublicationStatus is the bounded product-facing outcome of the
// independent Study editing stage. FailureReason is retained for run
// diagnostics; default UI copy does not present it as repository content.
type StudyPublicationStatus struct {
	Version       int    `json:"version"`
	State         string `json:"state"`
	FailureReason string `json:"failure_reason,omitempty"`
	Candidates    int    `json:"candidates,omitempty"`
	Selected      int    `json:"selected_directions,omitempty"`
	WallMillis    int64  `json:"wall_ms,omitempty"`
}

func readStudyPublicationStatus(statusPath string) (*StudyPublicationStatus, string) {
	file, err := os.Open(statusPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ""
		}
		return nil, fmt.Sprintf("study publication status: %v", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Sprintf("study publication status: %v", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSavedStudyStatusBytes {
		return nil, "study publication status: invalid artifact size"
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxSavedStudyStatusBytes+1))
	if err != nil {
		return nil, fmt.Sprintf("study publication status: %v", err)
	}
	var status StudyPublicationStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return nil, "study publication status: invalid json"
	}
	if status.Version != 1 {
		return nil, fmt.Sprintf("study publication status: unsupported version %d", status.Version)
	}
	switch status.State {
	case "started":
		if status.Selected != 0 {
			return nil, "study publication status: unfinished stage cannot contain published directions"
		}
	case "failed":
		if strings.TrimSpace(status.FailureReason) == "" || status.Selected != 0 {
			return nil, "study publication status: failed stage has inconsistent outcome"
		}
	case "published":
		if status.Selected < studymap.MinDirections || status.Selected > studymap.MaxDirections ||
			strings.TrimSpace(status.FailureReason) != "" {
			return nil, "study publication status: published stage has inconsistent outcome"
		}
	default:
		return nil, fmt.Sprintf("study publication status: unsupported state %q", status.State)
	}
	if status.Candidates < 0 || status.Candidates > studymap.MaxCandidates ||
		status.WallMillis < 0 {
		return nil, "study publication status: invalid metrics"
	}
	return &status, ""
}

const (
	studyPublicationWarningEditingDidNotFinish = "Study was not published because the editing stage did not finish."
	studyPublicationWarningChecksFailed        = "Study was not published because the proposed directions did not pass the required checks."
	studyPublicationWarningNoSourceAdapter     = "Study was not published because this repository has no supported source adapter."
	studyPublicationWarningNoSourceFunctions   = "Study was not published because no eligible source functions were found."

	studyPublicationMessageEditingDidNotFinish = "main.warning.study_editing_did_not_finish"
	studyPublicationMessageChecksFailed        = "main.warning.study_checks_failed"
	studyPublicationMessageNoSourceAdapter     = "main.warning.study_no_source_adapter"
	studyPublicationMessageNoSourceFunctions   = "main.warning.study_no_source_functions"
)

func studyPublicationUserWarning(status *StudyPublicationStatus) string {
	if status == nil || status.State == "published" {
		return ""
	}
	if status.State == "started" {
		return studyPublicationWarningEditingDidNotFinish
	}
	switch status.FailureReason {
	case "no_supported_source_adapter":
		return studyPublicationWarningNoSourceAdapter
	case "no_eligible_source_functions":
		return studyPublicationWarningNoSourceFunctions
	}
	return studyPublicationWarningChecksFailed
}

func studyPublicationWarningMessageID(status *StudyPublicationStatus) string {
	if status == nil || status.State == "published" {
		return ""
	}
	if status.State == "started" {
		return studyPublicationMessageEditingDidNotFinish
	}
	switch status.FailureReason {
	case "no_supported_source_adapter":
		return studyPublicationMessageNoSourceAdapter
	case "no_eligible_source_functions":
		return studyPublicationMessageNoSourceFunctions
	}
	return studyPublicationMessageChecksFailed
}

// RepositoryStudyMap is the user-facing projection of a locally reduced
// editorial record. Support IDs, confidence, reduction diagnostics, and full
// source functions remain in the saved record rather than the default view.
type RepositoryStudyMap struct {
	Version          int                     `json:"version"`
	RepositoryType   studymap.RepositoryType `json:"repository_type"`
	Brief            RepositoryBrief         `json:"brief"`
	Shape            []RepositoryStudyArea   `json:"shape"`
	Directions       []StudyDirection        `json:"directions"`
	HiddenDirections []StudyDirection        `json:"hidden_directions,omitempty"`
}

type RepositoryIncompleteStudy struct {
	Version    int              `json:"version"`
	Directions []StudyDirection `json:"directions"`
}

type RepositoryBrief struct {
	WhatItIs              string                      `json:"what_it_is,omitempty"`
	Problem               string                      `json:"problem,omitempty"`
	MainInput             string                      `json:"main_input,omitempty"`
	CentralResponsibility string                      `json:"central_responsibility,omitempty"`
	ObservableResult      string                      `json:"observable_result,omitempty"`
	DomainTerms           []RepositoryBriefDomainTerm `json:"domain_terms,omitempty"`
}

type RepositoryBriefDomainTerm struct {
	Term    string `json:"term"`
	Meaning string `json:"meaning"`
}

type RepositoryStudyArea struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Responsibility string            `json:"responsibility"`
	CodeLocation   *UserCodeLocation `json:"code_location,omitempty"`
	Source         *SourceSnippet    `json:"source,omitempty"`
	MapTarget      *UserMapTarget    `json:"map_target,omitempty"`
}

type StudyDirection struct {
	ID               string                   `json:"id"`
	Question         string                   `json:"question"`
	WhyItMatters     string                   `json:"why_it_matters"`
	LearningOutcome  string                   `json:"learning_outcome"`
	TargetUserJob    studymap.TargetJob       `json:"target_user_job"`
	LearningStage    studymap.LearningStage   `json:"learning_stage"`
	PrincipalAnchors []StudyCodeAnchor        `json:"principal_anchors"`
	ReadingAnchors   []StudyReadingAnchor     `json:"reading_anchors"`
	Documents        []StudyDocumentReference `json:"documents,omitempty"`
	Areas            []RepositoryStudyArea    `json:"areas,omitempty"`
	MechanismID      string                   `json:"mechanism_id,omitempty"`
	SearchQueries    []string                 `json:"search_queries,omitempty"`
	DebugCoverage    *StudyDirectionCoverage  `json:"debug_coverage,omitempty"`
}

type StudyCodeAnchor struct {
	Path   string `json:"path"`
	Symbol string `json:"symbol"`
	Line   int    `json:"line"`
}

type StudyReadingAnchor struct {
	Label         string           `json:"label"`
	WhatToLookFor string           `json:"what_to_look_for"`
	Location      UserCodeLocation `json:"location"`
	Source        SourceSnippet    `json:"source"`
}

type StudyDocumentReference struct {
	Label    string           `json:"label"`
	Location UserCodeLocation `json:"location"`
	Source   *SourceSnippet   `json:"source,omitempty"`
}

type StudyDirectionCoverage struct {
	Status                 string                      `json:"status"`
	Reasons                []string                    `json:"reasons,omitempty"`
	ReadingAnchorCount     int                         `json:"reading_anchor_count"`
	ReferencedDocuments    int                         `json:"referenced_documents"`
	SourcedDocuments       int                         `json:"sourced_documents"`
	PathOnlyDocuments      []string                    `json:"path_only_documents,omitempty"`
	QuestionCoverageStatus string                      `json:"question_coverage_status,omitempty"`
	MatchedQuestionTerms   int                         `json:"matched_question_terms"`
	TotalQuestionTerms     int                         `json:"total_question_terms"`
	UserVisible            bool                        `json:"user_visible"`
	QuestionTerms          []StudyQuestionTermCoverage `json:"question_terms,omitempty"`
}

type StudyQuestionTermCoverage struct {
	Term           string                `json:"term"`
	Status         string                `json:"status"`
	SupportTargets []StudyCoverageTarget `json:"support_targets,omitempty"`
}

type StudyCoverageTarget struct {
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Symbol string `json:"symbol,omitempty"`
	Label  string `json:"label,omitempty"`
	Line   int    `json:"line,omitempty"`
}

func replaySavedStudyMap(data *ReportData, recordPath string) string {
	if data == nil {
		return "study map unavailable: report data is required"
	}
	data.StudyMap = nil
	info, err := os.Lstat(recordPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return "study map unavailable: cannot inspect saved record"
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSavedStudyMapBytes {
		return "study map unavailable: saved record is not a bounded regular file"
	}
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		return "study map unavailable: cannot read saved record"
	}
	record, err := studymap.DecodeRecord(raw)
	if err != nil {
		return fmt.Sprintf("study map unavailable: %v", err)
	}
	tracked, err := readOperationalTrackedPaths(
		filepath.Join(filepath.Dir(recordPath), "snapshot.json"),
		record.Bundle.AllowedPaths,
	)
	if err != nil {
		return fmt.Sprintf("study map unavailable: %v", err)
	}
	projected, err := projectRepositoryStudyMap(data, record, tracked)
	if err != nil {
		return fmt.Sprintf("study map unavailable: %v", err)
	}
	data.StudyMap = projected
	return ""
}

type incompleteStudyAttempt struct {
	Version              int                                    `json:"version"`
	PromptVersion        string                                 `json:"prompt_version"`
	BundleSHA256         string                                 `json:"bundle_sha256"`
	ValidationState      string                                 `json:"validation_state"`
	FailureReason        string                                 `json:"failure_reason,omitempty"`
	Metrics              json.RawMessage                        `json:"metrics"`
	DirectionDiagnostics *studymap.DirectionProposalDiagnostics `json:"direction_diagnostics,omitempty"`
	Response             json.RawMessage                        `json:"response,omitempty"`
	RawResponse          string                                 `json:"raw_response,omitempty"`
}

func replaySavedIncompleteStudy(
	data *ReportData,
	bundlePath string,
	attemptPath string,
) string {
	if data == nil {
		return "incomplete study unavailable: report data is required"
	}
	data.IncompleteStudy = nil
	bundleRaw, exists, err := readBoundedStudyArtifact(bundlePath)
	if err != nil {
		return fmt.Sprintf("incomplete study unavailable: %v", err)
	}
	if !exists {
		return ""
	}
	attemptRaw, exists, err := readBoundedStudyArtifact(attemptPath)
	if err != nil {
		return fmt.Sprintf("incomplete study unavailable: %v", err)
	}
	if !exists {
		return ""
	}
	bundle, err := studymap.DecodeBundle(bundleRaw)
	if err != nil {
		return fmt.Sprintf("incomplete study unavailable: %v", err)
	}
	var attempt incompleteStudyAttempt
	decoder := json.NewDecoder(bytes.NewReader(attemptRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&attempt); err != nil {
		return fmt.Sprintf("incomplete study unavailable: decode attempt: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "incomplete study unavailable: attempt has trailing JSON"
	}
	if attempt.Version != 1 ||
		attempt.PromptVersion != semanticdiscovery.StudyCandidatesPromptVersion ||
		(attempt.ValidationState != "accepted" && attempt.ValidationState != "rejected") ||
		len(attempt.Response) == 0 {
		return "incomplete study unavailable: attempt has no transport-valid bounded response"
	}
	bundleSHA, err := studymap.BundleHash(bundle)
	if err != nil || bundleSHA != attempt.BundleSHA256 {
		return "incomplete study unavailable: attempt bundle hash mismatch"
	}
	directions, _, err := studymap.DecodeIncompleteDirections(attempt.Response, bundle)
	if err != nil {
		return fmt.Sprintf("incomplete study unavailable: %v", err)
	}
	tracked, err := readOperationalTrackedPaths(
		filepath.Join(filepath.Dir(bundlePath), "snapshot.json"),
		bundle.AllowedPaths,
	)
	if err != nil {
		return fmt.Sprintf("incomplete study unavailable: %v", err)
	}
	projected := projectIncompleteStudy(data, bundle, directions, tracked)
	if len(projected) > 0 {
		data.IncompleteStudy = &RepositoryIncompleteStudy{
			Version:    1,
			Directions: projected,
		}
	}
	return ""
}

func readBoundedStudyArtifact(filePath string) ([]byte, bool, error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("cannot inspect saved artifact")
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSavedStudyMapBytes {
		return nil, false, fmt.Errorf("saved artifact is not a bounded regular file")
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false, fmt.Errorf("cannot read saved artifact")
	}
	return raw, true, nil
}

func projectIncompleteStudy(
	data *ReportData,
	bundle studymap.Bundle,
	directions []studymap.IncompleteDirection,
	trackedPaths []string,
) []StudyDirection {
	openable := make(map[string]struct{}, len(data.OpenablePaths)+len(trackedPaths))
	for _, filePath := range data.OpenablePaths {
		openable[filePath] = struct{}{}
	}
	for _, filePath := range trackedPaths {
		openable[filePath] = struct{}{}
	}
	complete := make(map[string]struct{})
	if data.StudyMap != nil {
		for _, direction := range data.StudyMap.Directions {
			complete[direction.ID] = struct{}{}
		}
		for _, direction := range data.StudyMap.HiddenDirections {
			complete[direction.ID] = struct{}{}
		}
	}
	anchorByID := make(map[string]studymap.Anchor, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		anchorByID[anchor.ID] = anchor
	}
	result := make([]StudyDirection, 0, len(directions))
	for _, direction := range directions {
		if _, duplicate := complete[direction.DirectionID]; duplicate {
			continue
		}
		projected := StudyDirection{
			ID:              direction.DirectionID,
			Question:        direction.Question,
			WhyItMatters:    direction.WhyItMatters,
			LearningOutcome: direction.LearningOutcome,
			TargetUserJob:   direction.TargetJob,
			LearningStage:   direction.LearningStage,
		}
		valid := true
		candidatePaths := make([]string, 0, len(direction.ReadingAnchors))
		for _, reading := range direction.ReadingAnchors {
			anchor, ok := anchorByID[reading.AnchorID]
			if !ok {
				valid = false
				break
			}
			if _, ok := openable[anchor.Path]; !ok {
				valid = false
				break
			}
			source, ok := projectStudyAnchorSource(data, anchor)
			if !ok {
				valid = false
				break
			}
			projected.PrincipalAnchors = append(projected.PrincipalAnchors, StudyCodeAnchor{
				Path: anchor.Path, Symbol: anchor.Symbol, Line: anchor.Line,
			})
			projected.ReadingAnchors = append(projected.ReadingAnchors, StudyReadingAnchor{
				Label: reading.Label, WhatToLookFor: reading.WhatToLookFor,
				Location: UserCodeLocation{Path: anchor.Path, Line: anchor.Line},
				Source:   source,
			})
			candidatePaths = append(candidatePaths, anchor.Path)
		}
		if valid && len(projected.ReadingAnchors) > 0 {
			for _, filePath := range candidatePaths {
				data.OpenablePaths = appendUniqueString(data.OpenablePaths, filePath)
			}
			result = append(result, projected)
		}
	}
	sort.Strings(data.OpenablePaths)
	return result
}

func projectRepositoryStudyMap(
	data *ReportData,
	record studymap.Record,
	trackedPaths []string,
) (*RepositoryStudyMap, error) {
	openable := make(map[string]struct{}, len(data.OpenablePaths)+len(trackedPaths))
	for _, filePath := range data.OpenablePaths {
		openable[filePath] = struct{}{}
	}
	for _, filePath := range trackedPaths {
		openable[filePath] = struct{}{}
	}
	for _, filePath := range record.Bundle.AllowedPaths {
		if _, ok := openable[filePath]; !ok {
			return nil, fmt.Errorf("saved path %q is no longer openable", filePath)
		}
		data.OpenablePaths = appendUniqueString(data.OpenablePaths, filePath)
	}
	sort.Strings(data.OpenablePaths)
	mechanisms := make(map[string]struct{}, len(data.UserMechanisms))
	for _, mechanism := range data.UserMechanisms {
		mechanisms[mechanism.ArtifactID] = struct{}{}
	}
	for _, mechanism := range record.Bundle.Mechanisms {
		if _, ok := mechanisms[mechanism.ID]; !ok {
			return nil, fmt.Errorf("saved mechanism %q is not canonical in this report", mechanism.ID)
		}
	}
	anchorByID := make(map[string]studymap.Anchor, len(record.Bundle.Anchors))
	for _, anchor := range record.Bundle.Anchors {
		anchorByID[anchor.ID] = anchor
	}
	documentByID := make(map[string]studymap.Document, len(record.Bundle.Documents))
	for _, document := range record.Bundle.Documents {
		documentByID[document.ID] = document
	}
	areaByID := make(map[string]studymap.Area, len(record.Bundle.Areas))
	for _, area := range record.Bundle.Areas {
		areaByID[area.ID] = area
	}
	result := &RepositoryStudyMap{
		Version: record.Version, RepositoryType: record.RepositoryType,
		Brief: projectRepositoryBrief(record.Brief),
	}
	for _, areaID := range record.ShapeAreaIDs {
		result.Shape = append(result.Shape, projectStudyArea(data, areaByID[areaID], record.Bundle.Anchors))
	}
	for _, direction := range record.Directions {
		reviewSources := studyReviewSources(record.Bundle, direction)
		projected := StudyDirection{
			ID: direction.ID, Question: direction.Question,
			WhyItMatters:    direction.WhyItMatters,
			LearningOutcome: direction.LearningOutcome,
			TargetUserJob:   direction.TargetJob,
			LearningStage:   direction.LearningStage,
			MechanismID:     direction.MechanismID,
			SearchQueries:   append([]string(nil), direction.SearchQueries...),
		}
		if projected.MechanismID != "" {
			if _, ok := mechanisms[projected.MechanismID]; !ok {
				return nil, fmt.Errorf("direction references unavailable mechanism")
			}
		}
		for _, anchorID := range direction.AnchorIDs {
			anchor, ok := anchorByID[anchorID]
			if !ok {
				return nil, fmt.Errorf("direction references unavailable code anchor")
			}
			projected.PrincipalAnchors = append(projected.PrincipalAnchors, StudyCodeAnchor{
				Path: anchor.Path, Symbol: anchor.Symbol, Line: anchor.Line,
			})
		}
		for _, reading := range direction.ReadingAnchors {
			anchor, ok := anchorByID[reading.AnchorID]
			if !ok {
				return nil, fmt.Errorf("reading path references unavailable code anchor")
			}
			source, ok := projectStudyAnchorSource(data, anchor)
			if !ok {
				return nil, fmt.Errorf("reading path has no valid source for %s", anchor.Path)
			}
			if fragment, exists := reviewSources[reading.AnchorID]; exists {
				if reviewed, reviewErr := projectStudyReviewSource(source, anchor, fragment); reviewErr == nil {
					source = reviewed
				}
			}
			projected.ReadingAnchors = append(projected.ReadingAnchors, StudyReadingAnchor{
				Label: reading.Label, WhatToLookFor: reading.WhatToLookFor,
				Location: UserCodeLocation{Path: anchor.Path, Line: anchor.Line},
				Source:   source,
			})
		}
		for _, documentID := range direction.DocumentIDs {
			document := documentByID[documentID]
			reference := StudyDocumentReference{
				Label: document.Label, Location: UserCodeLocation{Path: document.Path},
			}
			if source, ok := projectStudyDocumentSource(data, document); ok {
				reference.Source = &source
			}
			projected.Documents = append(projected.Documents, reference)
		}
		for _, areaID := range direction.AreaIDs {
			projected.Areas = append(projected.Areas, projectStudyArea(data, areaByID[areaID], record.Bundle.Anchors))
		}
		projected.DebugCoverage = studyDirectionCoverage(projected)
		if len(projected.ReadingAnchors) < 3 {
			return nil, fmt.Errorf("direction has fewer than three source-backed reading anchors")
		}
		result.Directions = append(result.Directions, projected)
	}
	result.Directions, result.HiddenDirections = splitStudyDirectionsForVisibleCoverage(result.Directions)
	return result, nil
}

func studyDirectionCoverage(direction StudyDirection) *StudyDirectionCoverage {
	coverage := &StudyDirectionCoverage{
		Status:              "source_backed_navigation",
		ReadingAnchorCount:  len(direction.ReadingAnchors),
		ReferencedDocuments: len(direction.Documents),
		UserVisible:         true,
	}
	for _, document := range direction.Documents {
		if document.Source != nil {
			coverage.SourcedDocuments++
			continue
		}
		if strings.TrimSpace(document.Location.Path) != "" {
			coverage.PathOnlyDocuments = append(coverage.PathOnlyDocuments, document.Location.Path)
		}
	}
	if len(direction.Documents) == 0 {
		coverage.Status = "code_anchor_only"
		coverage.Reasons = append(coverage.Reasons, "no_document_reference")
	}
	if len(coverage.PathOnlyDocuments) > 0 {
		coverage.Status = "partial"
		coverage.Reasons = append(coverage.Reasons, "path_only_document_present")
	}
	coverage.QuestionTerms = studyQuestionTermCoverage(direction)
	coverage.TotalQuestionTerms = len(coverage.QuestionTerms)
	for _, term := range coverage.QuestionTerms {
		if term.Status == "matched" {
			coverage.MatchedQuestionTerms++
		}
	}
	coverage.QuestionCoverageStatus = studyQuestionCoverageStatus(coverage.QuestionTerms)
	switch coverage.QuestionCoverageStatus {
	case "no_terms_extracted":
		coverage.Reasons = append(coverage.Reasons, "question_terms_not_extracted")
	case "partial_terms_matched", "no_terms_matched":
		coverage.Reasons = append(coverage.Reasons, "question_term_uncovered")
	}
	return coverage
}

func splitStudyDirectionsForVisibleCoverage(directions []StudyDirection) ([]StudyDirection, []StudyDirection) {
	// Question-term coverage remains useful diagnostics, but it is not strong
	// enough to suppress a source-reviewed direction. Natural-language wording,
	// translations, and repository terminology routinely differ from exact code
	// tokens even when every published reading anchor is valid.
	for index := range directions {
		markStudyDirectionUserVisible(&directions[index], true, "")
	}
	return directions, nil
}

func markStudyDirectionUserVisible(direction *StudyDirection, visible bool, reason string) {
	if direction == nil || direction.DebugCoverage == nil {
		return
	}
	direction.DebugCoverage.UserVisible = visible
	if reason != "" && !containsString(direction.DebugCoverage.Reasons, reason) {
		direction.DebugCoverage.Reasons = append(direction.DebugCoverage.Reasons, reason)
	}
}

func studyQuestionCoverageStatus(terms []StudyQuestionTermCoverage) string {
	if len(terms) == 0 {
		return "no_terms_extracted"
	}
	covered := 0
	for _, term := range terms {
		if term.Status == "matched" {
			covered++
		}
	}
	if covered == len(terms) {
		return "all_terms_matched"
	}
	if covered > 0 {
		return "partial_terms_matched"
	}
	return "no_terms_matched"
}

func studyQuestionTermCoverage(direction StudyDirection) []StudyQuestionTermCoverage {
	terms := studyQuestionTerms(direction.Question)
	result := make([]StudyQuestionTermCoverage, 0, len(terms))
	for _, term := range terms {
		targets := studyQuestionTermSupportTargets(direction, term)
		status := "unmatched"
		if len(targets) > 0 {
			status = "matched"
		}
		result = append(result, StudyQuestionTermCoverage{
			Term:           term,
			Status:         status,
			SupportTargets: targets,
		})
	}
	return result
}

func studyQuestionTerms(question string) []string {
	stopWords := map[string]struct{}{
		"a": {}, "about": {}, "after": {}, "all": {}, "an": {}, "and": {}, "are": {},
		"as": {}, "at": {}, "be": {}, "between": {}, "by": {}, "can": {}, "do": {},
		"does": {}, "for": {}, "from": {}, "how": {}, "i": {}, "in": {}, "into": {},
		"is": {}, "it": {}, "of": {}, "on": {}, "or": {}, "overall": {}, "project": {},
		"get": {}, "like": {}, "repository": {}, "should": {}, "the": {}, "them": {},
		"this": {}, "through": {}, "to": {},
		"under": {}, "use": {}, "using": {}, "what": {}, "when": {}, "where": {},
		"which": {}, "with": {}, "work": {}, "works": {},
	}
	seen := map[string]struct{}{}
	var terms []string
	for _, token := range studyCoverageTokens(question) {
		if _, stop := stopWords[token]; stop {
			continue
		}
		term := studyQuestionTerm(token)
		if len(term) < 3 {
			continue
		}
		if _, stop := stopWords[term]; stop {
			continue
		}
		if _, exists := seen[term]; exists {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
		if len(terms) >= 8 {
			break
		}
	}
	return terms
}

func studyQuestionTerm(token string) string {
	switch {
	case len(token) > 5 && strings.HasSuffix(token, "ies"):
		return strings.TrimSuffix(token, "ies") + "y"
	case len(token) > 3 && strings.HasSuffix(token, "s") && !strings.HasSuffix(token, "ss"):
		return strings.TrimSuffix(token, "s")
	default:
		return token
	}
}

func studyQuestionTermSupportTargets(direction StudyDirection, term string) []StudyCoverageTarget {
	var targets []StudyCoverageTarget
	addTarget := func(target StudyCoverageTarget) {
		if len(targets) >= 5 {
			return
		}
		for _, existing := range targets {
			if existing == target {
				return
			}
		}
		targets = append(targets, target)
	}
	for _, anchor := range direction.ReadingAnchors {
		if studyTextMatchesTerm(strings.Join([]string{
			anchor.Label,
			anchor.WhatToLookFor,
			anchor.Location.Path,
			anchor.Source.EnclosingSymbol,
			anchor.Source.Content,
		}, "\n"), term) {
			addTarget(StudyCoverageTarget{
				Kind:   "code_anchor",
				Path:   anchor.Location.Path,
				Symbol: anchor.Source.EnclosingSymbol,
				Label:  anchor.Label,
				Line:   anchor.Location.Line,
			})
		}
	}
	for _, document := range direction.Documents {
		var content string
		if document.Source != nil {
			content = document.Source.Content
		}
		if studyTextMatchesTerm(strings.Join([]string{
			document.Label,
			document.Location.Path,
			content,
		}, "\n"), term) {
			addTarget(StudyCoverageTarget{
				Kind:  "document",
				Path:  document.Location.Path,
				Label: document.Label,
			})
		}
	}
	for _, area := range direction.Areas {
		var path string
		var sourceContent string
		if area.CodeLocation != nil {
			path = area.CodeLocation.Path
		}
		if area.Source != nil {
			sourceContent = area.Source.Content
		}
		if studyTextMatchesTerm(strings.Join([]string{
			area.Name,
			area.Responsibility,
			path,
			sourceContent,
		}, "\n"), term) {
			addTarget(StudyCoverageTarget{
				Kind:  "area",
				Path:  path,
				Label: area.Name,
			})
		}
	}
	return targets
}

func studyTextMatchesTerm(text, term string) bool {
	if term == "" {
		return false
	}
	tokens := studyCoverageTokenSet(text)
	for _, variant := range studyCoverageTermVariants(term) {
		if _, ok := tokens[variant]; ok {
			return true
		}
		if len(variant) < 5 {
			continue
		}
		for token := range tokens {
			if len(token) >= len(variant)+3 && strings.Contains(token, variant) {
				return true
			}
		}
	}
	return false
}

func studyCoverageTokenSet(text string) map[string]struct{} {
	tokens := studyCoverageTokens(text)
	result := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		result[token] = struct{}{}
		stem := studyCoverageStem(token)
		if stem != token {
			result[stem] = struct{}{}
		}
	}
	return result
}

func studyCoverageTokens(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r < '0' || r > '9' && r < 'A' || r > 'Z' && r < 'a' || r > 'z'
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.ToLower(strings.TrimSpace(field))
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func studyCoverageTermVariants(term string) []string {
	variants := []string{term}
	stem := studyCoverageStem(term)
	if stem != term {
		variants = append(variants, stem)
	}
	if strings.HasSuffix(term, "e") && len(term) > 4 {
		variants = append(variants, strings.TrimSuffix(term, "e"))
	}
	if strings.HasSuffix(term, "ed") && len(term) > 4 {
		base := strings.TrimSuffix(term, "ed")
		variants = append(variants, base, base+"e")
	}
	if strings.HasSuffix(term, "y") && len(term) > 3 {
		variants = append(variants, strings.TrimSuffix(term, "y")+"ies")
	}
	if !strings.HasSuffix(term, "s") && len(term) > 3 {
		variants = append(variants, term+"s")
	}
	return variants
}

func studyCoverageStem(token string) string {
	switch {
	case len(token) > 5 && strings.HasSuffix(token, "ies"):
		return strings.TrimSuffix(token, "ies") + "y"
	case len(token) > 5 && strings.HasSuffix(token, "ing"):
		return strings.TrimSuffix(token, "ing")
	case len(token) > 4 && strings.HasSuffix(token, "ed") &&
		strings.HasSuffix(strings.TrimSuffix(token, "d"), "e"):
		return strings.TrimSuffix(token, "d")
	case len(token) > 4 && strings.HasSuffix(token, "ed"):
		return strings.TrimSuffix(token, "ed")
	case len(token) > 3 && strings.HasSuffix(token, "s") && !strings.HasSuffix(token, "ss"):
		return strings.TrimSuffix(token, "s")
	default:
		return token
	}
}

func studyReviewSources(
	bundle studymap.Bundle,
	direction studymap.Direction,
) map[string][]studymap.ReviewSourceLine {
	candidate := studymap.DirectionCandidate{
		Question: direction.Question, WhyItMatters: direction.WhyItMatters,
		LearningOutcome: direction.LearningOutcome, TargetJob: direction.TargetJob,
		LearningStage:  direction.LearningStage,
		AnchorIDs:      append([]string(nil), direction.AnchorIDs...),
		DocumentIDs:    append([]string(nil), direction.DocumentIDs...),
		AreaIDs:        append([]string(nil), direction.AreaIDs...),
		MechanismID:    direction.MechanismID,
		ReadingAnchors: append([]studymap.ReadingAnchor(nil), direction.ReadingAnchors...),
		SearchQueries:  append([]string(nil), direction.SearchQueries...),
	}
	review, err := studymap.BuildReviewBundle(bundle, candidate)
	if err != nil {
		return nil
	}
	result := make(map[string][]studymap.ReviewSourceLine, len(review.Anchors))
	for _, anchor := range review.Anchors {
		result[anchor.AnchorID] = append([]studymap.ReviewSourceLine(nil), anchor.SourceFragment...)
	}
	return result
}

func projectStudyReviewSource(
	base SourceSnippet,
	anchor studymap.Anchor,
	fragment []studymap.ReviewSourceLine,
) (SourceSnippet, error) {
	if len(fragment) == 0 || len(fragment) > maxInlineSourceLines {
		return SourceSnippet{}, fmt.Errorf("study review source is outside line bounds")
	}
	lines := make([]SourceSnippetLine, 0, len(fragment))
	texts := make([]string, 0, len(fragment))
	for index, line := range fragment {
		if line.Line <= 0 || index > 0 && line.Line != fragment[index-1].Line+1 {
			return SourceSnippet{}, fmt.Errorf("study review source is not contiguous")
		}
		lines = append(lines, SourceSnippetLine{
			Line: line.Line, Text: line.Text, Highlight: line.Line == anchor.Line,
		})
		texts = append(texts, line.Text)
	}
	base.StartLine = lines[0].Line
	base.EndLine = lines[len(lines)-1].Line
	base.HighlightRanges = []SourceHighlight{{StartLine: anchor.Line, EndLine: anchor.Line}}
	base.Content = strings.Join(texts, "\n")
	base.Lines = lines
	if !base.SourceComplete {
		base.ContentSHA256 = sourceLinesSHA256(texts)
	}
	base.PresentationSHA256 = sourceSnippetPresentationSHA(base)
	if err := base.Validate(); err != nil {
		return SourceSnippet{}, err
	}
	return base, nil
}

func projectRepositoryBrief(brief studymap.Brief) RepositoryBrief {
	result := RepositoryBrief{
		WhatItIs:              brief.WhatItIs.Text,
		Problem:               brief.Problem.Text,
		MainInput:             brief.MainInput.Text,
		CentralResponsibility: brief.CentralResponsibility.Text,
		ObservableResult:      brief.ObservableResult.Text,
	}
	for _, term := range brief.DomainTerms {
		result.DomainTerms = append(result.DomainTerms, RepositoryBriefDomainTerm{
			Term: term.Term, Meaning: term.Meaning,
		})
	}
	return result
}

func projectStudyArea(
	data *ReportData,
	area studymap.Area,
	anchors []studymap.Anchor,
) RepositoryStudyArea {
	result := RepositoryStudyArea{
		ID: area.ID, Name: area.Name, Responsibility: area.Responsibility,
	}
	var sourceAnchor *studymap.Anchor
	for index := range anchors {
		anchor := &anchors[index]
		if anchor.Path == area.Path {
			sourceAnchor = anchor
			break
		}
		for _, areaID := range anchor.AreaIDs {
			if areaID == area.ID && sourceAnchor == nil {
				sourceAnchor = anchor
			}
		}
	}
	if area.Path != "" {
		result.CodeLocation = &UserCodeLocation{Path: area.Path, Line: area.Line}
	}
	if sourceAnchor != nil {
		if source, ok := projectStudyAnchorSource(data, *sourceAnchor); ok {
			result.Source = &source
			result.CodeLocation = &UserCodeLocation{Path: sourceAnchor.Path, Line: sourceAnchor.Line}
		}
	}
	if area.ComponentID != "" && data.ArchitectureCanvas != nil {
		for _, component := range data.ArchitectureCanvas.Components {
			if string(component.ID) != area.ComponentID {
				continue
			}
			result.MapTarget = &UserMapTarget{
				Kind:        SemanticSearchTargetComponent,
				ComponentID: componentmap.ComponentID(area.ComponentID),
			}
			break
		}
	}
	return result
}

func projectStudyAnchorSource(data *ReportData, anchor studymap.Anchor) (SourceSnippet, bool) {
	lines := make([]savedSourceLine, 0, len(anchor.Function.Lines))
	for index, text := range anchor.Function.Lines {
		lines = append(lines, savedSourceLine{
			line: anchor.Function.StartLine + index, text: text,
		})
	}
	candidate := savedSourceCandidate{
		path: anchor.Path, symbol: anchor.Symbol, lines: lines,
		contentSHA: anchor.Function.ContentSHA256,
		complete:   !anchor.Function.Partial, fullFunction: !anchor.Function.Partial,
	}
	reference := semanticdiscovery.EvidenceRef{
		ID: anchor.ID, Kind: "study_anchor", Label: "editorial reading anchor",
		Path: anchor.Path, Line: anchor.Line,
	}
	group := sourceSnippetGroup{
		candidate: candidate, evidence: []semanticdiscovery.EvidenceRef{reference},
		dominantEvidence: []semanticdiscovery.EvidenceRef{reference},
		inlineEvidence:   []semanticdiscovery.EvidenceRef{reference},
	}
	snippet, ok := sourceSnippetFromGroup(data, group, "primary")
	if !ok {
		return SourceSnippet{}, false
	}
	return snippet, true
}

func projectStudyDocumentSource(data *ReportData, document studymap.Document) (SourceSnippet, bool) {
	excerpt := strings.TrimSpace(document.Excerpt)
	if excerpt == "" {
		excerpt = readStudyDocumentPresentationExcerpt(data, document.Path)
	}
	if excerpt == "" {
		return SourceSnippet{}, false
	}
	rawLines := strings.Split(strings.ReplaceAll(excerpt, "\r\n", "\n"), "\n")
	if len(rawLines) > maxInlineSourceLines {
		rawLines = rawLines[:maxInlineSourceLines]
	}
	lines := make([]SourceSnippetLine, 0, len(rawLines))
	for index, line := range rawLines {
		lines = append(lines, SourceSnippetLine{Line: index + 1, Text: line})
	}
	snippet := SourceSnippet{
		Path:               document.Path,
		Language:           sourceLanguage(document.Path),
		StartLine:          1,
		EndLine:            len(lines),
		HighlightRanges:    []SourceHighlight{},
		Content:            strings.Join(rawLines, "\n"),
		Lines:              lines,
		ContentSHA256:      sourceLinesSHA256(rawLines),
		RelatedEvidenceIDs: []string{document.ID},
		Role:               "related",
		Revision:           reportSourceRevision(data),
	}
	snippet.PresentationSHA256 = sourceSnippetPresentationSHA(snippet)
	if err := snippet.Validate(); err != nil {
		return SourceSnippet{}, false
	}
	return snippet, true
}

func readStudyDocumentPresentationExcerpt(data *ReportData, filePath string) string {
	if data == nil || strings.TrimSpace(data.studyDocumentSourceRoot) == "" ||
		!stringSliceContains(data.OpenablePaths, filePath) ||
		!studyDocumentPresentationPath(filePath) {
		return ""
	}
	resolved, ok := resolveStudyDocumentPresentationPath(data.studyDocumentSourceRoot, filePath)
	if !ok {
		return ""
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return ""
	}
	file, err := os.Open(resolved)
	if err != nil {
		return ""
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxStudyDocumentPresentationReadBytes))
	if err != nil {
		return ""
	}
	return truncateStudyDocumentPresentationText(
		strings.TrimSpace(string(raw)),
		maxStudyDocumentPresentationExcerpt,
	)
}

func resolveStudyDocumentPresentationPath(root, filePath string) (string, bool) {
	cleaned := path.Clean(filePath)
	if filePath == "" || cleaned != filePath || cleaned == "." || path.IsAbs(filePath) ||
		strings.Contains(filePath, "\\") || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	resolved := filepath.Join(root, filepath.FromSlash(filePath))
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return resolved, true
}

func studyDocumentPresentationPath(filePath string) bool {
	base := strings.ToLower(path.Base(filePath))
	ext := strings.ToLower(path.Ext(base))
	return strings.HasPrefix(base, "readme") || ext == ".md" || ext == ".mdx" || ext == ".rst"
}

func truncateStudyDocumentPresentationText(value string, byteLimit int) string {
	if len(value) <= byteLimit {
		return value
	}
	for byteLimit > 0 && !utf8.RuneStart(value[byteLimit]) {
		byteLimit--
	}
	return strings.TrimSpace(value[:byteLimit])
}

func sortedStudyDirectionIDs(data *ReportData) []string {
	if data == nil || data.StudyMap == nil {
		return nil
	}
	result := make([]string, 0, len(data.StudyMap.Directions))
	for _, direction := range data.StudyMap.Directions {
		result = append(result, direction.ID)
	}
	sort.Strings(result)
	return result
}

func studyDirectionByID(data *ReportData, directionID string) (StudyDirection, bool) {
	if data == nil || data.StudyMap == nil {
		return StudyDirection{}, false
	}
	for _, direction := range data.StudyMap.Directions {
		if direction.ID == strings.TrimSpace(directionID) {
			return direction, true
		}
	}
	return StudyDirection{}, false
}

func studyMapRecordPath(runDir string) string {
	return filepath.Join(runDir, studymap.RecordFile)
}
