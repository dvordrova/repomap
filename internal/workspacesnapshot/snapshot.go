// Package workspacesnapshot defines the immutable repository authority shared
// by presentation-neutral workspace consumers.
package workspacesnapshot

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

const (
	maxAllowedPaths         = 4096
	maxRepositoryEntries    = 20_000
	maxCapturedInputs       = 20_000
	maxStagesPerInput       = 64
	maxPathBytes            = 4096
	maxAuthorityScalarBytes = 4 * 1024 * 1024
)

// Input is the complete presentation-neutral authority required to construct
// one immutable workspace snapshot.
type Input struct {
	AnalysisRoot   string
	Repository     freshness.RepositoryState
	CapturedInputs []freshness.CapturedInput
	AllowedPaths   []string
}

// Snapshot is one immutable, run-scoped source authority. New validates the
// repository and captured-input identities before sealing its source catalog.
type Snapshot struct {
	repository   freshness.RepositoryState
	analysisRoot string
	catalog      sourcecatalog.Catalog
}

// New validates and copies one bounded authority without reading the
// filesystem or invoking Git.
func New(input Input) (Snapshot, error) {
	if err := inputBounded(input); err != nil {
		return Snapshot{}, err
	}
	if err := input.Repository.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("workspace snapshot: repository state is invalid")
	}
	if _, err := input.Repository.Digest(); err != nil {
		return Snapshot{}, fmt.Errorf("workspace snapshot: repository digest is unavailable")
	}
	if _, err := freshness.CapturedInputsDigest(input.CapturedInputs); err != nil {
		return Snapshot{}, fmt.Errorf("workspace snapshot: captured input authority is invalid")
	}

	repository := cloneRepositoryState(input.Repository)
	capturedInputs := cloneCapturedInputs(input.CapturedInputs)
	allowedPaths := append([]string(nil), input.AllowedPaths...)
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: repository.Identity,
		AnalysisRoot:   input.AnalysisRoot,
		AllowedPaths:   allowedPaths,
		CapturedInputs: capturedInputs,
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("workspace snapshot: source authority is invalid")
	}
	return Snapshot{
		repository:   repository,
		analysisRoot: catalog.AnalysisRoot(),
		catalog:      catalog,
	}, nil
}

// RepositoryRoot returns the canonical repository identity.
func (snapshot Snapshot) RepositoryRoot() string {
	return snapshot.repository.Identity
}

// AnalysisRoot returns the canonical root for analysis-relative source paths.
func (snapshot Snapshot) AnalysisRoot() string {
	return snapshot.analysisRoot
}

// Catalog returns the immutable authorized source catalog.
func (snapshot Snapshot) Catalog() sourcecatalog.Catalog {
	return snapshot.catalog
}

func inputBounded(input Input) error {
	if len(input.AllowedPaths) > maxAllowedPaths {
		return fmt.Errorf("workspace snapshot: allowed source authority exceeds bounds")
	}
	if len(input.CapturedInputs) > maxCapturedInputs {
		return fmt.Errorf("workspace snapshot: captured input authority exceeds bounds")
	}
	if err := repositoryShapeBounded(input.Repository); err != nil {
		return err
	}
	for _, captured := range input.CapturedInputs {
		if len(captured.Path) > maxPathBytes {
			return fmt.Errorf("workspace snapshot: captured input path exceeds bounds")
		}
		if len(captured.Stages) > maxStagesPerInput {
			return fmt.Errorf("workspace snapshot: captured input stages exceed bounds")
		}
	}
	if len(input.AnalysisRoot) > maxPathBytes {
		return fmt.Errorf("workspace snapshot: analysis root exceeds bounds")
	}
	for _, allowedPath := range input.AllowedPaths {
		if len(allowedPath) > maxPathBytes {
			return fmt.Errorf("workspace snapshot: allowed source path exceeds bounds")
		}
	}
	budget := scalarByteBudget{remaining: maxAuthorityScalarBytes}
	if !consumeRepositoryScalars(&budget, input.Repository) ||
		!budget.consume(input.AnalysisRoot) {
		return fmt.Errorf("workspace snapshot: scalar authority exceeds bounds")
	}
	for _, captured := range input.CapturedInputs {
		if !consumeCapturedInputScalars(&budget, captured) {
			return fmt.Errorf("workspace snapshot: scalar authority exceeds bounds")
		}
	}
	for _, allowedPath := range input.AllowedPaths {
		if !budget.consume(allowedPath) {
			return fmt.Errorf("workspace snapshot: scalar authority exceeds bounds")
		}
	}
	return nil
}

func repositoryShapeBounded(repository freshness.RepositoryState) error {
	if len(repository.Dirty) > maxRepositoryEntries {
		return fmt.Errorf("workspace snapshot: repository dirty state exceeds bounds")
	}
	if len(repository.Submodules) > maxRepositoryEntries {
		return fmt.Errorf("workspace snapshot: repository submodule state exceeds bounds")
	}
	if len(repository.Identity) > maxPathBytes {
		return fmt.Errorf("workspace snapshot: repository root exceeds bounds")
	}
	for _, dirty := range repository.Dirty {
		if len(dirty.Path) > maxPathBytes || len(dirty.FromPath) > maxPathBytes {
			return fmt.Errorf("workspace snapshot: repository dirty path exceeds bounds")
		}
	}
	for _, submodule := range repository.Submodules {
		if len(submodule.Path) > maxPathBytes {
			return fmt.Errorf("workspace snapshot: repository submodule path exceeds bounds")
		}
	}
	return nil
}

type scalarByteBudget struct {
	remaining int
}

func (budget *scalarByteBudget) consume(values ...string) bool {
	for _, value := range values {
		if len(value) > budget.remaining {
			return false
		}
		budget.remaining -= len(value)
	}
	return true
}

func consumeRepositoryScalars(
	budget *scalarByteBudget,
	repository freshness.RepositoryState,
) bool {
	if !budget.consume(repository.Identity, repository.Head) {
		return false
	}
	for _, dirty := range repository.Dirty {
		if !budget.consume(
			dirty.Status,
			dirty.Path,
			dirty.FromPath,
			string(dirty.Kind),
			dirty.Mode,
			dirty.ContentSHA256,
		) {
			return false
		}
	}
	for _, submodule := range repository.Submodules {
		if !budget.consume(
			submodule.Path,
			submodule.RecordedGitlink,
			submodule.CurrentHead,
			string(submodule.Availability),
		) {
			return false
		}
	}
	return true
}

func consumeCapturedInputScalars(
	budget *scalarByteBudget,
	input freshness.CapturedInput,
) bool {
	if !budget.consume(
		input.ID,
		input.Path,
		string(input.Kind),
		input.Mode,
		input.ContentSHA256,
		input.OwningModuleID,
		input.OwningPackage,
	) {
		return false
	}
	for _, stage := range input.Stages {
		if !budget.consume(stage) {
			return false
		}
	}
	return true
}

func cloneRepositoryState(repository freshness.RepositoryState) freshness.RepositoryState {
	cloned := repository
	cloned.Dirty = cloneSlice(repository.Dirty)
	cloned.Submodules = cloneSlice(repository.Submodules)
	return cloned
}

func cloneCapturedInputs(inputs []freshness.CapturedInput) []freshness.CapturedInput {
	if inputs == nil {
		return nil
	}
	cloned := make([]freshness.CapturedInput, len(inputs))
	for index := range inputs {
		cloned[index] = inputs[index]
		cloned[index].Stages = cloneSlice(inputs[index].Stages)
	}
	return cloned
}

func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append([]T{}, values...)
}
