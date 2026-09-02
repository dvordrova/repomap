package main

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
)

// buildJSTSRepositoryProgramInput projects one short-lived, compiler-backed
// Result through the same atomic ProgramIndex boundary as every other language
// adapter. The compiler result is never copied into page or report authority.
func buildJSTSRepositoryProgramInput(
	request repositoryProgramBuildRequest,
) (programindex.Input, error) {
	result, err := jstsRepositoryProgramFacts(request.Target, request.Facts)
	if err != nil {
		return programindex.Input{}, err
	}
	if _, err := validateJSTSProjectCorpusBinding(request.Corpus, result); err != nil {
		return programindex.Input{}, fmt.Errorf("bind JavaScript/TypeScript compiler facts: %w", err)
	}
	input, err := jstsproject.BuildInputFromResult(result)
	if err != nil {
		return programindex.Input{}, fmt.Errorf("project JavaScript/TypeScript program input: %w", err)
	}
	return input, nil
}

func buildJSTSRepositoryDependencies(
	request repositoryDependencyBuildRequest,
) (dependencies.Catalog, error) {
	result, err := jstsRepositoryProgramFacts(request.Target, request.Facts)
	if err != nil {
		return dependencies.Catalog{}, err
	}
	if err := jstsproject.ValidateProgramIndex(result, request.ProgramIndex); err != nil {
		return dependencies.Catalog{}, fmt.Errorf(
			"validate JavaScript/TypeScript ProgramIndex authority: %w",
			err,
		)
	}
	catalog, err := jstsproject.BuildDependenciesFromResult(result)
	if err != nil {
		return dependencies.Catalog{}, fmt.Errorf(
			"project JavaScript/TypeScript dependencies: %w",
			err,
		)
	}
	if catalog.Coverage.State != dependencies.CoverageComplete {
		return dependencies.Catalog{}, fmt.Errorf(
			"JavaScript/TypeScript dependency authority is incomplete (%d unresolved imports); resolve the owner-prepared project imports before analysis",
			len(catalog.Coverage.Omissions),
		)
	}
	return catalog, nil
}

func jstsRepositoryProgramFacts(
	target repositoryTypedTarget,
	facts any,
) (jstsproject.Result, error) {
	result, ok := facts.(jstsproject.Result)
	selected, selectedOK := repositoryJSTSTarget(target)
	if !ok || !selectedOK {
		return jstsproject.Result{}, fmt.Errorf("invalid JavaScript/TypeScript compiler fact snapshot")
	}
	if err := result.Validate(); err != nil {
		return jstsproject.Result{}, fmt.Errorf("validate JavaScript/TypeScript compiler facts: %w", err)
	}
	materialized, err := jstsproject.TargetFromResult(result)
	if err != nil {
		return jstsproject.Result{}, fmt.Errorf("restore materialized JavaScript/TypeScript target: %w", err)
	}
	if selected != materialized {
		return jstsproject.Result{}, fmt.Errorf(
			"JavaScript/TypeScript compiler facts do not match the exact selected target",
		)
	}
	return result, nil
}
