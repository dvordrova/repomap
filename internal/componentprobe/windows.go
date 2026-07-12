package componentprobe

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/reporead"
	"github.com/dvordrova/repomap/internal/symbol"
)

type callsiteCandidate struct {
	probeID   string
	direction Direction
	call      symbol.CallFact
}

func collectCallsiteWindows(
	reader *reporead.Reader,
	probes []SymbolProbe,
	opts Options,
) ([]CallsiteWindow, []Warning) {
	candidates := callsiteCandidates(probes)
	candidates = prioritizeCallsiteCandidates(candidates, probes)
	warnings := []Warning{}
	if len(candidates) > opts.MaxCallsiteWindows {
		warnings = append(warnings, Warning{
			Code:    "callsites.truncated",
			Message: fmt.Sprintf("kept %d of %d static callsites", opts.MaxCallsiteWindows, len(candidates)),
		})
		candidates = candidates[:opts.MaxCallsiteWindows]
	}

	windows := make([]CallsiteWindow, 0, len(candidates))
	remainingBytes := opts.MaxCallsiteBytes
	for _, candidate := range candidates {
		if remainingBytes <= 0 {
			warnings = append(warnings, Warning{
				Code:    "callsites.byte_budget",
				Message: "stopped callsite source collection at the total byte bound",
			})
			break
		}
		windowLimit := min(opts.MaxCallsiteWindowBytes, remainingBytes)
		window, err := readCallsiteWindow(reader, candidate, opts, windowLimit)
		if err != nil {
			warnings = append(warnings, Warning{
				Code:     "callsite.window_failed",
				SymbolID: candidate.probeID,
				Message:  err.Error(),
			})
			continue
		}
		remainingBytes -= windowBytes(window)
		windows = append(windows, window)
	}
	return windows, warnings
}

func callsiteCandidates(probes []SymbolProbe) []callsiteCandidate {
	result := make([]callsiteCandidate, 0)
	coverage := probeSourceCoverage(probes)
	for _, probe := range probes {
		for _, call := range probe.Structural.IncomingCalls {
			if call.Callsite != nil && !coverage.contains(*call.Callsite) {
				result = append(result, callsiteCandidate{probeID: probe.ID, direction: DirectionIncoming, call: call})
			}
		}
		for _, call := range probe.Structural.OutgoingCalls {
			if call.Callsite != nil && !coverage.contains(*call.Callsite) {
				result = append(result, callsiteCandidate{probeID: probe.ID, direction: DirectionOutgoing, call: call})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i]
		right := result[j]
		if callsitePathTier(left) != callsitePathTier(right) {
			return callsitePathTier(left) < callsitePathTier(right)
		}
		if left.call.Callsite.Path != right.call.Callsite.Path {
			return left.call.Callsite.Path < right.call.Callsite.Path
		}
		if left.call.Callsite.Line != right.call.Callsite.Line {
			return left.call.Callsite.Line < right.call.Callsite.Line
		}
		if left.call.Callsite.Column != right.call.Callsite.Column {
			return left.call.Callsite.Column < right.call.Callsite.Column
		}
		if left.direction != right.direction {
			return left.direction < right.direction
		}
		if left.call.Caller.Name != right.call.Caller.Name {
			return left.call.Caller.Name < right.call.Caller.Name
		}
		if left.call.Callee.Name != right.call.Callee.Name {
			return left.call.Callee.Name < right.call.Callee.Name
		}
		if left.probeID != right.probeID {
			return left.probeID < right.probeID
		}
		return left.call.EvidenceID < right.call.EvidenceID
	})
	return result
}

func callsitePathTier(candidate callsiteCandidate) int {
	if strings.HasSuffix(strings.ToLower(candidate.call.Callsite.Path), "_test.go") {
		return 1
	}
	return 0
}

type lineRange struct {
	start int
	end   int
}

type sourceCoverage map[string][]lineRange

func probeSourceCoverage(probes []SymbolProbe) sourceCoverage {
	coverage := make(sourceCoverage)
	for _, probe := range probes {
		coverage[probe.Source.Target.Path] = append(coverage[probe.Source.Target.Path], lineRange{
			start: probe.Source.Window.StartLine,
			end:   probe.Source.Window.EndLine,
		})
	}
	return coverage
}

func (coverage sourceCoverage) contains(location evidence.Location) bool {
	for _, lines := range coverage[location.Path] {
		if location.Line >= lines.start && location.Line <= lines.end {
			return true
		}
	}
	return false
}

func prioritizeCallsiteCandidates(candidates []callsiteCandidate, probes []SymbolProbe) []callsiteCandidate {
	probeIDs := make([]string, 0, len(probes))
	for _, probe := range probes {
		probeIDs = append(probeIDs, probe.ID)
	}
	sort.Strings(probeIDs)
	result := make([]callsiteCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	key := func(candidate callsiteCandidate) string {
		return candidate.probeID + "\x00" + string(candidate.direction) + "\x00" + candidate.call.EvidenceID
	}
	addFirst := func(probeID string, direction Direction) {
		for _, candidate := range candidates {
			if candidate.probeID != probeID || candidate.direction != direction {
				continue
			}
			candidateKey := key(candidate)
			if _, exists := seen[candidateKey]; exists {
				continue
			}
			result = append(result, candidate)
			seen[candidateKey] = struct{}{}
			return
		}
	}
	for _, probeID := range probeIDs {
		addFirst(probeID, DirectionOutgoing)
	}
	for _, probeID := range probeIDs {
		addFirst(probeID, DirectionIncoming)
	}
	for _, candidate := range candidates {
		candidateKey := key(candidate)
		if _, exists := seen[candidateKey]; exists {
			continue
		}
		result = append(result, candidate)
		seen[candidateKey] = struct{}{}
	}
	return result
}

func readCallsiteWindow(
	reader *reporead.Reader,
	candidate callsiteCandidate,
	opts Options,
	maxBytes int,
) (CallsiteWindow, error) {
	callsite := *candidate.call.Callsite
	content, err := reader.ReadFile(callsite.Path, hardMaxCallsiteFileBytes)
	if err != nil {
		return CallsiteWindow{}, fmt.Errorf("read %s:%d: %w", callsite.Path, callsite.Line, err)
	}
	fileLines := splitSourceLines(content.Bytes)
	if callsite.Line <= 0 || callsite.Line > len(fileLines) {
		return CallsiteWindow{}, fmt.Errorf("callsite %s:%d is outside the bounded file prefix", callsite.Path, callsite.Line)
	}
	start := max(1, callsite.Line-opts.CallsiteLinesBefore)
	end := min(len(fileLines), callsite.Line+opts.CallsiteLinesAfter)
	lines := make([]SourceLine, 0, end-start+1)
	for lineNumber := start; lineNumber <= end; lineNumber++ {
		text, truncated := truncateSourceLine(fileLines[lineNumber-1], 1024)
		lines = append(lines, SourceLine{
			EvidenceID: stableID("ev", candidate.probeID, candidate.call.EvidenceID, callsite.Path, fmt.Sprint(lineNumber)),
			Line:       lineNumber,
			Text:       text,
		})
		content.Truncated = content.Truncated || truncated
	}
	lines, byteTruncated := fitSourceLines(lines, callsite.Line, maxBytes)
	if len(lines) == 0 {
		return CallsiteWindow{}, fmt.Errorf("callsite %s:%d cannot fit the source byte bound", callsite.Path, callsite.Line)
	}
	origin := EvidenceOrigin{
		ProbeID:  candidate.probeID,
		Artifact: ArtifactStructural,
		LocalID:  candidate.call.EvidenceID,
	}
	return CallsiteWindow{
		EvidenceID: stableID("ev", "callsite-window", candidate.probeID, candidate.call.EvidenceID, callsite.Path, fmt.Sprint(callsite.Line)),
		Direction:  candidate.direction,
		Caller:     candidate.call.Caller,
		Callee:     candidate.call.Callee,
		Callsite:   callsite,
		Certainty:  candidate.call.Certainty,
		Provenance: cloneProvenance(candidate.call.Provenance),
		Origin:     origin,
		Basis:      SupportSource,
		StartLine:  lines[0].Line,
		EndLine:    lines[len(lines)-1].Line,
		Lines:      lines,
		Truncated:  content.Truncated || byteTruncated || start > max(1, callsite.Line-opts.CallsiteLinesBefore) || end < min(len(fileLines), callsite.Line+opts.CallsiteLinesAfter),
	}, nil
}

func splitSourceLines(data []byte) []string {
	data = bytes.TrimSuffix(data, []byte("\n"))
	if len(data) == 0 {
		return nil
	}
	parts := bytes.Split(data, []byte("\n"))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = bytes.TrimSuffix(part, []byte("\r"))
		result = append(result, string(part))
	}
	return result
}

func fitSourceLines(lines []SourceLine, targetLine, maxBytes int) ([]SourceLine, bool) {
	if maxBytes <= 0 {
		return nil, true
	}
	truncated := false
	for windowLinesBytes(lines) > maxBytes && len(lines) > 1 {
		truncated = true
		before := targetLine - lines[0].Line
		after := lines[len(lines)-1].Line - targetLine
		if before >= after && before > 0 {
			lines = lines[1:]
		} else if after > 0 {
			lines = lines[:len(lines)-1]
		} else {
			break
		}
	}
	if len(lines) == 1 && len(lines[0].Text) > maxBytes {
		lines[0].Text, _ = truncateSourceLine(lines[0].Text, maxBytes)
		truncated = true
	}
	if windowLinesBytes(lines) > maxBytes {
		return nil, true
	}
	return lines, truncated
}

func truncateSourceLine(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	text = text[:limit]
	for len(text) > 0 && !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text, true
}

func windowLinesBytes(lines []SourceLine) int {
	total := 0
	for index, line := range lines {
		if index > 0 {
			total++
		}
		total += len(line.Text)
	}
	return total
}

func windowBytes(window CallsiteWindow) int {
	return windowLinesBytes(window.Lines)
}

func cloneProvenance(values []evidence.Provenance) []evidence.Provenance {
	result := make([]evidence.Provenance, len(values))
	copy(result, values)
	for index := range result {
		if result[index].Location != nil {
			location := *result[index].Location
			result[index].Location = &location
		}
	}
	return result
}
