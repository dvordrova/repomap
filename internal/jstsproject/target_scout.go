package jstsproject

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
)

const TargetVersion = 1

// Target is the lightweight, exact selected-package authority used before a
// JavaScript/TypeScript page is chosen for execution. It is sealed to the
// shared corpus namespace and deliberately contains no compiler-derived
// language, surface, source, or ProgramIndex facts.
type Target struct {
	Version         int    `json:"version"`
	CorpusSHA256    string `json:"corpus_sha256"`
	Ref             string `json:"ref"`
	Name            string `json:"name"`
	Selector        string `json:"selector"`
	ProjectDir      string `json:"project_dir"`
	ManifestPath    string `json:"manifest_path"`
	ManifestFileRef string `json:"manifest_file_ref"`
}

// Scout selects one package target without invoking Node or the TypeScript
// Compiler API.
func Scout(ctx context.Context, repository *corpus.Corpus) (Target, error) {
	return ScoutSelected(ctx, repository, "")
}

// ScoutSelected selects either the one automatic package target or the exact
// package named by a jsts:<manifest> selector. Selection uses only the shared
// corpus, package manifests, and package-owned source paths; compiler work is
// deferred until this target is actually dispatched as a page.
func ScoutSelected(
	ctx context.Context,
	repository *corpus.Corpus,
	selector string,
) (Target, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Target{}, err
	}
	if repository == nil {
		return Target{}, fmt.Errorf("jsts target scout: corpus is required")
	}
	if err := repository.Snapshot().Validate(); err != nil {
		return Target{}, fmt.Errorf("jsts target scout: corpus: %w", err)
	}

	entries := repository.Entries()
	for _, entry := range entries {
		if corpus.ForbiddenPath(entry.Path) {
			return Target{}, fmt.Errorf("jsts target scout: forbidden corpus path %q", entry.Path)
		}
	}

	selector = strings.TrimSpace(selector)
	var rootManifest *packageManifest
	if selector == "" {
		if _, ok := repository.ID("package.json"); ok {
			manifest, err := readPackageManifest(repository, "package.json")
			if err != nil {
				return Target{}, err
			}
			rootManifest = &manifest
		}
	}
	manifestPath, projectDir, err := selectProjectManifest(entries, selector, rootManifest)
	if err != nil {
		return Target{}, err
	}
	manifestID, ok := repository.ID(manifestPath)
	if !ok {
		return Target{}, fmt.Errorf("jsts target scout: selected package manifest is unavailable")
	}

	var manifest packageManifest
	if manifestPath == "package.json" && rootManifest != nil {
		manifest = *rootManifest
	} else {
		manifest, err = readPackageManifest(repository, manifestPath)
		if err != nil {
			return Target{}, err
		}
	}

	ref := "project:root-package"
	if projectDir != "." {
		ref = "project:package:" + string(manifestID)
	}
	target := Target{
		Version:         TargetVersion,
		CorpusSHA256:    repository.SHA256(),
		Ref:             ref,
		Name:            selectedPackageIdentityName(repository, projectDir, manifest.Name),
		Selector:        "jsts:" + manifestPath,
		ProjectDir:      projectDir,
		ManifestPath:    manifestPath,
		ManifestFileRef: string(manifestID),
	}
	if err := target.ValidateAgainst(repository); err != nil {
		return Target{}, err
	}
	return target, nil
}

// TargetFromResult projects the exact pre-compiler target identity from a
// fully validated adapter result. Callers can compare this comparable value to
// the scouted target without duplicating identity rules in orchestration.
func TargetFromResult(result Result) (Target, error) {
	if err := result.Validate(); err != nil {
		return Target{}, fmt.Errorf("jsts target from result: %w", err)
	}
	target := Target{
		Version:         TargetVersion,
		CorpusSHA256:    result.CorpusSHA256,
		Ref:             result.Project.Ref,
		Name:            result.Project.Name,
		Selector:        result.Project.Selector,
		ProjectDir:      path.Dir(result.Project.ManifestPath),
		ManifestPath:    result.Project.ManifestPath,
		ManifestFileRef: result.Project.ManifestFileRef,
	}
	if err := target.Validate(); err != nil {
		return Target{}, fmt.Errorf("jsts target from result: %w", err)
	}
	return target, nil
}

// Validate rejects unversioned, unsealed, or internally inconsistent target
// identities without reading repository files.
func (target Target) Validate() error {
	if target.Version != TargetVersion || !validSHA(target.CorpusSHA256) {
		return fmt.Errorf("jsts target: invalid producer identity")
	}
	if canonicalProjectIdentityName(target.Name) != target.Name ||
		!safeRepositoryPath(target.ManifestPath) || path.Base(target.ManifestPath) != "package.json" ||
		target.ProjectDir != path.Dir(target.ManifestPath) ||
		(target.ProjectDir != "." && !safeRepositoryPath(target.ProjectDir)) ||
		target.Selector != "jsts:"+target.ManifestPath || !validTargetFileRef(target.ManifestFileRef) {
		return fmt.Errorf("jsts target: invalid selected project")
	}
	wantRef := "project:root-package"
	if target.ProjectDir != "." {
		wantRef = "project:package:" + target.ManifestFileRef
	}
	if target.Ref != wantRef {
		return fmt.Errorf("jsts target: project ref binding mismatch")
	}
	return nil
}

// ValidateAgainst binds the target to the exact shared corpus namespace and
// manifest FileID. File contents may still change during an ordinary run; the
// product deliberately has no freshness gate.
func (target Target) ValidateAgainst(repository *corpus.Corpus) error {
	if err := target.Validate(); err != nil {
		return err
	}
	if repository == nil {
		return fmt.Errorf("jsts target: corpus is required")
	}
	if err := repository.Snapshot().Validate(); err != nil {
		return fmt.Errorf("jsts target: corpus: %w", err)
	}
	if target.CorpusSHA256 != repository.SHA256() {
		return fmt.Errorf("jsts target: corpus identity mismatch")
	}
	manifestID, ok := repository.ID(target.ManifestPath)
	if !ok || string(manifestID) != target.ManifestFileRef {
		return fmt.Errorf("jsts target: manifest FileRef binding mismatch")
	}
	return nil
}

// ValidateMaterialization checks that a compiler-backed result still owns the
// exact package selected by the scout. Name is deliberately excluded from the
// identity comparison: it is derived from package.json or a lockfile whose
// contents may change during an ordinary run, and repository changes are not a
// freshness gate.
func (target Target) ValidateMaterialization(materialized Target) error {
	if err := target.Validate(); err != nil {
		return fmt.Errorf("scout target: %w", err)
	}
	if err := materialized.Validate(); err != nil {
		return fmt.Errorf("materialized target: %w", err)
	}
	want := target
	want.Name = materialized.Name
	if want != materialized {
		return fmt.Errorf("materialized project does not match the selected scout package")
	}
	return nil
}

func validTargetFileRef(value string) bool {
	if len(value) < 2 || value[0] != 'f' || value[1] == '0' {
		return false
	}
	for _, character := range value[1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
