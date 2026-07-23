// Package reporead provides bounded reads of regular files contained in a
// resolved repository root.
package reporead

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const (
	maxReadBytes      = int64(^uint64(0) >> 1)
	lineReadChunkSize = 32 << 10
)

// Reader confines file reads to one resolved repository root.
type Reader struct {
	root *os.Root
}

// Content is a bounded file prefix. Truncated reports whether more bytes were
// available than the requested limit.
type Content struct {
	Bytes     []byte
	Truncated bool
}

// LineWindow is an inclusive range of physical, one-based source lines.
type LineWindow struct {
	Start int
	End   int
}

// WindowOptions bounds both source scanning and returned line retention.
// RetainBytes counts the original bytes occupied by complete returned lines,
// including their line endings.
type WindowOptions struct {
	ScanBytes   int64
	RetainBytes int64
	Windows     []LineWindow
}

// SourceLine is one complete physical source line. Text excludes the line
// ending and normalizes CRLF to LF by omitting the trailing carriage return.
type SourceLine struct {
	Number int
	Text   string
}

// WindowContent contains complete requested lines found within the scan
// bound. SourceTotalLines is populated only when SourceTotalLinesExact is true.
type WindowContent struct {
	Lines                 []SourceLine
	ScannedBytes          int64
	RetainedBytes         int64
	ScanTruncated         bool
	RetainTruncated       bool
	SourceTotalLines      int
	SourceTotalLinesExact bool
}

// New resolves repoPath and prepares a repository-confined reader.
func New(repoPath string) (*Reader, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("reporead: repository path is required")
	}

	absolute, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("reporead: resolve repository path: %w", err)
	}
	openedRoot, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("reporead: open repository root: %w", err)
	}

	return &Reader{root: openedRoot}, nil
}

// Close releases the repository root opened by New.
func (r *Reader) Close() error {
	if r == nil || r.root == nil {
		return nil
	}
	if err := r.root.Close(); err != nil {
		return fmt.Errorf("reporead: close repository root: %w", err)
	}
	return nil
}

// ReadFile reads at most maxBytes from a repository-relative regular file.
// Absolute paths, traversal, and symlinks resolving outside the repository are
// rejected.
func (r *Reader) ReadFile(repoRelativePath string, maxBytes int64) (Content, error) {
	if r == nil || r.root == nil {
		return Content{}, fmt.Errorf("reporead: reader is not initialized")
	}
	if maxBytes < 0 || maxBytes == maxReadBytes {
		return Content{}, fmt.Errorf("reporead: invalid byte limit %d", maxBytes)
	}

	file, err := r.openRegularFile(repoRelativePath)
	if err != nil {
		return Content{}, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return Content{}, fmt.Errorf("reporead: read file: %w", err)
	}
	if int64(len(data)) <= maxBytes {
		return Content{Bytes: data[:len(data):len(data)]}, nil
	}

	limit := int(maxBytes)
	return Content{
		Bytes:     data[:limit:limit],
		Truncated: true,
	}, nil
}

// ReadLineWindows scans at most opts.ScanBytes of a repository-relative
// regular file and retains complete lines in the requested windows. Windows
// are merged before scanning, so each line is returned at most once in
// ascending order. A line cut by the scan bound is never returned.
func (r *Reader) ReadLineWindows(
	repoRelativePath string,
	opts WindowOptions,
) (WindowContent, error) {
	if r == nil || r.root == nil {
		return WindowContent{}, fmt.Errorf("reporead: reader is not initialized")
	}
	if opts.ScanBytes < 0 || opts.ScanBytes == maxReadBytes {
		return WindowContent{}, fmt.Errorf(
			"reporead: invalid scan byte limit %d",
			opts.ScanBytes,
		)
	}
	if opts.RetainBytes < 0 || opts.RetainBytes == maxReadBytes {
		return WindowContent{}, fmt.Errorf(
			"reporead: invalid retained byte limit %d",
			opts.RetainBytes,
		)
	}
	windows, err := normalizeLineWindows(opts.Windows)
	if err != nil {
		return WindowContent{}, err
	}

	file, err := r.openRegularFile(repoRelativePath)
	if err != nil {
		return WindowContent{}, err
	}
	defer file.Close()

	content, err := scanLineWindows(file, windows, opts.ScanBytes, opts.RetainBytes)
	if err != nil {
		return WindowContent{}, fmt.Errorf("reporead: scan file: %w", err)
	}
	return content, nil
}

func (r *Reader) openRegularFile(repoRelativePath string) (*os.File, error) {
	localPath := filepath.FromSlash(repoRelativePath)
	if repoRelativePath == "" || filepath.IsAbs(localPath) || !filepath.IsLocal(localPath) {
		return nil, fmt.Errorf("reporead: invalid repository-relative path %q", repoRelativePath)
	}
	cleanPath := filepath.Clean(localPath)
	if cleanPath == "." || cleanPath != localPath {
		return nil, fmt.Errorf("reporead: path must be a clean repository-relative file")
	}

	file, err := r.root.Open(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("reporead: open file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("reporead: stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("reporead: path is not a regular file")
	}
	return file, nil
}

func normalizeLineWindows(windows []LineWindow) ([]LineWindow, error) {
	normalized := append([]LineWindow(nil), windows...)
	for _, window := range normalized {
		if window.Start < 1 || window.End < window.Start {
			return nil, fmt.Errorf(
				"reporead: invalid line window %d-%d",
				window.Start,
				window.End,
			)
		}
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Start != normalized[j].Start {
			return normalized[i].Start < normalized[j].Start
		}
		return normalized[i].End < normalized[j].End
	})

	merged := make([]LineWindow, 0, len(normalized))
	for _, window := range normalized {
		if len(merged) == 0 {
			merged = append(merged, window)
			continue
		}
		last := &merged[len(merged)-1]
		overlaps := window.Start <= last.End
		adjacent := window.Start > last.End && window.Start-last.End == 1
		if !overlaps && !adjacent {
			merged = append(merged, window)
			continue
		}
		if window.End > last.End {
			last.End = window.End
		}
	}
	return merged, nil
}

type lineWindowScanner struct {
	windows       []LineWindow
	windowIndex   int
	lineNumber    int
	lineSelected  bool
	lineStarted   bool
	lineBytes     []byte
	lineOverflow  bool
	retainedLimit int64
	content       WindowContent
}

func scanLineWindows(
	file *os.File,
	windows []LineWindow,
	scanLimit int64,
	retainedLimit int64,
) (WindowContent, error) {
	scanner := lineWindowScanner{
		windows:       windows,
		lineNumber:    1,
		retainedLimit: retainedLimit,
		content: WindowContent{
			Lines: make([]SourceLine, 0),
		},
	}
	scanner.selectCurrentLine()

	buffer := make([]byte, lineReadChunkSize)
	for scanner.content.ScannedBytes < scanLimit {
		remaining := scanLimit - scanner.content.ScannedBytes
		readSize := int64(len(buffer))
		if remaining < readSize {
			readSize = remaining
		}
		read, readErr := file.Read(buffer[:int(readSize)])
		if read > 0 {
			scanner.consume(buffer[:read])
			scanner.content.ScannedBytes += int64(read)
		}
		if readErr != nil {
			if readErr == io.EOF {
				scanner.finishAtEOF()
				return scanner.content, nil
			}
			return WindowContent{}, readErr
		}
		if read == 0 {
			return WindowContent{}, io.ErrNoProgress
		}
	}

	// As with ReadFile, one byte of lookahead distinguishes an exact-bound EOF
	// from a longer file. The byte is not scanned or retained.
	var lookahead [1]byte
	read, readErr := file.Read(lookahead[:])
	if readErr != nil && readErr != io.EOF {
		return WindowContent{}, readErr
	}
	if read == 0 && readErr == io.EOF {
		scanner.finishAtEOF()
		return scanner.content, nil
	}

	scanner.content.ScanTruncated = true
	return scanner.content, nil
}

func (scanner *lineWindowScanner) consume(data []byte) {
	for _, current := range data {
		if current == '\n' {
			scanner.finishLine(true)
			scanner.lineNumber++
			scanner.lineStarted = false
			scanner.lineBytes = scanner.lineBytes[:0]
			scanner.lineOverflow = false
			scanner.selectCurrentLine()
			continue
		}

		scanner.lineStarted = true
		if !scanner.lineSelected || scanner.lineOverflow {
			continue
		}
		remaining := scanner.retainedLimit - scanner.content.RetainedBytes
		if int64(len(scanner.lineBytes)) >= remaining {
			scanner.lineOverflow = true
			scanner.lineBytes = scanner.lineBytes[:0]
			continue
		}
		scanner.lineBytes = append(scanner.lineBytes, current)
	}
}

func (scanner *lineWindowScanner) finishAtEOF() {
	if scanner.lineStarted {
		scanner.finishLine(false)
	}
	scanner.content.SourceTotalLines = scanner.lineNumber
	if !scanner.lineStarted {
		scanner.content.SourceTotalLines--
	}
	scanner.content.SourceTotalLinesExact = true
}

func (scanner *lineWindowScanner) finishLine(terminated bool) {
	if !scanner.lineSelected {
		return
	}

	sourceBytes := int64(len(scanner.lineBytes))
	if terminated {
		sourceBytes++
	}
	remaining := scanner.retainedLimit - scanner.content.RetainedBytes
	if scanner.lineOverflow || sourceBytes > remaining {
		scanner.content.RetainTruncated = true
		return
	}

	text := scanner.lineBytes
	if terminated && len(text) > 0 && text[len(text)-1] == '\r' {
		text = text[:len(text)-1]
	}
	scanner.content.Lines = append(scanner.content.Lines, SourceLine{
		Number: scanner.lineNumber,
		Text:   string(text),
	})
	scanner.content.RetainedBytes += sourceBytes
}

func (scanner *lineWindowScanner) selectCurrentLine() {
	for scanner.windowIndex < len(scanner.windows) &&
		scanner.lineNumber > scanner.windows[scanner.windowIndex].End {
		scanner.windowIndex++
	}
	scanner.lineSelected = scanner.windowIndex < len(scanner.windows) &&
		scanner.lineNumber >= scanner.windows[scanner.windowIndex].Start
}
