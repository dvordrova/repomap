// Package inspection resolves and inspects exact repository declarations
// within one immutable authorized source catalog. It owns no report, HTTP,
// browser, provider, persistence, or user-facing semantics.
package inspection

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/symbol"
)

const (
	maxResolverCandidates = 20
	maxCandidates         = 8
	maxRankTerms          = 16
	maxReferences         = 5
	maxTestReferences     = 5

	maxSourceFileBytes   int64 = 1024 * 1024
	maxSourceWindowLines       = 20
	maxSourceWindowBytes       = 12 * 1024
	maxSourceLineBytes         = 4 * 1024
)

// ReferenceFinder is the smallest exact-reference boundary needed by the
// service. It intentionally uses only neutral evidence types.
type ReferenceFinder interface {
	References(context.Context, string, evidence.Location) (evidence.LocationSet, error)
}

// Dependencies are local analyzer boundaries. Each operation reports
// ErrorAnalyzerUnavailable when its required dependency is absent.
type Dependencies struct {
	Resolver        analyzer.LocationResolver
	ExactAnalyzer   analyzer.ExactSymbolAnalyzer
	ReferenceFinder ReferenceFinder
}

// Limits preserves the current browser product bounds. Zero values select the
// defaults; positive values may only narrow them.
type Limits struct {
	MaxResolverCandidates int
	MaxCandidates         int
	MaxRankTerms          int
	Symbol                symbol.Options
	Source                sourcecard.Limits
	MaxReferences         int
	MaxTestReferences     int
}

// DefaultLimits returns the retained exact-inspection limits used by the
// existing reportserver adapter.
func DefaultLimits() Limits {
	return Limits{
		MaxResolverCandidates: maxResolverCandidates,
		MaxCandidates:         maxCandidates,
		MaxRankTerms:          maxRankTerms,
		Symbol: symbol.Options{
			MaxCandidates:        1,
			MaxIncomingCalls:     5,
			MaxOutgoingCalls:     5,
			MaxProvenancePerFact: 1,
		},
		Source: sourcecard.Limits{
			MaxFileBytes:   maxSourceFileBytes,
			MaxWindowLines: maxSourceWindowLines,
			MaxWindowBytes: maxSourceWindowBytes,
			MaxLineBytes:   maxSourceLineBytes,
		},
		MaxReferences:     maxReferences,
		MaxTestReferences: maxTestReferences,
	}
}

// Service is a concrete exact-inspection service bound to one immutable source
// catalog.
type Service struct {
	catalog         sourcecatalog.Catalog
	resolver        analyzer.LocationResolver
	exactAnalyzer   analyzer.ExactSymbolAnalyzer
	referenceFinder ReferenceFinder
}

// New constructs a service without reading repository contents.
func New(catalog sourcecatalog.Catalog, dependencies Dependencies) (*Service, error) {
	root := catalog.AnalysisRoot()
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, inspectionError(ErrorInvalidRequest, "new", nil)
	}
	for _, path := range catalog.Paths() {
		source, ok := catalog.Lookup(path)
		if !ok || source.Path != path || source.ContentSHA256 == "" {
			return nil, inspectionError(ErrorInvalidRequest, "new", nil)
		}
	}
	return &Service{
		catalog:         catalog,
		resolver:        dependencies.Resolver,
		exactAnalyzer:   dependencies.ExactAnalyzer,
		referenceFinder: dependencies.ReferenceFinder,
	}, nil
}

func normalizeLimits(input Limits) (Limits, error) {
	defaults := DefaultLimits()
	if input.MaxResolverCandidates == 0 {
		input.MaxResolverCandidates = defaults.MaxResolverCandidates
	}
	if input.MaxCandidates == 0 {
		input.MaxCandidates = defaults.MaxCandidates
	}
	if input.MaxRankTerms == 0 {
		input.MaxRankTerms = defaults.MaxRankTerms
	}
	if input.Symbol.MaxCandidates == 0 {
		input.Symbol.MaxCandidates = defaults.Symbol.MaxCandidates
	}
	if input.Symbol.MaxIncomingCalls == 0 {
		input.Symbol.MaxIncomingCalls = defaults.Symbol.MaxIncomingCalls
	}
	if input.Symbol.MaxOutgoingCalls == 0 {
		input.Symbol.MaxOutgoingCalls = defaults.Symbol.MaxOutgoingCalls
	}
	if input.Symbol.MaxProvenancePerFact == 0 {
		input.Symbol.MaxProvenancePerFact = defaults.Symbol.MaxProvenancePerFact
	}
	if input.Source.MaxFileBytes == 0 {
		input.Source.MaxFileBytes = defaults.Source.MaxFileBytes
	}
	if input.Source.MaxWindowLines == 0 {
		input.Source.MaxWindowLines = defaults.Source.MaxWindowLines
	}
	if input.Source.MaxWindowBytes == 0 {
		input.Source.MaxWindowBytes = defaults.Source.MaxWindowBytes
	}
	if input.Source.MaxLineBytes == 0 {
		input.Source.MaxLineBytes = defaults.Source.MaxLineBytes
	}
	if input.MaxReferences == 0 {
		input.MaxReferences = defaults.MaxReferences
	}
	if input.MaxTestReferences == 0 {
		input.MaxTestReferences = defaults.MaxTestReferences
	}

	within := func(value, maximum int) bool {
		return value > 0 && value <= maximum
	}
	if !within(input.MaxResolverCandidates, defaults.MaxResolverCandidates) ||
		!within(input.MaxCandidates, defaults.MaxCandidates) ||
		!within(input.MaxRankTerms, defaults.MaxRankTerms) ||
		!within(input.Symbol.MaxCandidates, defaults.Symbol.MaxCandidates) ||
		!within(input.Symbol.MaxIncomingCalls, defaults.Symbol.MaxIncomingCalls) ||
		!within(input.Symbol.MaxOutgoingCalls, defaults.Symbol.MaxOutgoingCalls) ||
		!within(input.Symbol.MaxProvenancePerFact, defaults.Symbol.MaxProvenancePerFact) ||
		input.Source.MaxFileBytes <= 0 || input.Source.MaxFileBytes > defaults.Source.MaxFileBytes ||
		!within(input.Source.MaxWindowLines, defaults.Source.MaxWindowLines) ||
		!within(input.Source.MaxWindowBytes, defaults.Source.MaxWindowBytes) ||
		!within(input.Source.MaxLineBytes, defaults.Source.MaxLineBytes) ||
		!within(input.MaxReferences, defaults.MaxReferences) ||
		!within(input.MaxTestReferences, defaults.MaxTestReferences) {
		return Limits{}, inspectionError(ErrorInvalidRequest, "limits", nil)
	}
	return input, nil
}

func (s *Service) authorizedLocation(location evidence.Location) bool {
	if s == nil || location.Path == "" || location.Line <= 0 || location.Column < 0 {
		return false
	}
	_, ok := s.catalog.Lookup(location.Path)
	return ok
}

func cloneLocation(location *evidence.Location) *evidence.Location {
	if location == nil {
		return nil
	}
	copy := *location
	return &copy
}

func cloneEntity(entity evidence.Entity) evidence.Entity {
	entity.Location = cloneLocation(entity.Location)
	return entity
}

func cloneProvenance(values []evidence.Provenance, service *Service) []evidence.Provenance {
	result := make([]evidence.Provenance, 0, len(values))
	for _, value := range values {
		provenance := evidence.Provenance{
			Provider:  value.Provider,
			Version:   value.Version,
			Operation: value.Operation,
		}
		if value.Location != nil && service.authorizedLocation(*value.Location) {
			provenance.Location = cloneLocation(value.Location)
		}
		result = append(result, provenance)
	}
	return result
}

func (s *Service) safeProvenance(value evidence.Provenance) bool {
	if s == nil ||
		!safeText(value.Provider, s.catalog.AnalysisRoot(), 64) ||
		!safeText(value.Operation, s.catalog.AnalysisRoot(), 64) {
		return false
	}
	return value.Version == "" || safeText(value.Version, s.catalog.AnalysisRoot(), 128)
}

func (s *Service) safeScenario(value evidence.Scenario) bool {
	if s == nil || !safeText(value.ID, s.catalog.AnalysisRoot(), 128) ||
		(value.Name != "" && !safeText(value.Name, s.catalog.AnalysisRoot(), 256)) ||
		(value.Build.GOOS != "" && !safeText(value.Build.GOOS, s.catalog.AnalysisRoot(), 64)) ||
		(value.Build.GOARCH != "" && !safeText(value.Build.GOARCH, s.catalog.AnalysisRoot(), 64)) {
		return false
	}
	for _, tag := range value.Build.BuildTags {
		if !safeText(tag, s.catalog.AnalysisRoot(), 128) {
			return false
		}
	}
	return true
}

func safeText(value, root string, limit int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > limit || strings.Contains(value, root) ||
		strings.Contains(value, "file://") {
		return false
	}
	for _, field := range strings.Fields(value) {
		trimmed := strings.Trim(field, `"'(),:;[]`)
		if filepath.IsAbs(trimmed) {
			return false
		}
	}
	for _, character := range value {
		if character < ' ' && character != '\t' {
			return false
		}
	}
	return true
}

func analyzerFailure(operation string, ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return inspectionError(ErrorAnalysisFailed, operation, ctx.Err())
	}
	if err == nil {
		err = fmt.Errorf("analyzer returned invalid evidence")
	}
	return inspectionError(ErrorAnalysisFailed, operation, err)
}
