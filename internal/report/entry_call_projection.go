package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/entrycall"
)

// EntryCallReportProjectionVersion is the report-owned wire version for the
// flat exact-call-family projection. It deliberately does not claim a runtime
// sequence, endpoint, mechanism, or Canvas edge.
const EntryCallReportProjectionVersion = 1

// EntryCallReportProjection promotes only accepted entry-call v2 families
// whose exact root declaration belongs to the selected executable target.
type EntryCallReportProjection struct {
	Version  int                     `json:"version"`
	Families []EntryCallReportFamily `json:"families"`
}

// EntryCallReportFamily is one exact direct-call family restored from local
// authority. Request-local refs and provider ordering are intentionally absent.
type EntryCallReportFamily struct {
	RootDeclaration entrycall.Location   `json:"root_declaration"`
	CallerLabel     string               `json:"caller_label"`
	CalleeLabel     string               `json:"callee_label"`
	Invocation      entrycall.Invocation `json:"invocation"`
	WitnessCount    int                  `json:"witness_count"`
	Callsites       []entrycall.Location `json:"callsites"`
}

type entryCallReportArtifactBinding struct {
	statusSHA256          string
	resultSHA256          string
	repositoryStateSHA256 string
}

// loadEntryCallReportProjection reads the optional debug sidecar family. A
// missing or closed status has no report projection. Accepted artifacts are a
// fail-closed pair and are promoted only after all cross-artifact bindings
// have been proven.
func loadEntryCallReportProjection(
	runDir string,
	data *ReportData,
	expectedRepositoryStateSHA256 string,
) error {
	if data == nil {
		return fmt.Errorf("entry call report: report data is required")
	}
	data.EntryCall = nil
	data.entryCallArtifactBinding = nil

	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("entry call report: open run directory: %w", err)
	}
	defer root.Close()

	statusRaw, hasStatus, err := readOptionalEntryCallArtifact(
		root, entrycall.StatusArtifactFilename,
	)
	if err != nil {
		return err
	}
	if !hasStatus {
		if _, hasResult, readErr := readOptionalEntryCallArtifact(root, entrycall.ResultArtifactFilename); readErr != nil {
			return readErr
		} else if hasResult {
			return fmt.Errorf("entry call report: result is present without status")
		}
		return nil
	}

	status, err := entrycall.DecodeStatus(statusRaw)
	if err != nil {
		return fmt.Errorf("entry call report: status: %w", err)
	}
	if status.State != entrycall.StatusAccepted && status.State != entrycall.StatusAcceptedPartial {
		if _, hasResult, readErr := readOptionalEntryCallArtifact(root, entrycall.ResultArtifactFilename); readErr != nil {
			return readErr
		} else if hasResult {
			return fmt.Errorf("entry call report: closed status has an unexpected result")
		}
		return nil
	}

	resultRaw, hasResult, err := readOptionalEntryCallArtifact(
		root, entrycall.ResultArtifactFilename,
	)
	if err != nil {
		return err
	}
	if !hasResult {
		return fmt.Errorf("entry call report: accepted status requires result")
	}
	projection, binding, err := acceptedEntryCallReportProjection(
		statusRaw,
		resultRaw,
		data.AnalysisTarget,
		expectedRepositoryStateSHA256,
	)
	if err != nil {
		return err
	}
	data.EntryCall = projection
	data.entryCallArtifactBinding = binding
	return nil
}

func acceptedEntryCallReportProjection(
	statusRaw, resultRaw []byte,
	target *analysistarget.Target,
	expectedRepositoryStateSHA256 string,
) (*EntryCallReportProjection, *entryCallReportArtifactBinding, error) {
	status, err := entrycall.DecodeStatus(statusRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("entry call report: status: %w", err)
	}
	if status.State != entrycall.StatusAccepted && status.State != entrycall.StatusAcceptedPartial {
		return nil, nil, fmt.Errorf("entry call report: status is not accepted")
	}
	result, err := entrycall.DecodeResult(resultRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("entry call report: result: %w", err)
	}
	resultSHA256 := manifestSHA256(resultRaw)
	if status.ResultSHA256 != resultSHA256 ||
		status.PromptVersion != result.PromptVersion ||
		status.RequestRef != result.RequestRef ||
		status.RequestSHA256 != result.RequestSHA256 ||
		status.SubstrateSHA256 != result.SubstrateSHA256 ||
		status.RepositoryStateSHA256 != result.RepositoryStateSHA256 {
		return nil, nil, fmt.Errorf("entry call report: status/result identity mismatch")
	}
	if status.SelectedFamilies != result.SelectedFamilyCount() ||
		status.RejectedFamilies != result.RejectedFamilyCount() {
		return nil, nil, fmt.Errorf("entry call report: status/result family counts mismatch")
	}
	if expectedRepositoryStateSHA256 != "" &&
		result.RepositoryStateSHA256 != expectedRepositoryStateSHA256 {
		return nil, nil, fmt.Errorf("entry call report: repository state binding mismatch")
	}

	projection, err := projectEntryCallFamilies(target, result)
	if err != nil {
		return nil, nil, err
	}
	return projection, &entryCallReportArtifactBinding{
		statusSHA256:          manifestSHA256(statusRaw),
		resultSHA256:          resultSHA256,
		repositoryStateSHA256: result.RepositoryStateSHA256,
	}, nil
}

func projectEntryCallFamilies(
	target *analysistarget.Target,
	result entrycall.Result,
) (*EntryCallReportProjection, error) {
	if target == nil {
		return nil, fmt.Errorf("entry call report: accepted result requires analysis target")
	}
	if err := target.Validate(); err != nil {
		return nil, fmt.Errorf("entry call report: analysis target: %w", err)
	}
	if target.Kind == analysistarget.KindLibraryPackage || target.Kind == analysistarget.KindModuleLibrary {
		// Library roots are intentionally unresolved in target v1. The accepted
		// artifact remains hash-bound, but no process-root projection is invented.
		return nil, nil
	}
	if target.Kind != analysistarget.KindExecutablePackage {
		return nil, fmt.Errorf("entry call report: unsupported analysis target kind %q", target.Kind)
	}

	targetRoots := make(map[string]struct{}, len(target.Roots))
	for _, root := range target.Roots {
		targetRoots[entryCallRootKey(root.Path, root.Line)] = struct{}{}
	}
	seenRoots := make(map[string]struct{}, len(result.Entries))
	projection := &EntryCallReportProjection{
		Version:  EntryCallReportProjectionVersion,
		Families: make([]EntryCallReportFamily, 0, result.SelectedFamilyCount()),
	}
	for _, entry := range result.Entries {
		rootKey := entryCallRootKey(entry.Declaration.Path, entry.Declaration.Line)
		if _, exact := targetRoots[rootKey]; !exact {
			return nil, fmt.Errorf("entry call report: result root is outside analysis target")
		}
		if _, duplicate := seenRoots[rootKey]; duplicate {
			return nil, fmt.Errorf("entry call report: duplicate exact result root")
		}
		seenRoots[rootKey] = struct{}{}
		for _, family := range entry.Families {
			projection.Families = append(projection.Families, EntryCallReportFamily{
				RootDeclaration: entry.Declaration,
				CallerLabel:     family.CallerLabel,
				CalleeLabel:     family.CalleeLabel,
				Invocation:      family.Invocation,
				WitnessCount:    family.WitnessCount,
				Callsites:       append([]entrycall.Location{}, family.Callsites...),
			})
		}
	}
	return projection, nil
}

func validateEntryCallReportProjection(
	target *analysistarget.Target,
	projection *EntryCallReportProjection,
	openablePaths []string,
) error {
	if projection == nil {
		return nil
	}
	if target == nil || target.Kind != analysistarget.KindExecutablePackage {
		return fmt.Errorf("entry call projection requires executable analysis target")
	}
	if err := target.Validate(); err != nil {
		return fmt.Errorf("entry call projection analysis target: %w", err)
	}
	if projection.Version != EntryCallReportProjectionVersion || projection.Families == nil ||
		len(projection.Families) > entrycall.MaxRoots*entrycall.MaxSelectedFamiliesPerRoot {
		return fmt.Errorf("entry call projection identity is invalid")
	}
	targetRoots := make(map[string]struct{}, len(target.Roots))
	for _, root := range target.Roots {
		targetRoots[entryCallRootKey(root.Path, root.Line)] = struct{}{}
	}
	openable := make(map[string]struct{}, len(openablePaths))
	for _, path := range openablePaths {
		openable[path] = struct{}{}
	}
	for index, family := range projection.Families {
		if !validEntryCallReportLocation(family.RootDeclaration) {
			return fmt.Errorf("entry call family %d root declaration is invalid", index)
		}
		if _, exact := targetRoots[entryCallRootKey(family.RootDeclaration.Path, family.RootDeclaration.Line)]; !exact {
			return fmt.Errorf("entry call family %d root is outside analysis target", index)
		}
		if !validEntryCallReportLabel(family.CallerLabel) ||
			!validEntryCallReportLabel(family.CalleeLabel) ||
			!family.Invocation.Valid() || family.WitnessCount <= 0 ||
			len(family.Callsites) == 0 || len(family.Callsites) > entrycall.MaxRepresentativeCallsites {
			return fmt.Errorf("entry call family %d is invalid", index)
		}
		for callsiteIndex, callsite := range family.Callsites {
			if !validEntryCallReportLocation(callsite) {
				return fmt.Errorf("entry call family %d callsite %d is invalid", index, callsiteIndex)
			}
			if _, authorized := openable[callsite.Path]; !authorized {
				return fmt.Errorf("entry call family %d callsite %d is not openable", index, callsiteIndex)
			}
		}
	}
	return nil
}

func validEntryCallReportLocation(location entrycall.Location) bool {
	return location.Line > 0 && location.Column >= 0 && validateManifestPath(location.Path) == nil
}

func validEntryCallReportLabel(value string) bool {
	if value == "" || strings.TrimSpace(value) != value ||
		strings.Join(strings.Fields(value), " ") != value ||
		utf8.RuneCountInString(value) > entrycall.MaxLabelRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '/' || character == '\\' {
			return false
		}
	}
	return true
}

func entryCallRootKey(path string, line int) string {
	return fmt.Sprintf("%s\x00%d", path, line)
}

func readOptionalEntryCallArtifact(
	root *os.Root,
	name string,
) ([]byte, bool, error) {
	if _, err := root.Lstat(name); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("entry call report: inspect %s: %w", name, err)
	}
	data, err := readManifestFile(root, name, entrycall.MaxArtifactBytes)
	if err != nil {
		return nil, false, fmt.Errorf("entry call report: %w", err)
	}
	return data, true, nil
}

func entryCallReportMaterial(
	data *ReportData,
	repositoryStateSHA256 string,
) (string, string, error) {
	if data == nil {
		return "", "", fmt.Errorf("report data is required")
	}
	binding := data.entryCallArtifactBinding
	if binding == nil {
		if data.EntryCall != nil {
			return "", "", fmt.Errorf("projection lacks accepted artifact binding")
		}
		return "", "", nil
	}
	if !validManifestSHA256(binding.statusSHA256) ||
		!validManifestSHA256(binding.resultSHA256) ||
		binding.repositoryStateSHA256 != repositoryStateSHA256 {
		return "", "", fmt.Errorf("accepted artifact binding is invalid")
	}
	if data.EntryCall == nil {
		if data.AnalysisTarget == nil ||
			(data.AnalysisTarget.Kind != analysistarget.KindLibraryPackage &&
				data.AnalysisTarget.Kind != analysistarget.KindModuleLibrary) {
			return "", "", fmt.Errorf("accepted executable artifacts lack projection")
		}
	} else if err := validateEntryCallReportProjection(
		data.AnalysisTarget,
		data.EntryCall,
		data.OpenablePaths,
	); err != nil {
		return "", "", err
	}
	return binding.statusSHA256, binding.resultSHA256, nil
}

// VerifyEntryCallArtifacts binds accepted entry-call v2 bytes to the exact
// repository state and re-derives the public flat projection. A closed status
// may remain as optional debug state, but it must not carry a result or report
// projection.
func (m RunManifest) VerifyEntryCallArtifacts(runDir string, reportJSON []byte) error {
	if m.Version != CurrentRunManifestVersion {
		return fmt.Errorf("report manifest: unsupported version %d", m.Version)
	}
	var persisted struct {
		AnalysisTarget *analysistarget.Target     `json:"analysis_target"`
		EntryCall      *EntryCallReportProjection `json:"entry_call"`
		OpenablePaths  []string                   `json:"openable_paths"`
	}
	if err := json.Unmarshal(reportJSON, &persisted); err != nil {
		return fmt.Errorf("report manifest: decode entry call projection: %w", err)
	}

	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("report manifest: open entry call run: %w", err)
	}
	defer root.Close()
	statusRaw, hasStatus, err := readOptionalEntryCallArtifact(root, entrycall.StatusArtifactFilename)
	if err != nil {
		return fmt.Errorf("report manifest: %w", err)
	}
	resultRaw, hasResult, err := readOptionalEntryCallArtifact(root, entrycall.ResultArtifactFilename)
	if err != nil {
		return fmt.Errorf("report manifest: %w", err)
	}

	material := m.MaterialInputs
	bound := material.EntryCallStatusSHA256 != ""
	if !bound {
		if persisted.EntryCall != nil {
			return fmt.Errorf("report manifest: entry call projection lacks artifact authority")
		}
		if !hasStatus {
			if hasResult {
				return fmt.Errorf("report manifest: unbound entry call result is present")
			}
			return nil
		}
		status, decodeErr := entrycall.DecodeStatus(statusRaw)
		if decodeErr != nil {
			return fmt.Errorf("report manifest: entry call status: %w", decodeErr)
		}
		if status.State == entrycall.StatusAccepted || status.State == entrycall.StatusAcceptedPartial {
			return fmt.Errorf("report manifest: accepted entry call status is unbound")
		}
		if hasResult {
			return fmt.Errorf("report manifest: closed entry call status has an unexpected result")
		}
		return nil
	}
	if !hasStatus || !hasResult ||
		manifestSHA256(statusRaw) != material.EntryCallStatusSHA256 ||
		manifestSHA256(resultRaw) != material.EntryCallResultSHA256 {
		return fmt.Errorf("report manifest: entry call artifact sha256 mismatch")
	}
	projection, _, err := acceptedEntryCallReportProjection(
		statusRaw,
		resultRaw,
		persisted.AnalysisTarget,
		m.RepositoryStateSHA256,
	)
	if err != nil {
		return fmt.Errorf("report manifest: entry call artifacts: %w", err)
	}
	if err := validateEntryCallReportProjection(
		persisted.AnalysisTarget,
		projection,
		persisted.OpenablePaths,
	); err != nil {
		return fmt.Errorf("report manifest: %w", err)
	}
	if !reflect.DeepEqual(projection, persisted.EntryCall) {
		return fmt.Errorf("report manifest: entry call artifacts do not match report")
	}
	return nil
}
