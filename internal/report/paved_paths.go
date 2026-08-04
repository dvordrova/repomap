package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/pavedpath"
)

const maxSavedPavedPathBytes = 4 << 20

const (
	pavedPathPublicationDiagnosticsFile    = "paved_paths_publication_diagnostics.json"
	pavedPathPublicationDiagnosticsVersion = 1
)

type OperationalResultKind string

const (
	OperationalResultCommandOutput     OperationalResultKind = "command_output"
	OperationalResultGeneratedArtifact OperationalResultKind = "generated_artifact"
)

type RepositoryOperations struct {
	Version   int                   `json:"version"`
	Paths     []RepositoryPavedPath `json:"paths,omitempty"`
	Landmarks []OperationalLandmark `json:"landmarks,omitempty"`
}

type RepositoryPavedPath struct {
	ID              string                  `json:"id"`
	Title           string                  `json:"title"`
	Goal            string                  `json:"goal"`
	Prerequisites   []OperationalReference  `json:"prerequisites,omitempty"`
	Actions         []OperationalAction     `json:"actions"`
	ExpectedResults []OperationalResult     `json:"expected_results,omitempty"`
	Expected        []OperationalReference  `json:"expected,omitempty"`
	Troubleshooting []OperationalReference  `json:"troubleshooting,omitempty"`
	RelatedStudyIDs []string                `json:"related_study_direction_ids,omitempty"`
	OrderingBasis   pavedpath.OrderingBasis `json:"ordering_basis"`
}

// OperationalResult is a presentation-only completion signal derived from an
// exact action and the same bounded evidence that already supports it. A
// one-based AfterAction keeps the projection directly readable in report JSON.
type OperationalResult struct {
	Kind              OperationalResultKind `json:"kind"`
	Value             string                `json:"value"`
	AfterAction       int                   `json:"after_action"`
	ResultEvidenceIDs []string              `json:"result_evidence_ids"`
	Reference         OperationalReference  `json:"reference"`
}

type OperationalAction struct {
	Instruction string               `json:"instruction"`
	Command     string               `json:"command,omitempty"`
	CopyText    string               `json:"copy_text,omitempty"`
	Endpoint    string               `json:"endpoint,omitempty"`
	Reference   OperationalReference `json:"reference"`
}

type OperationalReference struct {
	Label    string           `json:"label"`
	Role     string           `json:"role"`
	Redacted bool             `json:"redacted,omitempty"`
	Location UserCodeLocation `json:"location"`
	Source   SourceSnippet    `json:"source"`
}

type OperationalLandmark struct {
	ID        string               `json:"id"`
	Label     string               `json:"label"`
	Role      string               `json:"role"`
	Command   string               `json:"command,omitempty"`
	CopyText  string               `json:"copy_text,omitempty"`
	Endpoint  string               `json:"endpoint,omitempty"`
	Reference OperationalReference `json:"reference"`
}

func bindOperationalRevision(operations *RepositoryOperations, revision string) {
	if operations == nil || revision == "" {
		return
	}
	bind := func(reference *OperationalReference) {
		reference.Source.Revision = revision
		reference.Source.PresentationSHA256 = sourceSnippetPresentationSHA(reference.Source)
	}
	for pathIndex := range operations.Paths {
		path := &operations.Paths[pathIndex]
		for index := range path.Prerequisites {
			bind(&path.Prerequisites[index])
		}
		for index := range path.Actions {
			bind(&path.Actions[index].Reference)
		}
		for index := range path.ExpectedResults {
			bind(&path.ExpectedResults[index].Reference)
		}
		for index := range path.Expected {
			bind(&path.Expected[index])
		}
		for index := range path.Troubleshooting {
			bind(&path.Troubleshooting[index])
		}
	}
	for index := range operations.Landmarks {
		bind(&operations.Landmarks[index].Reference)
	}
}

func replaySavedPavedPaths(data *ReportData, recordPath string) string {
	if data == nil {
		return "operating guide unavailable: report data is required"
	}
	data.Operations = nil
	info, err := os.Lstat(recordPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return "operating guide unavailable: cannot inspect saved record"
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSavedPavedPathBytes {
		return "operating guide unavailable: saved record is not a bounded regular file"
	}
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		return "operating guide unavailable: cannot read saved record"
	}
	record, err := pavedpath.DecodeRecord(raw)
	if err != nil {
		return fmt.Sprintf("operating guide unavailable: %v", err)
	}
	tracked, err := readOperationalTrackedPaths(
		filepath.Join(filepath.Dir(recordPath), "snapshot.json"),
		record.Bundle.AllowedPaths,
	)
	if err != nil {
		return fmt.Sprintf("operating guide unavailable: %v", err)
	}
	projected, err := projectRepositoryOperations(data, record, tracked)
	if err != nil {
		return fmt.Sprintf("operating guide unavailable: %v", err)
	}
	if len(projected.Paths) > 0 || len(projected.Landmarks) > 0 {
		data.Operations = projected
	}
	return ""
}

func projectRepositoryOperations(
	data *ReportData,
	record pavedpath.Record,
	trackedPaths []string,
) (*RepositoryOperations, error) {
	openable := make(map[string]struct{}, len(data.OpenablePaths)+len(trackedPaths))
	for _, filePath := range data.OpenablePaths {
		openable[filePath] = struct{}{}
	}
	for _, filePath := range trackedPaths {
		openable[filePath] = struct{}{}
	}
	evidence := make(map[string]pavedpath.Evidence, len(record.Bundle.Evidence))
	for _, item := range record.Bundle.Evidence {
		evidence[item.ID] = item
	}
	for _, filePath := range record.Bundle.AllowedPaths {
		if _, ok := openable[filePath]; !ok {
			return nil, fmt.Errorf("saved operational path %q is no longer openable", filePath)
		}
	}
	referencedEvidence := make(map[string]struct{})
	for _, saved := range record.Paths {
		for _, id := range saved.PrerequisiteEvidenceIDs {
			referencedEvidence[id] = struct{}{}
		}
		for _, action := range saved.Actions {
			referencedEvidence[action.EvidenceID] = struct{}{}
		}
		for _, id := range saved.ExpectedEvidenceIDs {
			referencedEvidence[id] = struct{}{}
		}
		for _, id := range saved.TroubleshootingEvidenceIDs {
			referencedEvidence[id] = struct{}{}
		}
	}
	for _, saved := range record.Landmarks {
		referencedEvidence[saved.EvidenceID] = struct{}{}
	}
	for id := range referencedEvidence {
		item, ok := evidence[id]
		if !ok {
			return nil, fmt.Errorf("operating guide references unavailable evidence")
		}
		filePath := item.Path
		data.OpenablePaths = appendUniqueString(data.OpenablePaths, filePath)
	}
	sort.Strings(data.OpenablePaths)
	studies := make(map[string]struct{})
	if data.StudyMap != nil {
		for _, direction := range data.StudyMap.Directions {
			studies[direction.ID] = struct{}{}
		}
	}
	result := &RepositoryOperations{Version: record.Version}
	incompleteActions := make(map[operationalActionIdentity]struct{})
	completeActions := make(map[operationalActionIdentity]struct{})
	for _, saved := range record.Paths {
		assessment := pavedpath.AssessPathPublication(saved, evidence)
		if assessment.IssueCode != "" {
			markOperationalActions(incompleteActions, saved)
			continue
		}
		projected := RepositoryPavedPath{
			ID: saved.ID, Title: saved.Title, Goal: saved.Goal,
			OrderingBasis: saved.OrderingBasis,
		}
		for _, id := range saved.RelatedStudyDirectionIDs {
			if _, ok := studies[id]; ok {
				projected.RelatedStudyIDs = append(projected.RelatedStudyIDs, id)
			}
		}
		var err error
		projected.Prerequisites, err = projectOperationalReferences(data, evidence, saved.PrerequisiteEvidenceIDs)
		if err != nil {
			return nil, err
		}
		projected.Expected, err = projectOperationalReferences(data, evidence, saved.ExpectedEvidenceIDs)
		if err != nil {
			return nil, err
		}
		projected.Troubleshooting, err = projectOperationalReferences(data, evidence, saved.TroubleshootingEvidenceIDs)
		if err != nil {
			return nil, err
		}
		for _, action := range saved.Actions {
			item, ok := evidence[action.EvidenceID]
			if !ok {
				return nil, fmt.Errorf("operating guide action references unavailable evidence")
			}
			reference, err := projectOperationalReference(data, item)
			if err != nil {
				return nil, err
			}
			projectedAction := OperationalAction{
				Instruction: action.Instruction, Command: action.Command,
				Endpoint: action.Endpoint, Reference: reference,
			}
			if action.SafeToCopy && approvedOperationalCopy(action.Command) {
				projectedAction.CopyText = action.Command
			}
			projected.Actions = append(projected.Actions, projectedAction)
		}
		projected.ExpectedResults, err = projectOperationalResults(data, assessment.Results, evidence)
		if err != nil {
			return nil, err
		}
		markOperationalActions(completeActions, saved)
		result.Paths = append(result.Paths, projected)
	}
	for _, saved := range record.Landmarks {
		item, ok := evidence[saved.EvidenceID]
		if !ok {
			return nil, fmt.Errorf("operating landmark references unavailable evidence")
		}
		reference, err := projectOperationalReference(data, item)
		if err != nil {
			return nil, err
		}
		landmark := OperationalLandmark{
			ID: saved.ID, Label: saved.Label, Role: string(saved.Role),
			Command: saved.Command, Endpoint: saved.Endpoint, Reference: reference,
		}
		viewOnly := evidenceOnlySupportsIncompletePath(
			operationalActionIdentity{
				evidenceID: saved.EvidenceID,
				command:    saved.Command,
				endpoint:   saved.Endpoint,
			},
			incompleteActions,
			completeActions,
		)
		if !viewOnly && saved.SafeToCopy && approvedOperationalCopy(saved.Command) {
			landmark.CopyText = saved.Command
		}
		result.Landmarks = append(result.Landmarks, landmark)
	}
	return result, nil
}

type operationalActionIdentity struct {
	evidenceID string
	command    string
	endpoint   string
}

func markOperationalActions(seen map[operationalActionIdentity]struct{}, saved pavedpath.Path) {
	for _, action := range saved.Actions {
		seen[operationalActionIdentity{
			evidenceID: action.EvidenceID,
			command:    action.Command,
			endpoint:   action.Endpoint,
		}] = struct{}{}
	}
}

func evidenceOnlySupportsIncompletePath(
	action operationalActionIdentity,
	incomplete map[operationalActionIdentity]struct{},
	complete map[operationalActionIdentity]struct{},
) bool {
	if _, usedByIncomplete := incomplete[action]; !usedByIncomplete {
		return false
	}
	_, usedByComplete := complete[action]
	return !usedByComplete
}

func readOperationalTrackedPaths(snapshotPath string, savedAllowedPaths ...[]string) ([]string, error) {
	info, err := os.Lstat(snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot inspect saved tracked inventory")
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 32<<20 {
		return nil, fmt.Errorf("saved tracked inventory is not bounded")
	}
	raw, err := os.ReadFile(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read saved tracked inventory")
	}
	var saved struct {
		FileTree        []string `json:"file_tree"`
		FilesConsidered int      `json:"files_considered"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil || len(saved.FileTree) > 20_000 {
		return nil, fmt.Errorf("cannot decode saved tracked inventory")
	}
	paths := make([]string, 0, len(saved.FileTree))
	seen := make(map[string]struct{}, len(saved.FileTree))
	for _, filePath := range saved.FileTree {
		if strings.HasPrefix(filePath, "... (") && strings.HasSuffix(filePath, " more)") {
			continue
		}
		if filePath == "" || filePath != path.Clean(filePath) || path.IsAbs(filePath) ||
			strings.HasPrefix(filePath, "../") || strings.Contains(filePath, "\\") {
			return nil, fmt.Errorf("saved tracked inventory contains an invalid path")
		}
		if _, duplicate := seen[filePath]; duplicate {
			continue
		}
		seen[filePath] = struct{}{}
		paths = append(paths, filePath)
	}
	if saved.FilesConsidered > len(paths) && len(savedAllowedPaths) > 0 {
		// snapshot.file_tree is a presentation projection and may be truncated.
		// In that case, authorize only exact paths already retained by the
		// validated, hash-bound semantic record. GenerateAuthorized subsequently
		// rechecks those referenced paths against repository freshness.
		for _, filePath := range savedAllowedPaths[0] {
			if filePath == "" || filePath != path.Clean(filePath) || path.IsAbs(filePath) ||
				strings.HasPrefix(filePath, "../") || strings.Contains(filePath, "\\") {
				return nil, fmt.Errorf("saved semantic record contains an invalid path")
			}
			if _, duplicate := seen[filePath]; duplicate {
				continue
			}
			seen[filePath] = struct{}{}
			paths = append(paths, filePath)
		}
	}
	return paths, nil
}

func projectOperationalReferences(
	data *ReportData,
	evidence map[string]pavedpath.Evidence,
	ids []string,
) ([]OperationalReference, error) {
	result := make([]OperationalReference, 0, len(ids))
	for _, id := range ids {
		item, ok := evidence[id]
		if !ok {
			return nil, fmt.Errorf("operating guide references unavailable evidence")
		}
		reference, err := projectOperationalReference(data, item)
		if err != nil {
			return nil, err
		}
		result = append(result, reference)
	}
	return result, nil
}

func projectOperationalReference(data *ReportData, item pavedpath.Evidence) (OperationalReference, error) {
	lines := make([]SourceSnippetLine, 0, len(item.Excerpt))
	for index, text := range item.Excerpt {
		lines = append(lines, SourceSnippetLine{Line: item.StartLine + index, Text: text})
	}
	snippet := SourceSnippet{
		Path: item.Path, Language: sourceLanguage(item.Path), StartLine: item.StartLine,
		EndLine: item.StartLine + len(lines) - 1, Content: strings.Join(item.Excerpt, "\n"),
		Lines: lines, ContentSHA256: sourceLinesSHA256(item.Excerpt),
		RelatedEvidenceIDs: []string{item.ID}, Role: "operational",
		Revision: data.CapturedRevision, SourceComplete: false,
	}
	snippet.PresentationSHA256 = sourceSnippetPresentationSHA(snippet)
	if err := snippet.Validate(); err != nil {
		return OperationalReference{}, fmt.Errorf("operating source %q: %w", item.Path, err)
	}
	return OperationalReference{
		Label: item.Label, Role: string(item.Role), Redacted: item.Redacted,
		Location: UserCodeLocation{Path: item.Path, Line: item.StartLine}, Source: snippet,
	}, nil
}

func deriveOperationalResults(
	data *ReportData,
	saved pavedpath.Path,
	evidence map[string]pavedpath.Evidence,
) ([]OperationalResult, error) {
	assessment := pavedpath.AssessPathPublication(saved, evidence)
	return projectOperationalResults(data, assessment.Results, evidence)
}

func projectOperationalResults(
	data *ReportData,
	classified []pavedpath.PublicationResult,
	evidence map[string]pavedpath.Evidence,
) ([]OperationalResult, error) {
	results := make([]OperationalResult, 0, len(classified))
	for _, candidate := range classified {
		item, ok := evidence[candidate.EvidenceID]
		if !ok {
			return nil, fmt.Errorf("operating result references unavailable evidence")
		}
		reference, err := projectOperationalResultReference(data, item, candidate)
		if err != nil {
			return nil, err
		}
		kind := OperationalResultKind(candidate.Kind)
		switch kind {
		case OperationalResultCommandOutput, OperationalResultGeneratedArtifact:
		default:
			return nil, fmt.Errorf("operating result has invalid kind")
		}
		results = append(results, OperationalResult{
			Kind: kind, Value: candidate.Value, AfterAction: candidate.AfterAction,
			ResultEvidenceIDs: []string{item.ID}, Reference: reference,
		})
	}
	return results, nil
}

func projectOperationalResultReference(
	data *ReportData,
	item pavedpath.Evidence,
	candidate pavedpath.PublicationResult,
) (OperationalReference, error) {
	if candidate.StartOffset < 0 || candidate.EndOffset < candidate.StartOffset ||
		candidate.EndOffset >= len(item.Excerpt) {
		return OperationalReference{}, fmt.Errorf("operating result has invalid source coordinates")
	}
	projected := item
	projected.StartLine = item.StartLine + candidate.StartOffset
	projected.EndLine = item.StartLine + candidate.EndOffset
	projected.Excerpt = append([]string{}, item.Excerpt[candidate.StartOffset:candidate.EndOffset+1]...)
	switch candidate.Kind {
	case pavedpath.PublicationResultCommandOutput:
		projected.Label = "Documented command output"
	case pavedpath.PublicationResultGeneratedArtifact:
		projected.Label = "Generated artifact path"
	default:
		return OperationalReference{}, fmt.Errorf("operating result has invalid kind")
	}
	return projectOperationalReference(data, projected)
}

// approvedOperationalCopy is deliberately stricter than evidence validity.
// The report may show any exact command, but exposes a clipboard payload only
// for a single low-risk literal without shell composition or credential-like
// content. The browser never executes it.
func approvedOperationalCopy(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || len(command) > 512 || strings.ContainsAny(command, "\r\n\x00;|&<>`") ||
		strings.Contains(command, "$(") {
		return false
	}
	for _, char := range command {
		if unicode.IsControl(char) && char != '\t' {
			return false
		}
	}
	lower := strings.ToLower(command)
	for _, marker := range []string{
		"sudo ", " rm ", "rm -", " reset", " restore", " init", " migrate", " seed",
		" prune", " delete", " destroy", " drop ", "password=", "token=", "secret=",
		"api_key=", "authorization:", "curl http://", "curl https://",
	} {
		if strings.Contains(" "+lower, marker) {
			return false
		}
	}
	return true
}

func pavedPathRecordPath(runDir string) string {
	return filepath.Join(runDir, pavedpath.RecordFile)
}

type pavedPathPublicationIssue struct {
	PathIndex int    `json:"path_index"`
	Code      string `json:"code"`
}

type pavedPathPublicationDiagnostics struct {
	Version         int                         `json:"version"`
	RecordRawSHA256 string                      `json:"record_raw_sha256"`
	BundleSHA256    string                      `json:"bundle_sha256"`
	RecordIssues    []pavedPathPublicationIssue `json:"record_issues"`
	ReplayIssues    []pavedPathPublicationIssue `json:"replay_issues"`
}

func writePavedPathPublicationDiagnostics(runDir string) error {
	diagnostics := loadPavedPathPublicationDiagnostics(runDir)
	target := filepath.Join(runDir, pavedPathPublicationDiagnosticsFile)
	if diagnostics == nil {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale paved path publication diagnostics: %w", err)
		}
		return nil
	}
	raw, err := json.MarshalIndent(diagnostics, "", "  ")
	if err != nil {
		return fmt.Errorf("encode paved path publication diagnostics: %w", err)
	}
	raw = append(raw, '\n')
	if len(raw) > 16<<10 {
		return fmt.Errorf("paved path publication diagnostics exceed internal bound")
	}

	temporary, err := os.CreateTemp(runDir, ".paved-path-publication-diagnostics-*.tmp")
	if err != nil {
		return fmt.Errorf("create paved path publication diagnostics temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure paved path publication diagnostics temporary file: %w", err)
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write paved path publication diagnostics temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync paved path publication diagnostics temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close paved path publication diagnostics temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("replace paved path publication diagnostics: %w", err)
	}
	committed = true
	return nil
}

func loadPavedPathPublicationDiagnostics(runDir string) *pavedPathPublicationDiagnostics {
	recordPath := pavedPathRecordPath(runDir)
	info, err := os.Lstat(recordPath)
	if err != nil || !info.Mode().IsRegular() ||
		info.Size() <= 0 || info.Size() > maxSavedPavedPathBytes {
		return nil
	}
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		return nil
	}
	record, err := pavedpath.DecodeRecord(raw)
	if err != nil {
		return nil
	}

	recordIssues := make([]pavedPathPublicationIssue, 0, pavedpath.MaxPaths)
	for _, issue := range record.Issues {
		if issue.PathIndex < 0 || issue.PathIndex >= pavedpath.MaxPaths ||
			!pavedPathPublicationIssueCode(issue.Code) {
			continue
		}
		recordIssues = append(recordIssues, pavedPathPublicationIssue{
			PathIndex: issue.PathIndex,
			Code:      issue.Code,
		})
		if len(recordIssues) == pavedpath.MaxPaths {
			break
		}
	}
	evidence := make(map[string]pavedpath.Evidence, len(record.Bundle.Evidence))
	for _, item := range record.Bundle.Evidence {
		evidence[item.ID] = item
	}
	replayIssues := make([]pavedPathPublicationIssue, 0, len(record.Paths))
	for index, saved := range record.Paths {
		assessment := pavedpath.AssessPathPublication(saved, evidence)
		if assessment.IssueCode == "" {
			continue
		}
		replayIssues = append(replayIssues, pavedPathPublicationIssue{
			PathIndex: index,
			Code:      assessment.IssueCode,
		})
	}
	if len(recordIssues) == 0 && len(replayIssues) == 0 {
		return nil
	}
	digest := sha256.Sum256(raw)
	return &pavedPathPublicationDiagnostics{
		Version:         pavedPathPublicationDiagnosticsVersion,
		RecordRawSHA256: hex.EncodeToString(digest[:]),
		BundleSHA256:    record.BundleSHA256,
		RecordIssues:    recordIssues,
		ReplayIssues:    replayIssues,
	}
}

func pavedPathPublicationIssueCode(code string) bool {
	switch code {
	case pavedpath.PublicationIssueMissingPrerequisite,
		pavedpath.PublicationIssueMissingActions,
		pavedpath.PublicationIssueMissingResult:
		return true
	default:
		return false
	}
}
