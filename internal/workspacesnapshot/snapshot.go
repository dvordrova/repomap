// Package workspacesnapshot defines the immutable repository authority shared
// by presentation-neutral workspace consumers.
package workspacesnapshot

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

const (
	advisoryMaximumAllowedPaths      = 4096
	advisoryMaximumRepositoryEntries = 20_000
	advisoryMaximumCapturedInputs    = 20_000
	advisoryMaximumStagesPerInput    = 64
	maxPathBytes                     = 4096
	advisoryAuthorityScalarBytes     = 4 * 1024 * 1024
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

// New validates and copies one complete authority without reading the
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
	if err := repositoryShapeBounded(input.Repository); err != nil {
		return err
	}
	for _, captured := range input.CapturedInputs {
		if len(captured.Path) > maxPathBytes {
			return fmt.Errorf("workspace snapshot: captured input path exceeds bounds")
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
	return nil
}

func repositoryShapeBounded(repository freshness.RepositoryState) error {
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

type ScaleWarningKind string

const (
	ScaleWarningAllowedPaths       ScaleWarningKind = "workspace_allowed_paths"
	ScaleWarningCapturedInputs     ScaleWarningKind = "workspace_captured_inputs"
	ScaleWarningDirtyEntries       ScaleWarningKind = "workspace_dirty_entries"
	ScaleWarningSubmodules         ScaleWarningKind = "workspace_submodules"
	ScaleWarningStagesPerInput     ScaleWarningKind = "workspace_stages_per_input"
	ScaleWarningAuthorityTextBytes ScaleWarningKind = "workspace_authority_text_bytes"
)

type ScaleWarning struct {
	Kind                ScaleWarningKind
	AdvisorySize        int
	AffectedCollections int
	MaximumRetained     int
}

// ScaleWarnings reports former aggregate thresholds over complete authority.
// It is diagnostic-only and never participates in Snapshot construction.
func ScaleWarnings(input Input) []ScaleWarning {
	warnings := []ScaleWarning{
		{Kind: ScaleWarningAllowedPaths, AdvisorySize: advisoryMaximumAllowedPaths},
		{Kind: ScaleWarningCapturedInputs, AdvisorySize: advisoryMaximumCapturedInputs},
		{Kind: ScaleWarningDirtyEntries, AdvisorySize: advisoryMaximumRepositoryEntries},
		{Kind: ScaleWarningSubmodules, AdvisorySize: advisoryMaximumRepositoryEntries},
		{Kind: ScaleWarningStagesPerInput, AdvisorySize: advisoryMaximumStagesPerInput},
		{Kind: ScaleWarningAuthorityTextBytes, AdvisorySize: advisoryAuthorityScalarBytes},
	}
	record := func(position, retained int) {
		if retained <= warnings[position].AdvisorySize {
			return
		}
		warnings[position].AffectedCollections++
		if retained > warnings[position].MaximumRetained {
			warnings[position].MaximumRetained = retained
		}
	}
	record(0, len(input.AllowedPaths))
	record(1, len(input.CapturedInputs))
	record(2, len(input.Repository.Dirty))
	record(3, len(input.Repository.Submodules))
	for _, captured := range input.CapturedInputs {
		record(4, len(captured.Stages))
	}
	record(5, authorityScalarBytes(input))
	result := make([]ScaleWarning, 0, len(warnings))
	for _, warning := range warnings {
		if warning.AffectedCollections > 0 {
			result = append(result, warning)
		}
	}
	return result
}

func authorityScalarBytes(input Input) int {
	total := len(input.Repository.Identity) + len(input.Repository.Head) + len(input.AnalysisRoot)
	for _, dirty := range input.Repository.Dirty {
		total += len(dirty.Status) + len(dirty.Path) + len(dirty.FromPath) + len(dirty.Kind) + len(dirty.Mode) + len(dirty.ContentSHA256)
	}
	for _, submodule := range input.Repository.Submodules {
		total += len(submodule.Path) + len(submodule.RecordedGitlink) + len(submodule.CurrentHead) + len(submodule.Availability)
	}
	for _, captured := range input.CapturedInputs {
		total += len(captured.ID) + len(captured.Path) + len(captured.Kind) + len(captured.Mode) + len(captured.ContentSHA256) + len(captured.OwningModuleID) + len(captured.OwningPackage)
		for _, stage := range captured.Stages {
			total += len(stage)
		}
	}
	for _, allowedPath := range input.AllowedPaths {
		total += len(allowedPath)
	}
	return total
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
