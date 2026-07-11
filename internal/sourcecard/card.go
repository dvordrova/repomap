// Package sourcecard reads bounded, line-addressable source evidence for one
// resolved repository symbol. It does not infer source semantics.
package sourcecard

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
)

const Version = 1

const (
	defaultMaxFileBytes   int64 = 1024 * 1024
	defaultMaxWindowLines       = 80
	defaultMaxWindowBytes       = 16 * 1024
	defaultMaxLineBytes         = 8 * 1024
)

type StopReason string

const (
	StopNextTopLevelFunc StopReason = "next_top_level_func"
	StopEndOfFile        StopReason = "end_of_file"
	StopLineLimit        StopReason = "line_limit"
	StopByteLimit        StopReason = "byte_limit"
)

type Request struct {
	RepoPath         string
	TargetEvidenceID string
	Target           evidence.Entity
}

type Limits struct {
	MaxFileBytes   int64
	MaxWindowLines int
	MaxWindowBytes int
	MaxLineBytes   int
}

type Card struct {
	Version    int       `json:"version"`
	Language   string    `json:"language"`
	RepoName   string    `json:"repo_name"`
	Target     Target    `json:"target"`
	FileSHA256 string    `json:"file_sha256"`
	Window     Window    `json:"window"`
	Lines      []Line    `json:"lines"`
	Warnings   []Warning `json:"warnings"`
}

type Target struct {
	EvidenceID string              `json:"evidence_id"`
	EntityID   string              `json:"entity_id"`
	Name       string              `json:"name"`
	Kind       evidence.EntityKind `json:"kind"`
	Path       string              `json:"path"`
	Line       int                 `json:"line"`
	Column     int                 `json:"column,omitempty"`
}

type Window struct {
	StartLine     int        `json:"start_line"`
	EndLine       int        `json:"end_line"`
	IncludedBytes int        `json:"included_bytes"`
	StopReason    StopReason `json:"stop_reason"`
	Truncated     bool       `json:"truncated"`
}

type Line struct {
	EvidenceID string `json:"evidence_id"`
	Line       int    `json:"line"`
	Text       string `json:"text"`
	Truncated  bool   `json:"truncated,omitempty"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Read(request Request, limits Limits) (Card, error) {
	limits = withDefaults(limits)
	if err := validateRequest(request); err != nil {
		return Card{}, err
	}

	repoRoot, sourcePath, repoRelativePath, err := resolveSourcePath(request.RepoPath, request.Target.Location.Path)
	if err != nil {
		return Card{}, err
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return Card{}, fmt.Errorf("sourcecard: open target: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Card{}, fmt.Errorf("sourcecard: stat target: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Card{}, fmt.Errorf("sourcecard: target path is not a regular file")
	}
	if info.Size() > limits.MaxFileBytes {
		return Card{}, fmt.Errorf("sourcecard: target file is %d bytes, limit is %d", info.Size(), limits.MaxFileBytes)
	}

	data, err := io.ReadAll(io.LimitReader(file, limits.MaxFileBytes+1))
	if err != nil {
		return Card{}, fmt.Errorf("sourcecard: read target: %w", err)
	}
	if int64(len(data)) > limits.MaxFileBytes {
		return Card{}, fmt.Errorf("sourcecard: target file exceeds %d byte limit", limits.MaxFileBytes)
	}

	fileLines := splitLines(data)
	targetLine := request.Target.Location.Line
	if targetLine > len(fileLines) {
		return Card{}, fmt.Errorf("sourcecard: target line %d exceeds file length %d", targetLine, len(fileLines))
	}
	if err := validateAnchor(fileLines[targetLine-1], request.Target.Name); err != nil {
		return Card{}, err
	}

	card := Card{
		Version:  Version,
		Language: "go",
		RepoName: filepath.Base(repoRoot),
		Target: Target{
			EvidenceID: request.TargetEvidenceID,
			EntityID:   request.Target.ID,
			Name:       request.Target.Name,
			Kind:       request.Target.Kind,
			Path:       repoRelativePath,
			Line:       targetLine,
			Column:     request.Target.Location.Column,
		},
		FileSHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
		Warnings: []Warning{{
			Code:    "boundary.lexical",
			Message: "window ends at the next top-level func declaration or a configured limit; it is not an ast-parsed function body",
		}},
	}

	stopReason := StopEndOfFile
	for index := targetLine - 1; index < len(fileLines); index++ {
		lineNumber := index + 1
		if lineNumber > targetLine && isTopLevelFunc(fileLines[index]) {
			stopReason = StopNextTopLevelFunc
			break
		}
		if len(card.Lines) >= limits.MaxWindowLines {
			stopReason = StopLineLimit
			card.Window.Truncated = true
			card.Warnings = append(card.Warnings, Warning{
				Code:    "window.line_limit",
				Message: fmt.Sprintf("source window stopped at %d lines", limits.MaxWindowLines),
			})
			break
		}

		text, wasTruncated := truncateUTF8(fileLines[index], limits.MaxLineBytes)
		lineBytes := len(text)
		if len(card.Lines) > 0 {
			lineBytes++
		}
		if card.Window.IncludedBytes+lineBytes > limits.MaxWindowBytes {
			stopReason = StopByteLimit
			card.Window.Truncated = true
			card.Warnings = append(card.Warnings, Warning{
				Code:    "window.byte_limit",
				Message: fmt.Sprintf("source window stopped at %d bytes", limits.MaxWindowBytes),
			})
			break
		}

		card.Lines = append(card.Lines, Line{
			EvidenceID: fmt.Sprintf("source-%d", lineNumber),
			Line:       lineNumber,
			Text:       text,
			Truncated:  wasTruncated,
		})
		card.Window.IncludedBytes += lineBytes
		if wasTruncated {
			card.Window.Truncated = true
			card.Warnings = append(card.Warnings, Warning{
				Code:    "line.truncated",
				Message: fmt.Sprintf("source line %d was truncated to %d bytes", lineNumber, limits.MaxLineBytes),
			})
		}
	}
	if len(card.Lines) == 0 {
		return Card{}, fmt.Errorf("sourcecard: source window is empty")
	}
	card.Window.StartLine = card.Lines[0].Line
	card.Window.EndLine = card.Lines[len(card.Lines)-1].Line
	card.Window.StopReason = stopReason
	if err := card.Validate(); err != nil {
		return Card{}, err
	}
	return card, nil
}

func (c Card) Validate() error {
	if c.Version != Version {
		return fmt.Errorf("sourcecard: unsupported version %d", c.Version)
	}
	if c.Language != "go" {
		return fmt.Errorf("sourcecard: unsupported language %q", c.Language)
	}
	if c.Target.EvidenceID == "" || c.Target.EntityID == "" || c.Target.Name == "" {
		return fmt.Errorf("sourcecard: target identity is incomplete")
	}
	if !validRepoRelativeGoPath(c.Target.Path) {
		return fmt.Errorf("sourcecard: invalid target path %q", c.Target.Path)
	}
	if c.Target.Line <= 0 || len(c.Lines) == 0 {
		return fmt.Errorf("sourcecard: target line and source lines are required")
	}
	if c.Window.StartLine != c.Target.Line || c.Window.EndLine < c.Window.StartLine {
		return fmt.Errorf("sourcecard: invalid window bounds")
	}
	if !c.Window.StopReason.valid() {
		return fmt.Errorf("sourcecard: invalid stop reason %q", c.Window.StopReason)
	}
	seen := make(map[string]struct{}, len(c.Lines))
	previousLine := 0
	for index, line := range c.Lines {
		if line.Line <= previousLine {
			return fmt.Errorf("sourcecard: lines are not strictly ordered at index %d", index)
		}
		if line.EvidenceID != fmt.Sprintf("source-%d", line.Line) {
			return fmt.Errorf("sourcecard: line %d has invalid evidence id %q", line.Line, line.EvidenceID)
		}
		if _, exists := seen[line.EvidenceID]; exists {
			return fmt.Errorf("sourcecard: duplicate evidence id %q", line.EvidenceID)
		}
		seen[line.EvidenceID] = struct{}{}
		previousLine = line.Line
	}
	if c.Lines[0].Line != c.Window.StartLine || c.Lines[len(c.Lines)-1].Line != c.Window.EndLine {
		return fmt.Errorf("sourcecard: window does not match included lines")
	}
	return nil
}

func withDefaults(limits Limits) Limits {
	if limits.MaxFileBytes <= 0 {
		limits.MaxFileBytes = defaultMaxFileBytes
	}
	if limits.MaxWindowLines <= 0 {
		limits.MaxWindowLines = defaultMaxWindowLines
	}
	if limits.MaxWindowBytes <= 0 {
		limits.MaxWindowBytes = defaultMaxWindowBytes
	}
	if limits.MaxLineBytes <= 0 {
		limits.MaxLineBytes = defaultMaxLineBytes
	}
	return limits
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.RepoPath) == "" {
		return fmt.Errorf("sourcecard: repository path is required")
	}
	if request.TargetEvidenceID == "" {
		return fmt.Errorf("sourcecard: target evidence id is required")
	}
	if request.Target.Kind != evidence.EntityFunction && request.Target.Kind != evidence.EntityMethod {
		return fmt.Errorf("sourcecard: target kind %q is not a function or method", request.Target.Kind)
	}
	if request.Target.Location == nil || request.Target.Location.Line <= 0 {
		return fmt.Errorf("sourcecard: target location is required")
	}
	return nil
}

func resolveSourcePath(repoPath, targetPath string) (string, string, string, error) {
	if !validRepoRelativeGoPath(targetPath) {
		return "", "", "", fmt.Errorf("sourcecard: invalid repository-relative go path %q", targetPath)
	}
	repoRoot, err := filepath.Abs(repoPath)
	if err != nil {
		return "", "", "", fmt.Errorf("sourcecard: resolve repository path: %w", err)
	}
	repoRoot, err = filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return "", "", "", fmt.Errorf("sourcecard: resolve repository symlinks: %w", err)
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(repoRoot, filepath.FromSlash(targetPath)))
	if err != nil {
		return "", "", "", fmt.Errorf("sourcecard: resolve target symlinks: %w", err)
	}
	relative, err := filepath.Rel(repoRoot, candidate)
	if err != nil {
		return "", "", "", fmt.Errorf("sourcecard: verify target containment: %w", err)
	}
	if !filepath.IsLocal(relative) || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("sourcecard: target resolves outside repository")
	}
	return repoRoot, candidate, filepath.ToSlash(filepath.Clean(filepath.FromSlash(targetPath))), nil
}

func validRepoRelativeGoPath(path string) bool {
	if path == "" || filepath.IsAbs(path) || !filepath.IsLocal(filepath.FromSlash(path)) {
		return false
	}
	cleaned := filepath.Clean(filepath.FromSlash(path))
	return cleaned != "." && strings.EqualFold(filepath.Ext(cleaned), ".go")
}

func validateAnchor(line, targetName string) error {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "func ") && !strings.HasPrefix(trimmed, "func\t") {
		return fmt.Errorf("sourcecard: target line is not a go func declaration")
	}
	name := targetName
	if index := strings.LastIndex(name, "."); index >= 0 {
		name = name[index+1:]
	}
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*(?:\[|\()`)
	if !pattern.MatchString(trimmed) {
		return fmt.Errorf("sourcecard: target line does not declare %q", targetName)
	}
	return nil
}

func splitLines(data []byte) []string {
	data = bytes.TrimSuffix(data, []byte("\n"))
	if len(data) == 0 {
		return []string{}
	}
	parts := bytes.Split(data, []byte("\n"))
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		part = bytes.TrimSuffix(part, []byte("\r"))
		lines = append(lines, string(part))
	}
	return lines
}

func isTopLevelFunc(line string) bool {
	return strings.HasPrefix(line, "func ") || strings.HasPrefix(line, "func\t")
}

func truncateUTF8(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

func (r StopReason) valid() bool {
	switch r {
	case StopNextTopLevelFunc, StopEndOfFile, StopLineLimit, StopByteLimit:
		return true
	default:
		return false
	}
}
