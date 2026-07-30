package report

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
	"github.com/dvordrova/repomap/internal/studymap"
)

func TestProjectRepositoryStudyMapKeepsEditorialDirectionsCodeFirst(t *testing.T) {
	t.Parallel()

	record := studyMapReportFixture(t)
	data := &ReportData{
		RepoName:         "fixture",
		CapturedRevision: "deadbeef",
		OpenablePaths:    append([]string(nil), record.Bundle.AllowedPaths...),
	}
	projected, err := projectRepositoryStudyMap(data, record, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Shape) != 3 ||
		len(projected.Directions) < studymap.MinDirections ||
		len(projected.Directions)+len(projected.HiddenDirections) != 4 {
		t.Fatalf(
			"projected Study Map = %d areas, %d visible directions, %d hidden directions",
			len(projected.Shape),
			len(projected.Directions),
			len(projected.HiddenDirections),
		)
	}
	for _, area := range projected.Shape {
		if area.CodeLocation == nil || area.Source == nil || len(area.Source.Lines) == 0 {
			t.Fatalf("repository area is not directly source-navigable: %#v", area)
		}
	}
	if sources := studyReviewSources(record.Bundle, record.Directions[0]); len(sources) != 3 {
		t.Fatalf("review sources = %#v", sources)
	}
	firstAnchor := record.Bundle.Anchors[0]
	base, ok := projectStudyAnchorSource(data, firstAnchor)
	if !ok {
		t.Fatal("fixture anchor source is unavailable")
	}
	fragment := studyReviewSources(record.Bundle, record.Directions[0])[firstAnchor.ID]
	if _, err := projectStudyReviewSource(base, firstAnchor, fragment); err != nil {
		t.Fatalf("project review source: %v", err)
	}
	for _, direction := range projected.Directions {
		if len(direction.ReadingAnchors) != 3 || len(direction.PrincipalAnchors) != 3 {
			t.Fatalf("direction %q = %d reading anchors, %d principal anchors", direction.Question, len(direction.ReadingAnchors), len(direction.PrincipalAnchors))
		}
		for _, reading := range direction.ReadingAnchors {
			if err := reading.Source.Validate(); err != nil {
				t.Fatalf("source for %q: %v", reading.Location.Path, err)
			}
			if len(reading.Source.Lines) <= preferredInlineSourceLines ||
				len(reading.Source.Lines) > maxInlineSourceLines {
				t.Fatalf("source for %q has %d inline lines", reading.Location.Path, len(reading.Source.Lines))
			}
			if !sourceSnippetContainsRange(reading.Source.Lines, SourceHighlight{
				StartLine: reading.Location.Line,
				EndLine:   reading.Location.Line,
			}) {
				t.Fatalf("source for %q does not show its exact anchor line %d", reading.Location.Path, reading.Location.Line)
			}
		}
		if len(direction.Documents) != 1 || direction.Documents[0].Source == nil {
			t.Fatalf("direction %q has no static documentation source: %#v", direction.Question, direction.Documents)
		}
		if err := direction.Documents[0].Source.Validate(); err != nil {
			t.Fatalf("documentation source for %q: %v", direction.Question, err)
		}
		if direction.DebugCoverage == nil ||
			direction.DebugCoverage.Status != "source_backed_navigation" ||
			direction.DebugCoverage.SourcedDocuments != 1 ||
			direction.DebugCoverage.QuestionCoverageStatus == "" ||
			len(direction.DebugCoverage.QuestionTerms) == 0 ||
			len(direction.DebugCoverage.PathOnlyDocuments) != 0 {
			t.Fatalf("debug coverage for %q = %#v", direction.Question, direction.DebugCoverage)
		}
	}
	if projected.Brief.WhatItIs == "" || projected.Brief.CentralResponsibility == "" {
		t.Fatalf("Repository Brief was not projected: %#v", projected.Brief)
	}
}

func TestReadStudyPublicationStatusKeepsFailedStageExplicit(t *testing.T) {
	t.Parallel()

	statusPath := filepath.Join(t.TempDir(), studymap.StatusFile)
	writeTestFileFullPath(t, statusPath, `{
		"version": 1,
		"state": "failed",
		"failure_reason": "reviewed selection has 0 directions; need at least 3",
		"candidates": 8,
		"wall_ms": 170625
	}`)

	status, warning := readStudyPublicationStatus(statusPath)
	if warning != "" {
		t.Fatalf("warning = %q", warning)
	}
	if status == nil || status.State != "failed" || status.Candidates != 8 {
		t.Fatalf("status = %#v", status)
	}
	if got := studyPublicationUserWarning(status); !strings.Contains(got, "not published") {
		t.Fatalf("user warning = %q", got)
	}
}

func TestReadStudyPublicationStatusAcceptsOnePublishedDirection(t *testing.T) {
	t.Parallel()

	statusPath := filepath.Join(t.TempDir(), studymap.StatusFile)
	writeTestFileFullPath(t, statusPath, `{
		"version": 1,
		"state": "published",
		"candidates": 4,
		"selected_directions": 1,
		"wall_ms": 170625
	}`)

	status, warning := readStudyPublicationStatus(statusPath)
	if warning != "" {
		t.Fatalf("warning = %q", warning)
	}
	if status == nil || status.State != "published" || status.Selected != 1 {
		t.Fatalf("status = %#v", status)
	}
}

func TestReadStudyPublicationStatusRejectsPublishedZeroDirections(t *testing.T) {
	t.Parallel()

	statusPath := filepath.Join(t.TempDir(), studymap.StatusFile)
	writeTestFileFullPath(t, statusPath, `{
		"version": 1,
		"state": "published",
		"candidates": 4,
		"wall_ms": 170625
	}`)

	status, warning := readStudyPublicationStatus(statusPath)
	if status != nil || !strings.Contains(warning, "inconsistent outcome") {
		t.Fatalf("status = %#v, warning = %q", status, warning)
	}
}

func TestReadStudyPublicationStatusRejectsFailedStageWithPublishedDirections(t *testing.T) {
	t.Parallel()

	statusPath := filepath.Join(t.TempDir(), studymap.StatusFile)
	writeTestFileFullPath(t, statusPath, `{
		"version": 1,
		"state": "failed",
		"failure_reason": "rejected",
		"selected_directions": 3
	}`)

	status, warning := readStudyPublicationStatus(statusPath)
	if status != nil || !strings.Contains(warning, "inconsistent outcome") {
		t.Fatalf("status = %#v, warning = %q", status, warning)
	}
}

func TestReplaySavedIncompleteStudyRetainsExactStartsFromRejectedAttempt(t *testing.T) {
	t.Parallel()

	record := studyMapReportFixture(t)
	bundle := record.Bundle
	bundleSHA, err := studymap.BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	responseRaw, err := json.Marshal(studymap.DirectionProposal{
		Version: studymap.DirectionProposalVersion,
		Directions: []studymap.DirectionCandidate{{
			Question:        "Как участнику начать изучение результата fixture?",
			WhyItMatters:    "Это указывает на точное объявление production-кода для изучения.",
			LearningOutcome: "Читатель сможет найти сохранённую реализацию результата.",
			TargetJob:       studymap.JobFirstContact,
			LearningStage:   studymap.StageOrientation,
			AnchorIDs:       []string{bundle.Anchors[2].ID},
			ReadingAnchors: []studymap.ReadingAnchor{{
				AnchorID:      bundle.Anchors[2].ID,
				Label:         "С чего начать",
				WhatToLookFor: "Изучите объявление и его локальную ответственность.",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	attemptRaw, err := json.Marshal(incompleteStudyAttempt{
		Version:         1,
		PromptVersion:   semanticdiscovery.StudyCandidatesPromptVersion,
		BundleSHA256:    bundleSHA,
		ValidationState: "rejected",
		FailureReason:   "complete Reading Pack validation rejected the response",
		Metrics:         json.RawMessage(`{"provider_call":true}`),
		Response:        responseRaw,
	})
	if err != nil {
		t.Fatal(err)
	}
	bundleRaw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	snapshotRaw, err := json.Marshal(map[string]any{
		"file_tree":        bundle.AllowedPaths,
		"files_considered": len(bundle.AllowedPaths),
	})
	if err != nil {
		t.Fatal(err)
	}

	runDir := t.TempDir()
	writeTestFile(t, runDir, studymap.BundleFile, string(bundleRaw))
	writeTestFile(t, runDir, studymap.DirectionsAttemptFile, string(attemptRaw))
	writeTestFile(t, runDir, "snapshot.json", string(snapshotRaw))
	data := &ReportData{RepoName: "fixture", CapturedRevision: "deadbeef"}
	if warning := replaySavedIncompleteStudy(
		data,
		filepath.Join(runDir, studymap.BundleFile),
		filepath.Join(runDir, studymap.DirectionsAttemptFile),
	); warning != "" {
		t.Fatal(warning)
	}
	if data.IncompleteStudy == nil || len(data.IncompleteStudy.Directions) != 1 {
		t.Fatalf("incomplete Study = %#v", data.IncompleteStudy)
	}
	direction := data.IncompleteStudy.Directions[0]
	if direction.Question != "Как участнику начать изучение результата fixture?" ||
		len(direction.ReadingAnchors) != 1 ||
		direction.ReadingAnchors[0].Label != "Start here" ||
		direction.ReadingAnchors[0].Location.Path != bundle.Anchors[2].Path ||
		direction.ReadingAnchors[0].Location.Line != bundle.Anchors[2].Line ||
		!containsString(data.OpenablePaths, bundle.Anchors[2].Path) {
		t.Fatalf("projected incomplete Study direction = %#v, openable=%#v", direction, data.OpenablePaths)
	}
	if err := direction.ReadingAnchors[0].Source.Validate(); err != nil {
		t.Fatalf("projected exact source: %v", err)
	}
}

func TestCanonicalStudyPublicationExcludesEtcdRejectedCandidatesAndTopics(t *testing.T) {
	t.Parallel()

	selectedQuestions := []string{
		"How is etcd organized into major subsystems and entry points?",
		"How does an etcd server start and become ready to serve requests?",
		"How are Raft proposals applied to replicated state?",
		"How does etcd grant, renew, and revoke leases?",
		"How does etcd persist and recover durable state?",
		"How do etcdctl and clientv3 send requests to an etcd server?",
	}
	data := &ReportData{
		StudyMap:        &RepositoryStudyMap{},
		IncompleteStudy: &RepositoryIncompleteStudy{Version: 1},
		UserTopics: []UserTopic{
			{Title: "Storage Quota Enforcement on Writes"},
			{Title: "Server Startup and Initialization"},
			{Title: "Snapshot Transmission to Followers"},
		},
	}
	for index, question := range selectedQuestions {
		data.StudyMap.Directions = append(data.StudyMap.Directions, StudyDirection{
			ID:       fmt.Sprintf("selected-%d", index+1),
			Question: question,
		})
	}
	data.IncompleteStudy.Directions = []StudyDirection{
		{ID: "rejected-1", Question: "How are metrics and profiling exposed?"},
		{ID: "rejected-2", Question: "How do integration test utilities launch clusters?"},
	}

	applyCanonicalStudyPublication(data)

	if len(data.StudyMap.Directions) != 6 {
		t.Fatalf("canonical Study directions = %d, want 6", len(data.StudyMap.Directions))
	}
	if data.IncompleteStudy != nil {
		t.Fatalf("pre-reduction Study candidates remain published: %#v", data.IncompleteStudy)
	}
	if len(data.UserTopics) != 0 {
		t.Fatalf("locally ineligible semantic topics remain published: %#v", data.UserTopics)
	}
	for index, question := range selectedQuestions {
		if data.StudyMap.Directions[index].Question != question {
			t.Fatalf("canonical direction %d = %q, want %q", index, data.StudyMap.Directions[index].Question, question)
		}
	}
}

func TestProjectIncompleteStudySkipsCompleteAndUnresolvedDirectionsAtomically(t *testing.T) {
	t.Parallel()

	record := studyMapReportFixture(t)
	valid := studymap.IncompleteDirection{
		DirectionID:     "incomplete-valid",
		Question:        "How can I inspect the bounded fixture core?",
		WhyItMatters:    "This provides exact production declarations worth examining.",
		LearningOutcome: "The reader can locate the relevant bounded implementation.",
		TargetJob:       studymap.JobFirstContact,
		LearningStage:   studymap.StageOrientation,
		ReadingAnchors: []studymap.ReadingAnchor{{
			AnchorID:      record.Bundle.Anchors[0].ID,
			Label:         "Start here",
			WhatToLookFor: "Inspect the exact saved declaration.",
		}},
	}
	unresolved := valid
	unresolved.DirectionID = "incomplete-unresolved"
	unresolved.Question = "How can I inspect an unresolved fixture path?"
	unresolved.ReadingAnchors = append(
		append([]studymap.ReadingAnchor(nil), valid.ReadingAnchors...),
		studymap.ReadingAnchor{
			AnchorID:      "missing-anchor",
			Label:         "Related implementation",
			WhatToLookFor: "Inspect an unavailable declaration.",
		},
	)
	duplicate := valid
	duplicate.DirectionID = "already-complete"
	data := &ReportData{
		RepoName: "fixture",
		StudyMap: &RepositoryStudyMap{
			Directions: []StudyDirection{{ID: duplicate.DirectionID}},
		},
	}
	got := projectIncompleteStudy(
		data,
		record.Bundle,
		[]studymap.IncompleteDirection{unresolved, duplicate, valid},
		record.Bundle.AllowedPaths,
	)
	if len(got) != 1 || got[0].ID != valid.DirectionID {
		t.Fatalf("projected incomplete directions = %#v", got)
	}
	if len(data.OpenablePaths) != 1 || data.OpenablePaths[0] != record.Bundle.Anchors[0].Path {
		t.Fatalf("invalid or duplicate direction mutated openable paths: %#v", data.OpenablePaths)
	}
}

func TestProjectRepositoryStudyMapDebugCoverageFlagsPathOnlyDocuments(t *testing.T) {
	t.Parallel()

	record := studyMapReportFixture(t)
	record.Bundle.Documents = append(record.Bundle.Documents, studymap.Document{
		ID: "doc-path-only", Path: "docs/path-only.md", Label: "Path-only",
	})
	record.Bundle.AllowedPaths = append(record.Bundle.AllowedPaths, "docs/path-only.md")
	record.Directions[0].DocumentIDs = []string{"doc-readme", "doc-path-only"}
	var err error
	record.BundleSHA256, err = studymap.BundleHash(record.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	data := &ReportData{
		RepoName:         "fixture",
		CapturedRevision: "deadbeef",
		OpenablePaths:    append([]string(nil), record.Bundle.AllowedPaths...),
	}
	projected, err := projectRepositoryStudyMap(data, record, nil)
	if err != nil {
		t.Fatal(err)
	}
	direction, ok := findProjectedStudyDirection(projected, record.Directions[0].ID)
	if !ok {
		t.Fatalf("projected direction %q was not retained in visible or hidden Study data", record.Directions[0].ID)
	}
	coverage := direction.DebugCoverage
	if coverage == nil || coverage.Status != "partial" ||
		coverage.SourcedDocuments != 1 ||
		!containsString(coverage.Reasons, "path_only_document_present") ||
		!containsString(coverage.PathOnlyDocuments, "docs/path-only.md") {
		t.Fatalf("debug coverage = %#v", coverage)
	}
}

func TestProjectRepositoryStudyMapAttachesAuthorizedDocumentSourceFallback(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	mkdirAll(t, filepath.Join(repoRoot, "docs"))
	writeTestFile(
		t,
		repoRoot,
		"docs/guide.md",
		"# Guide\n\nThis document explains component map validation from saved repository source.\n",
	)

	record := studyMapReportFixture(t)
	record.Bundle.Documents = append(record.Bundle.Documents, studymap.Document{
		ID: "doc-guide", Path: "docs/guide.md", Label: "Guide",
	})
	record.Bundle.AllowedPaths = append(record.Bundle.AllowedPaths, "docs/guide.md")
	record.Directions[0].DocumentIDs = []string{"doc-guide"}
	var err error
	record.BundleSHA256, err = studymap.BundleHash(record.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	data := &ReportData{
		RepoName:                "fixture",
		OpenablePaths:           append([]string(nil), record.Bundle.AllowedPaths...),
		studyDocumentSourceRoot: repoRoot,
	}
	projected, err := projectRepositoryStudyMap(data, record, nil)
	if err != nil {
		t.Fatal(err)
	}
	direction, ok := findProjectedStudyDirection(projected, record.Directions[0].ID)
	if !ok {
		t.Fatalf("projected direction %q was not retained", record.Directions[0].ID)
	}
	if len(direction.Documents) != 1 || direction.Documents[0].Source == nil {
		t.Fatalf("document fallback source was not attached: %#v", direction.Documents)
	}
	source := direction.Documents[0].Source
	if source.Path != "docs/guide.md" || !strings.Contains(source.Content, "component map validation") {
		t.Fatalf("document source = %#v", source)
	}
	if direction.DebugCoverage == nil || direction.DebugCoverage.SourcedDocuments != 1 ||
		len(direction.DebugCoverage.PathOnlyDocuments) != 0 ||
		containsString(direction.DebugCoverage.Reasons, "path_only_document_present") {
		t.Fatalf("debug coverage = %#v", direction.DebugCoverage)
	}
}

func TestGenerateAuthorizedAttachesStudyDocumentSourceFallbackWithoutChangingRecord(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	record := studyMapReportFixture(t)
	record.Bundle.Documents = append(record.Bundle.Documents, studymap.Document{
		ID: "doc-guide", Path: "docs/guide.md", Label: "Guide",
	})
	record.Bundle.AllowedPaths = append(record.Bundle.AllowedPaths, "docs/guide.md")
	record.Directions[0].DocumentIDs = []string{"doc-guide"}
	var err error
	record.BundleSHA256, err = studymap.BundleHash(record.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("fixture record: %v", err)
	}

	for _, filePath := range record.Bundle.AllowedPaths {
		mkdirAll(t, filepath.Join(repository, filepath.Dir(filePath)))
		switch {
		case filePath == "README.md":
			writeTestFile(t, repository, filePath, "# Fixture\n\nCanonical README excerpt.\n")
		case filePath == "docs/guide.md":
			writeTestFile(
				t,
				repository,
				filePath,
				"# Guide\n\nThis authorized document explains component map validation from local source.\n",
			)
		default:
			writeTestFile(t, repository, filePath, "package fixture\n\nfunc Work() int { return 1 }\n")
		}
	}
	runManifestGit(t, repository, "init", "--quiet")
	runManifestGit(t, repository, "add", ".")
	runManifestGit(
		t,
		repository,
		"-c", "user.name=repomap test",
		"-c", "user.email=repomap@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "fixture",
	)
	initialState, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	currentState, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ConfirmRunAuthority(repository, initialState, currentState)
	if err != nil {
		t.Fatalf("ConfirmRunAuthority: %v", err)
	}

	runDir := t.TempDir()
	writeRunManifestMetadata(t, runDir, repository)
	snapshotRaw, err := json.Marshal(map[string]any{
		"repo_name":        "fixture",
		"file_tree":        record.Bundle.AllowedPaths,
		"files_considered": len(record.Bundle.AllowedPaths),
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "snapshot.json", string(snapshotRaw))
	recordRaw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, studymap.RecordFile, string(recordRaw))

	if err := GenerateAuthorized(runDir, authority); err != nil {
		t.Fatalf("GenerateAuthorized: %v", err)
	}
	if _, err := ReadRunManifest(runDir); err != nil {
		t.Fatalf("ReadRunManifest: %v", err)
	}

	reportRaw, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var generated ReportData
	if err := json.Unmarshal(reportRaw, &generated); err != nil {
		t.Fatal(err)
	}
	direction, ok := findProjectedStudyDirection(generated.StudyMap, record.Directions[0].ID)
	if !ok {
		t.Fatalf("projected direction %q was not rendered", record.Directions[0].ID)
	}
	if len(direction.Documents) != 1 || direction.Documents[0].Source == nil ||
		!strings.Contains(direction.Documents[0].Source.Content, "component map validation") {
		t.Fatalf("generated document source = %#v", direction.Documents)
	}

	persistedRaw, err := os.ReadFile(filepath.Join(runDir, studymap.RecordFile))
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := studymap.DecodeRecord(persistedRaw)
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range persisted.Bundle.Documents {
		if document.ID == "doc-guide" && document.Excerpt != "" {
			t.Fatalf("presentation fallback changed canonical document excerpt: %#v", document)
		}
	}
}

func TestStudyDirectionCoverageMapsQuestionTermsToLocalSources(t *testing.T) {
	t.Parallel()

	coverage := studyDirectionCoverage(StudyDirection{
		Question: "How does routing serialize requests?",
		ReadingAnchors: []StudyReadingAnchor{
			{
				Label:         "Routing entry",
				WhatToLookFor: "Follow request routing into the handler.",
				Location:      UserCodeLocation{Path: "mux.go", Line: 42},
				Source: SourceSnippet{
					Path: "mux.go", EnclosingSymbol: "routeHTTP",
					Content: "func routeHTTP() { routeRequest() }",
				},
			},
		},
		Documents: []StudyDocumentReference{
			{
				Label:    "Serialization guide",
				Location: UserCodeLocation{Path: "docs/serialization.md"},
				Source: &SourceSnippet{
					Path:    "docs/serialization.md",
					Content: "Request bodies are serialized and decoded here.",
				},
			},
		},
	})

	if coverage.QuestionCoverageStatus != "all_terms_matched" {
		t.Fatalf("question coverage status = %q, coverage = %#v", coverage.QuestionCoverageStatus, coverage)
	}
	for _, term := range coverage.QuestionTerms {
		if term.Status != "matched" || len(term.SupportTargets) == 0 {
			t.Fatalf("term %q coverage = %#v", term.Term, term)
		}
	}
	if containsString(coverage.Reasons, "question_term_uncovered") {
		t.Fatalf("unexpected uncovered term reason: %#v", coverage.Reasons)
	}
}

func TestStudyDirectionCoverageFlagsUnmatchedQuestionTerms(t *testing.T) {
	t.Parallel()

	coverage := studyDirectionCoverage(StudyDirection{
		Question: "How does routing update database rows?",
		ReadingAnchors: []StudyReadingAnchor{
			{
				Label:         "Routing entry",
				WhatToLookFor: "Follow request routing into the handler.",
				Location:      UserCodeLocation{Path: "mux.go", Line: 42},
				Source: SourceSnippet{
					Path: "mux.go", EnclosingSymbol: "routeHTTP",
					Content: "func routeHTTP() { routeRequest() }",
				},
			},
		},
	})

	if coverage.QuestionCoverageStatus != "partial_terms_matched" ||
		!containsString(coverage.Reasons, "question_term_uncovered") {
		t.Fatalf("question coverage = %#v", coverage)
	}
	unmatched := map[string]bool{}
	for _, term := range coverage.QuestionTerms {
		if term.Status == "unmatched" {
			unmatched[term.Term] = true
		}
	}
	if !unmatched["update"] || !unmatched["database"] || !unmatched["row"] {
		t.Fatalf("unmatched terms = %#v in coverage %#v", unmatched, coverage)
	}
}

func TestSplitStudyDirectionsForVisibleCoverageKeepsReviewedDirectionsVisible(t *testing.T) {
	t.Parallel()

	directions := []StudyDirection{
		studyCoverageDirection("How does routing dispatch requests?", "routing dispatch requests"),
		studyCoverageDirection("How does storage write snapshots?", "storage write snapshots"),
		studyCoverageDirection("How does caching serve responses?", "caching serve responses"),
		studyCoverageDirection("How does database replication checkpoint snapshots?", "routing dispatch requests"),
	}

	visible, hidden := splitStudyDirectionsForVisibleCoverage(directions)
	if len(visible) != 4 || len(hidden) != 0 {
		t.Fatalf("visible/hidden = %d/%d", len(visible), len(hidden))
	}
	for _, direction := range visible {
		if direction.DebugCoverage == nil || !direction.DebugCoverage.UserVisible ||
			containsString(direction.DebugCoverage.Reasons, "weak_visible_question_coverage") {
			t.Fatalf("visible direction lost visibility: %#v", direction)
		}
	}
}

func TestSplitStudyDirectionsForVisibleCoverageKeepsSmallSetsVisible(t *testing.T) {
	t.Parallel()

	directions := []StudyDirection{
		studyCoverageDirection("How does routing dispatch requests?", "routing dispatch requests"),
		studyCoverageDirection("How does storage write snapshots?", "storage write snapshots"),
		studyCoverageDirection("How does database replication checkpoint snapshots?", "routing dispatch requests"),
	}

	visible, hidden := splitStudyDirectionsForVisibleCoverage(directions)
	if len(visible) != 3 || len(hidden) != 0 {
		t.Fatalf("visible/hidden = %d/%d", len(visible), len(hidden))
	}
	for _, direction := range visible {
		if direction.DebugCoverage == nil || !direction.DebugCoverage.UserVisible ||
			containsString(direction.DebugCoverage.Reasons, "weak_visible_question_coverage") {
			t.Fatalf("visible direction = %#v", direction)
		}
	}
}

func TestProjectRepositoryStudyMapRejectsPathOutsideCurrentAllowlist(t *testing.T) {
	t.Parallel()

	record := studyMapReportFixture(t)
	data := &ReportData{RepoName: "fixture", OpenablePaths: record.Bundle.AllowedPaths[1:]}
	if _, err := projectRepositoryStudyMap(data, record, nil); err == nil {
		t.Fatal("projectRepositoryStudyMap accepted a saved path outside the current report allowlist")
	}
}

func TestProjectRepositoryStudyMapAuthorizesTrackedPathOutsideCompactContext(t *testing.T) {
	t.Parallel()

	record := studyMapReportFixture(t)
	data := &ReportData{RepoName: "fixture", OpenablePaths: record.Bundle.AllowedPaths[1:]}
	tracked := []string{record.Bundle.AllowedPaths[0]}
	if _, err := projectRepositoryStudyMap(data, record, tracked); err != nil {
		t.Fatalf("tracked Study path was rejected: %v", err)
	}
}

func TestProjectStudyDocumentSourceKeepsFullBoundedExcerpt(t *testing.T) {
	t.Parallel()

	excerptLines := make([]string, 0, maxInlineSourceLines+5)
	for index := range maxInlineSourceLines + 5 {
		excerptLines = append(excerptLines, fmt.Sprintf("saved document line %02d", index+1))
	}
	document := studymap.Document{
		ID:      "doc-guide",
		Path:    "docs/guide.mdx",
		Label:   "Guide",
		Excerpt: strings.Join(excerptLines, "\n"),
	}
	snippet, ok := projectStudyDocumentSource(&ReportData{CapturedRevision: "deadbeef"}, document)
	if !ok {
		t.Fatal("projectStudyDocumentSource rejected a bounded saved document")
	}
	if len(snippet.Lines) != maxInlineSourceLines || snippet.StartLine != 1 ||
		snippet.EndLine != maxInlineSourceLines {
		t.Fatalf("document source bounds = %d lines, %d–%d", len(snippet.Lines), snippet.StartLine, snippet.EndLine)
	}
	if snippet.Content != strings.Join(excerptLines[:maxInlineSourceLines], "\n") {
		t.Fatal("document source did not preserve the full saved bounded excerpt")
	}
	for index, line := range snippet.Lines {
		if line.Line != index+1 || line.Text != excerptLines[index] || line.Highlight || line.GapBefore {
			t.Fatalf("document source line %d = %#v", index, line)
		}
	}
	if snippet.Revision != "deadbeef" || snippet.Role != "related" ||
		len(snippet.RelatedEvidenceIDs) != 1 || snippet.RelatedEvidenceIDs[0] != document.ID {
		t.Fatalf("document source provenance = %#v", snippet)
	}
	if err := snippet.Validate(); err != nil {
		t.Fatalf("document source: %v", err)
	}
}

func studyMapReportFixture(t *testing.T) studymap.Record {
	t.Helper()

	areas := []studymap.Area{
		{ID: "area-api", Name: "Public API", Responsibility: "Accepts public calls."},
		{ID: "area-core", Name: "Core", Responsibility: "Performs central work."},
		{ID: "area-output", Name: "Output", Responsibility: "Produces repository output."},
	}
	anchors := make([]studymap.Anchor, 0, 5)
	for index := 0; index < 5; index++ {
		filePath := fmt.Sprintf("internal/part%d/work.go", index+1)
		function := studyMapTestFunction(t, filePath, fmt.Sprintf("work%d", index+1), 10+index*100)
		areaID := areas[index%len(areas)].ID
		anchors = append(anchors, studymap.Anchor{
			ID: fmt.Sprintf("fact-%d", index+1), Path: filePath, Symbol: function.Symbol,
			Line: function.StartLine + 24, Role: artifactrole.RoleProductionCore,
			Statement: fmt.Sprintf("%s contains a bounded production implementation.", function.Symbol),
			AreaIDs:   []string{areaID}, Function: function,
		})
	}
	allowed := []string{"README.md"}
	for _, anchor := range anchors {
		allowed = append(allowed, anchor.Path)
	}
	bundle := studymap.Bundle{
		Version: studymap.BundleVersion, RepoName: "fixture",
		DocumentedPurpose:  "Fixture turns input into a visible result.",
		RepositoryTypeHint: studymap.RepositoryLibrary,
		Areas:              areas, Anchors: anchors,
		Documents:    []studymap.Document{{ID: "doc-readme", Path: "README.md", Label: "README", Excerpt: "Fixture overview."}},
		AllowedPaths: allowed,
	}
	questions := []string{
		"How should callers enter the library?",
		"How does the core represent work?",
		"Where are repository results produced?",
		"How can maintainers extend this project?",
	}
	stages := []studymap.LearningStage{
		studymap.StageOrientation,
		studymap.StageCoreModel,
		studymap.StageCentralOperation,
		studymap.StageContribution,
	}
	candidates := make([]studymap.Candidate, 0, len(questions))
	for index, question := range questions {
		anchorIDs := []string{
			anchors[index%len(anchors)].ID,
			anchors[(index+1)%len(anchors)].ID,
			anchors[(index+2)%len(anchors)].ID,
		}
		reading := make([]studymap.ReadingAnchor, 0, len(anchorIDs))
		for readingIndex, anchorID := range anchorIDs {
			label := "Related implementation"
			if readingIndex == 0 {
				label = "Start here"
			} else if readingIndex == 1 {
				label = "Then inspect"
			}
			reading = append(reading, studymap.ReadingAnchor{
				AnchorID: anchorID, Label: label,
				WhatToLookFor: "Inspect the bounded declaration and its local data operations.",
			})
		}
		candidates = append(candidates, studymap.Candidate{
			Question: question, WhyItMatters: "This gives a concrete place to begin reading.",
			LearningOutcome: "The reader can identify the relevant production responsibilities.",
			TargetJob:       studymap.JobFirstContact, LearningStage: stages[index],
			AnchorIDs: anchorIDs, DocumentIDs: []string{"doc-readme"},
			AreaIDs: []string{areas[index%len(areas)].ID}, ReadingAnchors: reading,
			Confidence: "high", SearchQueries: []string{question},
		})
	}
	proposal := studymap.Proposal{
		Version: studymap.ProposalVersion, RepositoryType: studymap.RepositoryLibrary,
		Brief: studymap.Brief{
			WhatItIs:              studymap.BriefStatement{Text: "Fixture is a source-backed library.", SupportIDs: []string{"doc-readme"}},
			Problem:               studymap.BriefStatement{Text: "It gives callers a bounded implementation.", SupportIDs: []string{"doc-readme"}},
			MainInput:             studymap.BriefStatement{Text: "A public library call.", SupportIDs: []string{"fact-1"}},
			CentralResponsibility: studymap.BriefStatement{Text: "It performs the central operation.", SupportIDs: []string{"fact-2"}},
			ObservableResult:      studymap.BriefStatement{Text: "It returns a result to the caller.", SupportIDs: []string{"fact-3"}},
		},
		ShapeAreaIDs: []string{"area-api", "area-core", "area-output"},
		Candidates:   candidates,
	}
	record, err := studymap.BuildRecord(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func studyCoverageDirection(question, visibleText string) StudyDirection {
	direction := StudyDirection{
		Question: question,
		ReadingAnchors: []StudyReadingAnchor{
			{
				Label:         "Start here",
				WhatToLookFor: visibleText,
				Location:      UserCodeLocation{Path: "internal/source.go", Line: 10},
				Source: SourceSnippet{
					Path:    "internal/source.go",
					Content: visibleText,
				},
			},
			{
				Label:         "Then inspect",
				WhatToLookFor: visibleText,
				Location:      UserCodeLocation{Path: "internal/next.go", Line: 20},
				Source: SourceSnippet{
					Path:    "internal/next.go",
					Content: visibleText,
				},
			},
			{
				Label:         "Related implementation",
				WhatToLookFor: visibleText,
				Location:      UserCodeLocation{Path: "internal/related.go", Line: 30},
				Source: SourceSnippet{
					Path:    "internal/related.go",
					Content: visibleText,
				},
			},
		},
	}
	direction.DebugCoverage = studyDirectionCoverage(direction)
	return direction
}

func findProjectedStudyDirection(projected *RepositoryStudyMap, directionID string) (StudyDirection, bool) {
	if projected == nil {
		return StudyDirection{}, false
	}
	for _, direction := range projected.Directions {
		if direction.ID == directionID {
			return direction, true
		}
	}
	for _, direction := range projected.HiddenDirections {
		if direction.ID == directionID {
			return direction, true
		}
	}
	return StudyDirection{}, false
}

func studyMapTestFunction(t *testing.T, filePath, symbol string, startLine int) sourcewindowfacts.Function {
	t.Helper()

	lines := []string{"func " + symbol + "() int {", "\tvalue := 0"}
	for index := 0; index < 50; index++ {
		lines = append(lines, fmt.Sprintf("\tvalue += %d", index+1))
	}
	lines = append(lines, "\treturn value", "}")
	window, err := sourcewindowfacts.NewWindow("window-"+symbol, filePath, startLine, lines)
	if err != nil {
		t.Fatal(err)
	}
	function, err := sourcewindowfacts.ExtractGoFunction(window, symbol)
	if err != nil {
		t.Fatal(err)
	}
	return function
}
