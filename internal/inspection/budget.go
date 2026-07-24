package inspection

import (
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/symbol"
)

// Raw analyzer-input budgets apply before validation, sorting, filtering, or
// inspection-owned allocation. High-level graph excess is rejected because a
// prefix can break entity/relation integrity. Metadata and navigation samples
// use deterministic prefix truncation because their public projections are
// already explicitly bounded.
const (
	maxRawRankTerms            = 64
	maxRawResolverCandidates   = maxResolverCandidates
	maxRawCandidateRankReasons = 32
	maxRawResolverWarnings     = 32
	maxRawGraphEntities        = 256
	maxRawGraphRelations       = 512
	maxRawGraphScenarios       = 16
	maxRawGraphWarnings        = 32
	maxRawBuildTags            = 32
	maxRawRelationProvenance   = 8
	maxRawRelationScenarios    = 16
	maxRawReferenceLocations   = 256
	maxRawReferenceProvenance  = maxReferenceProvenance
	maxRawReferenceScenarios   = maxReferenceScenarios
	maxStructuralWarnings      = maxRawGraphWarnings + 2
	maxStructuralAllowedPaths  = 2 + 2*(5+5)
)

func boundedPrefix[T any](values []T, limit int) []T {
	if limit < 0 {
		limit = 0
	}
	if len(values) > limit {
		return values[:limit]
	}
	return values
}

func cloneLocations(values []evidence.Location, limit int) []evidence.Location {
	values = boundedPrefix(values, limit)
	result := make([]evidence.Location, len(values))
	copy(result, values)
	return result
}

func cloneStrings(values []string, limit int) []string {
	values = boundedPrefix(values, limit)
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func cloneWithCapacity[T any](values []T, limit int) []T {
	if values == nil {
		return nil
	}
	values = boundedPrefix(values, limit)
	result := make([]T, 0, limit)
	return append(result, values...)
}

func cloneBuildContext(value evidence.BuildContext) evidence.BuildContext {
	return evidence.BuildContext{
		GOOS:      value.GOOS,
		GOARCH:    value.GOARCH,
		BuildTags: cloneStrings(value.BuildTags, maxRawBuildTags),
	}
}

func cloneRawProvenance(values []evidence.Provenance, limit int) []evidence.Provenance {
	values = boundedPrefix(values, limit)
	result := make([]evidence.Provenance, 0, limit)
	for _, value := range values {
		result = append(result, evidence.Provenance{
			Provider:  value.Provider,
			Version:   value.Version,
			Operation: value.Operation,
			Location:  cloneLocation(value.Location),
		})
	}
	return result
}

func cloneRawScenarios(values []evidence.Scenario, limit int) []evidence.Scenario {
	values = boundedPrefix(values, limit)
	result := make([]evidence.Scenario, 0, limit)
	for _, value := range values {
		result = append(result, evidence.Scenario{
			ID:    value.ID,
			Name:  value.Name,
			Build: cloneBuildContext(value.Build),
		})
	}
	return result
}

func boundStructuralBundle(value symbol.Bundle, limits Limits) symbol.Bundle {
	value.Candidates = cloneWithCapacity(value.Candidates, limits.Symbol.MaxCandidates)
	for index := range value.Candidates {
		boundStructuralFact(&value.Candidates[index], limits.Symbol.MaxProvenancePerFact)
	}
	value.IncomingCalls = cloneWithCapacity(
		value.IncomingCalls,
		limits.Symbol.MaxIncomingCalls,
	)
	for index := range value.IncomingCalls {
		boundStructuralCall(&value.IncomingCalls[index], limits.Symbol.MaxProvenancePerFact)
	}
	value.OutgoingCalls = cloneWithCapacity(
		value.OutgoingCalls,
		limits.Symbol.MaxOutgoingCalls,
	)
	for index := range value.OutgoingCalls {
		boundStructuralCall(&value.OutgoingCalls[index], limits.Symbol.MaxProvenancePerFact)
	}
	value.Scenarios = cloneWithCapacity(value.Scenarios, maxRawGraphScenarios)
	for index := range value.Scenarios {
		value.Scenarios[index].Build.BuildTags = cloneWithCapacity(
			value.Scenarios[index].Build.BuildTags,
			maxRawBuildTags,
		)
	}
	value.AllowedPaths = cloneWithCapacity(value.AllowedPaths, maxStructuralAllowedPaths)
	value.Warnings = cloneWithCapacity(value.Warnings, maxStructuralWarnings)
	boundStructuralFact(&value.Target, limits.Symbol.MaxProvenancePerFact)
	return value
}

func boundStructuralFact(value *symbol.Fact, maxProvenance int) {
	value.Provenance = cloneWithCapacity(value.Provenance, maxProvenance)
	value.Scenarios = cloneWithCapacity(value.Scenarios, maxRawRelationScenarios)
}

func boundStructuralCall(value *symbol.CallFact, maxProvenance int) {
	value.Provenance = cloneWithCapacity(value.Provenance, maxProvenance)
	value.Scenarios = cloneWithCapacity(value.Scenarios, maxRawRelationScenarios)
}
