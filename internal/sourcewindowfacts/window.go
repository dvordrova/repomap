// Package sourcewindowfacts turns already saved bounded Go source windows into
// locally verifiable inputs. It performs no repository-wide discovery.
package sourcewindowfacts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/modelresearch"
)

const (
	maxEvidenceBundleBytes int64 = 2 << 20
	maxGoSourceBytes       int64 = 16 << 20
	maxWindowLines               = 512
	maxSourceLineBytes           = 64 << 10
)

// Window is exact saved source text. ContentSHA256 hashes the line array, so
// line boundaries are part of the content identity.
type Window struct {
	EvidenceID    string   `json:"evidence_id"`
	Path          string   `json:"path"`
	StartLine     int      `json:"start_line"`
	EndLine       int      `json:"end_line"`
	Lines         []string `json:"lines"`
	ContentSHA256 string   `json:"content_sha256"`
}

// NewWindow constructs one bounded, code-bearing source window. It is useful
// for source-card lines that have already been read through an authorized local
// boundary. EndLine and ContentSHA256 are always derived locally.
func NewWindow(evidenceID, sourcePath string, startLine int, lines []string) (Window, error) {
	window := Window{
		EvidenceID: evidenceID,
		Path:       sourcePath,
		StartLine:  startLine,
		Lines:      append([]string(nil), lines...),
	}
	window.EndLine = startLine + len(window.Lines) - 1
	window.ContentSHA256 = linesSHA256(window.Lines)
	if err := window.Validate(); err != nil {
		return Window{}, err
	}
	return window, nil
}

// Validate checks the portable bounded representation. Repository contents are
// checked separately by LoadRun.
func (window Window) Validate() error {
	if err := validateOpaqueID("evidence id", window.EvidenceID); err != nil {
		return err
	}
	if err := validateGoPath(window.Path); err != nil {
		return err
	}
	if window.StartLine <= 0 || window.EndLine < window.StartLine {
		return fmt.Errorf("source window: invalid line bounds %d-%d", window.StartLine, window.EndLine)
	}
	wantLines := window.EndLine - window.StartLine + 1
	if len(window.Lines) == 0 || len(window.Lines) != wantLines || len(window.Lines) > maxWindowLines {
		return fmt.Errorf("source window: line count does not match bounds")
	}
	for index, line := range window.Lines {
		if !utf8.ValidString(line) || len(line) > maxSourceLineBytes {
			return fmt.Errorf("source window: line %d is invalid or too large", index)
		}
	}
	wantSHA := linesSHA256(window.Lines)
	if window.ContentSHA256 != wantSHA {
		return fmt.Errorf("source window: content sha256 does not match lines")
	}
	return nil
}

// LoadRun strictly loads source_window items from
// research/*/evidence_bundle.json, verifies them against current repository
// lines, and returns a deterministic order. Repeated identical evidence IDs are
// collapsed; conflicting reuse of an ID is rejected.
func LoadRun(runDir, repoRoot string) ([]Window, error) {
	return loadRun(runDir, repoRoot, false)
}

// LoadRunForDiscovery loads the same repository-verified windows as LoadRun,
// but omits evidence explicitly marked truncated. A truncated window cannot be
// turned into an exact source fact, while other independent windows in the run
// can still seed bounded candidate discovery. All other malformed, stale, or
// conflicting evidence remains an error; canonical consumers should continue
// to use the strict LoadRun entrypoint.
func LoadRunForDiscovery(runDir, repoRoot string) ([]Window, error) {
	return loadRun(runDir, repoRoot, true)
}

func loadRun(runDir, repoRoot string, skipTruncated bool) ([]Window, error) {
	if strings.TrimSpace(runDir) == "" || strings.TrimSpace(repoRoot) == "" {
		return nil, fmt.Errorf("source window: run directory and repository root are required")
	}
	researchDir := filepath.Join(runDir, "research")
	entries, err := os.ReadDir(researchDir)
	if err != nil {
		return nil, fmt.Errorf("source window: read research directory: %w", err)
	}
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("source window: resolve repository root: %w", err)
	}
	info, err := os.Stat(repoRoot)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("source window: repository root is not a directory")
	}

	byID := make(map[string]Window)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		bundlePath := filepath.Join(researchDir, entry.Name(), "evidence_bundle.json")
		bundle, found, readErr := readEvidenceBundle(bundlePath)
		if readErr != nil {
			return nil, readErr
		}
		if !found {
			continue
		}
		for index, item := range bundle.Evidence {
			if item.Kind != modelresearch.EvidenceSource {
				continue
			}
			if skipTruncated && item.Window != nil && item.Window.Truncated {
				continue
			}
			window, convertErr := windowFromEvidence(item)
			if convertErr != nil {
				return nil, fmt.Errorf(
					"source window: %s evidence[%d]: %w",
					entry.Name(),
					index,
					convertErr,
				)
			}
			if verifyErr := verifyRepositoryWindow(repoRoot, window); verifyErr != nil {
				return nil, fmt.Errorf("source window: evidence %q: %w", window.EvidenceID, verifyErr)
			}
			if previous, exists := byID[window.EvidenceID]; exists {
				if !reflect.DeepEqual(previous, window) {
					return nil, fmt.Errorf("source window: evidence id %q has conflicting contents", window.EvidenceID)
				}
				continue
			}
			byID[window.EvidenceID] = window
		}
	}

	windows := make([]Window, 0, len(byID))
	for _, window := range byID {
		windows = append(windows, window)
	}
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].Path != windows[j].Path {
			return windows[i].Path < windows[j].Path
		}
		if windows[i].StartLine != windows[j].StartLine {
			return windows[i].StartLine < windows[j].StartLine
		}
		if windows[i].EndLine != windows[j].EndLine {
			return windows[i].EndLine < windows[j].EndLine
		}
		return windows[i].EvidenceID < windows[j].EvidenceID
	})
	return windows, nil
}

func readEvidenceBundle(bundlePath string) (modelresearch.EvidenceBundle, bool, error) {
	info, err := os.Lstat(bundlePath)
	if err != nil {
		if os.IsNotExist(err) {
			return modelresearch.EvidenceBundle{}, false, nil
		}
		return modelresearch.EvidenceBundle{}, false, fmt.Errorf("source window: inspect %s: %w", bundlePath, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxEvidenceBundleBytes {
		return modelresearch.EvidenceBundle{}, false, fmt.Errorf("source window: %s is not a bounded regular file", bundlePath)
	}
	raw, err := os.ReadFile(bundlePath)
	if err != nil {
		return modelresearch.EvidenceBundle{}, false, fmt.Errorf("source window: read %s: %w", bundlePath, err)
	}
	var bundle modelresearch.EvidenceBundle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return modelresearch.EvidenceBundle{}, false, fmt.Errorf("source window: decode %s: %w", bundlePath, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return modelresearch.EvidenceBundle{}, false, fmt.Errorf("source window: %s contains trailing json", bundlePath)
	}
	if bundle.Version != modelresearch.ContractVersion {
		return modelresearch.EvidenceBundle{}, false, fmt.Errorf("source window: %s has unsupported version %d", bundlePath, bundle.Version)
	}
	return bundle, true, nil
}

func windowFromEvidence(item modelresearch.EvidenceItem) (Window, error) {
	if item.Location == nil || item.Window == nil {
		return Window{}, fmt.Errorf("source evidence has no location or window")
	}
	if !item.Window.CodeBearing {
		return Window{}, fmt.Errorf("source evidence is not code-bearing")
	}
	if item.Window.Truncated {
		return Window{}, fmt.Errorf("source evidence is truncated")
	}
	if item.Location.Line != item.Window.StartLine {
		return Window{}, fmt.Errorf("source evidence location does not match its bounds")
	}
	window, err := NewWindow(
		item.ID,
		item.Location.Path,
		item.Window.StartLine,
		item.Window.Lines,
	)
	if err != nil {
		return Window{}, err
	}
	if window.EndLine != item.Window.EndLine {
		return Window{}, fmt.Errorf("source evidence end line does not match its lines")
	}
	return window, nil
}

func verifyRepositoryWindow(repoRoot string, window Window) error {
	filePath := filepath.Join(repoRoot, filepath.FromSlash(window.Path))
	info, err := os.Lstat(filePath)
	if err != nil {
		return fmt.Errorf("inspect repository source: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxGoSourceBytes {
		return fmt.Errorf("repository source is not a bounded regular file")
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read repository source: %w", err)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("repository source is not valid utf-8")
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if window.EndLine > len(lines) {
		return fmt.Errorf("saved bounds exceed current repository source")
	}
	current := lines[window.StartLine-1 : window.EndLine]
	if !reflect.DeepEqual(current, window.Lines) {
		return fmt.Errorf("saved lines differ from current repository source")
	}
	return nil
}

func validateGoPath(sourcePath string) error {
	if sourcePath == "" || !utf8.ValidString(sourcePath) || strings.Contains(sourcePath, "\\") ||
		path.IsAbs(sourcePath) || filepath.IsAbs(sourcePath) {
		return fmt.Errorf("source window: path %q is not a portable repository-relative path", sourcePath)
	}
	cleaned := path.Clean(sourcePath)
	if cleaned != sourcePath || cleaned == "." || strings.HasPrefix(cleaned, "../") || path.Ext(cleaned) != ".go" {
		return fmt.Errorf("source window: path %q is not a canonical repository-relative Go path", sourcePath)
	}
	for _, char := range sourcePath {
		if char < 0x20 || char == 0x7f {
			return fmt.Errorf("source window: path contains a control character")
		}
	}
	return nil
}

func validateOpaqueID(field, value string) error {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return fmt.Errorf("source window: %s is empty, malformed, or too long", field)
	}
	for _, char := range value {
		if char <= 0x20 || char == 0x7f || char == '/' || char == '\\' {
			return fmt.Errorf("source window: %s contains whitespace, control, or path characters", field)
		}
	}
	return nil
}

func linesSHA256(lines []string) string {
	raw, _ := json.Marshal(lines)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
