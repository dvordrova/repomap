// Package freshness defines the local inputs that make persisted repository
// facts and model claims safe to reuse.
package freshness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/evidence"
)

const (
	RepositoryStateVersion = 1
	FactContextVersion     = 1
	ClaimContextVersion    = 1
)

type FileKind string

const (
	FileRegular   FileKind = "file"
	FileSymlink   FileKind = "symlink"
	FileMissing   FileKind = "missing"
	FileDirectory FileKind = "directory"
)

type DirtyFile struct {
	Status        string   `json:"status"`
	Path          string   `json:"path"`
	FromPath      string   `json:"from_path,omitempty"`
	Kind          FileKind `json:"kind"`
	ContentSHA256 string   `json:"content_sha256,omitempty"`
}

type RepositoryState struct {
	Version  int         `json:"version"`
	Identity string      `json:"identity"`
	Head     string      `json:"head"`
	Dirty    []DirtyFile `json:"dirty"`
}

func (s RepositoryState) Validate() error {
	if s.Version != RepositoryStateVersion {
		return fmt.Errorf("freshness: unsupported repository-state version %d", s.Version)
	}
	if !filepath.IsAbs(s.Identity) || filepath.Clean(s.Identity) != s.Identity {
		return fmt.Errorf("freshness: repository identity must be a clean absolute path")
	}
	if !validHexDigest(s.Head, 40, 64) {
		return fmt.Errorf("freshness: repository HEAD must be a 40- or 64-character lowercase hex object ID")
	}
	previous := ""
	for index, file := range s.Dirty {
		if err := file.validate(); err != nil {
			return fmt.Errorf("freshness: dirty file %d: %w", index, err)
		}
		key := dirtyFileKey(file)
		if previous != "" && key <= previous {
			return fmt.Errorf("freshness: dirty files must be uniquely sorted")
		}
		previous = key
	}
	return nil
}

func (s RepositoryState) Digest() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	canonical := s
	canonical.Dirty = append([]DirtyFile{}, s.Dirty...)
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("freshness: encode repository state: %w", err)
	}
	return sha256Hex(data), nil
}

func (f DirtyFile) validate() error {
	if !validStatus(f.Status) {
		return fmt.Errorf("invalid status %q", f.Status)
	}
	if err := validateRelativePath(f.Path); err != nil {
		return err
	}
	if f.FromPath != "" {
		if err := validateRelativePath(f.FromPath); err != nil {
			return fmt.Errorf("from path: %w", err)
		}
	}
	switch f.Kind {
	case FileRegular, FileSymlink:
		if !validHexDigest(f.ContentSHA256, 64) {
			return fmt.Errorf("%s content SHA-256 is required", f.Kind)
		}
	case FileMissing:
		if f.ContentSHA256 != "" {
			return fmt.Errorf("missing file cannot have a content SHA-256")
		}
	case FileDirectory:
		return fmt.Errorf("dirty directory state is not safely fingerprintable")
	default:
		return fmt.Errorf("invalid kind %q", f.Kind)
	}
	return nil
}

type FactContext struct {
	Version          int                   `json:"version"`
	Repository       RepositoryState       `json:"repository"`
	GoVersion        string                `json:"go_version"`
	Analyzer         string                `json:"analyzer"`
	AnalyzerVersion  string                `json:"analyzer_version"`
	Collector        string                `json:"collector"`
	CollectorVersion string                `json:"collector_version"`
	InputsSHA256     string                `json:"inputs_sha256"`
	Build            evidence.BuildContext `json:"build"`
}

func (c FactContext) Validate() error {
	if c.Version != FactContextVersion {
		return fmt.Errorf("freshness: unsupported fact-context version %d", c.Version)
	}
	if err := c.Repository.Validate(); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"Go version":        c.GoVersion,
		"analyzer":          c.Analyzer,
		"analyzer version":  c.AnalyzerVersion,
		"collector":         c.Collector,
		"collector version": c.CollectorVersion,
		"GOOS":              c.Build.GOOS,
		"GOARCH":            c.Build.GOARCH,
	} {
		if !validLabel(value) {
			return fmt.Errorf("freshness: %s is required and must not contain control characters", name)
		}
	}
	if !validHexDigest(c.InputsSHA256, 64) {
		return fmt.Errorf("freshness: analysis inputs must have a SHA-256 digest")
	}
	previous := ""
	for _, tag := range c.Build.BuildTags {
		if !validLabel(tag) || (previous != "" && tag <= previous) {
			return fmt.Errorf("freshness: build tags must be non-empty, unique, and sorted")
		}
		previous = tag
	}
	return nil
}

func (c FactContext) Digest() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	canonical := c
	canonical.Repository.Dirty = append([]DirtyFile{}, c.Repository.Dirty...)
	canonical.Build.BuildTags = append([]string{}, c.Build.BuildTags...)
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("freshness: encode fact context: %w", err)
	}
	return sha256Hex(data), nil
}

type ClaimContext struct {
	Version          int    `json:"version"`
	FactDigest       string `json:"fact_digest"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	PromptVersion    string `json:"prompt_version"`
	ParserVersion    int    `json:"parser_version"`
	EvaluatorVersion int    `json:"evaluator_version"`
}

func (c ClaimContext) Validate() error {
	if c.Version != ClaimContextVersion {
		return fmt.Errorf("freshness: unsupported claim-context version %d", c.Version)
	}
	if !validHexDigest(c.FactDigest, 64) {
		return fmt.Errorf("freshness: claim fact digest must be a SHA-256")
	}
	for name, value := range map[string]string{
		"provider":       c.Provider,
		"model":          c.Model,
		"prompt version": c.PromptVersion,
	} {
		if !validLabel(value) {
			return fmt.Errorf("freshness: claim %s is required and must not contain control characters", name)
		}
	}
	if c.ParserVersion <= 0 || c.EvaluatorVersion <= 0 {
		return fmt.Errorf("freshness: positive parser and evaluator versions are required")
	}
	return nil
}

func (c ClaimContext) Digest() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("freshness: encode claim context: %w", err)
	}
	return sha256Hex(data), nil
}

type Reason string

const (
	ReasonRepositoryIdentity Reason = "repository_identity"
	ReasonRepositoryHead     Reason = "repository_head"
	ReasonRepositoryDirty    Reason = "repository_dirty"
	ReasonGoVersion          Reason = "go_version"
	ReasonAnalyzer           Reason = "analyzer"
	ReasonAnalyzerVersion    Reason = "analyzer_version"
	ReasonCollector          Reason = "collector"
	ReasonCollectorVersion   Reason = "collector_version"
	ReasonAnalysisInputs     Reason = "analysis_inputs"
	ReasonBuildContext       Reason = "build_context"
	ReasonFactDigest         Reason = "fact_digest"
	ReasonProvider           Reason = "provider"
	ReasonModel              Reason = "model"
	ReasonPromptVersion      Reason = "prompt_version"
	ReasonParserVersion      Reason = "parser_version"
	ReasonEvaluatorVersion   Reason = "evaluator_version"
)

type Difference struct {
	Reason  Reason   `json:"reason"`
	Paths   []string `json:"paths,omitempty"`
	Saved   string   `json:"saved,omitempty"`
	Current string   `json:"current,omitempty"`
}

func (d Difference) String() string {
	if len(d.Paths) > 0 {
		return fmt.Sprintf("%s changed: %s", d.Reason, strings.Join(d.Paths, ", "))
	}
	if d.Saved != "" || d.Current != "" {
		return fmt.Sprintf("%s changed from %q to %q", d.Reason, d.Saved, d.Current)
	}
	return string(d.Reason)
}

func CompareRepository(saved, current RepositoryState) []Difference {
	var differences []Difference
	if saved.Identity != current.Identity {
		differences = append(differences, Difference{Reason: ReasonRepositoryIdentity, Saved: saved.Identity, Current: current.Identity})
	}
	if saved.Head != current.Head {
		differences = append(differences, Difference{Reason: ReasonRepositoryHead, Saved: saved.Head, Current: current.Head})
	}
	if !reflect.DeepEqual(normalizedDirty(saved.Dirty), normalizedDirty(current.Dirty)) {
		differences = append(differences, Difference{Reason: ReasonRepositoryDirty, Paths: changedDirtyPaths(saved.Dirty, current.Dirty)})
	}
	return differences
}

func CompareFactContext(saved, current FactContext) []Difference {
	differences := CompareRepository(saved.Repository, current.Repository)
	differences = appendStringDifference(differences, ReasonGoVersion, saved.GoVersion, current.GoVersion)
	differences = appendStringDifference(differences, ReasonAnalyzer, saved.Analyzer, current.Analyzer)
	differences = appendStringDifference(differences, ReasonAnalyzerVersion, saved.AnalyzerVersion, current.AnalyzerVersion)
	differences = appendStringDifference(differences, ReasonCollector, saved.Collector, current.Collector)
	differences = appendStringDifference(differences, ReasonCollectorVersion, saved.CollectorVersion, current.CollectorVersion)
	differences = appendStringDifference(differences, ReasonAnalysisInputs, saved.InputsSHA256, current.InputsSHA256)
	if !reflect.DeepEqual(normalizedBuild(saved.Build), normalizedBuild(current.Build)) {
		differences = append(differences, Difference{Reason: ReasonBuildContext})
	}
	return differences
}

func CompareClaimContext(saved, current ClaimContext) []Difference {
	var differences []Difference
	differences = appendStringDifference(differences, ReasonFactDigest, saved.FactDigest, current.FactDigest)
	differences = appendStringDifference(differences, ReasonProvider, saved.Provider, current.Provider)
	differences = appendStringDifference(differences, ReasonModel, saved.Model, current.Model)
	differences = appendStringDifference(differences, ReasonPromptVersion, saved.PromptVersion, current.PromptVersion)
	if saved.ParserVersion != current.ParserVersion {
		differences = append(differences, Difference{Reason: ReasonParserVersion, Saved: fmt.Sprint(saved.ParserVersion), Current: fmt.Sprint(current.ParserVersion)})
	}
	if saved.EvaluatorVersion != current.EvaluatorVersion {
		differences = append(differences, Difference{Reason: ReasonEvaluatorVersion, Saved: fmt.Sprint(saved.EvaluatorVersion), Current: fmt.Sprint(current.EvaluatorVersion)})
	}
	return differences
}

func appendStringDifference(differences []Difference, reason Reason, saved, current string) []Difference {
	if saved == current {
		return differences
	}
	return append(differences, Difference{Reason: reason, Saved: saved, Current: current})
}

func changedDirtyPaths(saved, current []DirtyFile) []string {
	savedByPath := dirtyByPath(saved)
	currentByPath := dirtyByPath(current)
	pathSet := make(map[string]struct{}, len(savedByPath)+len(currentByPath))
	for path, file := range savedByPath {
		if other, ok := currentByPath[path]; !ok || file != other {
			addDirtyPaths(pathSet, file)
			if ok {
				addDirtyPaths(pathSet, other)
			}
		}
	}
	for path, file := range currentByPath {
		if other, ok := savedByPath[path]; !ok || file != other {
			addDirtyPaths(pathSet, file)
			if ok {
				addDirtyPaths(pathSet, other)
			}
		}
	}
	paths := make([]string, 0, len(pathSet))
	for path := range pathSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func addDirtyPaths(paths map[string]struct{}, file DirtyFile) {
	paths[file.Path] = struct{}{}
	if file.FromPath != "" {
		paths[file.FromPath] = struct{}{}
	}
}

func dirtyByPath(files []DirtyFile) map[string]DirtyFile {
	result := make(map[string]DirtyFile, len(files))
	for _, file := range files {
		result[file.Path] = file
	}
	return result
}

func normalizedDirty(files []DirtyFile) []DirtyFile {
	return append([]DirtyFile{}, files...)
}

func normalizedBuild(build evidence.BuildContext) evidence.BuildContext {
	build.BuildTags = append([]string{}, build.BuildTags...)
	return build
}

func dirtyFileKey(file DirtyFile) string {
	return file.Path + "\x00" + file.FromPath
}

func validateRelativePath(path string) error {
	if path == "" || filepath.IsAbs(path) {
		return fmt.Errorf("invalid repository-relative path %q", path)
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if cleaned != path || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.ContainsRune(path, 0) {
		return fmt.Errorf("invalid repository-relative path %q", path)
	}
	return nil
}

func validStatus(status string) bool {
	switch status {
	case "added", "copied", "conflicted", "deleted", "ignored", "modified", "renamed", "type_changed", "untracked":
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
