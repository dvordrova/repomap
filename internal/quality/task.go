// Package quality defines and loads replayable product-quality tasks. It is a
// concrete contract for the current orientation-to-source journey, not a
// general benchmark or provider framework.
package quality

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/sourceexplain"
)

const (
	TaskVersion                        = 2
	OrientationGroundingContextVersion = 1
)

const (
	maxTaskIDBytes       = 128
	maxRepositoryBytes   = 128
	maxGoalBytes         = 4 * 1024
	maxMetadataBytes     = 512
	maxArtifactPathBytes = 4 * 1024
	maxExpectationBytes  = 512
	maxDirections        = 16
	maxItemsPerList      = 128
	maxGroundingPaths    = 4096
	maxCaptureBytes      = 64 * 1024 * 1024
)

var (
	idPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	gitHashPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	sha256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	goTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._/+:-]+$`)
)

type Task struct {
	Version    int          `json:"version"`
	ID         string       `json:"id"`
	Repository Repository   `json:"repository"`
	Goal       string       `json:"goal"`
	Captures   Captures     `json:"captures"`
	Artifacts  Artifacts    `json:"artifacts"`
	Expected   Expectations `json:"expected"`
}

type Repository struct {
	Name     string        `json:"name"`
	Revision string        `json:"revision"`
	Scenario BuildScenario `json:"scenario"`
}

type BuildScenario struct {
	Orientation  string   `json:"orientation"`
	GOOS         string   `json:"goos"`
	GOARCH       string   `json:"goarch"`
	GoVersion    string   `json:"go_version"`
	GoplsVersion string   `json:"gopls_version"`
	BuildTags    []string `json:"build_tags"`
}

type Captures struct {
	Orientation StageCapture `json:"orientation"`
	Source      StageCapture `json:"source"`
}

type ResponseForm string

const (
	ResponseFormProviderContent  ResponseForm = "provider_content"
	ResponseFormNormalizedReport ResponseForm = "normalized_report"
)

// StageCapture distinguishes the semantic model context from its provider
// request envelope and records whether the replay contains the provider JSON
// field set before parsing or a post-parser legacy report. Fixtures may
// normalize JSON whitespace. A nil provider request or latency means the
// capture did not retain it.
type StageCapture struct {
	Provider      string       `json:"provider"`
	Model         string       `json:"model"`
	PromptVersion string       `json:"prompt_version"`
	ResponseForm  ResponseForm `json:"response_form"`
	// CapturedAt is RFC3339 when the exact time was retained, or YYYY-MM-DD
	// when a legacy fixture recorded only day precision.
	CapturedAt            string  `json:"captured_at"`
	ModelContextSHA256    string  `json:"model_context_sha256"`
	ModelContextBytes     int     `json:"model_context_bytes"`
	ProviderRequestSHA256 *string `json:"provider_request_sha256"`
	ProviderRequestBytes  *int    `json:"provider_request_bytes"`
	LatencyMillis         *int64  `json:"latency_ms"`
}

type ArtifactRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Artifacts struct {
	OrientationContext  ArtifactRef `json:"orientation_context"`
	OrientationResponse ArtifactRef `json:"orientation_response"`
	SourceBundle        ArtifactRef `json:"source_bundle"`
	SourceResponse      ArtifactRef `json:"source_response"`
	TestEvidence        ArtifactRef `json:"test_evidence"`
}

type Expectations struct {
	Directions          []DirectionExpectation `json:"directions"`
	Drilldown           DrilldownExpectation   `json:"drilldown"`
	ForbiddenOverclaims []ForbiddenOverclaim   `json:"forbidden_overclaims"`
}

type DirectionExpectation struct {
	ID             string   `json:"id"`
	Aliases        []string `json:"aliases"`
	ImportantPaths []string `json:"important_paths"`
}

type DrilldownExpectation struct {
	Symbol             string                    `json:"symbol"`
	Path               string                    `json:"path"`
	SourcePredicates   []sourceexplain.Predicate `json:"source_predicates"`
	TestReferencePaths []string                  `json:"test_reference_paths"`
}

type OverclaimScope string

const (
	OverclaimScopeOrientation OverclaimScope = "orientation"
	OverclaimScopeDrilldown   OverclaimScope = "drilldown"
)

type ForbiddenOverclaim struct {
	ID          string         `json:"id"`
	Scope       OverclaimScope `json:"scope"`
	ContainsAll []string       `json:"contains_all"`
}

// OrientationGroundingContext is the bounded derivative retained instead of
// the original orientation request. It contains only what replay needs to
// check repository identity and path grounding.
type OrientationGroundingContext struct {
	Version      int      `json:"version"`
	RepoName     string   `json:"repo_name"`
	AllowedPaths []string `json:"allowed_paths"`
}

func (t Task) Validate() error {
	if t.Version != TaskVersion {
		return fmt.Errorf("quality: unsupported task version %d", t.Version)
	}
	if !validID(t.ID) {
		return fmt.Errorf("quality: invalid task id %q", t.ID)
	}
	if err := t.Repository.validate(); err != nil {
		return err
	}
	if err := requiredText("goal", t.Goal, maxGoalBytes); err != nil {
		return err
	}
	if err := t.Captures.validate(); err != nil {
		return err
	}
	if err := t.Artifacts.validate(); err != nil {
		return err
	}
	if err := t.Expected.validate(); err != nil {
		return err
	}
	return nil
}

func (r Repository) validate() error {
	if err := requiredText("repository name", r.Name, maxRepositoryBytes); err != nil {
		return err
	}
	if !gitHashPattern.MatchString(r.Revision) {
		return fmt.Errorf("quality: repository revision must be a full lowercase git hash")
	}
	if err := r.Scenario.validate(); err != nil {
		return err
	}
	return nil
}

func (s BuildScenario) validate() error {
	metadata := []struct {
		name  string
		value string
	}{
		{name: "orientation scenario", value: s.Orientation},
		{name: "goos", value: s.GOOS},
		{name: "goarch", value: s.GOARCH},
		{name: "go version", value: s.GoVersion},
		{name: "gopls version", value: s.GoplsVersion},
	}
	for _, item := range metadata {
		if err := requiredText(item.name, item.value, maxMetadataBytes); err != nil {
			return err
		}
	}
	if s.BuildTags == nil {
		return fmt.Errorf("quality: build_tags must be explicit, use an empty array when none apply")
	}
	if len(s.BuildTags) > maxItemsPerList {
		return fmt.Errorf("quality: too many build tags")
	}
	seen := make(map[string]struct{}, len(s.BuildTags))
	for _, tag := range s.BuildTags {
		if len(tag) == 0 || len(tag) > maxMetadataBytes || !goTokenPattern.MatchString(tag) {
			return fmt.Errorf("quality: invalid build tag %q", tag)
		}
		if _, exists := seen[tag]; exists {
			return fmt.Errorf("quality: duplicate build tag %q", tag)
		}
		seen[tag] = struct{}{}
	}
	return nil
}

func (c Captures) validate() error {
	if err := c.Orientation.validate("orientation"); err != nil {
		return err
	}
	if err := c.Source.validate("source"); err != nil {
		return err
	}
	if c.Source.ResponseForm != ResponseFormProviderContent {
		return fmt.Errorf("quality: source response_form must be %q", ResponseFormProviderContent)
	}
	return nil
}

func (c StageCapture) validate(stage string) error {
	metadata := []struct {
		name  string
		value string
	}{
		{name: stage + " provider", value: c.Provider},
		{name: stage + " model", value: c.Model},
		{name: stage + " prompt version", value: c.PromptVersion},
	}
	for _, item := range metadata {
		if err := requiredText(item.name, item.value, maxMetadataBytes); err != nil {
			return err
		}
	}
	if c.ResponseForm != ResponseFormProviderContent && c.ResponseForm != ResponseFormNormalizedReport {
		return fmt.Errorf(
			"quality: %s response_form must be %q or %q",
			stage,
			ResponseFormProviderContent,
			ResponseFormNormalizedReport,
		)
	}
	if !validCapturedAt(c.CapturedAt) {
		return fmt.Errorf("quality: %s captured_at must be rfc3339 or yyyy-mm-dd", stage)
	}
	if !sha256Pattern.MatchString(c.ModelContextSHA256) {
		return fmt.Errorf("quality: %s model context sha256 is invalid", stage)
	}
	if c.ModelContextBytes <= 0 || c.ModelContextBytes > maxCaptureBytes {
		return fmt.Errorf("quality: %s model context byte count %d is outside bounds", stage, c.ModelContextBytes)
	}
	requestSHAKnown := c.ProviderRequestSHA256 != nil
	requestBytesKnown := c.ProviderRequestBytes != nil
	if requestSHAKnown != requestBytesKnown {
		return fmt.Errorf("quality: %s provider request sha256 and bytes must both be known or both be null", stage)
	}
	if requestSHAKnown {
		if !sha256Pattern.MatchString(*c.ProviderRequestSHA256) {
			return fmt.Errorf("quality: %s provider request sha256 is invalid", stage)
		}
		if *c.ProviderRequestBytes <= 0 || *c.ProviderRequestBytes > maxCaptureBytes {
			return fmt.Errorf("quality: %s provider request byte count %d is outside bounds", stage, *c.ProviderRequestBytes)
		}
	}
	if c.LatencyMillis != nil && *c.LatencyMillis < 0 {
		return fmt.Errorf("quality: %s latency cannot be negative", stage)
	}
	return nil
}

func (a Artifacts) validate() error {
	seenPaths := make(map[string]string, len(a.named()))
	for _, artifact := range a.named() {
		if err := artifact.ref.validate(artifact.name); err != nil {
			return err
		}
		if previous, exists := seenPaths[artifact.ref.Path]; exists {
			return fmt.Errorf("quality: artifacts %s and %s use the same path %q", previous, artifact.name, artifact.ref.Path)
		}
		seenPaths[artifact.ref.Path] = artifact.name
	}
	return nil
}

type namedArtifact struct {
	name string
	ref  ArtifactRef
}

func (a Artifacts) named() []namedArtifact {
	return []namedArtifact{
		{name: "orientation_context", ref: a.OrientationContext},
		{name: "orientation_response", ref: a.OrientationResponse},
		{name: "source_bundle", ref: a.SourceBundle},
		{name: "source_response", ref: a.SourceResponse},
		{name: "test_evidence", ref: a.TestEvidence},
	}
}

func (r ArtifactRef) validate(name string) error {
	if !validRelativePath(r.Path) || len(r.Path) > maxArtifactPathBytes {
		return fmt.Errorf("quality: artifact %s has invalid manifest-relative path %q", name, r.Path)
	}
	if !sha256Pattern.MatchString(r.SHA256) {
		return fmt.Errorf("quality: artifact %s has invalid sha256", name)
	}
	return nil
}

func (e Expectations) validate() error {
	if len(e.Directions) == 0 || len(e.Directions) > maxDirections {
		return fmt.Errorf("quality: directions must contain between 1 and %d items", maxDirections)
	}
	seenDirections := make(map[string]struct{}, len(e.Directions))
	for index, direction := range e.Directions {
		if err := direction.validate(index); err != nil {
			return err
		}
		if _, exists := seenDirections[direction.ID]; exists {
			return fmt.Errorf("quality: duplicate direction id %q", direction.ID)
		}
		seenDirections[direction.ID] = struct{}{}
	}
	if err := e.Drilldown.validate(); err != nil {
		return err
	}
	if len(e.ForbiddenOverclaims) == 0 || len(e.ForbiddenOverclaims) > maxItemsPerList {
		return fmt.Errorf("quality: forbidden_overclaims must be non-empty and bounded")
	}
	seenOverclaims := make(map[string]struct{}, len(e.ForbiddenOverclaims))
	for index, overclaim := range e.ForbiddenOverclaims {
		if err := overclaim.validate(index); err != nil {
			return err
		}
		if _, exists := seenOverclaims[overclaim.ID]; exists {
			return fmt.Errorf("quality: duplicate forbidden overclaim id %q", overclaim.ID)
		}
		seenOverclaims[overclaim.ID] = struct{}{}
	}
	return nil
}

func (e DirectionExpectation) validate(index int) error {
	if !validID(e.ID) {
		return fmt.Errorf("quality: directions[%d] has invalid id %q", index, e.ID)
	}
	if err := validateTextList(fmt.Sprintf("directions[%d].aliases", index), e.Aliases, false); err != nil {
		return err
	}
	if err := validatePathList(fmt.Sprintf("directions[%d].important_paths", index), e.ImportantPaths, false, false); err != nil {
		return err
	}
	return nil
}

func (e DrilldownExpectation) validate() error {
	if err := requiredText("drilldown symbol", e.Symbol, maxExpectationBytes); err != nil {
		return err
	}
	if !validRelativePath(e.Path) || !strings.EqualFold(filepath.Ext(e.Path), ".go") {
		return fmt.Errorf("quality: drilldown path must be a repository-relative go file")
	}
	if len(e.SourcePredicates) == 0 || len(e.SourcePredicates) > maxItemsPerList {
		return fmt.Errorf("quality: source_predicates must be non-empty and bounded")
	}
	seenPredicates := make(map[sourceexplain.Predicate]struct{}, len(e.SourcePredicates))
	for _, predicate := range e.SourcePredicates {
		if !validPredicate(predicate) {
			return fmt.Errorf("quality: invalid source predicate %q", predicate)
		}
		if _, exists := seenPredicates[predicate]; exists {
			return fmt.Errorf("quality: duplicate source predicate %q", predicate)
		}
		seenPredicates[predicate] = struct{}{}
	}
	if err := validatePathList("test_reference_paths", e.TestReferencePaths, false, true); err != nil {
		return err
	}
	return nil
}

func (o ForbiddenOverclaim) validate(index int) error {
	if !validID(o.ID) {
		return fmt.Errorf("quality: forbidden_overclaims[%d] has invalid id %q", index, o.ID)
	}
	if o.Scope != OverclaimScopeOrientation && o.Scope != OverclaimScopeDrilldown {
		return fmt.Errorf("quality: forbidden_overclaims[%d] has invalid scope %q", index, o.Scope)
	}
	if err := validateTextList(fmt.Sprintf("forbidden_overclaims[%d].contains_all", index), o.ContainsAll, false); err != nil {
		return err
	}
	return nil
}

func (c OrientationGroundingContext) Validate() error {
	if c.Version != OrientationGroundingContextVersion {
		return fmt.Errorf("quality: unsupported orientation grounding context version %d", c.Version)
	}
	if err := requiredText("orientation grounding repo name", c.RepoName, maxRepositoryBytes); err != nil {
		return err
	}
	if err := validatePathListLimit("orientation grounding allowed_paths", c.AllowedPaths, true, false, maxGroundingPaths); err != nil {
		return err
	}
	return nil
}

func validateTextList(name string, values []string, allowEmpty bool) error {
	if values == nil || (!allowEmpty && len(values) == 0) || len(values) > maxItemsPerList {
		return fmt.Errorf("quality: %s must be explicit, non-empty, and bounded", name)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || trimmed != value || len(value) > maxExpectationBytes {
			return fmt.Errorf("quality: %s contains invalid value %q", name, value)
		}
		normalized := strings.ToLower(value)
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("quality: %s contains duplicate value %q", name, value)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

func validatePathList(name string, values []string, requireSorted, requireTest bool) error {
	return validatePathListLimit(name, values, requireSorted, requireTest, maxItemsPerList)
}

func validatePathListLimit(name string, values []string, requireSorted, requireTest bool, maxItems int) error {
	if values == nil || len(values) == 0 || len(values) > maxItems {
		return fmt.Errorf("quality: %s must be explicit, non-empty, and bounded", name)
	}
	seen := make(map[string]struct{}, len(values))
	previous := ""
	for _, path := range values {
		if !validRelativePath(path) || len(path) > maxArtifactPathBytes {
			return fmt.Errorf("quality: %s contains invalid path %q", name, path)
		}
		if requireTest && !strings.HasSuffix(strings.ToLower(path), "_test.go") {
			return fmt.Errorf("quality: %s contains non-test path %q", name, path)
		}
		if _, exists := seen[path]; exists {
			return fmt.Errorf("quality: %s contains duplicate path %q", name, path)
		}
		if requireSorted && previous != "" && path < previous {
			return fmt.Errorf("quality: %s must be sorted", name)
		}
		seen[path] = struct{}{}
		previous = path
	}
	return nil
}

func validCapturedAt(value string) bool {
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return true
	}
	_, err := time.Parse(time.DateOnly, value)
	return err == nil
}

func validID(value string) bool {
	return len(value) > 0 && len(value) <= maxTaskIDBytes && idPattern.MatchString(value)
}

func validRelativePath(path string) bool {
	if path == "" || len(path) > maxArtifactPathBytes || strings.Contains(path, `\`) {
		return false
	}
	native := filepath.FromSlash(path)
	if filepath.IsAbs(native) || !filepath.IsLocal(native) {
		return false
	}
	cleaned := filepath.ToSlash(filepath.Clean(native))
	return cleaned != "." && cleaned == path
}

func validPredicate(predicate sourceexplain.Predicate) bool {
	switch predicate {
	case sourceexplain.PredicateValidatesInput,
		sourceexplain.PredicateDelegatesOperation,
		sourceexplain.PredicateMapsError,
		sourceexplain.PredicateFillsResponse,
		sourceexplain.PredicatePersistsState,
		sourceexplain.PredicatePerformsIO,
		sourceexplain.PredicateChecksCallResult,
		sourceexplain.PredicateReturnsCallResult,
		sourceexplain.PredicateCallsFromBranch:
		return true
	default:
		return false
	}
}

func requiredText(name, value string, maxBytes int) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value || len(value) > maxBytes {
		return fmt.Errorf("quality: %s is missing or invalid", name)
	}
	return nil
}
