package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/memory"
	"github.com/dvordrova/repomap/internal/sourceexplain"
)

func reconcileSessionRepository(
	ctx context.Context,
	session investigation.Session,
) (investigation.Session, freshness.RepositoryState, bool, error) {
	repository, err := freshness.CaptureRepository(ctx, session.Repository.Path)
	if err != nil {
		return session, freshness.RepositoryState{}, false, err
	}
	if repository.Identity != session.Repository.Path {
		return session, freshness.RepositoryState{}, false, fmt.Errorf("repository identity changed from %q to %q", session.Repository.Path, repository.Identity)
	}
	revision, err := repository.Digest()
	if err != nil {
		return session, freshness.RepositoryState{}, false, err
	}
	if revision == session.Repository.Revision {
		return session, repository, false, nil
	}
	next, _, err := investigation.Reduce(session, investigation.Event{
		Kind:     investigation.EventRepositoryChanged,
		Revision: revision,
	})
	if err != nil {
		return session, freshness.RepositoryState{}, false, err
	}
	return next, repository, true, nil
}

func buildRunCheckpoint(
	ctx context.Context,
	cfg config,
	session investigation.Session,
	repository freshness.RepositoryState,
	facts *freshness.FactContext,
	artifacts runArtifacts,
) (memory.Input, error) {
	input := memory.Input{
		Session:    session,
		Repository: repository,
		Facts:      facts,
	}
	if session.SourceReport == nil {
		return input, nil
	}
	claims, err := claimContextForRun(ctx, cfg, repository, facts, artifacts)
	if err != nil {
		return memory.Input{}, err
	}
	claims.FactDigest = ""
	input.Claims = &claims
	return input, nil
}

func claimContextForRun(
	ctx context.Context,
	cfg config,
	repository freshness.RepositoryState,
	facts *freshness.FactContext,
	artifacts runArtifacts,
) (freshness.ClaimContext, error) {
	if artifacts.sourceProvider != "" || artifacts.sourceModel != "" || artifacts.sourcePromptVersion != "" {
		context := freshness.ClaimContext{
			Version:          freshness.ClaimContextVersion,
			Provider:         artifacts.sourceProvider,
			Model:            artifacts.sourceModel,
			PromptVersion:    artifacts.sourcePromptVersion,
			ParserVersion:    sourceexplain.ParserVersion,
			EvaluatorVersion: sourceexplain.EvaluationVersion,
		}
		if context.Provider == "" || context.Model == "" || context.PromptVersion == "" {
			return freshness.ClaimContext{}, fmt.Errorf("model claim provenance is incomplete")
		}
		return context, nil
	}
	if cfg.resumePath == "" || facts == nil {
		return freshness.ClaimContext{}, fmt.Errorf("saved model claim provenance is unavailable")
	}
	data, err := readBoundedFile("investigation session", cfg.resumePath)
	if err != nil {
		return freshness.ClaimContext{}, err
	}
	var header struct {
		MemoryVersion *int `json:"memory_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return freshness.ClaimContext{}, fmt.Errorf("decode investigation session header: %w", err)
	}
	if header.MemoryVersion == nil {
		return freshness.ClaimContext{}, fmt.Errorf("legacy session has no versioned model claim provenance")
	}
	currentClaims := freshness.ClaimContext{
		Version:          freshness.ClaimContextVersion,
		PromptVersion:    deepseek.SourcePromptVersionJSON,
		ParserVersion:    sourceexplain.ParserVersion,
		EvaluatorVersion: sourceexplain.EvaluationVersion,
	}
	record, err := memory.Load(cfg.resumePath, memory.Current{
		Repository: repository,
		Facts:      facts,
		Claims:     &currentClaims,
	})
	if err != nil {
		return freshness.ClaimContext{}, err
	}
	if len(record.Changes) > 0 || record.Claims == nil {
		return freshness.ClaimContext{}, fmt.Errorf("saved model claims are no longer reusable")
	}
	return *record.Claims, nil
}

func sessionHasFacts(session investigation.Session) bool {
	return session.Symbol != nil || session.Source != nil || session.Assessment != nil || session.Tests != nil
}

func sessionNeedsFactContext(session investigation.Session) bool {
	if sessionHasFacts(session) {
		return true
	}
	if len(session.Next) != 1 {
		return false
	}
	switch session.Next[0].Kind {
	case investigation.ActionResolveSymbol, investigation.ActionReadSource, investigation.ActionFindTests, investigation.ActionFindTestReferences:
		return true
	default:
		return false
	}
}

func formatFreshnessDifferences(differences []freshness.Difference) string {
	values := make([]string, 0, len(differences))
	for _, difference := range differences {
		values = append(values, difference.String())
	}
	return strings.Join(values, "; ")
}
