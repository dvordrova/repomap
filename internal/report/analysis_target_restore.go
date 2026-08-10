package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/dvordrova/repomap/internal/snapshot"
)

const maxReportTargetMetadataBytes = 4 << 20

type reportTargetMetadata struct {
	AnalysisTargetRef         string `json:"analysis_target_ref"`
	AnalysisTargetKind        string `json:"analysis_target_kind"`
	AnalysisTargetModule      string `json:"analysis_target_module"`
	AnalysisTargetDisplayPath string `json:"analysis_target_display_path"`
	AnalysisTargetPackage     string `json:"analysis_target_package"`
}

// restoreAnalysisTargetFromRunContainer recovers only the exact selected
// target authority when persisted-artifact secret scanning made snapshot.json
// unavailable. The sealed container owns the target; metadata selects one
// exact member and must independently agree with every public target field.
func restoreAnalysisTargetFromRunContainer(runDir string, data *ReportData) error {
	if data == nil || data.AnalysisTarget != nil {
		return nil
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("report analysis target: open run directory: %w", err)
	}
	defer root.Close()

	if _, err := root.Lstat(snapshot.TargetRunContainerArtifactFilename); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("report analysis target: inspect target run container: %w", err)
	}
	containerRaw, err := readManifestFile(
		root,
		snapshot.TargetRunContainerArtifactFilename,
		snapshot.MaxTargetRunContainerBytes,
	)
	if err != nil {
		return fmt.Errorf("report analysis target: %w", err)
	}
	container, err := snapshot.DecodeTargetRunContainer(containerRaw)
	if err != nil {
		return fmt.Errorf("report analysis target: target run container: %w", err)
	}

	metadataRaw, err := readManifestFile(root, "metadata.json", maxReportTargetMetadataBytes)
	if err != nil {
		return fmt.Errorf("report analysis target: %w", err)
	}
	var metadata reportTargetMetadata
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		return fmt.Errorf("report analysis target: invalid metadata JSON")
	}
	for _, projection := range container.Targets {
		target := projection.Target
		if target.Ref != metadata.AnalysisTargetRef {
			continue
		}
		if metadata.AnalysisTargetKind != string(target.Kind) ||
			metadata.AnalysisTargetModule != target.ModulePath ||
			metadata.AnalysisTargetDisplayPath != target.DisplayPath() ||
			metadata.AnalysisTargetPackage != target.PackagePath {
			return fmt.Errorf("report analysis target: metadata/container binding mismatch")
		}
		restored := target.Snapshot()
		data.AnalysisTarget = &restored
		return nil
	}
	return fmt.Errorf("report analysis target: metadata target is absent from container")
}
