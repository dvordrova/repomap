package main

import (
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
)

// buildJSTSProgramForRun projects the already selected sealed adapter result.
// Selection owns the run's one compiler pass; repository drift does not cause
// a second parse or replace that exact project authority before publication.
func buildJSTSProgramForRun(
	runDir string,
	repository *corpus.Corpus,
	selection jsTSTargetRunSelection,
	output *runOutput,
) (jstsproject.Result, programindex.Index, dependencies.Catalog, error) {
	started := time.Now()
	result, index, catalog, err := prepareJSTSProgramForRun(repository, selection)
	if err != nil {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{}, err
	}
	if err := jstsproject.PersistAll(runDir, result, index, catalog); err != nil {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{}, err
	}
	reportJSTSProgramForRun(
		output,
		result,
		index,
		time.Since(started),
		"semantic authority: default",
		"artifacts: "+jstsproject.ArtifactFilename+", "+jstsproject.ProgramIndexFilename,
	)
	return result, index, catalog, nil
}

func prepareJSTSProgramForRun(
	repository *corpus.Corpus,
	selection jsTSTargetRunSelection,
) (jstsproject.Result, programindex.Index, dependencies.Catalog, error) {
	if repository == nil {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{},
			fmt.Errorf("JavaScript/TypeScript program build: repository corpus is unavailable")
	}
	result := selection.Project.Snapshot()
	if err := result.Validate(); err != nil {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{},
			fmt.Errorf("validate JavaScript/TypeScript project authority: %w", err)
	}
	if _, err := validateJSTSProjectCorpusBinding(repository, result); err != nil {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{},
			fmt.Errorf("bind selected JavaScript/TypeScript project authority: %w", err)
	}
	if result.Project.Ref != selection.Outcome.SelectedRef {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{},
			fmt.Errorf("JavaScript/TypeScript package project does not match its selected exact project authority")
	}
	index, catalog, err := jstsproject.BuildFromResult(result)
	if err != nil {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{},
			fmt.Errorf("project JavaScript/TypeScript program: %w", err)
	}
	if err := jstsproject.ValidateProgramIndex(result, index); err != nil {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{},
			fmt.Errorf("validate JavaScript/TypeScript ProgramIndex authority: %w", err)
	}
	if result.ProgramTargetID != index.Target.ID || result.Project.Language != index.Target.Language ||
		result.Project.Selector != index.Target.Selector {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{},
			fmt.Errorf("JavaScript/TypeScript project and ProgramTarget identity disagree")
	}
	if err := catalog.Validate(); err != nil {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{},
			fmt.Errorf("validate JavaScript/TypeScript dependency authority: %w", err)
	}
	if catalog.Coverage.State != dependencies.CoverageComplete {
		return jstsproject.Result{}, programindex.Index{}, dependencies.Catalog{},
			fmt.Errorf(
				"JavaScript/TypeScript dependency authority is incomplete (%d unresolved imports); resolve the owner-prepared project imports before analysis",
				len(catalog.Coverage.Omissions),
			)
	}
	return result, index, catalog, nil
}

func reportJSTSProgramForRun(
	output *runOutput,
	result jstsproject.Result,
	index programindex.Index,
	elapsed time.Duration,
	authorityDetail string,
	artifactDetail string,
) {
	if output != nil {
		output.State(
			"JavaScript/TypeScript program index", "ready",
			"language: "+index.Target.Language,
			fmt.Sprintf("objects: %d", len(index.Objects)),
			fmt.Sprintf("relations: %d", len(index.Relations)),
			fmt.Sprintf("product surfaces: %d", jsTSProductSurfaceCount(result)),
			fmt.Sprintf("all classified surfaces: %d", len(result.Surfaces)),
			fmt.Sprintf("HTTP routes: %d", len(result.Routes)),
			fmt.Sprintf("client HTTP uses: %d", len(result.HTTPUses)),
			fmt.Sprintf("cross-surface paths: %d", len(result.ProductPaths)),
			authorityDetail,
			formatRunOutputWallDuration(elapsed),
			artifactDetail,
		)
	}
}
