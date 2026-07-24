package inspection

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/reporead"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/symbol"
)

type InspectRequest struct {
	Target            evidence.Entity
	Limits            Limits
	IncludeReferences bool
}

type Warning struct {
	Code string
}

type Result struct {
	Structural symbol.Bundle
	Source     sourcecard.Card
	References *evidence.LocationSet
	Tests      *evidence.LocationSet
	Warnings   []Warning
	Provenance []evidence.Provenance
}

// Inspect confirms one selected Go declaration, returns its bounded Go
// structural/source contracts, and optionally collects authorized exact
// references with Go _test.go classification.
func (s *Service) Inspect(ctx context.Context, request InspectRequest) (Result, error) {
	if ctx == nil || s == nil || request.Target.ID == "" || request.Target.Name == "" ||
		(request.Target.Kind != evidence.EntityFunction && request.Target.Kind != evidence.EntityMethod) ||
		request.Target.Location == nil || request.Target.Location.Column <= 0 {
		return Result{}, inspectionError(ErrorInvalidRequest, "inspect", nil)
	}
	if !safeText(request.Target.ID, s.catalog.AnalysisRoot(), 512) ||
		!safeText(request.Target.Name, s.catalog.AnalysisRoot(), 256) ||
		(request.Target.Language != "" && !safeText(request.Target.Language, s.catalog.AnalysisRoot(), 32)) {
		return Result{}, inspectionError(ErrorInvalidRequest, "inspect", nil)
	}
	if !s.authorizedLocation(*request.Target.Location) {
		return Result{}, inspectionError(ErrorUnauthorized, "inspect", nil)
	}
	if s.exactAnalyzer == nil {
		return Result{}, inspectionError(ErrorAnalyzerUnavailable, "inspect", nil)
	}
	limits, err := normalizeLimits(request.Limits)
	if err != nil {
		return Result{}, err
	}
	graph, err := s.exactAnalyzer.AnalyzeExactSymbol(ctx, analyzer.ExactSymbolRequest{
		RepoPath: s.catalog.AnalysisRoot(),
		Symbol:   cloneEntity(request.Target),
	})
	if err != nil {
		return Result{}, analyzerFailure("exact_symbol", ctx, err)
	}
	authorizedGraph, err := s.authorizeGraph(graph)
	if err != nil {
		return Result{}, err
	}
	structural, err := symbol.Build(authorizedGraph, limits.Symbol)
	if err != nil || !reflect.DeepEqual(structural.Target.Entity, request.Target) {
		return Result{}, analyzerFailure("exact_symbol", ctx, err)
	}
	structural = boundStructuralBundle(structural, limits)

	capturedSource, ok := s.catalog.Lookup(structural.Target.Entity.Location.Path)
	if !ok {
		return Result{}, inspectionError(ErrorUnauthorized, "source", nil)
	}
	if err := verifyCurrentSourceHash(
		s.catalog.AnalysisRoot(),
		capturedSource.Path,
		capturedSource.ContentSHA256,
		limits.Source.MaxFileBytes,
	); err != nil {
		return Result{}, err
	}
	card, err := sourcecard.Read(sourcecard.Request{
		RepoPath:         s.catalog.AnalysisRoot(),
		TargetEvidenceID: structural.Target.EvidenceID,
		Target:           structural.Target.Entity,
	}, limits.Source)
	if err != nil {
		return Result{}, inspectionError(ErrorAnalysisFailed, "source", err)
	}
	source, ok := s.catalog.Lookup(card.Target.Path)
	if !ok {
		return Result{}, inspectionError(ErrorUnauthorized, "source", nil)
	}
	if card.FileSHA256 != source.ContentSHA256 {
		return Result{}, inspectionError(ErrorSourceChanged, "source", nil)
	}
	if err := sourcecard.ValidateForRemote(card); err != nil {
		return Result{}, inspectionError(ErrorAnalysisFailed, "source", err)
	}

	result := Result{
		Structural: structural,
		Source:     card,
		Provenance: cloneProvenance(structural.Target.Provenance, s, maxAggregateProvenance),
	}
	if request.IncludeReferences {
		if s.referenceFinder == nil {
			result.Warnings = append(result.Warnings, Warning{Code: "references.unavailable"})
		} else if err := s.collectReferences(ctx, request.Target, limits, &result); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func verifyCurrentSourceHash(root, path, capturedSHA256 string, maxBytes int64) error {
	if maxBytes == 0 {
		openedRoot, err := os.OpenRoot(root)
		if err != nil {
			return inspectionError(ErrorAnalysisFailed, "source", err)
		}
		defer openedRoot.Close()
		info, err := openedRoot.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return inspectionError(ErrorAnalysisFailed, "source", err)
		}
		file, err := openedRoot.Open(path)
		if err != nil {
			return inspectionError(ErrorAnalysisFailed, "source", err)
		}
		defer file.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			return inspectionError(ErrorAnalysisFailed, "source", err)
		}
		if fmt.Sprintf("%x", hash.Sum(nil)) != capturedSHA256 {
			return inspectionError(ErrorSourceChanged, "source", nil)
		}
		return nil
	}
	reader, err := reporead.New(root)
	if err != nil {
		return inspectionError(ErrorAnalysisFailed, "source", err)
	}
	defer reader.Close()
	content, err := reader.ReadFile(path, maxBytes)
	if err != nil {
		return inspectionError(ErrorAnalysisFailed, "source", err)
	}
	if content.Truncated {
		return inspectionError(ErrorAnalysisFailed, "source", nil)
	}
	currentSHA256 := fmt.Sprintf("%x", sha256.Sum256(content.Bytes))
	if currentSHA256 != capturedSHA256 {
		return inspectionError(ErrorSourceChanged, "source", nil)
	}
	return nil
}

func (s *Service) authorizeGraph(graph evidence.Graph) (evidence.Graph, error) {
	graph, err := s.boundAnalyzerGraph(graph)
	if err != nil {
		return evidence.Graph{}, err
	}
	if graph.RepoPath != s.catalog.AnalysisRoot() ||
		!safeText(graph.Query, s.catalog.AnalysisRoot(), 256) ||
		!s.safeBuildContext(graph.Build) {
		return evidence.Graph{}, analyzerFailure("exact_symbol", nil, nil)
	}
	if err := graph.Validate(); err != nil {
		return evidence.Graph{}, analyzerFailure("exact_symbol", nil, err)
	}
	for _, scenario := range boundedPrefix(graph.Scenarios, maxRawGraphScenarios) {
		if !s.safeScenario(scenario) {
			return evidence.Graph{}, analyzerFailure("exact_symbol", nil, nil)
		}
	}
	authorized := evidence.NewGraph(s.catalog.AnalysisRoot(), graph.Query)
	authorized.Build = cloneBuildContext(graph.Build)
	authorized.Warnings = cloneStrings(graph.Warnings, maxRawGraphWarnings)
	authorized.Scenarios = cloneScenarios(graph.Scenarios, maxRawGraphScenarios)
	authorized.Entities = make([]evidence.Entity, 0, min(len(graph.Entities), maxRawGraphEntities))
	authorized.Relations = make([]evidence.Relation, 0, min(len(graph.Relations), maxRawGraphRelations))

	kept := make(map[string]struct{}, min(len(graph.Entities), maxRawGraphEntities))
	for _, entity := range boundedPrefix(graph.Entities, maxRawGraphEntities) {
		if !s.safeEntity(entity) ||
			(entity.Location != nil && !s.authorizedLocation(*entity.Location)) {
			continue
		}
		authorized.Entities = append(authorized.Entities, cloneEntity(entity))
		kept[entity.ID] = struct{}{}
	}
	for _, relation := range boundedPrefix(graph.Relations, maxRawGraphRelations) {
		if _, ok := kept[relation.From]; !ok {
			continue
		}
		if _, ok := kept[relation.To]; !ok {
			continue
		}
		provenanceAuthorized := true
		for _, provenance := range boundedPrefix(relation.Provenance, maxRawRelationProvenance) {
			if !s.safeProvenance(provenance) ||
				(provenance.Location != nil && !s.authorizedLocation(*provenance.Location)) {
				provenanceAuthorized = false
				break
			}
		}
		if !provenanceAuthorized {
			continue
		}
		copy := relation
		copy.Provenance = cloneProvenance(
			relation.Provenance,
			s,
			maxRawRelationProvenance,
		)
		copy.Scenarios = cloneStrings(relation.Scenarios, maxRawRelationScenarios)
		authorized.Relations = append(authorized.Relations, copy)
	}
	authorized.Sort()
	if err := authorized.Validate(); err != nil {
		return evidence.Graph{}, analyzerFailure("exact_symbol", nil, err)
	}
	return authorized, nil
}

func (s *Service) boundAnalyzerGraph(graph evidence.Graph) (evidence.Graph, error) {
	if len(graph.Entities) > maxRawGraphEntities ||
		len(graph.Relations) > maxRawGraphRelations ||
		len(graph.Scenarios) > maxRawGraphScenarios ||
		len(graph.Build.BuildTags) > maxRawBuildTags {
		return evidence.Graph{}, analyzerFailure("exact_symbol", nil, nil)
	}
	for _, scenario := range graph.Scenarios {
		if len(scenario.Build.BuildTags) > maxRawBuildTags {
			return evidence.Graph{}, analyzerFailure("exact_symbol", nil, nil)
		}
	}

	bounded := evidence.Graph{
		Version:   graph.Version,
		RepoPath:  graph.RepoPath,
		Query:     graph.Query,
		Build:     cloneBuildContext(graph.Build),
		Entities:  make([]evidence.Entity, 0, min(len(graph.Entities), maxRawGraphEntities)),
		Relations: make([]evidence.Relation, 0, min(len(graph.Relations), maxRawGraphRelations)),
		Scenarios: cloneRawScenarios(graph.Scenarios, maxRawGraphScenarios),
		Warnings:  cloneStrings(graph.Warnings, maxRawGraphWarnings),
	}
	for _, entity := range graph.Entities {
		bounded.Entities = append(bounded.Entities, cloneEntity(entity))
	}
	for _, relation := range graph.Relations {
		copy := relation
		copy.Provenance = cloneRawProvenance(
			relation.Provenance,
			maxRawRelationProvenance,
		)
		copy.Scenarios = cloneStrings(relation.Scenarios, maxRawRelationScenarios)
		bounded.Relations = append(bounded.Relations, copy)
	}
	return bounded, nil
}

func (s *Service) collectReferences(
	ctx context.Context,
	target evidence.Entity,
	limits Limits,
	result *Result,
) error {
	locationSet, err := s.referenceFinder.References(ctx, s.catalog.AnalysisRoot(), *target.Location)
	if err != nil {
		return analyzerFailure("references", ctx, err)
	}
	locationSet, err = s.boundReferenceLocationSet(locationSet)
	if err != nil {
		return err
	}
	if err := locationSet.Validate(); err != nil {
		return analyzerFailure("references", ctx, err)
	}
	provenance := boundedPrefix(locationSet.Provenance, maxReferenceProvenance)
	scenarios := boundedPrefix(locationSet.Scenarios, maxReferenceScenarios)
	for _, provenance := range provenance {
		if !s.safeProvenance(provenance) {
			return analyzerFailure("references", ctx, nil)
		}
	}
	for _, scenario := range scenarios {
		if !s.safeScenario(scenario) {
			return analyzerFailure("references", ctx, nil)
		}
	}
	references := make([]evidence.Location, 0, limits.MaxReferences)
	tests := make([]evidence.Location, 0, limits.MaxTestReferences)
	for _, location := range sortedUniqueLocations(locationSet.Locations, maxRawReferenceLocations) {
		if !s.authorizedLocation(location) || location.Column <= 0 {
			continue
		}
		if strings.HasSuffix(location.Path, "_test.go") {
			if len(tests) < limits.MaxTestReferences {
				tests = append(tests, location)
			}
			continue
		}
		if len(references) < limits.MaxReferences {
			references = append(references, location)
		}
	}
	base := evidence.LocationSet{
		Certainty:  locationSet.Certainty,
		Provenance: cloneProvenance(provenance, s, maxReferenceProvenance),
		Scenarios:  cloneScenarios(scenarios, maxReferenceScenarios),
	}
	referenceSet := base
	referenceSet.Locations = references
	testSet := base
	testSet.Provenance = cloneProvenance(base.Provenance, s, maxReferenceProvenance)
	testSet.Scenarios = cloneScenarios(base.Scenarios, maxReferenceScenarios)
	testSet.Locations = tests
	if err := referenceSet.Validate(); err != nil {
		return analyzerFailure("references", ctx, err)
	}
	if err := testSet.Validate(); err != nil {
		return analyzerFailure("references", ctx, err)
	}
	result.References = &referenceSet
	result.Tests = &testSet
	result.Provenance = appendProvenance(
		result.Provenance,
		cloneProvenance(base.Provenance, s, maxReferenceProvenance),
		maxAggregateProvenance,
	)
	return nil
}

func (s *Service) boundReferenceLocationSet(
	locationSet evidence.LocationSet,
) (evidence.LocationSet, error) {
	provenance := boundedPrefix(locationSet.Provenance, maxRawReferenceProvenance)
	scenarios := boundedPrefix(locationSet.Scenarios, maxRawReferenceScenarios)
	for _, scenario := range scenarios {
		if len(scenario.Build.BuildTags) > maxRawBuildTags {
			return evidence.LocationSet{}, analyzerFailure("references", nil, nil)
		}
	}
	return evidence.LocationSet{
		Locations:  cloneLocations(locationSet.Locations, maxRawReferenceLocations),
		Certainty:  locationSet.Certainty,
		Provenance: cloneRawProvenance(provenance, maxRawReferenceProvenance),
		Scenarios:  cloneRawScenarios(scenarios, maxRawReferenceScenarios),
	}, nil
}

func cloneScenarios(values []evidence.Scenario, limit int) []evidence.Scenario {
	values = boundedPrefix(values, limit)
	result := make([]evidence.Scenario, 0, limit)
	for _, value := range values {
		copy := evidence.Scenario{
			ID:    value.ID,
			Name:  value.Name,
			Build: cloneBuildContext(value.Build),
		}
		result = append(result, copy)
	}
	return result
}

func sortedUniqueLocations(values []evidence.Location, limit int) []evidence.Location {
	result := cloneLocations(values, limit)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		return result[i].Column < result[j].Column
	})
	write := 0
	for _, location := range result {
		if filepath.IsAbs(filepath.FromSlash(location.Path)) {
			continue
		}
		if write > 0 && result[write-1] == location {
			continue
		}
		result[write] = location
		write++
	}
	return result[:write]
}
