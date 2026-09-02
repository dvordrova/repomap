package main

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/jstsproject"
)

type jsTSProjectDiscoverer func(
	context.Context,
	*corpus.Corpus,
	string,
	string,
) (jstsproject.Result, error)

// jsTSTargetScout establishes every exact package-target authority from the
// shared corpus without invoking the TypeScript compiler. Full project
// discovery is execution work and runs only after each target is selected.
type jsTSTargetScout func(
	context.Context,
	*corpus.Corpus,
	string,
) ([]jstsproject.Target, error)

func exactJSTSManifestSelector(override string) string {
	override = strings.TrimSpace(override)
	if strings.HasPrefix(override, "jsts:") {
		return override
	}
	return ""
}

func jsTSOwnerPreparationError(err error) bool {
	return errors.Is(err, jstsproject.ErrTypeScriptCompilerUnavailable)
}

func jsTSProductSurfaceCount(result jstsproject.Result) int {
	count := 0
	for _, surface := range result.Surfaces {
		if surface.Role == jstsproject.SurfaceProduct &&
			(surface.Kind == jstsproject.SurfaceBrowser || surface.Kind == jstsproject.SurfaceServer || surface.Kind == jstsproject.SurfaceCLI) {
			count++
		}
	}
	return count
}

func validateJSTSProjectCorpusBinding(
	repository *corpus.Corpus,
	result jstsproject.Result,
) (corpus.FileID, error) {
	if repository == nil || result.CorpusSHA256 != repository.SHA256() {
		return "", fmt.Errorf("project corpus identity does not match")
	}
	exactFileRef := func(filePath, fileRef string) error {
		ref, ok := repository.ID(filePath)
		if !ok || string(ref) != fileRef {
			return fmt.Errorf("project file %q is not bound to its exact current FileRef", filePath)
		}
		return nil
	}
	if path.Base(result.Project.ManifestPath) != "package.json" || result.Project.Selector != "jsts:"+result.Project.ManifestPath {
		return "", fmt.Errorf("project manifest/selector identity is invalid")
	}
	if err := exactFileRef(result.Project.ManifestPath, result.Project.ManifestFileRef); err != nil {
		return "", err
	}
	if result.Project.ConfigPath != "" {
		if err := exactFileRef(result.Project.ConfigPath, result.Project.ConfigFileRef); err != nil {
			return "", err
		}
	}
	if result.Project.LockfilePath != "" {
		if err := exactFileRef(result.Project.LockfilePath, result.Project.LockfileFileRef); err != nil {
			return "", err
		}
	}
	for _, file := range result.Project.ToolConfigs {
		if err := exactFileRef(file.Path, file.FileRef); err != nil {
			return "", err
		}
	}
	for _, binary := range result.Project.Binaries {
		if err := exactFileRef(binary.Path, binary.FileRef); err != nil {
			return "", err
		}
	}
	for _, file := range result.Files {
		if err := exactFileRef(file.Path, file.FileRef); err != nil {
			return "", err
		}
	}
	manifestRef, _ := repository.ID(result.Project.ManifestPath)
	return manifestRef, nil
}

func validateJSTSTargetMaterialization(
	repository *corpus.Corpus,
	target jstsproject.Target,
	result jstsproject.Result,
) error {
	if err := target.ValidateAgainst(repository); err != nil {
		return fmt.Errorf("scout target: %w", err)
	}
	if err := result.Validate(); err != nil {
		return fmt.Errorf("project result: %w", err)
	}
	if _, err := validateJSTSProjectCorpusBinding(repository, result); err != nil {
		return fmt.Errorf("project corpus binding: %w", err)
	}
	materialized, err := jstsproject.TargetFromResult(result)
	if err != nil {
		return fmt.Errorf("restore materialized target: %w", err)
	}
	if err := target.ValidateMaterialization(materialized); err != nil {
		return err
	}
	return nil
}

func rebindMaterializedJSTSTarget(
	target repositoryTypedTarget,
	result jstsproject.Result,
) (repositoryTypedTarget, error) {
	scout, ok := repositoryJSTSTarget(target)
	if target.Key.Adapter != repositoryTargetAdapterJSTS || !ok {
		return repositoryTypedTarget{}, fmt.Errorf("selected target is not JavaScript/TypeScript")
	}
	materialized, err := jstsproject.TargetFromResult(result)
	if err != nil {
		return repositoryTypedTarget{}, fmt.Errorf("restore materialized target: %w", err)
	}
	if err := scout.ValidateMaterialization(materialized); err != nil {
		return repositoryTypedTarget{}, err
	}
	rebound := target
	rebound.Display = materialized.Name
	rebound.native = materialized
	if err := rebound.Validate(); err != nil {
		return repositoryTypedTarget{}, fmt.Errorf("rebind materialized target: %w", err)
	}
	return rebound, nil
}
