package report

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

type ReportInputScaleWarning struct {
	Kind         string
	Retained     int
	AdvisorySize int
}

// TargetReportScaleWarning binds one transient transport measurement to the
// exact analyzed target whose canonical payload produced it. SelectedTargetID
// is navigation identity; ProgramTargetID is the sealed semantic identity.
// Diagnostics are observational and never participate in publication.
type TargetReportScaleWarning struct {
	SelectedTargetID string
	ProgramTargetID  string
	Warning          ReportInputScaleWarning
}

// GenerationDiagnostics carries transient measurements observed while the
// browser payload already exists in memory. It is not persisted report
// authority and callers cannot use it to change publication behavior.
type GenerationDiagnostics struct {
	rawStandaloneBundlePayloadBytes int64
	scaleWarnings                   []ReportInputScaleWarning
	targetScaleWarnings             []TargetReportScaleWarning
}

// ScaleWarnings returns a caller-owned copy of every scale measurement that
// completed, including one observed before a later publication failure.
func (diagnostics GenerationDiagnostics) ScaleWarnings() []ReportInputScaleWarning {
	return append([]ReportInputScaleWarning(nil), diagnostics.scaleWarnings...)
}

// TargetScaleWarnings returns caller-owned target-bound transport warnings.
func (diagnostics GenerationDiagnostics) TargetScaleWarnings() []TargetReportScaleWarning {
	return append([]TargetReportScaleWarning(nil), diagnostics.targetScaleWarnings...)
}

const (
	ReportScaleWarningTargetMetadataBytes         = "target_metadata_bytes"
	ReportScaleWarningManifestBytes               = "run_manifest_bytes"
	ReportScaleWarningDirtyEntries                = "repository_dirty_entries"
	ReportScaleWarningReportJSONBytes             = "report_json_bytes"
	ReportScaleWarningReportHTMLBytes             = "report_html_bytes"
	ReportScaleWarningSnapshotBytes               = "snapshot_bytes"
	ReportScaleWarningPreparedHTMLBytes           = "prepared_report_html_bytes"
	ReportScaleWarningBundleBytes                 = "standalone_bundle_bytes"
	ReportScaleWarningTargetBundleRawBytes        = "standalone_target_payload_bytes"
	ReportScaleWarningTargetBundleCompressedBytes = "standalone_target_gzip_bytes"
	ReportScaleWarningSemanticMetadata            = "semantic_metadata_bytes"
)

const (
	AdvisoryStandaloneTargetPayloadBytes    int64 = 64 << 20
	AdvisoryStandaloneTargetCompressedBytes int64 = 65 << 20
)

// ReportInputScaleWarnings reports former local handoff and legacy projection
// thresholds without participating in parsing, validation, or publication.
func ReportInputScaleWarnings(data *ReportData) []ReportInputScaleWarning {
	if data == nil {
		return nil
	}
	warnings := make([]ReportInputScaleWarning, 0)
	if data.targetMetadataBytes > advisoryReportTargetMetadataBytes {
		warnings = append(warnings, ReportInputScaleWarning{
			Kind: ReportScaleWarningTargetMetadataBytes, Retained: data.targetMetadataBytes,
			AdvisorySize: advisoryReportTargetMetadataBytes,
		})
	}
	if encoded, err := encodeReportJSON(data, 0); err == nil && len(encoded) > MaxReportJSONBytes {
		warnings = append(warnings, ReportInputScaleWarning{
			Kind: ReportScaleWarningReportJSONBytes, Retained: len(encoded),
			AdvisorySize: MaxReportJSONBytes,
		})
	}
	return warnings
}

// PublishedReportScaleWarnings inspects only file metadata after successful
// publication. Missing/stat failures are ignored because diagnostics must
// never create a new publication failure path.
func PublishedReportScaleWarnings(runDir string) []ReportInputScaleWarning {
	checks := []struct {
		name     string
		kind     string
		advisory int
	}{
		{name: "report.json", kind: ReportScaleWarningReportJSONBytes, advisory: MaxReportJSONBytes},
		{name: "report.html", kind: ReportScaleWarningReportHTMLBytes, advisory: MaxOrdinaryReportHTMLBytes},
		{name: "snapshot.json", kind: ReportScaleWarningSnapshotBytes, advisory: advisoryManifestSnapshotBytes},
		{name: "metadata.json", kind: ReportScaleWarningSemanticMetadata, advisory: 4 << 20},
	}
	warnings := make([]ReportInputScaleWarning, 0, len(checks))
	for _, check := range checks {
		info, err := os.Stat(filepath.Join(runDir, check.name))
		if err != nil || !info.Mode().IsRegular() || info.Size() <= int64(check.advisory) {
			continue
		}
		retained := int(info.Size())
		if int64(retained) != info.Size() {
			retained = int(^uint(0) >> 1)
		}
		warnings = append(warnings, ReportInputScaleWarning{
			Kind: check.kind, Retained: retained, AdvisorySize: check.advisory,
		})
	}
	return warnings
}

// CapturedReportInputFileScaleWarnings measures only inputs that ReadRunDir
// has already accepted. It deliberately excludes report.json/report.html,
// whose final names may still belong to a prior publication transaction.
func CapturedReportInputFileScaleWarnings(runDir string) []ReportInputScaleWarning {
	checks := []struct {
		name     string
		kind     string
		advisory int
	}{
		{name: "snapshot.json", kind: ReportScaleWarningSnapshotBytes, advisory: advisoryManifestSnapshotBytes},
		{name: "metadata.json", kind: ReportScaleWarningSemanticMetadata, advisory: 4 << 20},
	}
	warnings := make([]ReportInputScaleWarning, 0, len(checks))
	for _, check := range checks {
		info, err := os.Stat(filepath.Join(runDir, check.name))
		if err != nil || !info.Mode().IsRegular() || info.Size() <= int64(check.advisory) {
			continue
		}
		retained := int(info.Size())
		if int64(retained) != info.Size() {
			retained = int(^uint(0) >> 1)
		}
		warnings = append(warnings, ReportInputScaleWarning{
			Kind: check.kind, Retained: retained, AdvisorySize: check.advisory,
		})
	}
	return warnings
}

func standaloneBundlePayloadScaleWarnings(retained int64) []ReportInputScaleWarning {
	if retained <= AdvisoryStandaloneTargetBundlePayloadBytes {
		return nil
	}
	maximum := int64(^uint(0) >> 1)
	retainedInt := int(retained)
	if retained > maximum {
		retainedInt = int(maximum)
	}
	return []ReportInputScaleWarning{{
		Kind: ReportScaleWarningBundleBytes, Retained: retainedInt,
		AdvisorySize: int(AdvisoryStandaloneTargetBundlePayloadBytes),
	}}
}

func finalReportArtifactScaleWarnings(
	reportJSON []byte,
	manifest RunManifest,
) []ReportInputScaleWarning {
	warnings := reportJSONScaleWarnings(reportJSON)
	warnings = append(warnings, RunManifestScaleWarnings(manifest)...)
	return warnings
}

func reportJSONScaleWarnings(reportJSON []byte) []ReportInputScaleWarning {
	warnings := make([]ReportInputScaleWarning, 0, 1)
	if len(reportJSON) > MaxReportJSONBytes {
		warnings = append(warnings, ReportInputScaleWarning{
			Kind: ReportScaleWarningReportJSONBytes, Retained: len(reportJSON),
			AdvisorySize: MaxReportJSONBytes,
		})
	}
	return warnings
}

func reportHTMLScaleWarningsForSize(retained int64) []ReportInputScaleWarning {
	checks := []struct {
		kind     string
		advisory int64
	}{
		{ReportScaleWarningReportHTMLBytes, int64(MaxOrdinaryReportHTMLBytes)},
	}
	warnings := make([]ReportInputScaleWarning, 0, len(checks))
	maximum := int64(^uint(0) >> 1)
	retainedInt := int(retained)
	if retained > maximum {
		retainedInt = int(maximum)
	}
	for _, check := range checks {
		if retained <= check.advisory {
			continue
		}
		warnings = append(warnings, ReportInputScaleWarning{
			Kind: check.kind, Retained: retainedInt, AdvisorySize: int(check.advisory),
		})
	}
	return warnings
}

// RunManifestScaleWarnings measures complete manifest authority. The former
// 4 MiB and dirty-entry thresholds are diagnostic only.
func RunManifestScaleWarnings(manifest RunManifest) []ReportInputScaleWarning {
	warnings := make([]ReportInputScaleWarning, 0, 8)
	for _, warning := range workspacesnapshot.ScaleWarnings(workspacesnapshot.Input{
		AnalysisRoot: manifest.AnalysisRoot, Repository: manifest.RepositoryState,
		CapturedInputs: manifest.CapturedInputs, AllowedPaths: manifest.OpenablePaths,
	}) {
		warnings = append(warnings, ReportInputScaleWarning{
			Kind: "workspace_snapshot:" + string(warning.Kind), Retained: warning.MaximumRetained,
			AdvisorySize: warning.AdvisorySize,
		})
	}
	if len(manifest.RepositoryState.Dirty) > maxManifestRepositoryDirtyFiles {
		warnings = append(warnings, ReportInputScaleWarning{
			Kind: ReportScaleWarningDirtyEntries, Retained: len(manifest.RepositoryState.Dirty),
			AdvisorySize: maxManifestRepositoryDirtyFiles,
		})
	}
	if encoded, err := json.Marshal(manifest); err == nil && len(encoded) > advisoryRunManifestBytes {
		warnings = append(warnings, ReportInputScaleWarning{
			Kind: ReportScaleWarningManifestBytes, Retained: len(encoded),
			AdvisorySize: advisoryRunManifestBytes,
		})
	}
	return warnings
}
