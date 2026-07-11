// Package testevidence connects source-supported symbol claims to concrete Go
// test references. A reference is navigation evidence; it does not establish
// what the test asserts or whether the behavior executes at runtime.
package testevidence

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
)

const BundleVersion = 1

type EvidenceKind string

const EvidenceKindTestReference EvidenceKind = "test_reference"

type ReferenceFinder interface {
	References(ctx context.Context, repoPath string, location evidence.Location) (evidence.LocationSet, error)
}

type Options struct {
	MaxSearches            int
	MaxReferencesPerSearch int
}

type Bundle struct {
	Version    int                 `json:"version"`
	TargetName string              `json:"target_name"`
	Searches   []Search            `json:"searches"`
	References []Reference         `json:"references"`
	Scenarios  []evidence.Scenario `json:"scenarios"`
	Warnings   []Warning           `json:"warnings"`
}

type Search struct {
	AnchorEvidenceID  string                  `json:"anchor_evidence_id"`
	SymbolName        string                  `json:"symbol_name"`
	Location          evidence.Location       `json:"location"`
	Predicate         sourceexplain.Predicate `json:"predicate,omitempty"`
	SourceEvidenceIDs []string                `json:"source_evidence_ids"`
}

type Reference struct {
	EvidenceID     string                  `json:"evidence_id"`
	SearchAnchorID string                  `json:"search_anchor_id"`
	Predicate      sourceexplain.Predicate `json:"predicate,omitempty"`
	Path           string                  `json:"path"`
	Line           int                     `json:"line"`
	Column         int                     `json:"column"`
	Kind           EvidenceKind            `json:"kind"`
	Certainty      evidence.Certainty      `json:"certainty"`
	Provenance     []evidence.Provenance   `json:"provenance"`
	Scenarios      []string                `json:"scenarios"`
}

type Warning struct {
	Code    string `json:"code"`
	Anchor  string `json:"anchor_evidence_id,omitempty"`
	Message string `json:"message"`
}

func Collect(
	ctx context.Context,
	finder ReferenceFinder,
	repoPath string,
	structural symbol.Bundle,
	assessment sourceexplain.Bundle,
	report sourceexplain.Report,
	opts Options,
) (Bundle, error) {
	if finder == nil {
		return Bundle{}, fmt.Errorf("test evidence: reference finder is required")
	}
	if strings.TrimSpace(repoPath) == "" {
		return Bundle{}, fmt.Errorf("test evidence: repository path is required")
	}
	if err := validateInputs(structural, assessment, report); err != nil {
		return Bundle{}, err
	}
	opts = withDefaults(opts)
	searches := buildSearches(structural, report)
	truncatedSearches := 0
	if len(searches) > opts.MaxSearches {
		truncatedSearches = len(searches) - opts.MaxSearches
		searches = searches[:opts.MaxSearches]
	}

	bundle := Bundle{
		Version:    BundleVersion,
		TargetName: structural.Target.Entity.Name,
		Searches:   searches,
		References: []Reference{},
		Scenarios:  []evidence.Scenario{},
		Warnings: []Warning{{
			Code:    "support.reference_only",
			Message: "test references are navigation evidence only; no claim is test_supported until bounded test source is assessed",
		}},
	}
	if truncatedSearches > 0 {
		bundle.Warnings = append(bundle.Warnings, Warning{
			Code:    "searches.truncated",
			Message: fmt.Sprintf("omitted %d lower-priority reference searches", truncatedSearches),
		})
	}

	seenReferences := make(map[string]struct{})
	for _, search := range searches {
		locationSet, err := finder.References(ctx, repoPath, search.Location)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return Bundle{}, ctx.Err()
			}
			bundle.Warnings = append(bundle.Warnings, Warning{
				Code:    "references.failed",
				Anchor:  search.AnchorEvidenceID,
				Message: "reference lookup failed: " + err.Error(),
			})
			continue
		}
		if err := locationSet.Validate(); err != nil {
			bundle.Warnings = append(bundle.Warnings, Warning{
				Code:    "references.invalid",
				Anchor:  search.AnchorEvidenceID,
				Message: "reference lookup returned invalid evidence: " + err.Error(),
			})
			continue
		}
		bundle.Scenarios = appendUniqueScenarios(bundle.Scenarios, locationSet.Scenarios)
		testLocations := filterTestLocations(locationSet.Locations)
		if len(testLocations) > opts.MaxReferencesPerSearch {
			bundle.Warnings = append(bundle.Warnings, Warning{
				Code:    "references.truncated",
				Anchor:  search.AnchorEvidenceID,
				Message: fmt.Sprintf("test references truncated from %d to %d", len(testLocations), opts.MaxReferencesPerSearch),
			})
			testLocations = testLocations[:opts.MaxReferencesPerSearch]
		}
		for _, location := range testLocations {
			key := fmt.Sprintf("%s\x00%s\x00%d\x00%d", search.AnchorEvidenceID, location.Path, location.Line, location.Column)
			if _, exists := seenReferences[key]; exists {
				continue
			}
			seenReferences[key] = struct{}{}
			bundle.References = append(bundle.References, Reference{
				SearchAnchorID: search.AnchorEvidenceID,
				Predicate:      search.Predicate,
				Path:           location.Path,
				Line:           location.Line,
				Column:         location.Column,
				Kind:           EvidenceKindTestReference,
				Certainty:      locationSet.Certainty,
				Provenance:     cloneProvenance(locationSet.Provenance),
				Scenarios:      scenarioIDs(locationSet.Scenarios),
			})
		}
	}
	for index := range bundle.References {
		bundle.References[index].EvidenceID = fmt.Sprintf("test-ref-%03d", index+1)
	}
	if err := bundle.Validate(); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func (b Bundle) Validate() error {
	if b.Version != BundleVersion || b.TargetName == "" || len(b.Searches) == 0 {
		return fmt.Errorf("test evidence: invalid bundle identity")
	}
	searches := make(map[string]Search, len(b.Searches))
	for index, search := range b.Searches {
		if search.AnchorEvidenceID == "" || search.SymbolName == "" || search.Location.Path == "" || search.Location.Line <= 0 || search.Location.Column <= 0 {
			return fmt.Errorf("test evidence: searches[%d] is incomplete", index)
		}
		if _, exists := searches[search.AnchorEvidenceID]; exists {
			return fmt.Errorf("test evidence: duplicate search anchor %q", search.AnchorEvidenceID)
		}
		searches[search.AnchorEvidenceID] = search
	}
	seen := make(map[string]struct{}, len(b.References))
	knownScenarios := make(map[string]struct{}, len(b.Scenarios))
	for index, scenario := range b.Scenarios {
		if scenario.ID == "" || scenario.Name == "" {
			return fmt.Errorf("test evidence: scenarios[%d] is incomplete", index)
		}
		if _, exists := knownScenarios[scenario.ID]; exists {
			return fmt.Errorf("test evidence: duplicate scenario %q", scenario.ID)
		}
		knownScenarios[scenario.ID] = struct{}{}
	}
	for index, reference := range b.References {
		if reference.EvidenceID != fmt.Sprintf("test-ref-%03d", index+1) {
			return fmt.Errorf("test evidence: references[%d] has invalid evidence id %q", index, reference.EvidenceID)
		}
		if _, exists := seen[reference.EvidenceID]; exists {
			return fmt.Errorf("test evidence: duplicate reference evidence id %q", reference.EvidenceID)
		}
		seen[reference.EvidenceID] = struct{}{}
		search, ok := searches[reference.SearchAnchorID]
		if !ok || reference.Predicate != search.Predicate {
			return fmt.Errorf("test evidence: references[%d] has invalid search anchor", index)
		}
		if !validTestPath(reference.Path) || reference.Line <= 0 || reference.Column <= 0 {
			return fmt.Errorf("test evidence: references[%d] has invalid test location", index)
		}
		if reference.Kind != EvidenceKindTestReference || !reference.Certainty.Valid() {
			return fmt.Errorf("test evidence: references[%d] overstates support", index)
		}
		if len(reference.Provenance) == 0 || len(reference.Scenarios) == 0 {
			return fmt.Errorf("test evidence: references[%d] lost provider context", index)
		}
		for provenanceIndex, provenance := range reference.Provenance {
			if provenance.Provider == "" || provenance.Operation == "" {
				return fmt.Errorf("test evidence: references[%d].provenance[%d] is incomplete", index, provenanceIndex)
			}
		}
		for _, scenarioID := range reference.Scenarios {
			if _, exists := knownScenarios[scenarioID]; !exists {
				return fmt.Errorf("test evidence: references[%d] has unknown scenario %q", index, scenarioID)
			}
		}
	}
	return nil
}

func appendUniqueScenarios(dst, values []evidence.Scenario) []evidence.Scenario {
	seen := make(map[string]struct{}, len(dst)+len(values))
	for _, scenario := range dst {
		seen[scenario.ID] = struct{}{}
	}
	for _, scenario := range values {
		if _, exists := seen[scenario.ID]; exists {
			continue
		}
		seen[scenario.ID] = struct{}{}
		dst = append(dst, scenario)
	}
	sort.Slice(dst, func(i, j int) bool { return dst[i].ID < dst[j].ID })
	return dst
}

func scenarioIDs(scenarios []evidence.Scenario) []string {
	result := make([]string, 0, len(scenarios))
	for _, scenario := range scenarios {
		result = append(result, scenario.ID)
	}
	sort.Strings(result)
	return result
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

func validateInputs(structural symbol.Bundle, assessment sourceexplain.Bundle, report sourceexplain.Report) error {
	if structural.Target.Entity.Location == nil {
		return fmt.Errorf("test evidence: structural target has no location")
	}
	if err := sourceexplain.ValidateReport(assessment, report); err != nil {
		return fmt.Errorf("test evidence: invalid source assessment: %w", err)
	}
	if structural.Target.Entity.Name != report.Target.Name ||
		structural.Target.EvidenceID != report.Target.EvidenceID ||
		structural.Target.Entity.Location.Path != report.Target.Path ||
		structural.Target.Entity.Location.Line != report.Target.Line {
		return fmt.Errorf("test evidence: structural and source report targets do not agree")
	}
	return nil
}

func withDefaults(opts Options) Options {
	if opts.MaxSearches <= 0 {
		opts.MaxSearches = 4
	}
	if opts.MaxReferencesPerSearch <= 0 {
		opts.MaxReferencesPerSearch = 20
	}
	return opts
}

func buildSearches(structural symbol.Bundle, report sourceexplain.Report) []Search {
	target := structural.Target.Entity
	searches := []Search{{
		AnchorEvidenceID:  structural.Target.EvidenceID,
		SymbolName:        target.Name,
		Location:          *target.Location,
		SourceEvidenceIDs: []string{},
	}}
	calls := make(map[string]symbol.CallFact, len(structural.OutgoingCalls))
	for _, call := range structural.OutgoingCalls {
		calls[call.EvidenceID] = call
	}
	type rankedSearch struct {
		search Search
		weight int
	}
	ranked := make([]rankedSearch, 0, len(report.Claims))
	for _, claim := range report.Claims {
		if len(claim.StructuralEvidenceIDs) != 1 {
			continue
		}
		anchor := claim.StructuralEvidenceIDs[0]
		call, ok := calls[anchor]
		if !ok || call.Callee.Location == nil {
			continue
		}
		ranked = append(ranked, rankedSearch{
			search: Search{
				AnchorEvidenceID:  anchor,
				SymbolName:        call.Callee.Name,
				Location:          *call.Callee.Location,
				Predicate:         claim.Predicate,
				SourceEvidenceIDs: append([]string{}, claim.SourceEvidenceIDs...),
			},
			weight: predicateWeight(claim.Predicate),
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].weight > ranked[j].weight
	})
	for _, candidate := range ranked {
		searches = append(searches, candidate.search)
	}
	return searches
}

func filterTestLocations(locations []evidence.Location) []evidence.Location {
	seen := make(map[string]struct{})
	result := make([]evidence.Location, 0)
	for _, location := range locations {
		if !validTestPath(location.Path) || location.Line <= 0 || location.Column <= 0 {
			continue
		}
		key := fmt.Sprintf("%s\x00%d\x00%d", location.Path, location.Line, location.Column)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, location)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		return result[i].Column < result[j].Column
	})
	return result
}

func validTestPath(path string) bool {
	localPath := filepath.FromSlash(path)
	return path != "" && !filepath.IsAbs(localPath) && filepath.IsLocal(localPath) && strings.HasSuffix(path, "_test.go")
}

func predicateWeight(predicate sourceexplain.Predicate) int {
	switch predicate {
	case sourceexplain.PredicateValidatesInput:
		return 100
	case sourceexplain.PredicateMapsError:
		return 90
	case sourceexplain.PredicateDelegatesOperation:
		return 80
	case sourceexplain.PredicatePersistsState, sourceexplain.PredicatePerformsIO:
		return 70
	case sourceexplain.PredicateFillsResponse:
		return 50
	default:
		return 0
	}
}
