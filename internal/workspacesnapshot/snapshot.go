// Package workspacesnapshot defines the immutable repository authority shared
// by presentation-neutral workspace consumers.
package workspacesnapshot

import (
	"context"
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
	unavailableDiagnostic   = "current repository state is outside authorized bounds"
	uninitializedDiagnostic = "workspace snapshot is unavailable"
)

// Input is the complete presentation-neutral authority required to construct
// one immutable workspace snapshot.
type Input struct {
	AnalysisRoot   string
	Repository     freshness.RepositoryState
	CapturedInputs []freshness.CapturedInput
	AllowedPaths   []string
}

// Snapshot is one immutable, run-scoped repository authority. Its catalog and
// digests use the existing sourcecatalog and freshness formulas.
type Snapshot struct {
	repository           freshness.RepositoryState
	analysisRoot         string
	capturedInputs       []freshness.CapturedInput
	repositoryDigest     string
	capturedInputsDigest string
	catalog              sourcecatalog.Catalog
	initialized          bool
}

// New validates and copies one bounded authority without reading the
// filesystem or invoking Git.
func New(input Input) (Snapshot, error) {
	if err := inputCountsBounded(input); err != nil {
		return Snapshot{}, err
	}
	if err := input.Repository.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("workspace snapshot: repository state is invalid")
	}
	repositoryDigest, err := input.Repository.Digest()
	if err != nil {
		return Snapshot{}, fmt.Errorf("workspace snapshot: repository digest is unavailable")
	}
	capturedInputsDigest, err := freshness.CapturedInputsDigest(input.CapturedInputs)
	if err != nil {
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
		repository:           repository,
		analysisRoot:         catalog.AnalysisRoot(),
		capturedInputs:       capturedInputs,
		repositoryDigest:     repositoryDigest,
		capturedInputsDigest: capturedInputsDigest,
		catalog:              catalog,
		initialized:          true,
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

// Revision returns the selected repository revision.
func (snapshot Snapshot) Revision() string {
	return snapshot.repository.Head
}

// RepositoryDigest returns the existing canonical repository-state digest.
func (snapshot Snapshot) RepositoryDigest() string {
	return snapshot.repositoryDigest
}

// CapturedInputsDigest returns the existing canonical captured-input digest.
func (snapshot Snapshot) CapturedInputsDigest() string {
	return snapshot.capturedInputsDigest
}

// Catalog returns the immutable authorized source catalog.
func (snapshot Snapshot) Catalog() sourcecatalog.Catalog {
	return snapshot.catalog
}

// Assess applies the existing captured-input freshness policy and returns a
// defensive result copy.
func (snapshot Snapshot) Assess(current freshness.RepositoryState) freshness.FreshnessResult {
	if !snapshot.initialized {
		return unavailableResult(uninitializedDiagnostic)
	}
	if err := repositoryCountsBounded(current); err != nil {
		return unavailableResult(unavailableDiagnostic)
	}
	result := freshness.AssessInputs(
		context.Background(),
		snapshot.repository,
		current,
		snapshot.capturedInputs,
	)
	return cloneFreshnessResult(result)
}

// Verify accepts only fresh or unrelated current repository changes.
func (snapshot Snapshot) Verify(current freshness.RepositoryState) error {
	return verifyFreshnessResult(snapshot.Assess(current))
}

func inputCountsBounded(input Input) error {
	if len(input.AllowedPaths) > maxAllowedPaths {
		return fmt.Errorf("workspace snapshot: allowed source authority exceeds bounds")
	}
	if len(input.CapturedInputs) > maxCapturedInputs {
		return fmt.Errorf("workspace snapshot: captured input authority exceeds bounds")
	}
	if err := repositoryCountsBounded(input.Repository); err != nil {
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
	return nil
}

func repositoryCountsBounded(repository freshness.RepositoryState) error {
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

func verifyFreshnessResult(result freshness.FreshnessResult) error {
	switch result.State {
	case freshness.FreshnessFresh, freshness.FreshnessUnrelatedChanges:
		return nil
	default:
		return fmt.Errorf("workspace snapshot: analyzed inputs are %s", result.State)
	}
}

func unavailableResult(diagnostic string) freshness.FreshnessResult {
	result := freshness.NewFreshnessResult(freshness.FreshnessUnavailable)
	result.Diagnostics = []string{diagnostic}
	return result
}

func cloneRepositoryState(repository freshness.RepositoryState) freshness.RepositoryState {
	cloned := repository
	cloned.Dirty = append([]freshness.DirtyFile(nil), repository.Dirty...)
	cloned.Submodules = append([]freshness.SubmoduleState(nil), repository.Submodules...)
	return cloned
}

func cloneCapturedInputs(inputs []freshness.CapturedInput) []freshness.CapturedInput {
	cloned := make([]freshness.CapturedInput, len(inputs))
	for index := range inputs {
		cloned[index] = inputs[index]
		cloned[index].Stages = append([]string(nil), inputs[index].Stages...)
	}
	return cloned
}

func cloneFreshnessResult(result freshness.FreshnessResult) freshness.FreshnessResult {
	cloned := result
	cloned.AffectedInputIDs = cloneStrings(result.AffectedInputIDs)
	cloned.AffectedPaths = cloneStrings(result.AffectedPaths)
	cloned.AffectedSubmodules = cloneStrings(result.AffectedSubmodules)
	cloned.Diagnostics = cloneStrings(result.Diagnostics)
	return cloned
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}
