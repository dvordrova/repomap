// Package sourceexplain builds and validates bounded source-assessment
// contracts. Models assess locally seeded questions; local code owns facts,
// claim text, evidence levels, and executable actions.
package sourceexplain

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/symbol"
)

const (
	BundleVersion = 1

	maxProviderSourceLines     = 160
	maxProviderSourceBytes     = 32 * 1024
	maxProviderSourceLineBytes = 16 * 1024
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Predicate string

const (
	PredicateValidatesInput     Predicate = "validates_input"
	PredicateDelegatesOperation Predicate = "delegates_operation"
	PredicateMapsError          Predicate = "maps_error"
	PredicateFillsResponse      Predicate = "fills_response"
	PredicatePersistsState      Predicate = "persists_state"
	PredicatePerformsIO         Predicate = "performs_io"
)

type Operation string

const (
	OperationFindTests  Operation = "find_tests"
	OperationReadCallee Operation = "read_callee"
)

type Bundle struct {
	Version        int               `json:"version"`
	RepoName       string            `json:"repo_name"`
	Target         sourcecard.Target `json:"target"`
	Source         Source            `json:"source"`
	Questions      []Question        `json:"questions"`
	AllowedActions []AllowedAction   `json:"allowed_actions"`
	Warnings       []string          `json:"warnings"`
}

type Source struct {
	FileSHA256 string            `json:"file_sha256"`
	Window     sourcecard.Window `json:"window"`
	// Complete is retained as an explicit reminder that a lexical window is
	// never a complete function body. It must always be false.
	Complete   bool                  `json:"complete"`
	StopReason sourcecard.StopReason `json:"stop_reason"`
	Lines      []sourcecard.Line     `json:"lines"`
}

type Question struct {
	ID                         string    `json:"id"`
	Predicate                  Predicate `json:"predicate"`
	AnchorEvidenceID           string    `json:"anchor_evidence_id"`
	AnchorSourceEvidenceID     string    `json:"anchor_source_evidence_id"`
	CalleeName                 string    `json:"callee_name"`
	CandidateSourceEvidenceIDs []string  `json:"candidate_source_evidence_ids"`
}

type AllowedAction struct {
	ID               string    `json:"id"`
	Operation        Operation `json:"operation"`
	AnchorEvidenceID string    `json:"anchor_evidence_id"`
}

func Build(structural symbol.Bundle, card sourcecard.Card) (Bundle, error) {
	if err := card.Validate(); err != nil {
		return Bundle{}, fmt.Errorf("source explain: invalid source card: %w", err)
	}
	if err := validateTargetAgreement(structural, card); err != nil {
		return Bundle{}, err
	}

	lineByNumber := make(map[int]sourcecard.Line, len(card.Lines))
	for _, line := range card.Lines {
		lineByNumber[line.Line] = line
	}

	questions := make([]Question, 0, len(structural.OutgoingCalls))
	for _, call := range structural.OutgoingCalls {
		if call.Callsite == nil || call.Callsite.Path != card.Target.Path {
			continue
		}
		anchorLine, ok := lineByNumber[call.Callsite.Line]
		if !ok || anchorLine.Truncated {
			continue
		}
		predicate, ok := classifyCall(structural.Target.Entity.Name, call.Callee.Name)
		if !ok {
			continue
		}
		candidateIDs := candidateSourceIDs(predicate, call.Callsite.Line, call.Callee.Name, lineByNumber)
		if len(candidateIDs) == 0 {
			continue
		}
		questions = append(questions, Question{
			ID:                         "question-" + call.EvidenceID,
			Predicate:                  predicate,
			AnchorEvidenceID:           call.EvidenceID,
			AnchorSourceEvidenceID:     anchorLine.EvidenceID,
			CalleeName:                 call.Callee.Name,
			CandidateSourceEvidenceIDs: candidateIDs,
		})
	}
	sort.Slice(questions, func(i, j int) bool {
		left := lineByEvidenceID(card.Lines, questions[i].AnchorSourceEvidenceID)
		right := lineByEvidenceID(card.Lines, questions[j].AnchorSourceEvidenceID)
		if left != right {
			return left < right
		}
		return questions[i].ID < questions[j].ID
	})
	if len(questions) == 0 {
		return Bundle{}, fmt.Errorf("source explain: no source questions could be seeded from bounded call evidence")
	}

	actions := []AllowedAction{{
		ID:               "action-find-tests",
		Operation:        OperationFindTests,
		AnchorEvidenceID: structural.Target.EvidenceID,
	}}
	for _, question := range questions {
		actions = append(actions, AllowedAction{
			ID:               "action-read-" + question.AnchorEvidenceID,
			Operation:        OperationReadCallee,
			AnchorEvidenceID: question.AnchorEvidenceID,
		})
	}

	bundle := Bundle{
		Version:  BundleVersion,
		RepoName: card.RepoName,
		Target:   card.Target,
		Source: Source{
			FileSHA256: card.FileSHA256,
			Window:     card.Window,
			Complete:   false,
			StopReason: card.Window.StopReason,
			Lines:      append([]sourcecard.Line{}, card.Lines...),
		},
		Questions:      questions,
		AllowedActions: actions,
		Warnings: []string{
			"source lines show written control flow, not runtime reachability",
			"callee names and calls do not establish callee behavior",
		},
	}
	if err := bundle.Validate(); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func (b Bundle) Validate() error {
	if b.Version != BundleVersion {
		return fmt.Errorf("source explain: unsupported bundle version %d", b.Version)
	}
	if b.Target.EvidenceID == "" || b.Target.EntityID == "" || b.Target.Name == "" || b.Target.Path == "" || b.Target.Line <= 0 {
		return fmt.Errorf("source explain: target identity is incomplete")
	}
	if b.Target.Kind != evidence.EntityFunction && b.Target.Kind != evidence.EntityMethod {
		return fmt.Errorf("source explain: target kind %q is not a function or method", b.Target.Kind)
	}
	path := filepath.FromSlash(b.Target.Path)
	if filepath.IsAbs(path) || !filepath.IsLocal(path) || !strings.EqualFold(filepath.Ext(path), ".go") {
		return fmt.Errorf("source explain: target path is not a repository-relative go file")
	}
	if filepath.ToSlash(filepath.Clean(path)) != b.Target.Path {
		return fmt.Errorf("source explain: target path is not canonical")
	}
	if len(b.Source.Lines) == 0 || len(b.Questions) == 0 {
		return fmt.Errorf("source explain: source lines and questions are required")
	}
	lineIDs, err := validateSource(b.Target, b.Source)
	if err != nil {
		return err
	}
	questionIDs := make(map[string]struct{}, len(b.Questions))
	anchors := map[string]struct{}{b.Target.EvidenceID: {}}
	for index, question := range b.Questions {
		if question.ID == "" || !question.Predicate.valid() || question.AnchorEvidenceID == "" || question.CalleeName == "" {
			return fmt.Errorf("source explain: questions[%d] is incomplete", index)
		}
		if _, exists := questionIDs[question.ID]; exists {
			return fmt.Errorf("source explain: duplicate question id %q", question.ID)
		}
		questionIDs[question.ID] = struct{}{}
		anchors[question.AnchorEvidenceID] = struct{}{}
		if len(question.CandidateSourceEvidenceIDs) == 0 {
			return fmt.Errorf("source explain: question %q has no candidate source evidence", question.ID)
		}
		hasAnchor := false
		seenCandidates := make(map[string]struct{}, len(question.CandidateSourceEvidenceIDs))
		for _, id := range question.CandidateSourceEvidenceIDs {
			line, ok := lineIDs[id]
			if !ok {
				return fmt.Errorf("source explain: question %q references unknown source evidence %q", question.ID, id)
			}
			if line.Truncated {
				return fmt.Errorf("source explain: question %q references truncated source evidence %q", question.ID, id)
			}
			if _, exists := seenCandidates[id]; exists {
				return fmt.Errorf("source explain: question %q repeats source evidence %q", question.ID, id)
			}
			seenCandidates[id] = struct{}{}
			if id == question.AnchorSourceEvidenceID {
				hasAnchor = true
			}
		}
		if !hasAnchor {
			return fmt.Errorf("source explain: question %q candidates omit anchor source evidence", question.ID)
		}
		if !mentionsCall(lineIDs[question.AnchorSourceEvidenceID].Text, question.CalleeName) {
			return fmt.Errorf("source explain: question %q anchor does not call callee %q", question.ID, question.CalleeName)
		}
	}
	if len(b.AllowedActions) == 0 || b.AllowedActions[0].Operation != OperationFindTests {
		return fmt.Errorf("source explain: find_tests must be the default allowed action")
	}
	actionIDs := make(map[string]struct{}, len(b.AllowedActions))
	for index, action := range b.AllowedActions {
		if action.ID == "" || !action.Operation.valid() {
			return fmt.Errorf("source explain: allowed_actions[%d] is incomplete", index)
		}
		if _, exists := actionIDs[action.ID]; exists {
			return fmt.Errorf("source explain: duplicate action id %q", action.ID)
		}
		actionIDs[action.ID] = struct{}{}
		if _, ok := anchors[action.AnchorEvidenceID]; !ok {
			return fmt.Errorf("source explain: action %q has unknown anchor %q", action.ID, action.AnchorEvidenceID)
		}
	}
	return nil
}

func validateSource(target sourcecard.Target, source Source) (map[string]sourcecard.Line, error) {
	if !sha256Pattern.MatchString(source.FileSHA256) {
		return nil, fmt.Errorf("source explain: source file hash must be a lowercase sha-256 digest")
	}
	if source.Complete {
		return nil, fmt.Errorf("source explain: lexical source windows cannot be marked complete")
	}
	if source.StopReason != source.Window.StopReason {
		return nil, fmt.Errorf("source explain: source stop reason does not match window metadata")
	}
	switch source.Window.StopReason {
	case sourcecard.StopNextTopLevelFunc,
		sourcecard.StopEndOfFile,
		sourcecard.StopLineLimit,
		sourcecard.StopByteLimit:
	default:
		return nil, fmt.Errorf("source explain: invalid source stop reason %q", source.Window.StopReason)
	}
	stoppedByLimit := source.Window.StopReason == sourcecard.StopLineLimit || source.Window.StopReason == sourcecard.StopByteLimit
	if stoppedByLimit && !source.Window.Truncated {
		return nil, fmt.Errorf("source explain: source window stopped by a limit without truncation metadata")
	}
	if source.Window.StartLine != target.Line || source.Window.EndLine < source.Window.StartLine {
		return nil, fmt.Errorf("source explain: source window does not start at the target line")
	}
	if len(source.Lines) > maxProviderSourceLines {
		return nil, fmt.Errorf("source explain: source window has %d lines, limit is %d", len(source.Lines), maxProviderSourceLines)
	}
	if source.Window.IncludedBytes <= 0 || source.Window.IncludedBytes > maxProviderSourceBytes {
		return nil, fmt.Errorf("source explain: source window byte count %d exceeds provider-safe bounds", source.Window.IncludedBytes)
	}

	lineIDs := make(map[string]sourcecard.Line, len(source.Lines))
	includedBytes := 0
	hasTruncatedLine := false
	for index, line := range source.Lines {
		expectedLine := source.Window.StartLine + index
		if line.Line != expectedLine {
			return nil, fmt.Errorf("source explain: source lines are not contiguous at index %d", index)
		}
		expectedID := fmt.Sprintf("source-%d", line.Line)
		if line.EvidenceID != expectedID {
			return nil, fmt.Errorf("source explain: line %d has invalid evidence id %q", line.Line, line.EvidenceID)
		}
		if len(line.Text) > maxProviderSourceLineBytes {
			return nil, fmt.Errorf("source explain: source line %d is %d bytes, limit is %d", line.Line, len(line.Text), maxProviderSourceLineBytes)
		}
		if _, exists := lineIDs[line.EvidenceID]; exists {
			return nil, fmt.Errorf("source explain: duplicate source evidence id %q", line.EvidenceID)
		}
		lineIDs[line.EvidenceID] = line
		if index > 0 {
			includedBytes++
		}
		includedBytes += len(line.Text)
		hasTruncatedLine = hasTruncatedLine || line.Truncated
	}
	if source.Lines[0].Line != target.Line || source.Lines[len(source.Lines)-1].Line != source.Window.EndLine {
		return nil, fmt.Errorf("source explain: source window does not match included lines")
	}
	if includedBytes != source.Window.IncludedBytes {
		return nil, fmt.Errorf("source explain: source window byte metadata is %d, calculated %d", source.Window.IncludedBytes, includedBytes)
	}
	if includedBytes > maxProviderSourceBytes {
		return nil, fmt.Errorf("source explain: source window is %d bytes, limit is %d", includedBytes, maxProviderSourceBytes)
	}
	if hasTruncatedLine && !source.Window.Truncated {
		return nil, fmt.Errorf("source explain: truncated source line is missing window truncation metadata")
	}
	if !isTargetDeclaration(source.Lines[0].Text, target.Name) {
		return nil, fmt.Errorf("source explain: first source line does not declare target %q", target.Name)
	}
	return lineIDs, nil
}

func isTargetDeclaration(line, targetName string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "func ") && !strings.HasPrefix(trimmed, "func\t") {
		return false
	}
	name := targetName
	if index := strings.LastIndex(name, "."); index >= 0 {
		name = name[index+1:]
	}
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*(?:\[|\()`)
	return pattern.MatchString(trimmed)
}

func validateTargetAgreement(structural symbol.Bundle, card sourcecard.Card) error {
	target := structural.Target.Entity
	if target.Location == nil {
		return fmt.Errorf("source explain: structural target has no location")
	}
	if structural.Target.EvidenceID != card.Target.EvidenceID ||
		target.ID != card.Target.EntityID ||
		target.Name != card.Target.Name ||
		target.Kind != card.Target.Kind ||
		target.Location.Path != card.Target.Path ||
		target.Location.Line != card.Target.Line {
		return fmt.Errorf("source explain: structural target and source card do not agree")
	}
	return nil
}

func classifyCall(targetName, calleeName string) (Predicate, bool) {
	callee := strings.ToLower(calleeName)
	switch {
	case strings.Contains(callee, "check") || strings.Contains(callee, "validate"):
		return PredicateValidatesInput, true
	case strings.Contains(callee, "error"):
		return PredicateMapsError, true
	case strings.Contains(callee, "fill") || strings.Contains(callee, "enrich"):
		return PredicateFillsResponse, true
	case baseName(targetName) == baseName(calleeName):
		return PredicateDelegatesOperation, true
	case containsAny(callee, "save", "store", "persist", "commit"):
		return PredicatePersistsState, true
	case containsAny(callee, "sync", "flush", "fsync", "read", "write"):
		return PredicatePerformsIO, true
	default:
		return "", false
	}
}

func candidateSourceIDs(
	predicate Predicate,
	anchorLine int,
	calleeName string,
	lines map[int]sourcecard.Line,
) []string {
	if predicate == PredicateValidatesInput {
		orderedLines := make([]sourcecard.Line, 0, len(lines))
		for _, line := range lines {
			orderedLines = append(orderedLines, line)
		}
		sort.Slice(orderedLines, func(i, j int) bool {
			return orderedLines[i].Line < orderedLines[j].Line
		})
		anchor, ok := lines[anchorLine]
		if ok {
			question := Question{
				Predicate:              predicate,
				AnchorSourceEvidenceID: anchor.EvidenceID,
				CalleeName:             calleeName,
			}
			if proofIDs, _, proven := validationProofSourceIDs(question, orderedLines); proven {
				return proofIDs
			}
		}
	}

	lineNumbers := []int{anchorLine}
	switch predicate {
	case PredicateMapsError:
		lineNumbers = append([]int{anchorLine - 1}, lineNumbers...)
	}
	result := make([]string, 0, len(lineNumbers))
	for _, number := range lineNumbers {
		line, ok := lines[number]
		if !ok || line.Truncated {
			continue
		}
		result = append(result, line.EvidenceID)
	}
	return result
}

func lineByEvidenceID(lines []sourcecard.Line, id string) int {
	for _, line := range lines {
		if line.EvidenceID == id {
			return line.Line
		}
	}
	return int(^uint(0) >> 1)
}

func baseName(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return strings.ToLower(name[index+1:])
	}
	return strings.ToLower(name)
}

func containsAny(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

func mentionsIdentifier(line, identifier string) bool {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(identifier) + `\b`)
	return pattern.MatchString(line)
}

func (p Predicate) valid() bool {
	switch p {
	case PredicateValidatesInput,
		PredicateDelegatesOperation,
		PredicateMapsError,
		PredicateFillsResponse,
		PredicatePersistsState,
		PredicatePerformsIO:
		return true
	default:
		return false
	}
}

func (o Operation) valid() bool {
	return o == OperationFindTests || o == OperationReadCallee
}
