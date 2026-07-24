package inspection

import (
	"context"

	"github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
)

type ResolveRequest struct {
	Location  evidence.Location
	RankTerms []string
	Limits    Limits
}

type Candidate struct {
	Entity      evidence.Entity
	Match       string
	Certainty   evidence.Certainty
	Distance    int
	RankReasons []string
}

type ResolveResult struct {
	Location   evidence.Location
	Candidates []Candidate
	Certainty  evidence.Certainty
	Provenance evidence.Provenance
	Warnings   []string
}

// Resolve returns only callable, investigable declarations in the exact
// authorized source file.
func (s *Service) Resolve(ctx context.Context, request ResolveRequest) (ResolveResult, error) {
	if ctx == nil || s == nil || !s.authorizedLocation(request.Location) {
		kind := ErrorInvalidRequest
		if s != nil && request.Location.Path != "" {
			kind = ErrorUnauthorized
		}
		return ResolveResult{}, inspectionError(kind, "resolve", nil)
	}
	if s.resolver == nil {
		return ResolveResult{}, inspectionError(ErrorAnalyzerUnavailable, "resolve", nil)
	}
	limits, err := normalizeLimits(request.Limits)
	if err != nil {
		return ResolveResult{}, err
	}
	source, ok := s.catalog.Lookup(request.Location.Path)
	if !ok {
		return ResolveResult{}, inspectionError(ErrorUnauthorized, "resolve", nil)
	}
	if err := verifyCurrentSourceHash(
		s.catalog.AnalysisRoot(),
		source.Path,
		source.ContentSHA256,
		0,
	); err != nil {
		return ResolveResult{}, err
	}
	rankTerms := make([]string, 0, limits.MaxRankTerms)
	for _, term := range boundedPrefix(request.RankTerms, maxRawRankTerms) {
		if len(rankTerms) == limits.MaxRankTerms {
			break
		}
		if safeText(term, s.catalog.AnalysisRoot(), 64) {
			rankTerms = append(rankTerms, term)
		}
	}
	resolution, err := s.resolver.ResolveLocation(ctx, analyzer.LocationRequest{
		RepoPath:      s.catalog.AnalysisRoot(),
		Location:      request.Location,
		MaxCandidates: limits.MaxResolverCandidates,
		RankTerms:     rankTerms,
	})
	if err != nil {
		return ResolveResult{}, analyzerFailure("resolve", ctx, err)
	}
	if resolution.Location.Path != request.Location.Path ||
		resolution.Location.Line != request.Location.Line ||
		!s.authorizedLocation(resolution.Location) ||
		!resolution.Certainty.Valid() ||
		!s.safeProvenance(resolution.Provenance) {
		return ResolveResult{}, analyzerFailure("resolve", ctx, nil)
	}

	result := ResolveResult{
		Location:  resolution.Location,
		Certainty: resolution.Certainty,
		Provenance: evidence.Provenance{
			Provider:  resolution.Provenance.Provider,
			Version:   resolution.Provenance.Version,
			Operation: resolution.Provenance.Operation,
		},
		Candidates: make([]Candidate, 0, limits.MaxCandidates),
	}
	if resolution.Provenance.Location != nil && s.authorizedLocation(*resolution.Provenance.Location) {
		result.Provenance.Location = cloneLocation(resolution.Provenance.Location)
	}
	rawCandidateLimit := min(maxRawResolverCandidates, limits.MaxResolverCandidates)
	for _, candidate := range boundedPrefix(resolution.Candidates, rawCandidateLimit) {
		if len(result.Candidates) == limits.MaxCandidates {
			break
		}
		location := candidate.Entity.Location
		if !candidate.Investigable || location == nil ||
			(candidate.Entity.Kind != evidence.EntityFunction && candidate.Entity.Kind != evidence.EntityMethod) ||
			location.Path != request.Location.Path || location.Line <= 0 || location.Column <= 0 ||
			!s.authorizedLocation(*location) || !candidate.Certainty.Valid() ||
			!safeText(candidate.Entity.Name, s.catalog.AnalysisRoot(), 256) ||
			(candidate.Entity.ID != "" && !safeText(candidate.Entity.ID, s.catalog.AnalysisRoot(), 512)) ||
			(candidate.Entity.Language != "" && !safeText(candidate.Entity.Language, s.catalog.AnalysisRoot(), 32)) ||
			(candidate.Match != "" && !safeText(candidate.Match, s.catalog.AnalysisRoot(), 64)) {
			continue
		}
		rankReasons := make([]string, 0, maxRankReasons)
		for _, reason := range boundedPrefix(candidate.RankReasons, maxRawCandidateRankReasons) {
			if len(rankReasons) == maxRankReasons {
				break
			}
			if safeText(reason, s.catalog.AnalysisRoot(), 256) {
				rankReasons = append(rankReasons, reason)
			}
		}
		result.Candidates = append(result.Candidates, Candidate{
			Entity:      cloneEntity(candidate.Entity),
			Match:       candidate.Match,
			Certainty:   candidate.Certainty,
			Distance:    candidate.Distance,
			RankReasons: rankReasons,
		})
	}
	result.Warnings = make([]string, 0, maxResolverWarnings)
	for _, warning := range boundedPrefix(resolution.Warnings, maxRawResolverWarnings) {
		if safeText(warning, s.catalog.AnalysisRoot(), 256) {
			result.Warnings = append(result.Warnings, warning)
		}
		if len(result.Warnings) == maxResolverWarnings {
			break
		}
	}
	if len(result.Candidates) == 0 {
		return ResolveResult{}, inspectionError(ErrorNotFound, "resolve", nil)
	}
	return result, nil
}
