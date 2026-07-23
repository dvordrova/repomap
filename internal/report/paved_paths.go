package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/pavedpath"
)

const maxSavedPavedPathBytes = 4 << 20

const maxOperationalResultBytes = 4 << 10

var operationalOutputFlagRE = regexp.MustCompile(
	`(?:^|[ \t])(?:-o|--output|-coverprofile)(?:=|[ \t]+)(?:"([^"\r\n]+)"|'([^'\r\n]+)'|([^ \t\r\n]+))`,
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
	for _, saved := range record.Paths {
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
		projected.ExpectedResults, err = deriveOperationalResults(data, saved, evidence)
		if err != nil {
			return nil, err
		}
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
		if saved.SafeToCopy && approvedOperationalCopy(saved.Command) {
			landmark.CopyText = saved.Command
		}
		result.Landmarks = append(result.Landmarks, landmark)
	}
	return result, nil
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

type operationalResultCandidate struct {
	kind        OperationalResultKind
	value       string
	startOffset int
	endOffset   int
}

func deriveOperationalResults(
	data *ReportData,
	saved pavedpath.Path,
	evidence map[string]pavedpath.Evidence,
) ([]OperationalResult, error) {
	results := []OperationalResult{}
	for actionIndex, action := range saved.Actions {
		if strings.TrimSpace(action.Command) == "" {
			continue
		}
		item, ok := evidence[action.EvidenceID]
		if !ok {
			return nil, fmt.Errorf("operating result references unavailable evidence")
		}
		if item.Redacted {
			continue
		}

		candidates := operationalResultCandidates(action, item)
		seen := make(map[string]struct{}, len(candidates))
		for _, candidate := range candidates {
			key := string(candidate.kind) + "\x00" + candidate.value
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			reference, err := projectOperationalResultReference(data, item, candidate)
			if err != nil {
				return nil, err
			}
			results = append(results, OperationalResult{
				Kind: candidate.kind, Value: candidate.value, AfterAction: actionIndex + 1,
				ResultEvidenceIDs: []string{item.ID}, Reference: reference,
			})
		}
	}
	return results, nil
}

func operationalResultCandidates(
	action pavedpath.Action,
	item pavedpath.Evidence,
) []operationalResultCandidate {
	result := []operationalResultCandidate{}
	if value, startOffset, endOffset, ok := documentedCommandOutput(action.Command, item); ok {
		result = append(result, operationalResultCandidate{
			kind: OperationalResultCommandOutput, value: value,
			startOffset: startOffset, endOffset: endOffset,
		})
	}
	for _, value := range operationalOutputFlagValues(action.Command, operationalOutputFlagRE) {
		offset, ok := operationalOutputFlagLine(item.Excerpt, value, operationalOutputFlagRE)
		if !ok {
			continue
		}
		result = append(result, operationalResultCandidate{
			kind: OperationalResultGeneratedArtifact, value: value,
			startOffset: offset, endOffset: offset,
		})
	}

	isMakeTarget := item.Role == pavedpath.RoleBuildTarget &&
		strings.HasPrefix(strings.TrimSpace(action.Command), "make ")
	if !isMakeTarget {
		return result
	}
	for offset, line := range item.Excerpt {
		if !strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for _, value := range operationalMakeCoverProfileValues(line) {
			result = append(result, operationalResultCandidate{
				kind: OperationalResultGeneratedArtifact, value: value,
				startOffset: offset, endOffset: offset,
			})
		}
	}
	return result
}

func documentedCommandOutput(
	command string,
	item pavedpath.Evidence,
) (string, int, int, bool) {
	if item.Role != pavedpath.RoleDocumentedProcedure {
		return "", 0, 0, false
	}
	prompt := "$ " + strings.TrimSpace(command)
	for index, line := range item.Excerpt {
		if strings.TrimSpace(line) != prompt {
			continue
		}
		start := index + 1
		end := len(item.Excerpt) - 1
		for offset := start; offset < len(item.Excerpt); offset++ {
			trimmed := strings.TrimSpace(item.Excerpt[offset])
			isNextPrompt := strings.HasPrefix(trimmed, "$ ") ||
				strings.HasPrefix(trimmed, "% ") || strings.HasPrefix(trimmed, "> ")
			if isNextPrompt {
				end = offset - 1
				break
			}
		}
		for start <= end && strings.TrimSpace(item.Excerpt[start]) == "" {
			start++
		}
		for end >= start && strings.TrimSpace(item.Excerpt[end]) == "" {
			end--
		}
		if start > end {
			continue
		}
		value := dedentOperationalOutput(item.Excerpt[start : end+1])
		if validOperationalResultValue(value) {
			return value, start, end, true
		}
	}
	return "", 0, 0, false
}

func dedentOperationalOutput(lines []string) string {
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		current := len(line) - len(strings.TrimLeft(line, " \t"))
		if indent < 0 || current < indent {
			indent = current
		}
	}
	if indent <= 0 {
		return strings.Join(lines, "\n")
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) >= indent {
			line = line[indent:]
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func operationalOutputFlagValues(command string, expression *regexp.Regexp) []string {
	values := []string{}
	for _, match := range expression.FindAllStringSubmatch(command, -1) {
		for index := 1; index < len(match); index++ {
			if validOperationalArtifactPath(match[index]) {
				values = append(values, match[index])
				break
			}
		}
	}
	return values
}

func operationalMakeCoverProfileValues(line string) []string {
	const marker = "-coverprofile="
	values := []string{}
	remaining := line
	for {
		index := strings.Index(remaining, marker)
		if index < 0 {
			return values
		}
		remaining = remaining[index+len(marker):]
		value := strings.Fields(remaining)
		if len(value) == 0 {
			return values
		}
		candidate := strings.Trim(value[0], `"'`)
		if validOperationalArtifactPath(candidate) {
			values = append(values, candidate)
		}
		remaining = remaining[len(value[0]):]
	}
}

func operationalOutputFlagLine(lines []string, value string, expression *regexp.Regexp) (int, bool) {
	for index, line := range lines {
		for _, candidate := range operationalOutputFlagValues(line, expression) {
			if candidate == value {
				return index, true
			}
		}
	}
	return 0, false
}

func validOperationalArtifactPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 512 || strings.HasPrefix(value, "-") ||
		strings.ContainsAny(value, "\r\n\x00`$|;&<>{}()") {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func validOperationalResultValue(value string) bool {
	if strings.TrimSpace(value) == "" || len(value) > maxOperationalResultBytes {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) && char != '\t' && char != '\n' {
			return false
		}
	}
	return true
}

func projectOperationalResultReference(
	data *ReportData,
	item pavedpath.Evidence,
	candidate operationalResultCandidate,
) (OperationalReference, error) {
	if candidate.startOffset < 0 || candidate.endOffset < candidate.startOffset ||
		candidate.endOffset >= len(item.Excerpt) {
		return OperationalReference{}, fmt.Errorf("operating result has invalid source coordinates")
	}
	projected := item
	projected.StartLine = item.StartLine + candidate.startOffset
	projected.EndLine = item.StartLine + candidate.endOffset
	projected.Excerpt = append([]string{}, item.Excerpt[candidate.startOffset:candidate.endOffset+1]...)
	switch candidate.kind {
	case OperationalResultCommandOutput:
		projected.Label = "Documented command output"
	case OperationalResultGeneratedArtifact:
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
