package report

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/workspacecontent"
)

const (
	maxInlineSourceLines         = 60
	preferredInlineSourceLines   = 40
	inlineSourceContextBefore    = 7
	inlineSourceContextAfter     = 3
	maxSourceEvidenceLineGap     = 10
	maxFullFunctionSourceLines   = 240
	maxFullFunctionSourceBytes   = 128 << 10
	maxSourceNotices             = 2
	maxSourceNoticeTextBytes     = 180
	maxSourceLandmarkReasonBytes = 240
	omittedSourceLinesMarker     = "… lines omitted …"
	legacyOmittedLinesMarker     = "…"
	boundedPrimaryPathSyntaxKind = "bounded_primary_path_syntax"
)

// SourceSnippet is an immutable presentation projection of already saved
// source bytes. It is not evidence and never participates in Mechanism
// identity, validation, or content hashing.
type SourceSnippet struct {
	Path                  string              `json:"path"`
	Language              string              `json:"language"`
	EnclosingSymbol       string              `json:"enclosing_symbol,omitempty"`
	StartLine             int                 `json:"start_line"`
	EndLine               int                 `json:"end_line"`
	HighlightRanges       []SourceHighlight   `json:"highlight_ranges"`
	Content               string              `json:"content"`
	Lines                 []SourceSnippetLine `json:"lines"`
	FullFunctionStartLine int                 `json:"full_function_start_line,omitempty"`
	FullFunctionEndLine   int                 `json:"full_function_end_line,omitempty"`
	FullFunctionLines     []SourceSnippetLine `json:"full_function_lines,omitempty"`
	ContentSHA256         string              `json:"content_sha256"`
	PresentationSHA256    string              `json:"presentation_sha256"`
	RelatedEvidenceIDs    []string            `json:"related_evidence_ids,omitempty"`
	Role                  string              `json:"role"`
	LandmarkKind          SourceLandmarkKind  `json:"landmark_kind,omitempty"`
	LandmarkReason        string              `json:"landmark_reason,omitempty"`
	// PresentationLandmarkReason is an optional terminal-render overlay. It is
	// deliberately excluded from PresentationSHA256 and source authority.
	PresentationLandmarkReason string `json:"presentation_landmark_reason,omitempty"`
	Revision                   string `json:"revision,omitempty"`
	SourceComplete             bool   `json:"source_complete,omitempty"`
	noticeCandidates           []sourceNoticeCandidate
}

// SourceLandmarkKind is presentation-only navigation context for an Overview
// source excerpt. It does not classify repository behavior or participate in
// semantic identity.
type SourceLandmarkKind string

const (
	SourceLandmarkCLIEntrypoint SourceLandmarkKind = "cli_entrypoint"
	SourceLandmarkPublicAPI     SourceLandmarkKind = "public_api"
	SourceLandmarkQuickstart    SourceLandmarkKind = "quickstart_example"
	SourceLandmarkOrientation   SourceLandmarkKind = "orientation_start"
	SourceLandmarkConstructor   SourceLandmarkKind = "constructor"
	SourceLandmarkHandler       SourceLandmarkKind = "handler"
	SourceLandmarkTest          SourceLandmarkKind = "test"
	SourceLandmarkCore          SourceLandmarkKind = "core"
)

type SourceHighlight struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

type SourceSnippetLine struct {
	Line      int    `json:"line"`
	Text      string `json:"text"`
	Highlight bool   `json:"highlight,omitempty"`
	GapBefore bool   `json:"gap_before,omitempty"`
}

// SourceNotice is a presentation-only excerpt of an accepted statement tied
// to the exact source ranges that already support that statement. It does not
// participate in Mechanism identity or content hashing.
type SourceNotice struct {
	Text             string            `json:"text"`
	Path             string            `json:"path"`
	SupportingRanges []SourceHighlight `json:"supporting_ranges"`
}

// Validate checks that a notice contains usable repository-local coordinates.
// Whether the text and ranges belong to an accepted statement is established
// by projectStepSourceNotices while canonical inputs are still available.
func (notice SourceNotice) Validate() error {
	cleaned := path.Clean(notice.Path)
	if strings.TrimSpace(notice.Text) == "" || notice.Path == "" || cleaned != notice.Path ||
		cleaned == "." || path.IsAbs(notice.Path) || strings.Contains(notice.Path, "\\") ||
		strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("source notice: invalid content")
	}
	if !validOrderedSourceRanges(notice.SupportingRanges) {
		return fmt.Errorf("source notice: invalid supporting ranges")
	}
	return nil
}

// Validate checks only the bounded presentation contract. It does not promote
// the snippet to evidence or compare it with a working tree.
func (snippet SourceSnippet) Validate() error {
	cleaned := path.Clean(snippet.Path)
	if snippet.Path == "" || cleaned != snippet.Path || cleaned == "." || path.IsAbs(snippet.Path) ||
		strings.Contains(snippet.Path, "\\") || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("source snippet: invalid repository-relative path")
	}
	if snippet.StartLine <= 0 || snippet.EndLine < snippet.StartLine ||
		len(snippet.Lines) == 0 || len(snippet.Lines) > maxInlineSourceLines {
		return fmt.Errorf("source snippet: invalid line bounds")
	}
	content := make([]string, 0, len(snippet.Lines)+4)
	legacyContent := make([]string, 0, len(snippet.Lines)+4)
	previous := 0
	for index, line := range snippet.Lines {
		if line.Line < snippet.StartLine || line.Line > snippet.EndLine ||
			(previous > 0 && line.Line <= previous) || len(line.Text) > 64<<10 {
			return fmt.Errorf("source snippet: invalid line %d", index)
		}
		gap := previous > 0 && line.Line != previous+1
		if gap != line.GapBefore {
			return fmt.Errorf("source snippet: inconsistent omitted-line marker")
		}
		if gap {
			content = append(content, omittedSourceLinesMarker)
			legacyContent = append(legacyContent, legacyOmittedLinesMarker)
		}
		if line.Highlight != sourceLineIsHighlighted(line.Line, snippet.HighlightRanges) {
			return fmt.Errorf("source snippet: inconsistent highlight marker")
		}
		content = append(content, line.Text)
		legacyContent = append(legacyContent, line.Text)
		previous = line.Line
	}
	if snippet.Lines[0].Line != snippet.StartLine || snippet.Lines[len(snippet.Lines)-1].Line != snippet.EndLine ||
		(snippet.Content != strings.Join(content, "\n") &&
			snippet.Content != strings.Join(legacyContent, "\n")) {
		return fmt.Errorf("source snippet: content does not match lines")
	}
	for _, highlight := range snippet.HighlightRanges {
		if highlight.StartLine <= 0 || highlight.EndLine < highlight.StartLine ||
			highlight.StartLine < snippet.StartLine || highlight.EndLine > snippet.EndLine ||
			!sourceSnippetContainsRange(snippet.Lines, highlight) {
			return fmt.Errorf("source snippet: invalid highlight range")
		}
	}
	if err := validateFullFunctionSource(snippet); err != nil {
		return err
	}
	if !validSourceLandmark(snippet.LandmarkKind, snippet.LandmarkReason) {
		return fmt.Errorf("source snippet: invalid landmark metadata")
	}
	if !validSourceSnippetSHA(snippet.ContentSHA256) || !validSourceSnippetSHA(snippet.PresentationSHA256) {
		return fmt.Errorf("source snippet: invalid content identity")
	}
	if sourceSnippetPresentationSHA(snippet) != snippet.PresentationSHA256 {
		return fmt.Errorf("source snippet: presentation sha256 mismatch")
	}
	return nil
}

func validSourceLandmark(kind SourceLandmarkKind, reason string) bool {
	if kind == "" {
		return reason == ""
	}
	switch kind {
	case SourceLandmarkCLIEntrypoint,
		SourceLandmarkPublicAPI,
		SourceLandmarkQuickstart,
		SourceLandmarkOrientation,
		SourceLandmarkConstructor,
		SourceLandmarkHandler,
		SourceLandmarkTest,
		SourceLandmarkCore:
	default:
		return false
	}
	return reason != "" && reason == strings.TrimSpace(reason) &&
		len(reason) <= maxSourceLandmarkReasonBytes && !strings.ContainsAny(reason, "\r\n\x00")
}

type savedSourceLine struct {
	line int
	text string
}

type savedSourceCandidate struct {
	path         string
	symbol       string
	lines        []savedSourceLine
	contentSHA   string
	complete     bool
	fullFunction bool
	seed         bool
	depth        int
}

type sourceSnippetGroup struct {
	candidate         savedSourceCandidate
	evidence          []semanticdiscovery.EvidenceRef
	dominantEvidence  []semanticdiscovery.EvidenceRef
	inlineEvidence    []semanticdiscovery.EvidenceRef
	noticeCandidates  []sourceNoticeCandidate
	supportOrder      int
	firstEvidence     int
	executableCount   int
	dominantScore     int
	dominantStrongest int
	dominantLineSpan  int
}

type sourceEvidenceMetadata struct {
	rank            int
	substantial     bool
	noticeCandidate *sourceNoticeCandidate
}

type sourceEvidenceCluster struct {
	evidence    []semanticdiscovery.EvidenceRef
	score       int
	strongest   int
	substantial int
	startLine   int
	endLine     int
}

type sourceNoticeCandidate struct {
	EvidenceID string
	Path       string
	Line       int
	Capability semanticdiscovery.Capability
	Text       string
}

func projectStepSourceSnippets(
	data *ReportData,
	step semanticdiscovery.Step,
	statements map[string]semanticdiscovery.Statement,
	probe *goldenmechanism.Result,
) []SourceSnippet {
	candidates := savedSourceCandidates(data, probe)
	if len(candidates) == 0 {
		return nil
	}
	evidenceMetadata := sourceEvidenceMetadataFromProbe(probe)

	facts := make(map[string]semanticdiscovery.Fact, len(data.SemanticSupplementalFacts))
	for _, fact := range data.SemanticSupplementalFacts {
		facts[fact.ID] = fact
	}
	supportOrder := make(map[string]int)
	nextSupport := 0
	for _, statementID := range step.StatementIDs {
		statement, ok := statements[statementID]
		if !ok {
			continue
		}
		for _, factID := range statement.SupportIDs {
			fact, ok := facts[factID]
			if !ok || fact.Source == nil {
				continue
			}
			key := sourceCandidateIdentity(fact.Source.Path, fact.Source.EnclosingSymbol)
			if _, exists := supportOrder[key]; !exists {
				supportOrder[key] = nextSupport
				nextSupport++
			}
		}
	}

	groups := make(map[int]*sourceSnippetGroup)
	for evidenceIndex, reference := range step.Evidence {
		candidateIndex := narrowestContainingCandidate(candidates, reference.Path, reference.Line)
		if candidateIndex < 0 {
			continue
		}
		group := groups[candidateIndex]
		if group == nil {
			group = &sourceSnippetGroup{
				candidate:     candidates[candidateIndex],
				supportOrder:  1 << 30,
				firstEvidence: evidenceIndex,
			}
			if order, ok := supportOrder[sourceCandidateIdentity(group.candidate.path, group.candidate.symbol)]; ok {
				group.supportOrder = order
			}
			groups[candidateIndex] = group
		}
		group.evidence = append(group.evidence, reference)
		if reference.Kind != "saved_source_window" && reference.Label != "saved bounded source window" {
			group.executableCount++
		}
	}

	ordered := make([]*sourceSnippetGroup, 0, len(groups))
	for _, group := range groups {
		cluster, inlineEvidence := selectInlineSourceEvidence(group.evidence, evidenceMetadata)
		group.dominantEvidence = cluster.evidence
		group.inlineEvidence = inlineEvidence
		group.dominantScore = cluster.score
		group.dominantStrongest = cluster.strongest
		group.dominantLineSpan = cluster.endLine - cluster.startLine
		for _, reference := range inlineEvidence {
			metadata := evidenceMetadata[reference.ID]
			if metadata.noticeCandidate != nil {
				group.noticeCandidates = append(group.noticeCandidates, *metadata.noticeCandidate)
			}
		}
		sort.Slice(group.noticeCandidates, func(i, j int) bool {
			if group.noticeCandidates[i].Path != group.noticeCandidates[j].Path {
				return group.noticeCandidates[i].Path < group.noticeCandidates[j].Path
			}
			if group.noticeCandidates[i].Line != group.noticeCandidates[j].Line {
				return group.noticeCandidates[i].Line < group.noticeCandidates[j].Line
			}
			return group.noticeCandidates[i].EvidenceID < group.noticeCandidates[j].EvidenceID
		})
		ordered = append(ordered, group)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left.supportOrder != right.supportOrder {
			return left.supportOrder < right.supportOrder
		}
		if sourcePathPenalty(left.candidate.path) != sourcePathPenalty(right.candidate.path) {
			return sourcePathPenalty(left.candidate.path) < sourcePathPenalty(right.candidate.path)
		}
		if left.dominantScore != right.dominantScore {
			return left.dominantScore > right.dominantScore
		}
		if left.dominantStrongest != right.dominantStrongest {
			return left.dominantStrongest > right.dominantStrongest
		}
		if left.dominantLineSpan != right.dominantLineSpan {
			return left.dominantLineSpan < right.dominantLineSpan
		}
		if left.executableCount != right.executableCount {
			return left.executableCount > right.executableCount
		}
		if len(left.evidence) != len(right.evidence) {
			return len(left.evidence) > len(right.evidence)
		}
		if left.candidate.seed != right.candidate.seed {
			return left.candidate.seed
		}
		if left.candidate.depth != right.candidate.depth {
			return left.candidate.depth < right.candidate.depth
		}
		if left.firstEvidence != right.firstEvidence {
			return left.firstEvidence < right.firstEvidence
		}
		if left.candidate.path != right.candidate.path {
			return left.candidate.path < right.candidate.path
		}
		return left.candidate.symbol < right.candidate.symbol
	})
	if len(ordered) > 3 {
		ordered = ordered[:3]
	}

	result := make([]SourceSnippet, 0, len(ordered))
	for index, group := range ordered {
		role := "related"
		if index == 0 {
			role = "primary"
		}
		if snippet, ok := sourceSnippetFromGroup(data, *group, role); ok {
			result = append(result, snippet)
		}
	}
	return result
}

func savedSourceCandidates(data *ReportData, probe *goldenmechanism.Result) []savedSourceCandidate {
	result := make([]savedSourceCandidate, 0)
	seen := make(map[string]struct{})
	if probe != nil {
		for _, function := range probe.Functions {
			lines := make([]savedSourceLine, 0, len(function.Source))
			texts := make([]string, 0, len(function.Source))
			for _, line := range function.Source {
				if line.Location.Path != function.Path || line.Location.Line <= 0 || line.Truncated {
					continue
				}
				lines = append(lines, savedSourceLine{line: line.Location.Line, text: line.Text})
				texts = append(texts, line.Text)
			}
			if !continuousSavedSource(lines) {
				continue
			}
			candidate := savedSourceCandidate{
				path: function.Path, symbol: function.Symbol, lines: lines,
				contentSHA: sourceLinesSHA256(texts), complete: !function.SourceTruncated,
				fullFunction: !function.SourceTruncated,
				seed:         function.Seed,
				depth:        function.Depth,
			}
			key := savedSourceKey(candidate)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, candidate)
		}
	}

	if data != nil && data.ModelResearch != nil {
		windows := data.ModelResearch.Theory.GroundedFacts
		for _, fact := range data.SemanticSupplementalFacts {
			if fact.Source == nil {
				continue
			}
			candidate, ok := candidateFromFactSource(*fact.Source, windows)
			if !ok {
				continue
			}
			key := savedSourceKey(candidate)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, candidate)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].path != result[j].path {
			return result[i].path < result[j].path
		}
		if result[i].lines[0].line != result[j].lines[0].line {
			return result[i].lines[0].line < result[j].lines[0].line
		}
		return result[i].symbol < result[j].symbol
	})
	return result
}

func candidateFromFactSource(
	source semanticdiscovery.FactSource,
	windows []modelresearch.EvidenceItem,
) (savedSourceCandidate, bool) {
	for _, item := range windows {
		if item.Kind != modelresearch.EvidenceSource || item.Location == nil || item.Window == nil ||
			!item.Window.CodeBearing || item.Window.Truncated || item.Location.Path != source.Path ||
			source.StartLine < item.Window.StartLine || source.EndLine > item.Window.EndLine {
			continue
		}
		start := source.StartLine - item.Window.StartLine
		end := source.EndLine - item.Window.StartLine + 1
		if start < 0 || end > len(item.Window.Lines) || start >= end {
			continue
		}
		texts := append([]string(nil), item.Window.Lines[start:end]...)
		if sourceLinesSHA256(texts) != source.ContentSHA256 {
			continue
		}
		lines := make([]savedSourceLine, 0, len(texts))
		for index, text := range texts {
			lines = append(lines, savedSourceLine{line: source.StartLine + index, text: text})
		}
		return savedSourceCandidate{
			path: source.Path, symbol: source.EnclosingSymbol, lines: lines,
			contentSHA: source.ContentSHA256, complete: true,
		}, true
	}
	return savedSourceCandidate{}, false
}

func sourceSnippetFromGroup(data *ReportData, group sourceSnippetGroup, role string) (SourceSnippet, bool) {
	highlightEvidence := group.inlineEvidence
	if len(highlightEvidence) == 0 {
		highlightEvidence = group.dominantEvidence
	}
	if len(highlightEvidence) == 0 {
		highlightEvidence = group.evidence
	}
	highlightLines := make([]int, 0, len(highlightEvidence))
	evidenceIDs := make([]string, 0, len(group.evidence))
	seenLines := make(map[int]struct{})
	seenEvidence := make(map[string]struct{})
	for _, reference := range highlightEvidence {
		if _, duplicate := seenLines[reference.Line]; !duplicate {
			seenLines[reference.Line] = struct{}{}
			highlightLines = append(highlightLines, reference.Line)
		}
	}
	for _, reference := range group.evidence {
		if reference.ID != "" {
			if _, duplicate := seenEvidence[reference.ID]; !duplicate {
				seenEvidence[reference.ID] = struct{}{}
				evidenceIDs = append(evidenceIDs, reference.ID)
			}
		}
	}
	sort.Ints(highlightLines)
	sort.Strings(evidenceIDs)
	selected := selectInlineSourceLines(group.candidate.lines, highlightLines)
	if len(selected) == 0 {
		return SourceSnippet{}, false
	}
	highlightSet := make(map[int]struct{}, len(highlightLines))
	for _, line := range highlightLines {
		highlightSet[line] = struct{}{}
	}
	lines := make([]SourceSnippetLine, 0, len(selected))
	content := make([]string, 0, len(selected)+4)
	previous := 0
	for _, line := range selected {
		gap := previous > 0 && line.line != previous+1
		if gap {
			content = append(content, omittedSourceLinesMarker)
		}
		_, highlighted := highlightSet[line.line]
		lines = append(lines, SourceSnippetLine{
			Line: line.line, Text: line.text, Highlight: highlighted, GapBefore: gap,
		})
		content = append(content, line.text)
		previous = line.line
	}
	snippet := SourceSnippet{
		Path: group.candidate.path, Language: sourceLanguage(group.candidate.path),
		EnclosingSymbol: group.candidate.symbol,
		StartLine:       lines[0].Line, EndLine: lines[len(lines)-1].Line,
		HighlightRanges: sourceHighlightRanges(highlightLines),
		Content:         strings.Join(content, "\n"), Lines: lines,
		ContentSHA256:      group.candidate.contentSHA,
		RelatedEvidenceIDs: evidenceIDs, Role: role,
		Revision: reportSourceRevision(data), SourceComplete: group.candidate.complete,
		noticeCandidates: append([]sourceNoticeCandidate(nil), group.noticeCandidates...),
	}
	if fullLines, startLine, endLine := projectFullFunctionSource(
		group.candidate,
		highlightSet,
		len(selected),
	); len(fullLines) > 0 {
		snippet.FullFunctionStartLine = startLine
		snippet.FullFunctionEndLine = endLine
		snippet.FullFunctionLines = fullLines
	}
	snippet.PresentationSHA256 = sourceSnippetPresentationSHA(snippet)
	if err := snippet.Validate(); err != nil {
		return SourceSnippet{}, false
	}
	return snippet, true
}

func selectInlineSourceLines(lines []savedSourceLine, highlights []int) []savedSourceLine {
	indexByLine := make(map[int]int, len(lines))
	for index, line := range lines {
		indexByLine[line.line] = index
	}
	var boundedFallback []savedSourceLine
	totalContext := inlineSourceContextBefore + inlineSourceContextAfter
	for contextLines := totalContext; contextLines >= 0; contextLines-- {
		contextBefore := min(inlineSourceContextBefore, contextLines)
		contextAfter := contextLines - contextBefore
		selected := make(map[int]struct{})
		for _, highlight := range highlights {
			index, ok := indexByLine[highlight]
			if !ok {
				continue
			}
			start := max(0, index-contextBefore)
			end := min(len(lines)-1, index+contextAfter)
			for cursor := start; cursor <= end; cursor++ {
				selected[cursor] = struct{}{}
			}
		}
		if len(selected) == 0 {
			end := min(len(lines), preferredInlineSourceLines)
			return append([]savedSourceLine(nil), lines[:end]...)
		}
		if len(selected) <= maxInlineSourceLines {
			indices := make([]int, 0, len(selected))
			for index := range selected {
				indices = append(indices, index)
			}
			sort.Ints(indices)
			result := make([]savedSourceLine, 0, len(indices))
			for _, index := range indices {
				result = append(result, lines[index])
			}
			if len(result) <= preferredInlineSourceLines {
				return result
			}
			if boundedFallback == nil {
				boundedFallback = result
			}
		}
	}
	return boundedFallback
}

func sourceEvidenceMetadataFromProbe(
	probe *goldenmechanism.Result,
) map[string]sourceEvidenceMetadata {
	result := make(map[string]sourceEvidenceMetadata)
	if probe == nil {
		return result
	}
	for _, observation := range probe.Observations {
		rank := sourceObservationRank(observation.Basis)
		substantial := sourceObservationIsSubstantial(observation.Basis)
		candidate, candidateOK := sourceNoticeCandidateFromObservation(observation)
		for _, reference := range observation.Evidence {
			if reference.ID == "" {
				continue
			}
			metadata := result[reference.ID]
			if rank > metadata.rank {
				metadata.rank = rank
			}
			metadata.substantial = metadata.substantial || substantial
			if candidateOK && candidate.EvidenceID == reference.ID &&
				(metadata.noticeCandidate == nil || rank > sourceNoticeCandidateRank(*metadata.noticeCandidate)) {
				copy := candidate
				metadata.noticeCandidate = &copy
			}
			result[reference.ID] = metadata
		}
	}
	return result
}

func sourceObservationRank(basis goldenmechanism.SyntaxBasis) int {
	switch basis {
	case goldenmechanism.BasisDirectCall, goldenmechanism.BasisOutput:
		return 6
	case goldenmechanism.BasisAssignment, goldenmechanism.BasisReturn,
		goldenmechanism.BasisErrorHandoff:
		return 5
	case goldenmechanism.BasisBranch, goldenmechanism.BasisRead,
		goldenmechanism.BasisTransform:
		return 4
	case goldenmechanism.BasisDeclaration, goldenmechanism.BasisLexicalOrder:
		return 0
	default:
		return 1
	}
}

func sourceObservationIsSubstantial(basis goldenmechanism.SyntaxBasis) bool {
	switch basis {
	case goldenmechanism.BasisDirectCall,
		goldenmechanism.BasisOutput,
		goldenmechanism.BasisAssignment,
		goldenmechanism.BasisReturn,
		goldenmechanism.BasisErrorHandoff,
		goldenmechanism.BasisTransform:
		return true
	default:
		return false
	}
}

func sourceEvidenceReferenceRank(
	reference semanticdiscovery.EvidenceRef,
	metadata map[string]sourceEvidenceMetadata,
) int {
	rank := metadata[reference.ID].rank
	labelRank := 1
	switch strings.ToLower(strings.TrimSpace(reference.Label)) {
	case "direct_call", "output", "output_effect":
		labelRank = 6
	case "assignment", "data_write", "return", "error_return", "error_path":
		labelRank = 5
	case "branch", "data_read", "data_transformation":
		labelRank = 4
	case "declaration", "saved bounded source window":
		labelRank = 0
	}
	if reference.Kind == "saved_source_window" {
		labelRank = 0
	}
	return max(rank, labelRank)
}

func sourceEvidenceReferenceIsSubstantial(
	reference semanticdiscovery.EvidenceRef,
	metadata map[string]sourceEvidenceMetadata,
) bool {
	if reference.Kind == boundedPrimaryPathSyntaxKind {
		return true
	}
	if metadata[reference.ID].substantial {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(reference.Label)) {
	case "direct_call", "output", "output_effect", "assignment", "data_write",
		"return", "error_return", "error_path", "data_transformation":
		return true
	default:
		return false
	}
}

func sourceEvidenceClusters(
	evidence []semanticdiscovery.EvidenceRef,
	metadata map[string]sourceEvidenceMetadata,
) []sourceEvidenceCluster {
	ordered := append([]semanticdiscovery.EvidenceRef(nil), evidence...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Path != ordered[j].Path {
			return ordered[i].Path < ordered[j].Path
		}
		if ordered[i].Line != ordered[j].Line {
			return ordered[i].Line < ordered[j].Line
		}
		if ordered[i].Column != ordered[j].Column {
			return ordered[i].Column < ordered[j].Column
		}
		return ordered[i].ID < ordered[j].ID
	})
	clusters := make([]sourceEvidenceCluster, 0, len(ordered))
	for _, reference := range ordered {
		if reference.Line <= 0 {
			continue
		}
		rank := sourceEvidenceReferenceRank(reference, metadata)
		substantial := 0
		if sourceEvidenceReferenceIsSubstantial(reference, metadata) {
			substantial = 1
		}
		if len(clusters) == 0 || reference.Path != clusters[len(clusters)-1].evidence[0].Path ||
			reference.Line-clusters[len(clusters)-1].endLine > maxSourceEvidenceLineGap {
			clusters = append(clusters, sourceEvidenceCluster{
				evidence: []semanticdiscovery.EvidenceRef{reference},
				score:    rank, strongest: rank, substantial: substantial,
				startLine: reference.Line, endLine: reference.Line,
			})
			continue
		}
		cluster := &clusters[len(clusters)-1]
		cluster.evidence = append(cluster.evidence, reference)
		cluster.score += rank
		cluster.strongest = max(cluster.strongest, rank)
		cluster.substantial += substantial
		cluster.endLine = reference.Line
	}
	return clusters
}

func selectInlineSourceEvidence(
	evidence []semanticdiscovery.EvidenceRef,
	metadata map[string]sourceEvidenceMetadata,
) (sourceEvidenceCluster, []semanticdiscovery.EvidenceRef) {
	clusters := sourceEvidenceClusters(evidence, metadata)
	if len(clusters) == 0 {
		return sourceEvidenceCluster{}, nil
	}
	bestIndex := 0
	for index := 1; index < len(clusters); index++ {
		if sourceEvidenceClusterBetter(clusters[index], clusters[bestIndex]) {
			bestIndex = index
		}
	}

	selected := map[int]struct{}{bestIndex: {}}
	selectedLines := make(map[int]struct{})
	for _, reference := range clusters[bestIndex].evidence {
		selectedLines[reference.Line] = struct{}{}
	}
	additional := make([]int, 0, len(clusters)-1)
	for index, cluster := range clusters {
		if index != bestIndex && cluster.substantial > 0 {
			additional = append(additional, index)
		}
	}
	sort.SliceStable(additional, func(i, j int) bool {
		return sourceEvidenceClusterBetter(clusters[additional[i]], clusters[additional[j]])
	})
	for _, index := range additional {
		additionalLines := 0
		for _, reference := range clusters[index].evidence {
			if _, exists := selectedLines[reference.Line]; !exists {
				additionalLines++
			}
		}
		if len(selectedLines)+additionalLines > maxInlineSourceLines {
			continue
		}
		selected[index] = struct{}{}
		for _, reference := range clusters[index].evidence {
			selectedLines[reference.Line] = struct{}{}
		}
	}

	inline := make([]semanticdiscovery.EvidenceRef, 0, len(evidence))
	for index, cluster := range clusters {
		if _, keep := selected[index]; keep {
			inline = append(inline, cluster.evidence...)
		}
	}
	return clusters[bestIndex], inline
}

func sourceEvidenceClusterBetter(left, right sourceEvidenceCluster) bool {
	if left.score != right.score {
		return left.score > right.score
	}
	if left.strongest != right.strongest {
		return left.strongest > right.strongest
	}
	leftSpan := left.endLine - left.startLine
	rightSpan := right.endLine - right.startLine
	if leftSpan != rightSpan {
		return leftSpan < rightSpan
	}
	if left.startLine != right.startLine {
		return left.startLine < right.startLine
	}
	return left.evidence[0].ID < right.evidence[0].ID
}

func sourceNoticeCandidateFromObservation(
	observation goldenmechanism.Observation,
) (sourceNoticeCandidate, bool) {
	if len(observation.Evidence) != 1 {
		return sourceNoticeCandidate{}, false
	}
	reference := observation.Evidence[0]
	if reference.ID == "" || reference.Location.Path == "" || reference.Location.Line <= 0 {
		return sourceNoticeCandidate{}, false
	}
	candidate := sourceNoticeCandidate{
		EvidenceID: reference.ID,
		Path:       reference.Location.Path,
		Line:       reference.Location.Line,
		Capability: observation.Capability,
	}
	switch {
	case observation.Basis == goldenmechanism.BasisBranch &&
		observation.Capability == semanticdiscovery.CapabilityBranch:
		subject := strings.TrimSpace(observation.Object)
		if !safeSourceNoticeSubject(subject) {
			return sourceNoticeCandidate{}, false
		}
		candidate.Text = "Checks " + subject + "."
	case observation.Basis == goldenmechanism.BasisDirectCall &&
		observation.Capability == semanticdiscovery.CapabilityDirectCall:
		subject := strings.TrimSpace(observation.TargetSymbol)
		if !safeSourceNoticeSubject(subject) {
			return sourceNoticeCandidate{}, false
		}
		candidate.Text = "Calls " + subject + "."
	default:
		return sourceNoticeCandidate{}, false
	}
	if len(candidate.Text) > maxSourceNoticeTextBytes {
		return sourceNoticeCandidate{}, false
	}
	return candidate, true
}

func sourceNoticeCandidateRank(candidate sourceNoticeCandidate) int {
	switch candidate.Capability {
	case semanticdiscovery.CapabilityDirectCall:
		return 6
	case semanticdiscovery.CapabilityBranch:
		return 4
	default:
		return 0
	}
}

func safeSourceNoticeSubject(subject string) bool {
	return subject != "" && len(subject) <= maxSourceNoticeTextBytes-16 &&
		!strings.ContainsAny(subject, "\r\n\x00")
}

func projectFullFunctionSource(
	candidate savedSourceCandidate,
	highlights map[int]struct{},
	selectedLineCount int,
) ([]SourceSnippetLine, int, int) {
	if !candidate.fullFunction || !candidate.complete || len(candidate.lines) == 0 ||
		len(candidate.lines) <= selectedLineCount || len(candidate.lines) > maxFullFunctionSourceLines {
		return nil, 0, 0
	}
	byteCount := 0
	lines := make([]SourceSnippetLine, 0, len(candidate.lines))
	for _, line := range candidate.lines {
		byteCount += len(line.text)
		if byteCount > maxFullFunctionSourceBytes {
			return nil, 0, 0
		}
		_, highlighted := highlights[line.line]
		lines = append(lines, SourceSnippetLine{
			Line: line.line, Text: line.text, Highlight: highlighted,
		})
	}
	return lines, lines[0].Line, lines[len(lines)-1].Line
}

func validateFullFunctionSource(snippet SourceSnippet) error {
	hasFullFunction := snippet.FullFunctionStartLine != 0 || snippet.FullFunctionEndLine != 0 ||
		len(snippet.FullFunctionLines) > 0
	if !hasFullFunction {
		return nil
	}
	if !snippet.SourceComplete || snippet.FullFunctionStartLine <= 0 ||
		snippet.FullFunctionEndLine < snippet.FullFunctionStartLine ||
		len(snippet.FullFunctionLines) == 0 ||
		len(snippet.FullFunctionLines) > maxFullFunctionSourceLines ||
		snippet.StartLine < snippet.FullFunctionStartLine || snippet.EndLine > snippet.FullFunctionEndLine {
		return fmt.Errorf("source snippet: invalid full function bounds")
	}

	texts := make([]string, 0, len(snippet.FullFunctionLines))
	byLine := make(map[int]SourceSnippetLine, len(snippet.FullFunctionLines))
	byteCount := 0
	previous := 0
	for index, line := range snippet.FullFunctionLines {
		if line.Line < snippet.FullFunctionStartLine || line.Line > snippet.FullFunctionEndLine ||
			(previous > 0 && line.Line != previous+1) || line.GapBefore || len(line.Text) > 64<<10 {
			return fmt.Errorf("source snippet: invalid full function line %d", index)
		}
		if line.Highlight != sourceLineIsHighlighted(line.Line, snippet.HighlightRanges) {
			return fmt.Errorf("source snippet: inconsistent full function highlight marker")
		}
		byteCount += len(line.Text)
		if byteCount > maxFullFunctionSourceBytes {
			return fmt.Errorf("source snippet: full function exceeds byte bound")
		}
		texts = append(texts, line.Text)
		byLine[line.Line] = line
		previous = line.Line
	}
	if snippet.FullFunctionLines[0].Line != snippet.FullFunctionStartLine ||
		snippet.FullFunctionLines[len(snippet.FullFunctionLines)-1].Line != snippet.FullFunctionEndLine ||
		sourceLinesSHA256(texts) != snippet.ContentSHA256 {
		return fmt.Errorf("source snippet: full function content mismatch")
	}
	for _, line := range snippet.Lines {
		full, ok := byLine[line.Line]
		if !ok || full.Text != line.Text || full.Highlight != line.Highlight {
			return fmt.Errorf("source snippet: compact source differs from full function")
		}
	}
	return nil
}

func sourceLineIsHighlighted(line int, ranges []SourceHighlight) bool {
	for _, sourceRange := range ranges {
		if line >= sourceRange.StartLine && line <= sourceRange.EndLine {
			return true
		}
	}
	return false
}

func sourceSnippetContainsRange(lines []SourceSnippetLine, sourceRange SourceHighlight) bool {
	if sourceRange.StartLine <= 0 || sourceRange.EndLine < sourceRange.StartLine ||
		sourceRange.EndLine-sourceRange.StartLine+1 > len(lines) {
		return false
	}
	found := 0
	for _, line := range lines {
		if line.Line >= sourceRange.StartLine && line.Line <= sourceRange.EndLine {
			found++
		}
	}
	return found == sourceRange.EndLine-sourceRange.StartLine+1
}

func validOrderedSourceRanges(ranges []SourceHighlight) bool {
	if len(ranges) == 0 {
		return false
	}
	previousEnd := 0
	for _, sourceRange := range ranges {
		if sourceRange.StartLine <= previousEnd || sourceRange.EndLine < sourceRange.StartLine {
			return false
		}
		previousEnd = sourceRange.EndLine
	}
	return true
}

func projectStepSourceNotices(
	data *ReportData,
	step semanticdiscovery.Step,
	statements map[string]semanticdiscovery.Statement,
	sources []SourceSnippet,
) []SourceNotice {
	if data == nil || len(sources) == 0 {
		return nil
	}
	facts := make(map[string]semanticdiscovery.Fact, len(data.SemanticSupplementalFacts))
	for _, fact := range data.SemanticSupplementalFacts {
		facts[fact.ID] = fact
	}
	stepEvidence := make(map[string]semanticdiscovery.EvidenceRef, len(step.Evidence))
	for _, reference := range step.Evidence {
		if reference.ID != "" {
			stepEvidence[reference.ID] = reference
		}
	}
	acceptedCapabilities := make(map[string]map[semanticdiscovery.Capability]struct{})
	for _, statementID := range step.StatementIDs {
		statement, ok := statements[statementID]
		if !ok || (statement.Basis != semanticdiscovery.ClaimDirect &&
			statement.Basis != semanticdiscovery.ClaimCompositional) {
			continue
		}
		for _, supportID := range statement.SupportIDs {
			fact, ok := facts[supportID]
			if !ok {
				continue
			}
			for _, reference := range fact.Evidence {
				stepReference, ok := stepEvidence[reference.ID]
				if !ok || reference.Path == "" || reference.Line <= 0 ||
					stepReference.Path != reference.Path || stepReference.Line != reference.Line {
					continue
				}
				capabilities := acceptedCapabilities[reference.ID]
				if capabilities == nil {
					capabilities = make(map[semanticdiscovery.Capability]struct{})
					acceptedCapabilities[reference.ID] = capabilities
				}
				for _, capability := range fact.Capabilities {
					capabilities[capability] = struct{}{}
				}
			}
		}
	}

	result := make([]SourceNotice, 0, maxSourceNotices)
	seen := make(map[string]struct{})
	primary := sources[0]
	for _, candidate := range primary.noticeCandidates {
		stepReference, stepReferenceOK := stepEvidence[candidate.EvidenceID]
		capabilities := acceptedCapabilities[candidate.EvidenceID]
		if _, accepted := capabilities[candidate.Capability]; !accepted || !stepReferenceOK ||
			stepReference.Path != candidate.Path || stepReference.Line != candidate.Line ||
			candidate.Path != primary.Path || candidate.Line <= 0 {
			continue
		}
		sourceRange := SourceHighlight{StartLine: candidate.Line, EndLine: candidate.Line}
		if !sourceSnippetContainsRange(primary.Lines, sourceRange) ||
			!sourceLineIsHighlighted(candidate.Line, primary.HighlightRanges) {
			continue
		}
		key := candidate.Path + "\x00" + fmt.Sprintf("%d", candidate.Line) + "\x00" + candidate.Text
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		notice := SourceNotice{
			Text:             candidate.Text,
			Path:             candidate.Path,
			SupportingRanges: []SourceHighlight{sourceRange},
		}
		if err := notice.Validate(); err != nil {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, notice)
		if len(result) >= maxSourceNotices {
			return result
		}
	}
	return result
}

type overviewSourceCandidate struct {
	snippet            SourceSnippet
	evidenceID         string
	landmarkRank       int
	recommended        bool
	recommendationRank int
	finding            bool
	findingRank        int
	declarationRank    int
	focusLine          int
}

type overviewSourceRecommendation struct {
	rank   int
	reason string
}

type overviewSourceFinding struct {
	rank   int
	reason string
}

type overviewSourceDeclaration struct {
	line   int
	symbol string
	rank   int
	kind   SourceLandmarkKind
}

func projectOverviewSourceSnippets(data *ReportData) []SourceSnippet {
	if data == nil || data.ModelResearch == nil {
		return nil
	}
	recommendations := overviewSourceRecommendations(data.FirstFilesToOpen)
	findings := overviewSourceFindings(data.ModelResearch.Theory.AcceptedModelInterpretations)
	candidates := make([]overviewSourceCandidate, 0)
	evidenceIDsByPath := make(map[string]map[string]struct{})
	for _, item := range data.ModelResearch.Theory.GroundedFacts {
		if item.Kind != modelresearch.EvidenceSource || item.Location == nil || item.Window == nil ||
			!item.Window.CodeBearing || len(item.Window.Lines) == 0 {
			continue
		}
		sourcePath := item.Location.Path
		if !stringSliceContains(data.OpenablePaths, sourcePath) {
			continue
		}
		lines := make([]savedSourceLine, 0, len(item.Window.Lines))
		for index, text := range item.Window.Lines {
			lines = append(lines, savedSourceLine{line: item.Window.StartLine + index, text: text})
		}
		finding, findingOK := findings[item.ID]
		declaration := selectOverviewSourceDeclaration(
			sourcePath,
			lines,
			item.Symbol,
			finding.reason,
			item.Location.Line,
		)
		if declaration.line <= 0 {
			continue
		}
		role := overviewSourceRole(data, sourcePath)
		recommendation, recommended := recommendations[sourcePath]
		kind, reason := overviewSourceLandmarkMetadata(
			sourcePath,
			role,
			declaration,
			recommendation,
			recommended,
			finding,
			findingOK,
		)
		group := sourceSnippetGroup{
			candidate: savedSourceCandidate{
				path: sourcePath, symbol: declaration.symbol, lines: lines,
				contentSHA: sourceLinesSHA256(item.Window.Lines), complete: !item.Window.Truncated,
			},
			evidence: []semanticdiscovery.EvidenceRef{{
				ID: item.ID, Kind: string(item.Kind), Path: sourcePath, Line: declaration.line,
			}},
		}
		snippet, ok := sourceSnippetFromGroup(data, group, role)
		if !ok {
			continue
		}
		snippet.LandmarkKind = kind
		snippet.LandmarkReason = reason
		snippet.PresentationSHA256 = sourceSnippetPresentationSHA(snippet)
		if err := snippet.Validate(); err != nil {
			continue
		}
		candidates = append(candidates, overviewSourceCandidate{
			snippet:            snippet,
			evidenceID:         item.ID,
			landmarkRank:       sourceLandmarkRank(kind),
			recommended:        recommended,
			recommendationRank: recommendation.rank,
			finding:            findingOK,
			findingRank:        finding.rank,
			declarationRank:    declaration.rank,
			focusLine:          declaration.line,
		})
		if evidenceIDsByPath[sourcePath] == nil {
			evidenceIDsByPath[sourcePath] = make(map[string]struct{})
		}
		if item.ID != "" {
			evidenceIDsByPath[sourcePath][item.ID] = struct{}{}
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return overviewSourceCandidateLess(candidates[i], candidates[j])
	})
	result := make([]SourceSnippet, 0, len(candidates))
	seenPaths := make(map[string]struct{})
	for _, candidate := range candidates {
		if _, duplicate := seenPaths[candidate.snippet.Path]; duplicate {
			continue
		}
		seenPaths[candidate.snippet.Path] = struct{}{}
		candidate.snippet.RelatedEvidenceIDs = sortedSourceEvidenceIDs(
			evidenceIDsByPath[candidate.snippet.Path],
		)
		candidate.snippet.PresentationSHA256 = sourceSnippetPresentationSHA(candidate.snippet)
		if err := candidate.snippet.Validate(); err != nil {
			continue
		}
		result = append(result, candidate.snippet)
	}
	return result
}

type overviewSourceTarget struct {
	path string
	line int
}

// PrepareAuthorizedSourceCoverage atomically installs the exact, authorized
// source excerpts needed by every currently visible Overview object. Runtime
// semantic stages may call it after repository authority is confirmed; final
// report generation calls the same function again and must obtain the same
// idempotent projection before replaying Atlas Study artifacts.
func PrepareAuthorizedSourceCoverage(
	ctx context.Context,
	data *ReportData,
	authority *RunAuthority,
) error {
	if data == nil || authority == nil {
		return fmt.Errorf("report source coverage: authorized report data is required")
	}
	prepared := *data
	prepared.UserSources = cloneOverviewSourceSnippets(data.UserSources)
	prepared.CapturedRevision = authority.repository.Head
	preparedAuthority := *authority
	preparedAuthority.inputs = append([]freshness.CapturedInput(nil), authority.inputs...)
	if authority.inputs == nil {
		preparedAuthority.inputs = nil
	}
	if err := completeOverviewSourceCoverage(ctx, &prepared, &preparedAuthority); err != nil {
		return err
	}
	data.UserSources = prepared.UserSources
	data.CapturedRevision = prepared.CapturedRevision
	authority.inputs = preparedAuthority.inputs
	return nil
}

// completeOverviewSourceCoverage atomically verifies the existing editorial
// source projection and extends it with exact authorized excerpts for every
// Overview object that has a persisted visible source location. It never
// reduces the target set to fit a presentation budget.
func completeOverviewSourceCoverage(
	ctx context.Context,
	data *ReportData,
	authority *RunAuthority,
) error {
	if data == nil || authority == nil {
		return fmt.Errorf("report source coverage: authorized report data is required")
	}
	if err := authority.validate(); err != nil {
		return err
	}
	targets := overviewSourceTargets(data)
	if len(targets) == 0 && len(data.UserSources) == 0 {
		return nil
	}
	if strings.TrimSpace(data.CapturedRevision) != authority.repository.Head {
		return fmt.Errorf("report source coverage: captured revision does not match authorized repository")
	}
	capturedInputs := authority.inputs
	capturedInputsChanged := false
	if capturedInputs == nil {
		repositoryPaths, err := repositoryRelativeInputPaths(
			authority.repository.Identity,
			authority.analysisRoot,
			data.OpenablePaths,
		)
		if err != nil {
			return err
		}
		inputs, err := freshness.CaptureInputs(ctx, authority.repository, repositoryPaths)
		if err != nil {
			return fmt.Errorf("report source coverage: capture authorized inputs: %w", err)
		}
		capturedInputs = inputs
		capturedInputsChanged = true
	}
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: authority.repository.Identity,
		AnalysisRoot:   authority.analysisRoot,
		AllowedPaths:   data.OpenablePaths,
		CapturedInputs: capturedInputs,
	})
	if err != nil {
		return fmt.Errorf("report source coverage: authorized source catalog: %w", err)
	}
	content, err := workspacecontent.New(catalog)
	if err != nil {
		return fmt.Errorf("report source coverage: open authorized source content: %w", err)
	}

	projected := cloneOverviewSourceSnippets(data.UserSources)
	verifiedExisting := make(map[string]struct{})
	for _, target := range targets {
		resolved, conflict := resolveOverviewSourceSnippet(projected, target)
		if conflict {
			_ = content.Close()
			return fmt.Errorf("report source coverage: conflicting exact saved excerpt")
		}
		if resolved {
			var insertionIndex int
			projected, resolved, insertionIndex, err = rebindOverviewSourceSnippets(
				ctx,
				data,
				content,
				projected,
				target,
				verifiedExisting,
			)
			if err != nil {
				_ = content.Close()
				return err
			}
			if resolved {
				continue
			}
			snippet, snippetErr := authorizedOverviewSourceSnippet(ctx, data, content, target)
			if snippetErr != nil {
				_ = content.Close()
				return snippetErr
			}
			projected = insertExactOverviewSourceSnippet(projected, snippet, insertionIndex)
			continue
		}
		snippet, err := authorizedOverviewSourceSnippet(ctx, data, content, target)
		if err != nil {
			_ = content.Close()
			return err
		}
		projected = mergeExactOverviewSourceSnippet(projected, snippet)
	}
	projected, err = retainAuthorizedOverviewSourceSnippets(
		ctx,
		data,
		content,
		projected,
		verifiedExisting,
	)
	if err != nil {
		_ = content.Close()
		return err
	}
	if err := content.Close(); err != nil {
		return fmt.Errorf("report source coverage: close authorized source content: %w", err)
	}
	for _, target := range targets {
		resolved, conflict := resolveOverviewSourceSnippet(projected, target)
		if conflict || !resolved {
			return fmt.Errorf("report source coverage: incomplete exact saved excerpt projection")
		}
	}
	data.UserSources = projected
	if capturedInputsChanged {
		authority.inputs = capturedInputs
	}
	return nil
}

// retainAuthorizedOverviewSourceSnippets removes any stale non-target excerpt
// from the private projection while preserving the exact identity and order of
// every excerpt that still matches the captured workspace. Authority failures
// abort the whole projection rather than publishing a partially filtered list.
func retainAuthorizedOverviewSourceSnippets(
	ctx context.Context,
	data *ReportData,
	content *workspacecontent.Service,
	sources []SourceSnippet,
	verified map[string]struct{},
) ([]SourceSnippet, error) {
	result := make([]SourceSnippet, 0, len(sources))
	for _, snippet := range sources {
		_, matches := verified[snippet.PresentationSHA256]
		if !matches {
			var err error
			matches, err = authorizedOverviewSourceSnippetMatches(ctx, data, content, snippet)
			if err != nil {
				return nil, err
			}
			if matches {
				verified[snippet.PresentationSHA256] = struct{}{}
			}
		}
		if matches {
			result = append(result, snippet)
		}
	}
	return result, nil
}

// rebindOverviewSourceSnippets admits an existing excerpt only after every
// displayed source line has been read through the captured-input-authorized
// content service. Stale excerpts are removed from the private projection and
// replaced later at the first removed position; caller-owned data is untouched
// until the complete projection succeeds.
func rebindOverviewSourceSnippets(
	ctx context.Context,
	data *ReportData,
	content *workspacecontent.Service,
	sources []SourceSnippet,
	target overviewSourceTarget,
	verified map[string]struct{},
) ([]SourceSnippet, bool, int, error) {
	result := make([]SourceSnippet, 0, len(sources))
	resolved := false
	insertionIndex := -1
	for _, snippet := range sources {
		if !overviewSourceSnippetCoversTarget(snippet, target) {
			result = append(result, snippet)
			continue
		}
		_, matches := verified[snippet.PresentationSHA256]
		if !matches {
			var err error
			matches, err = authorizedOverviewSourceSnippetMatches(ctx, data, content, snippet)
			if err != nil {
				return nil, false, -1, err
			}
			if matches {
				verified[snippet.PresentationSHA256] = struct{}{}
			}
		}
		if !matches {
			if insertionIndex < 0 {
				insertionIndex = len(result)
			}
			continue
		}
		resolved = true
		result = append(result, snippet)
	}
	return result, resolved, insertionIndex, nil
}

func authorizedOverviewSourceSnippetMatches(
	ctx context.Context,
	data *ReportData,
	content *workspacecontent.Service,
	snippet SourceSnippet,
) (bool, error) {
	if err := snippet.Validate(); err != nil || snippet.Revision != reportSourceRevision(data) {
		return false, nil
	}
	if _, found := secretscan.DetectAlways(snippet.Content); found {
		return false, fmt.Errorf("report source coverage: unsafe exact source excerpt")
	}
	lines := snippet.Lines
	if len(snippet.FullFunctionLines) > 0 {
		lines = snippet.FullFunctionLines
	}
	for start := 0; start < len(lines); {
		end := start + 1
		for end < len(lines) && end-start < maxInlineSourceLines &&
			lines[end].Line == lines[end-1].Line+1 {
			end++
		}
		result, err := content.Read(ctx, workspacecontent.Request{
			Path: snippet.Path,
			Range: workspacecontent.Range{
				StartLine: lines[start].Line,
				EndLine:   lines[end-1].Line,
				FocusLine: lines[start].Line,
			},
		})
		if err != nil {
			return false, fmt.Errorf("report source coverage: exact authorized read failed: %w", err)
		}
		authorized := make(map[int]string, len(result.Lines))
		for _, line := range result.Lines {
			if line.Truncated {
				return false, fmt.Errorf("report source coverage: truncated exact source line")
			}
			authorized[line.Number] = line.Text
		}
		for _, line := range lines[start:end] {
			if text, ok := authorized[line.Line]; !ok || text != line.Text {
				return false, nil
			}
		}
		start = end
	}
	return true, nil
}

func overviewSourceSnippetCoversTarget(snippet SourceSnippet, target overviewSourceTarget) bool {
	return snippet.Path == target.path &&
		sourceSnippetContainsRange(snippet.Lines, SourceHighlight{
			StartLine: target.line,
			EndLine:   target.line,
		})
}

func insertExactOverviewSourceSnippet(
	sources []SourceSnippet,
	snippet SourceSnippet,
	index int,
) []SourceSnippet {
	if index < 0 || index > len(sources) {
		return mergeExactOverviewSourceSnippet(sources, snippet)
	}
	sources = append(sources, SourceSnippet{})
	copy(sources[index+1:], sources[index:])
	sources[index] = snippet
	return sources
}

func cloneOverviewSourceSnippets(sources []SourceSnippet) []SourceSnippet {
	if sources == nil {
		return nil
	}
	cloned := make([]SourceSnippet, len(sources))
	for index, source := range sources {
		cloned[index] = source
		cloned[index].HighlightRanges = cloneOverviewSourceSlice(source.HighlightRanges)
		cloned[index].Lines = cloneOverviewSourceSlice(source.Lines)
		cloned[index].FullFunctionLines = cloneOverviewSourceSlice(source.FullFunctionLines)
		cloned[index].RelatedEvidenceIDs = cloneOverviewSourceSlice(source.RelatedEvidenceIDs)
		cloned[index].noticeCandidates = cloneOverviewSourceSlice(source.noticeCandidates)
	}
	return cloned
}

func cloneOverviewSourceSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	cloned := make([]T, len(values))
	copy(cloned, values)
	return cloned
}

func overviewSourceTargets(data *ReportData) []overviewSourceTarget {
	if data == nil {
		return nil
	}
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, sourcePath := range data.OpenablePaths {
		openable[sourcePath] = struct{}{}
	}
	result := make([]overviewSourceTarget, 0)
	seen := make(map[overviewSourceTarget]struct{})
	appendTarget := func(sourcePath string, line int) {
		target := overviewSourceTarget{path: sourcePath, line: line}
		if sourcePath == "" || line <= 0 {
			return
		}
		if _, ok := openable[sourcePath]; !ok {
			return
		}
		if _, duplicate := seen[target]; duplicate {
			return
		}
		seen[target] = struct{}{}
		result = append(result, target)
	}

	if data.DiscoveredSurfaces != nil {
		idCounts := make(map[string]int)
		for index := range data.DiscoveredSurfaces.Triggers {
			trigger := &data.DiscoveredSurfaces.Triggers[index]
			if overviewEntrySurfaceEligible(trigger) {
				idCounts[trigger.ID]++
			}
		}
		for index := range data.DiscoveredSurfaces.Triggers {
			trigger := &data.DiscoveredSurfaces.Triggers[index]
			if !overviewEntrySurfaceEligible(trigger) || idCounts[trigger.ID] != 1 {
				continue
			}
			location := overviewEntrySurfaceLocation(trigger)
			if location != nil {
				appendTarget(location.Path, location.Line)
			}
		}
	}

	if data.RepositoryAtlas != nil && data.Navigator != nil &&
		data.Navigator.Recommendation != nil {
		evidenceByID := make(map[string]repositoryatlas.Evidence, len(data.RepositoryAtlas.Evidence))
		for _, item := range data.RepositoryAtlas.Evidence {
			evidenceByID[item.ID] = item
		}
		for _, evidenceID := range data.Navigator.Recommendation.EvidenceIDs {
			item, ok := evidenceByID[evidenceID]
			if !ok {
				continue
			}
			appendTarget(item.Location.Path, item.Location.Line)
		}
	}

	if data.ArchitectureCanvas == nil {
		return result
	}
	componentIDCounts := make(map[string]int)
	for _, component := range data.ArchitectureCanvas.Components {
		if component.ID != "" {
			componentIDCounts[string(component.ID)]++
		}
	}
	for _, component := range data.ArchitectureCanvas.Components {
		if component.ID == "" || componentIDCounts[string(component.ID)] != 1 {
			continue
		}
		for _, location := range overviewArchitectureComponentLocations(data, component, openable) {
			appendTarget(location.Path, location.Line)
		}
	}
	return result
}

func overviewEntrySurfaceEligible(trigger *DiscoveredTrigger) bool {
	return trigger != nil && trigger.ID != "" && !trigger.ProvisionalID &&
		trigger.SurfaceRole == SurfaceRoleEntrySurface &&
		trigger.ApplicationClass == SurfaceApplicationOwned &&
		trigger.Availability == SurfaceAvailabilityAvailable &&
		trigger.ExecutableRole != ExecutableRoleTestOrHelper
}

func overviewEntrySurfaceLocation(trigger *DiscoveredTrigger) *SurfaceLocation {
	if trigger == nil {
		return nil
	}
	for _, location := range []*SurfaceLocation{
		trigger.HandlerLocation,
		trigger.RegistrationSite,
		trigger.DescriptorSite,
		trigger.ServerStartSite,
		trigger.ProcessEntrypoint.Location,
	} {
		if location != nil {
			return location
		}
	}
	return nil
}

func overviewArchitectureComponentLocations(
	data *ReportData,
	component ArchitectureComponent,
	openable map[string]struct{},
) []SurfaceLocation {
	locations := make([]SurfaceLocation, 0)
	seen := make(map[overviewSourceTarget]struct{})
	appendLocation := func(sourcePath string, line, column int) {
		key := overviewSourceTarget{path: sourcePath, line: line}
		if sourcePath == "" || line <= 0 {
			return
		}
		if _, ok := openable[sourcePath]; !ok {
			return
		}
		if _, duplicate := seen[key]; duplicate {
			return
		}
		seen[key] = struct{}{}
		locations = append(locations, SurfaceLocation{Path: sourcePath, Line: line, Column: column})
	}

	componentFiles := make(map[string]struct{})
	packageByPath := make(map[string]PackageInfo)
	if data.RepositoryGraph != nil {
		for _, pkg := range data.RepositoryGraph.Packages {
			if pkg.CanonicalPath != "" {
				packageByPath[pkg.CanonicalPath] = pkg
			}
		}
	}
	for _, member := range component.Members {
		if member.ID.Kind == "package" {
			for _, fact := range member.Facts {
				if fact.Kind != "declaration" {
					continue
				}
				pkg, ok := packageByPath[fact.Value]
				if !ok {
					continue
				}
				for _, sourcePath := range pkg.Files {
					if _, ok := openable[sourcePath]; ok {
						componentFiles[sourcePath] = struct{}{}
					}
				}
				break
			}
		}
		for _, fact := range member.Facts {
			if fact.Location == nil {
				continue
			}
			if _, ok := openable[fact.Location.Path]; !ok {
				continue
			}
			componentFiles[fact.Location.Path] = struct{}{}
			appendLocation(fact.Location.Path, fact.Location.Line, fact.Location.Column)
		}
	}
	hasPreciseMemberSource := len(locations) > 0
	hasExactComponentFiles := len(componentFiles) > 0
	if !hasPreciseMemberSource {
		anchorByID := make(map[string]componentmap.BehaviorAnchor)
		for _, anchor := range data.ArchitectureCanvas.BehaviorAnchors {
			if anchor.ID != "" {
				anchorByID[anchor.ID] = anchor
			}
		}
		for _, anchorID := range component.AnchorIDs {
			anchor, ok := anchorByID[anchorID]
			if !ok || anchor.Location.Line <= 0 {
				continue
			}
			if _, ok := openable[anchor.Location.Path]; !ok {
				continue
			}
			if hasExactComponentFiles {
				if _, ok := componentFiles[anchor.Location.Path]; !ok {
					continue
				}
			}
			appendLocation(anchor.Location.Path, anchor.Location.Line, anchor.Location.Column)
		}
	}
	sort.SliceStable(locations, func(i, j int) bool {
		if locations[i].Path != locations[j].Path {
			return locations[i].Path < locations[j].Path
		}
		return locations[i].Line < locations[j].Line
	})
	return locations
}

func authorizedOverviewSourceSnippet(
	ctx context.Context,
	data *ReportData,
	content *workspacecontent.Service,
	target overviewSourceTarget,
) (SourceSnippet, error) {
	result, err := content.Read(ctx, workspacecontent.Request{
		Path: target.path,
		Range: workspacecontent.Range{
			StartLine: target.line,
			EndLine:   target.line,
			FocusLine: target.line,
		},
	})
	if err != nil {
		return SourceSnippet{}, fmt.Errorf("report source coverage: exact authorized read failed: %w", err)
	}
	lines := make([]savedSourceLine, 0, len(result.Lines))
	texts := make([]string, 0, len(result.Lines))
	for _, line := range result.Lines {
		if line.Truncated {
			return SourceSnippet{}, fmt.Errorf("report source coverage: truncated exact source line")
		}
		lines = append(lines, savedSourceLine{line: line.Number, text: line.Text})
		texts = append(texts, line.Text)
	}
	group := sourceSnippetGroup{
		candidate: savedSourceCandidate{
			path: target.path, lines: lines,
			contentSHA: sourceLinesSHA256(texts),
		},
		evidence: []semanticdiscovery.EvidenceRef{{Path: target.path, Line: target.line}},
	}
	snippet, ok := sourceSnippetFromGroup(data, group, overviewSourceRole(data, target.path))
	if !ok {
		return SourceSnippet{}, fmt.Errorf("report source coverage: cannot project exact source excerpt")
	}
	if _, found := secretscan.DetectAlways(snippet.Content); found {
		return SourceSnippet{}, fmt.Errorf("report source coverage: unsafe exact source excerpt")
	}
	return snippet, nil
}

func mergeExactOverviewSourceSnippet(sources []SourceSnippet, snippet SourceSnippet) []SourceSnippet {
	for index := range sources {
		current := &sources[index]
		if current.Path != snippet.Path || current.StartLine != snippet.StartLine ||
			current.EndLine != snippet.EndLine || current.Revision != snippet.Revision ||
			current.ContentSHA256 != snippet.ContentSHA256 || current.Content != snippet.Content {
			continue
		}
		merged := append([]SourceHighlight(nil), current.HighlightRanges...)
		merged = append(merged, snippet.HighlightRanges...)
		current.HighlightRanges = normalizeSourceHighlightRanges(merged)
		for lineIndex := range current.Lines {
			current.Lines[lineIndex].Highlight = sourceLineIsHighlighted(
				current.Lines[lineIndex].Line,
				current.HighlightRanges,
			)
		}
		current.PresentationSHA256 = sourceSnippetPresentationSHA(*current)
		return sources
	}
	return append(sources, snippet)
}

func normalizeSourceHighlightRanges(values []SourceHighlight) []SourceHighlight {
	lines := make([]int, 0)
	seen := make(map[int]struct{})
	for _, value := range values {
		for line := value.StartLine; line <= value.EndLine; line++ {
			if _, duplicate := seen[line]; duplicate {
				continue
			}
			seen[line] = struct{}{}
			lines = append(lines, line)
		}
	}
	sort.Ints(lines)
	return sourceHighlightRanges(lines)
}

func resolveOverviewSourceSnippet(sources []SourceSnippet, target overviewSourceTarget) (bool, bool) {
	type match struct {
		snippet SourceSnippet
		span    int
	}
	matches := make([]match, 0)
	seen := make(map[string]struct{})
	for _, snippet := range sources {
		if !overviewSourceSnippetCoversTarget(snippet, target) {
			continue
		}
		identity := fmt.Sprintf("%s\x00%d\x00%d\x00%s\x00%s",
			snippet.Path, snippet.StartLine, snippet.EndLine, snippet.Revision, snippet.ContentSHA256)
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		matches = append(matches, match{snippet: snippet, span: snippet.EndLine - snippet.StartLine})
	}
	if len(matches) == 0 {
		return false, false
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].span != matches[j].span {
			return matches[i].span < matches[j].span
		}
		if matches[i].snippet.StartLine != matches[j].snippet.StartLine {
			return matches[i].snippet.StartLine < matches[j].snippet.StartLine
		}
		return matches[i].snippet.PresentationSHA256 < matches[j].snippet.PresentationSHA256
	})
	minimumSpan := matches[0].span
	intervalIdentity := make(map[string]string)
	for _, candidate := range matches {
		if candidate.span != minimumSpan {
			break
		}
		interval := fmt.Sprintf("%s\x00%d\x00%d", candidate.snippet.Path,
			candidate.snippet.StartLine, candidate.snippet.EndLine)
		identity := candidate.snippet.Revision + "\x00" + candidate.snippet.ContentSHA256
		if previous := intervalIdentity[interval]; previous != "" && previous != identity {
			return false, true
		}
		intervalIdentity[interval] = identity
	}
	return true, false
}

func overviewSourceRecommendations(files []FileItem) map[string]overviewSourceRecommendation {
	result := make(map[string]overviewSourceRecommendation, len(files))
	for index, file := range files {
		sourcePath := strings.TrimSpace(file.Path)
		if sourcePath == "" {
			continue
		}
		if _, exists := result[sourcePath]; exists {
			continue
		}
		result[sourcePath] = overviewSourceRecommendation{
			rank: index, reason: boundedSourceLandmarkReason(file.Reason, "Recommended starting point."),
		}
	}
	return result
}

func overviewSourceFindings(
	findings []modelresearch.ValidatedFinding,
) map[string]overviewSourceFinding {
	result := make(map[string]overviewSourceFinding)
	for index, finding := range findings {
		reason := boundedSourceLandmarkReason(finding.ResponsibilityName, "Relevant implementation.")
		for _, evidenceID := range finding.EvidenceIDs {
			if evidenceID == "" {
				continue
			}
			if _, exists := result[evidenceID]; exists {
				continue
			}
			result[evidenceID] = overviewSourceFinding{rank: index, reason: reason}
		}
	}
	return result
}

func overviewSourceLandmarkMetadata(
	sourcePath string,
	role string,
	declaration overviewSourceDeclaration,
	recommendation overviewSourceRecommendation,
	recommended bool,
	finding overviewSourceFinding,
	findingOK bool,
) (SourceLandmarkKind, string) {
	if declaration.kind == SourceLandmarkCLIEntrypoint && path.Base(sourcePath) == "main.go" {
		return SourceLandmarkCLIEntrypoint, "Command-line entrypoint."
	}
	if role == "test" {
		return SourceLandmarkTest, "Saved test implementation."
	}
	if role == "example" {
		return SourceLandmarkQuickstart, "Quickstart or example implementation."
	}
	if declaration.kind == SourceLandmarkPublicAPI {
		return SourceLandmarkPublicAPI, boundedSourceLandmarkReason(
			"Exported API: "+declaration.symbol+".",
			"Exported public API.",
		)
	}
	if recommended {
		return SourceLandmarkOrientation, recommendation.reason
	}
	if declaration.kind == SourceLandmarkConstructor {
		return SourceLandmarkConstructor, boundedSourceLandmarkReason(
			"Constructor: "+declaration.symbol+".",
			"Constructor implementation.",
		)
	}
	if declaration.kind == SourceLandmarkHandler {
		return SourceLandmarkHandler, boundedSourceLandmarkReason(
			"Handler entry: "+declaration.symbol+".",
			"Handler implementation.",
		)
	}
	if findingOK {
		return SourceLandmarkCore, finding.reason
	}
	if declaration.symbol != "" {
		return SourceLandmarkCore, boundedSourceLandmarkReason(
			"Core implementation: "+declaration.symbol+".",
			"Core implementation excerpt.",
		)
	}
	return SourceLandmarkCore, "Core implementation excerpt."
}

func sourceLandmarkRank(kind SourceLandmarkKind) int {
	switch kind {
	case SourceLandmarkCLIEntrypoint:
		return 0
	case SourceLandmarkPublicAPI:
		return 1
	case SourceLandmarkQuickstart:
		return 2
	case SourceLandmarkOrientation:
		return 3
	case SourceLandmarkConstructor, SourceLandmarkHandler:
		return 4
	case SourceLandmarkTest:
		return 5
	default:
		return 6
	}
}

func overviewSourceCandidateLess(left, right overviewSourceCandidate) bool {
	if left.landmarkRank != right.landmarkRank {
		return left.landmarkRank < right.landmarkRank
	}
	if left.recommended != right.recommended {
		return left.recommended
	}
	if left.recommended && left.recommendationRank != right.recommendationRank {
		return left.recommendationRank < right.recommendationRank
	}
	if left.finding != right.finding {
		return left.finding
	}
	if left.finding && left.findingRank != right.findingRank {
		return left.findingRank < right.findingRank
	}
	if overviewRoleOrder(left.snippet.Role) != overviewRoleOrder(right.snippet.Role) {
		return overviewRoleOrder(left.snippet.Role) < overviewRoleOrder(right.snippet.Role)
	}
	if left.declarationRank != right.declarationRank {
		return left.declarationRank < right.declarationRank
	}
	if left.snippet.Path != right.snippet.Path {
		return left.snippet.Path < right.snippet.Path
	}
	if left.focusLine != right.focusLine {
		return left.focusLine < right.focusLine
	}
	return left.evidenceID < right.evidenceID
}

func sortedSourceEvidenceIDs(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func selectOverviewSourceDeclaration(
	sourcePath string,
	lines []savedSourceLine,
	itemSymbol string,
	findingResponsibility string,
	fallbackLine int,
) overviewSourceDeclaration {
	declarations := make([]overviewSourceDeclaration, 0)
	for _, line := range lines {
		if symbol, rank, kind, ok := boundedSourceDeclaration(sourcePath, line.text); ok {
			declarations = append(declarations, overviewSourceDeclaration{
				line: line.line, symbol: symbol, rank: rank, kind: kind,
			})
		}
	}
	for _, preferred := range []string{itemSymbol, findingResponsibility} {
		for _, declaration := range declarations {
			if sourceSymbolMatches(declaration.symbol, preferred) {
				return declaration
			}
		}
	}
	if len(declarations) > 0 {
		sort.SliceStable(declarations, func(i, j int) bool {
			if declarations[i].rank != declarations[j].rank {
				return declarations[i].rank < declarations[j].rank
			}
			return declarations[i].line < declarations[j].line
		})
		return declarations[0]
	}
	if fallbackLine > 0 {
		for _, line := range lines {
			if line.line == fallbackLine && meaningfulSourceLine(line.text) {
				return overviewSourceDeclaration{
					line: line.line, symbol: strings.TrimSpace(itemSymbol),
					rank: sourceLandmarkRank(SourceLandmarkCore)*10 + 2,
					kind: SourceLandmarkCore,
				}
			}
		}
	}
	for _, line := range lines {
		if meaningfulSourceLine(line.text) {
			return overviewSourceDeclaration{
				line: line.line, symbol: strings.TrimSpace(itemSymbol),
				rank: sourceLandmarkRank(SourceLandmarkCore)*10 + 2,
				kind: SourceLandmarkCore,
			}
		}
	}
	return overviewSourceDeclaration{}
}

func boundedSourceDeclaration(
	sourcePath string,
	text string,
) (string, int, SourceLandmarkKind, bool) {
	trimmed := strings.TrimSpace(text)
	switch sourceLanguage(sourcePath) {
	case "go":
		if strings.HasPrefix(trimmed, "func ") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "func "))
			if strings.HasPrefix(name, "(") {
				closing := strings.Index(name, ")")
				if closing < 0 {
					return "", 0, "", false
				}
				name = strings.TrimSpace(name[closing+1:])
			}
			if identifier := sourceIdentifierPrefix(name); identifier != "" {
				kind := sourceDeclarationLandmarkKind(sourcePath, identifier, false)
				return identifier, sourceLandmarkRank(kind) * 10, kind, true
			}
		}
		if strings.HasPrefix(trimmed, "type ") {
			if identifier := sourceIdentifierPrefix(strings.TrimSpace(strings.TrimPrefix(trimmed, "type "))); identifier != "" {
				kind := sourceDeclarationLandmarkKind(sourcePath, identifier, true)
				return identifier, sourceLandmarkRank(kind)*10 + 1, kind, true
			}
		}
	case "python":
		name := trimmed
		if strings.HasPrefix(name, "async def ") {
			name = strings.TrimSpace(strings.TrimPrefix(name, "async def "))
		} else if strings.HasPrefix(name, "def ") {
			name = strings.TrimSpace(strings.TrimPrefix(name, "def "))
		} else {
			name = ""
		}
		if identifier := sourceIdentifierPrefix(name); identifier != "" {
			kind := sourceDeclarationLandmarkKind(sourcePath, identifier, false)
			return identifier, sourceLandmarkRank(kind) * 10, kind, true
		}
	case "javascript", "typescript":
		if strings.HasPrefix(trimmed, "function ") {
			if identifier := sourceIdentifierPrefix(strings.TrimSpace(strings.TrimPrefix(trimmed, "function "))); identifier != "" {
				kind := sourceDeclarationLandmarkKind(sourcePath, identifier, false)
				return identifier, sourceLandmarkRank(kind) * 10, kind, true
			}
		}
	case "rust":
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "pub "))
		if strings.HasPrefix(name, "async ") {
			name = strings.TrimSpace(strings.TrimPrefix(name, "async "))
		}
		if strings.HasPrefix(name, "fn ") {
			if identifier := sourceIdentifierPrefix(strings.TrimSpace(strings.TrimPrefix(name, "fn "))); identifier != "" {
				kind := sourceDeclarationLandmarkKind(sourcePath, identifier, false)
				return identifier, sourceLandmarkRank(kind) * 10, kind, true
			}
		}
	}
	return "", 0, "", false
}

func sourceDeclarationLandmarkKind(
	sourcePath string,
	symbol string,
	typeDeclaration bool,
) SourceLandmarkKind {
	lower := strings.ToLower(symbol)
	normalizedPath := strings.ToLower(filepath.ToSlash(sourcePath))
	isExampleOrTest := strings.Contains(normalizedPath, "/example") ||
		strings.HasPrefix(normalizedPath, "example") || strings.HasSuffix(normalizedPath, "_test.go") ||
		strings.Contains(normalizedPath, "/test/") || strings.Contains(normalizedPath, "/tests/")
	if !typeDeclaration && !isExampleOrTest && symbol == "main" && path.Base(sourcePath) == "main.go" {
		return SourceLandmarkCLIEntrypoint
	}
	if !typeDeclaration && strings.HasPrefix(symbol, "New") && len(symbol) > len("New") {
		return SourceLandmarkConstructor
	}
	if !typeDeclaration && len(symbol) > 0 && symbol[0] >= 'A' && symbol[0] <= 'Z' {
		return SourceLandmarkPublicAPI
	}
	if !typeDeclaration && (strings.Contains(lower, "handler") || strings.HasPrefix(lower, "handle") ||
		lower == "servehttp" || strings.Contains(lower, "dispatch")) {
		return SourceLandmarkHandler
	}
	return SourceLandmarkCore
}

func sourceIdentifierPrefix(value string) string {
	value = strings.TrimSpace(value)
	end := 0
	for end < len(value) && sourceIdentifierByte(value[end], end == 0) {
		end++
	}
	return value[:end]
}

func sourceIdentifierByte(value byte, first bool) bool {
	if value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' {
		return true
	}
	return !first && value >= '0' && value <= '9'
}

func sourceSymbolMatches(symbol, preferred string) bool {
	symbol = strings.TrimSpace(symbol)
	preferred = strings.TrimSpace(preferred)
	if symbol == "" || preferred == "" {
		return false
	}
	return symbol == preferred || strings.HasSuffix(preferred, "."+symbol)
}

func meaningfulSourceLine(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "(" || value == ")" || value == "{" || value == "}" ||
		strings.HasPrefix(value, "//") || strings.HasPrefix(value, "/*") || strings.HasPrefix(value, "*") ||
		strings.HasPrefix(value, "package ") || strings.HasPrefix(value, "import ") ||
		strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "`") {
		return false
	}
	return true
}

func boundedSourceLandmarkReason(value, fallback string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		value = fallback
	}
	if len(value) <= maxSourceLandmarkReasonBytes {
		return value
	}
	const ellipsis = "…"
	var result strings.Builder
	for _, current := range value {
		if result.Len()+len(string(current)) > maxSourceLandmarkReasonBytes-len(ellipsis) {
			break
		}
		result.WriteRune(current)
	}
	return strings.TrimSpace(result.String()) + ellipsis
}

func overviewSourceRole(data *ReportData, sourcePath string) string {
	lower := strings.ToLower(filepath.ToSlash(sourcePath))
	if strings.HasSuffix(lower, "_test.go") || strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") {
		return "test"
	}
	if strings.Contains(lower, "/example") || strings.HasPrefix(lower, "example") || strings.Contains(lower, "/examples/") {
		return "example"
	}
	for _, item := range data.FirstFilesToOpen {
		if item.Path == sourcePath {
			return "entrypoint"
		}
	}
	return "core"
}

func overviewRoleOrder(role string) int {
	switch role {
	case "entrypoint":
		return 0
	case "core":
		return 1
	case "example", "test":
		return 2
	default:
		return 3
	}
}

func narrowestContainingCandidate(candidates []savedSourceCandidate, sourcePath string, line int) int {
	best := -1
	bestSpan := int(^uint(0) >> 1)
	for index, candidate := range candidates {
		if candidate.path != sourcePath || len(candidate.lines) == 0 ||
			line < candidate.lines[0].line || line > candidate.lines[len(candidate.lines)-1].line {
			continue
		}
		span := candidate.lines[len(candidate.lines)-1].line - candidate.lines[0].line
		if span < bestSpan {
			best = index
			bestSpan = span
		}
	}
	return best
}

func sourceHighlightRanges(lines []int) []SourceHighlight {
	if len(lines) == 0 {
		return nil
	}
	lines = append([]int(nil), lines...)
	sort.Ints(lines)
	result := make([]SourceHighlight, 0, len(lines))
	for _, line := range lines {
		if len(result) == 0 || line > result[len(result)-1].EndLine+1 {
			result = append(result, SourceHighlight{StartLine: line, EndLine: line})
			continue
		}
		if line > result[len(result)-1].EndLine {
			result[len(result)-1].EndLine = line
		}
	}
	return result
}

func sourceLinesSHA256(lines []string) string {
	raw, _ := json.Marshal(lines)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func sourceSnippetPresentationSHA(snippet SourceSnippet) string {
	snippet.PresentationSHA256 = ""
	snippet.PresentationLandmarkReason = ""
	raw, _ := json.Marshal(snippet)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func validSourceSnippetSHA(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sourceCandidateIdentity(sourcePath, symbol string) string {
	return sourcePath + "\x00" + symbol
}

func savedSourceKey(candidate savedSourceCandidate) string {
	if len(candidate.lines) == 0 {
		return sourceCandidateIdentity(candidate.path, candidate.symbol)
	}
	return sourceCandidateIdentity(candidate.path, candidate.symbol) + "\x00" +
		candidate.contentSHA
}

func continuousSavedSource(lines []savedSourceLine) bool {
	if len(lines) == 0 {
		return false
	}
	for index := 1; index < len(lines); index++ {
		if lines[index].line != lines[index-1].line+1 {
			return false
		}
	}
	return true
}

func sourcePathPenalty(sourcePath string) int {
	lower := strings.ToLower(filepath.ToSlash(sourcePath))
	if strings.HasSuffix(lower, "_test.go") || strings.Contains(lower, "/testdata/") {
		return 2
	}
	if strings.Contains(lower, "/generated/") || strings.HasSuffix(lower, ".gen.go") ||
		strings.Contains(lower, "/examples/") {
		return 1
	}
	return 0
}

func sourceLanguage(sourcePath string) string {
	switch strings.ToLower(filepath.Ext(sourcePath)) {
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	default:
		return "text"
	}
}

func reportSourceRevision(data *ReportData) string {
	if data == nil {
		return ""
	}
	if strings.TrimSpace(data.CapturedRevision) != "" {
		return strings.TrimSpace(data.CapturedRevision)
	}
	if data.ModelResearch != nil {
		return strings.TrimSpace(data.ModelResearch.Repository.Revision)
	}
	return ""
}
