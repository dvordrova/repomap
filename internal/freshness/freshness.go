// Package freshness captures the repository revision and exact content
// identities used by one report. It does not compare repository states or
// reject changes made while analysis is running.
package freshness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	RepositoryStateVersion = 3
	CapturedInputVersion   = 1
)

type FileKind string

const (
	FileRegular FileKind = "file"
	FileSymlink FileKind = "symlink"
	FileMissing FileKind = "missing"
)

type DirtyFile struct {
	Status        string   `json:"status"`
	Path          string   `json:"path"`
	FromPath      string   `json:"from_path,omitempty"`
	Kind          FileKind `json:"kind"`
	Mode          string   `json:"mode,omitempty"`
	ContentSHA256 string   `json:"content_sha256,omitempty"`
}

type RepositoryState struct {
	Version    int              `json:"version"`
	Identity   string           `json:"identity"`
	Head       string           `json:"head"`
	Dirty      []DirtyFile      `json:"dirty"`
	Submodules []SubmoduleState `json:"submodules,omitempty"`
}

type SubmoduleAvailability string

const (
	SubmoduleClean       SubmoduleAvailability = "clean"
	SubmoduleUnavailable SubmoduleAvailability = "unavailable"
)

type SubmoduleState struct {
	Path               string                `json:"path"`
	IncludedInAnalysis bool                  `json:"included_in_analysis"`
	RecordedGitlink    string                `json:"recorded_gitlink,omitempty"`
	CurrentHead        string                `json:"current_head,omitempty"`
	GitlinkChanged     bool                  `json:"gitlink_changed,omitempty"`
	WorktreeModified   bool                  `json:"worktree_modified,omitempty"`
	WorktreeUntracked  bool                  `json:"worktree_untracked,omitempty"`
	Availability       SubmoduleAvailability `json:"availability"`
}

type CapturedInput struct {
	Version        int      `json:"version"`
	ID             string   `json:"id"`
	Path           string   `json:"path"`
	Kind           FileKind `json:"kind"`
	Mode           string   `json:"mode,omitempty"`
	ContentSHA256  string   `json:"content_sha256,omitempty"`
	OwningModuleID string   `json:"owning_module_id,omitempty"`
	OwningPackage  string   `json:"owning_package,omitempty"`
	Stages         []string `json:"stages"`
}

func (state RepositoryState) Validate() error {
	if state.Version != RepositoryStateVersion {
		return fmt.Errorf("freshness: unsupported repository-state version %d", state.Version)
	}
	if !filepath.IsAbs(state.Identity) || filepath.Clean(state.Identity) != state.Identity {
		return fmt.Errorf("freshness: repository identity must be a clean absolute path")
	}
	if !validHexDigest(state.Head, 40, 64) {
		return fmt.Errorf("freshness: repository HEAD must be a 40- or 64-character lowercase hex object ID")
	}
	previous := ""
	for index, file := range state.Dirty {
		if err := file.validate(); err != nil {
			return fmt.Errorf("freshness: dirty file %d: %w", index, err)
		}
		key := dirtyFileKey(file)
		if previous != "" && key <= previous {
			return fmt.Errorf("freshness: dirty files must be uniquely sorted")
		}
		previous = key
	}
	previous = ""
	for index, submodule := range state.Submodules {
		if err := submodule.validate(); err != nil {
			return fmt.Errorf("freshness: submodule %d: %w", index, err)
		}
		if previous != "" && submodule.Path <= previous {
			return fmt.Errorf("freshness: submodules must be uniquely sorted")
		}
		previous = submodule.Path
	}
	return nil
}

func (state RepositoryState) Digest() (string, error) {
	if err := state.Validate(); err != nil {
		return "", err
	}
	canonical := state
	canonical.Dirty = append([]DirtyFile{}, state.Dirty...)
	canonical.Submodules = append([]SubmoduleState{}, state.Submodules...)
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("freshness: encode repository state: %w", err)
	}
	return sha256Hex(data), nil
}

func (file DirtyFile) validate() error {
	if !validStatus(file.Status) {
		return fmt.Errorf("invalid status %q", file.Status)
	}
	if err := validateRelativePath(file.Path); err != nil {
		return err
	}
	if file.FromPath != "" {
		if err := validateRelativePath(file.FromPath); err != nil {
			return fmt.Errorf("from path: %w", err)
		}
	}
	switch file.Kind {
	case FileRegular, FileSymlink:
		if !validHexDigest(file.ContentSHA256, 64) {
			return fmt.Errorf("%s content SHA-256 is required", file.Kind)
		}
	case FileMissing:
		if file.ContentSHA256 != "" {
			return fmt.Errorf("missing file cannot have a content SHA-256")
		}
	default:
		return fmt.Errorf("invalid kind %q", file.Kind)
	}
	return nil
}

func (state SubmoduleState) validate() error {
	if err := validateRelativePath(state.Path); err != nil {
		return err
	}
	if state.RecordedGitlink != "" && !validHexDigest(state.RecordedGitlink, 40, 64) {
		return fmt.Errorf("recorded gitlink is malformed")
	}
	if state.CurrentHead != "" && !validHexDigest(state.CurrentHead, 40, 64) {
		return fmt.Errorf("current submodule HEAD is malformed")
	}
	if state.Availability != SubmoduleClean && state.Availability != SubmoduleUnavailable {
		return fmt.Errorf("invalid submodule availability %q", state.Availability)
	}
	return nil
}

func (input CapturedInput) Validate() error {
	if input.Version != CapturedInputVersion || !validHexDigest(input.ID, 64) {
		return fmt.Errorf("freshness: captured input version or id is invalid")
	}
	if err := validateRelativePath(input.Path); err != nil {
		return err
	}
	if input.Kind != FileRegular && input.Kind != FileSymlink && input.Kind != FileMissing {
		return fmt.Errorf("freshness: captured input kind %q is unsupported", input.Kind)
	}
	if input.Kind != FileMissing && !validHexDigest(input.ContentSHA256, 64) {
		return fmt.Errorf("freshness: captured input content SHA-256 is required")
	}
	if input.Kind == FileMissing && input.ContentSHA256 != "" {
		return fmt.Errorf("freshness: missing captured input has content")
	}
	previous := ""
	for _, stage := range input.Stages {
		if !validLabel(stage) || (previous != "" && stage <= previous) {
			return fmt.Errorf("freshness: captured input stages must be uniquely sorted")
		}
		previous = stage
	}
	if len(input.Stages) == 0 {
		return fmt.Errorf("freshness: captured input has no consuming stage")
	}
	return nil
}

func CapturedInputsDigest(inputs []CapturedInput) (string, error) {
	canonical := append([]CapturedInput(nil), inputs...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].Path < canonical[j].Path })
	previous := ""
	for index := range canonical {
		if err := canonical[index].Validate(); err != nil {
			return "", fmt.Errorf("freshness: captured input %d: %w", index, err)
		}
		if previous != "" && canonical[index].Path <= previous {
			return "", fmt.Errorf("freshness: captured inputs must be uniquely sorted")
		}
		previous = canonical[index].Path
		canonical[index].Stages = append([]string(nil), canonical[index].Stages...)
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("freshness: encode captured inputs: %w", err)
	}
	return sha256Hex(encoded), nil
}

func dirtyFileKey(file DirtyFile) string {
	return file.Path + "\x00" + file.FromPath
}

func validateRelativePath(value string) error {
	if value == "" || filepath.IsAbs(value) {
		return fmt.Errorf("invalid repository-relative path %q", value)
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if cleaned != value || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.ContainsRune(value, 0) {
		return fmt.Errorf("invalid repository-relative path %q", value)
	}
	return nil
}

func validStatus(status string) bool {
	switch status {
	case "added", "copied", "conflicted", "deleted", "modified", "renamed", "type_changed":
		return true
	default:
		return false
	}
}

func validLabel(value string) bool {
	return strings.TrimSpace(value) != "" && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validHexDigest(value string, lengths ...int) bool {
	validLength := false
	for _, length := range lengths {
		if len(value) == length {
			validLength = true
			break
		}
	}
	if !validLength || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sha256Hex(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
