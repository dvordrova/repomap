// Package workspacecontent reads bounded exact text from one immutable
// catalog-authorized repository scope.
package workspacecontent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/reporead"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

const (
	maxPathBytes = 4096
	maxLineValue = 10_000_000

	maxFileBytes int64 = 8 << 20
	maxLines           = 80
	maxTextBytes       = 32 << 10
	maxLineBytes       = 32 << 10
)

// Range identifies one saved one-based inclusive line span and its optional
// focus. A zero or out-of-span focus falls back to StartLine.
type Range struct {
	StartLine int
	EndLine   int
	FocusLine int
}

// Limits may narrow, but never raise, the fixed service maxima. Zero fields
// select the corresponding maximum.
type Limits struct {
	MaxFileBytes int64
	MaxLines     int
	MaxBytes     int
	MaxLineBytes int
}

// Request selects one exact catalog-authorized file and bounded line span.
type Request struct {
	Path   string
	Range  Range
	Limits Limits
}

// Line is one complete physical source line with a one-based number.
type Line struct {
	Number    int
	Text      string
	Truncated bool
}

// StopReason describes why the successful returned range ended.
type StopReason string

const (
	StopRangeComplete StopReason = "range_complete"
	StopEndOfFile     StopReason = "end_of_file"
	StopLineLimit     StopReason = "line_limit"
)

// Result contains only verified, bounded, analysis-root-relative content.
// Text preserves CR bytes before LF so an adapter can apply the same scanning
// policy as the pre-extraction source-context path.
type Result struct {
	Path          string
	ContentSHA256 string
	StartLine     int
	EndLine       int
	FocusLine     int
	TotalLines    int
	Text          string
	Lines         []Line
	Truncated     bool
	StopReason    StopReason
}

// Service is bound to one immutable catalog and one descriptor-bound analysis
// root.
type Service struct {
	catalog sourcecatalog.Catalog
	reader  *reporead.Reader
}

// New binds an authorized content service to one catalog analysis root.
func New(catalog sourcecatalog.Catalog) (*Service, error) {
	if catalog.AnalysisRoot() == "" {
		return nil, workspaceContentError(ErrorInvalidRequest, StageRequest)
	}
	reader, err := reporead.New(catalog.AnalysisRoot())
	if err != nil {
		return nil, workspaceContentError(ErrorUnavailable, StageRead)
	}
	return &Service{catalog: catalog, reader: reader}, nil
}

// Close releases the descriptor-bound analysis root.
func (s *Service) Close() error {
	if s == nil || s.reader == nil {
		return nil
	}
	if err := s.reader.Close(); err != nil {
		return workspaceContentError(ErrorReadFailed, StageRead)
	}
	return nil
}

// Read returns one verified and allocation-bounded exact text range.
func (s *Service) Read(ctx context.Context, request Request) (Result, error) {
	if s == nil || s.reader == nil || s.catalog.AnalysisRoot() == "" {
		return Result{}, workspaceContentError(ErrorInvalidRequest, StageRequest)
	}
	if err := contextFailure(ctx); err != nil {
		return Result{}, err
	}
	limits, err := normalizeLimits(request.Limits)
	if err != nil || !validPath(request.Path) {
		return Result{}, workspaceContentError(ErrorInvalidRequest, StageRequest)
	}
	if !validRange(request.Range) {
		return Result{}, workspaceContentError(ErrorInvalidRequest, StageRange)
	}
	source, ok := s.catalog.Lookup(request.Path)
	if !ok || source.Path != request.Path || source.ContentSHA256 == "" {
		return Result{}, workspaceContentError(ErrorUnauthorized, StageAuthority)
	}

	current, readErr := s.reader.ReadFileNoSymlinks(request.Path, limits.MaxFileBytes)
	if readErr != nil {
		return Result{}, workspaceContentError(ErrorUnavailable, StageRead)
	}
	if current.Truncated {
		return Result{}, workspaceContentLimit(LimitFile)
	}
	if !utf8.Valid(current.Bytes) {
		return Result{}, workspaceContentError(ErrorUnsupportedText, StageRead)
	}
	if err := contextFailure(ctx); err != nil {
		return Result{}, err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(current.Bytes))
	if digest != source.ContentSHA256 {
		return Result{}, workspaceContentError(ErrorSourceChanged, StageRead)
	}

	totalLines := physicalLineCount(current.Bytes)
	startLine, endLine, focusLine, truncated, ok := selectRange(
		request.Range,
		totalLines,
		limits.MaxLines,
	)
	if !ok {
		return Result{}, workspaceContentError(ErrorUnavailable, StageRange)
	}
	lines, text, err := selectLines(
		current.Bytes,
		startLine,
		endLine,
		limits.MaxBytes,
		limits.MaxLineBytes,
		limits.MaxLines,
	)
	if err != nil {
		return Result{}, err
	}
	if err := contextFailure(ctx); err != nil {
		return Result{}, err
	}

	stopReason := StopRangeComplete
	if truncated {
		stopReason = StopLineLimit
	} else if endLine == totalLines {
		stopReason = StopEndOfFile
	}
	return Result{
		Path:          source.Path,
		ContentSHA256: digest,
		StartLine:     startLine,
		EndLine:       endLine,
		FocusLine:     focusLine,
		TotalLines:    totalLines,
		Text:          text,
		Lines:         lines,
		Truncated:     truncated,
		StopReason:    stopReason,
	}, nil
}

func contextFailure(ctx context.Context) error {
	if ctx == nil {
		return workspaceContentError(ErrorInvalidRequest, StageRequest)
	}
	if ctx.Err() != nil {
		return workspaceContentError(ErrorCanceled, StageRead)
	}
	return nil
}

func normalizeLimits(value Limits) (Limits, error) {
	if value.MaxFileBytes == 0 {
		value.MaxFileBytes = maxFileBytes
	}
	if value.MaxLines == 0 {
		value.MaxLines = maxLines
	}
	if value.MaxBytes == 0 {
		value.MaxBytes = maxTextBytes
	}
	if value.MaxLineBytes == 0 {
		value.MaxLineBytes = maxLineBytes
	}
	if value.MaxFileBytes < 1 || value.MaxFileBytes > maxFileBytes ||
		value.MaxLines < 1 || value.MaxLines > maxLines ||
		value.MaxBytes < 1 || value.MaxBytes > maxTextBytes ||
		value.MaxLineBytes < 1 || value.MaxLineBytes > maxLineBytes {
		return Limits{}, workspaceContentError(ErrorInvalidRequest, StageRequest)
	}
	return value, nil
}

func validPath(value string) bool {
	if len(value) == 0 || len(value) > maxPathBytes || !utf8.ValidString(value) ||
		value == "." || !fs.ValidPath(value) || path.Clean(value) != value ||
		strings.ContainsRune(value, '\\') {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validRange(value Range) bool {
	return value.StartLine > 0 && value.StartLine <= maxLineValue &&
		value.EndLine >= value.StartLine && value.EndLine <= maxLineValue &&
		value.FocusLine >= 0 && value.FocusLine <= maxLineValue
}

func physicalLineCount(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	count := bytes.Count(content, []byte{'\n'})
	if content[len(content)-1] != '\n' {
		count++
	}
	return count
}

func selectRange(
	request Range,
	lineCount int,
	lineLimit int,
) (start, end, focus int, truncated, ok bool) {
	if lineCount <= 0 || request.StartLine > lineCount {
		return 0, 0, 0, false, false
	}
	requestEnd := min(request.EndLine, lineCount)
	focus = request.FocusLine
	if focus < request.StartLine || focus > request.EndLine {
		focus = request.StartLine
	}

	start = request.StartLine
	end = requestEnd
	if requestEnd-request.StartLine+1 <= 60 {
		start = max(1, request.StartLine-10)
		end = min(lineCount, requestEnd+10)
	} else {
		start = max(1, focus-20)
		end = min(lineCount, start+maxLines-1)
	}

	if end-start+1 > lineLimit {
		candidateStart, candidateEnd := start, end
		before := min(20, lineLimit-1)
		start = max(candidateStart, focus-before)
		end = start + lineLimit - 1
		if end > candidateEnd {
			end = candidateEnd
			start = max(candidateStart, end-lineLimit+1)
		}
		truncated = true
	}
	if start > request.StartLine || end < requestEnd {
		truncated = true
	}
	return start, end, focus, truncated, start > 0 && end >= start &&
		focus >= start && focus <= end
}

func selectLines(
	content []byte,
	startLine, endLine, textLimit, lineLimit, countLimit int,
) ([]Line, string, error) {
	capacity := min(endLine-startLine+1, countLimit)
	lines := make([]Line, 0, capacity)
	var selected strings.Builder
	selected.Grow(min(textLimit, len(content)))

	lineNumber := 1
	lineStart := 0
	for index := 0; index <= len(content); index++ {
		atEOF := index == len(content)
		if !atEOF && content[index] != '\n' {
			continue
		}
		if atEOF && (lineStart == len(content) || len(content) == 0) {
			break
		}
		if lineNumber >= startLine && lineNumber <= endLine {
			rawLine := content[lineStart:index]
			if len(rawLine) > lineLimit {
				return nil, "", workspaceContentLimit(LimitLine)
			}
			added := len(rawLine)
			if len(lines) > 0 {
				added++
			}
			if selected.Len()+added > textLimit {
				return nil, "", workspaceContentLimit(LimitText)
			}
			if len(lines) > 0 {
				selected.WriteByte('\n')
			}
			selected.Write(rawLine)
			textLine := rawLine
			if len(textLine) > 0 && textLine[len(textLine)-1] == '\r' {
				textLine = textLine[:len(textLine)-1]
			}
			lines = append(lines, Line{
				Number: lineNumber,
				Text:   string(textLine),
			})
		}
		if lineNumber >= endLine || atEOF {
			break
		}
		lineNumber++
		lineStart = index + 1
	}
	if len(lines) == 0 || len(lines) > countLimit {
		return nil, "", workspaceContentLimit(LimitLines)
	}
	return lines[:len(lines):len(lines)], selected.String(), nil
}
