package llmbundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strings"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

const (
	OrientationContextSelectionFilename = "orientation_context_selection.v2.json"
	OrientationContextSelectionVersion  = 2
	MaxOrientationContextSelectionBytes = 4 * 1024 * 1024
	maxSelectionCutoffs                 = 64
	maxSelectionCutoffSamples           = 12
)

type SelectionCaps struct {
	ReadmeBytes          int `json:"readme_bytes"`
	Modules              int `json:"modules"`
	Entrypoints          int `json:"entrypoints"`
	CandidateFiles       int `json:"candidate_files"`
	Edges                int `json:"edges"`
	SourceSignalsTotal   int `json:"source_signals_total"`
	SourceSignalsPerFile int `json:"source_signals_per_file"`
	KnownDocs            int `json:"known_docs"`
	CommandTraces        int `json:"command_traces"`
	BundleBytes          int `json:"bundle_bytes"`
}

type SelectionCount struct {
	Before  int `json:"before"`
	After   int `json:"after"`
	Omitted int `json:"omitted"`
}

type SelectionCounts struct {
	Candidates            SelectionCount `json:"candidates"`
	Entrypoints           SelectionCount `json:"entrypoints"`
	Modules               SelectionCount `json:"modules"`
	Edges                 SelectionCount `json:"edges"`
	SourceSignals         SelectionCount `json:"source_signals"`
	OrientationCandidates SelectionCount `json:"orientation_candidates"`
	KnownDocs             SelectionCount `json:"known_docs"`
	CommandTraces         SelectionCount `json:"command_traces"`
	ReadmeBytes           SelectionCount `json:"readme_bytes"`
}

type ByteFitTrace struct {
	Attempts     int  `json:"attempts"`
	Applied      bool `json:"applied"`
	Fit          bool `json:"fit"`
	InitialBytes int  `json:"initial_bytes"`
	FittedBytes  int  `json:"fitted_bytes"`
}

type SelectionCutoff struct {
	Category string   `json:"category"`
	Stage    string   `json:"stage"`
	Reason   string   `json:"reason"`
	Before   int      `json:"before"`
	After    int      `json:"after"`
	Omitted  int      `json:"omitted"`
	Samples  []string `json:"samples,omitempty"`
}

type CandidateSelectionRow struct {
	Path    string   `json:"path"`
	Kind    string   `json:"kind"`
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
	Signals []string `json:"signals"`
}

// BuildTrace is produced by the same invocation that returns the Bundle. It
// contains no reconstructed ranking inputs.
type BuildTrace struct {
	ConfiguredCaps          SelectionCaps
	EffectiveCaps           SelectionCaps
	ByteFit                 ByteFitTrace
	Counts                  SelectionCounts
	Cutoffs                 []SelectionCutoff
	SelectedCandidates      []CandidateSelectionRow
	OmittedCandidateSamples []CandidateSelectionRow
}

type OrientationContextSelection struct {
	Version                 int                     `json:"version"`
	CanonicalBundleSHA256   string                  `json:"canonical_bundle_sha256"`
	PersistedBundleSHA256   string                  `json:"persisted_bundle_sha256"`
	TypedWireSHA256         string                  `json:"typed_wire_sha256"`
	CanonicalBundleBytes    int                     `json:"canonical_bundle_bytes"`
	PersistedBundleBytes    int                     `json:"persisted_bundle_bytes"`
	TypedWireBytes          int                     `json:"typed_wire_bytes"`
	ConfiguredCaps          SelectionCaps           `json:"configured_caps"`
	EffectiveCaps           SelectionCaps           `json:"effective_caps"`
	ByteFit                 ByteFitTrace            `json:"byte_fit"`
	Counts                  SelectionCounts         `json:"counts"`
	Cutoffs                 []SelectionCutoff       `json:"cutoffs,omitempty"`
	Warnings                []string                `json:"warnings,omitempty"`
	SelectedCandidates      []CandidateSelectionRow `json:"selected_candidates"`
	OmittedCandidateSamples []CandidateSelectionRow `json:"omitted_candidate_samples,omitempty"`
	SourceSignalScan        sourcesignals.ScanTrace `json:"source_signal_scan"`
}

func FinalizeOrientationContextSelection(
	trace BuildTrace,
	bundle Bundle,
	canonicalBundleJSON []byte,
	persistedBundleBytes []byte,
	typedWireJSON []byte,
	signalScan sourcesignals.ScanTrace,
) (OrientationContextSelection, error) {
	if !json.Valid(canonicalBundleJSON) || len(persistedBundleBytes) == 0 || !json.Valid(typedWireJSON) {
		return OrientationContextSelection{}, fmt.Errorf("orientation context selection: canonical bundle and wire must be valid json and persisted bundle must be non-empty")
	}
	selected := candidateSelectionRows(bundle.CandidateFileIndex)
	if !equalCandidateSelectionRows(selected, trace.SelectedCandidates) {
		return OrientationContextSelection{}, fmt.Errorf("orientation context selection: traced candidates differ from bundle")
	}
	reconcileFinalBundleBytes(&trace, len(canonicalBundleJSON))
	actualCounts := trace.Counts
	actualCounts.Candidates.After = len(bundle.CandidateFileIndex)
	actualCounts.Entrypoints.After = len(bundle.Go.Entrypoints)
	actualCounts.Modules.After = len(bundle.Go.ModuleSummaries)
	actualCounts.Edges.After = len(bundle.Go.ImportantEdges)
	actualCounts.SourceSignals.After = len(bundle.SourceSignals)
	actualCounts.OrientationCandidates.After = len(bundle.Go.OrientationCandidates)
	actualCounts.KnownDocs.After = len(bundle.KnownDocs)
	actualCounts.CommandTraces.After = len(bundle.Go.CommandTraces)
	if err := completeSelectionCounts(&actualCounts); err != nil {
		return OrientationContextSelection{}, err
	}
	manifest := OrientationContextSelection{
		Version:               OrientationContextSelectionVersion,
		CanonicalBundleSHA256: selectionSHA256(canonicalBundleJSON),
		PersistedBundleSHA256: selectionSHA256(persistedBundleBytes),
		TypedWireSHA256:       selectionSHA256(typedWireJSON),
		CanonicalBundleBytes:  len(canonicalBundleJSON),
		PersistedBundleBytes:  len(persistedBundleBytes),
		TypedWireBytes:        len(typedWireJSON),
		ConfiguredCaps:        trace.ConfiguredCaps,
		EffectiveCaps:         trace.EffectiveCaps,
		ByteFit:               trace.ByteFit,
		Counts:                actualCounts,
		Cutoffs:               append([]SelectionCutoff(nil), trace.Cutoffs...),
		Warnings:              append([]string(nil), bundle.Warnings...),
		SelectedCandidates:    selected,
		OmittedCandidateSamples: append(
			[]CandidateSelectionRow(nil), trace.OmittedCandidateSamples...,
		),
		SourceSignalScan: signalScan,
	}
	if err := manifest.Validate(); err != nil {
		return OrientationContextSelection{}, err
	}
	return manifest, nil
}

func reconcileFinalBundleBytes(trace *BuildTrace, finalBytes int) {
	if trace == nil {
		return
	}
	wasFit := trace.ByteFit.Fit
	trace.ByteFit.FittedBytes = finalBytes
	maxBytes := trace.ConfiguredCaps.BundleBytes
	trace.ByteFit.Fit = maxBytes == 0 || finalBytes <= maxBytes
	if !wasFit || trace.ByteFit.Fit || maxBytes <= 0 {
		return
	}
	for index := range trace.Cutoffs {
		if trace.Cutoffs[index].Stage == "byte_fit" {
			trace.Cutoffs[index].Reason = "request_byte_budget_exhausted"
			return
		}
	}
	appendSelectionCutoff(
		trace,
		"bundle_bytes",
		"byte_fit",
		"request_byte_budget_exhausted",
		finalBytes,
		maxBytes,
		nil,
	)
}

func EncodeOrientationContextSelection(manifest OrientationContextSelection) ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if kind, found := detectSelectionManifestSecret(reflect.ValueOf(manifest)); found {
		return nil, fmt.Errorf("orientation context selection: unsafe %s detected", kind)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("orientation context selection: encode: %w", err)
	}
	data = append(data, '\n')
	if len(data) > MaxOrientationContextSelectionBytes {
		return nil, fmt.Errorf("orientation context selection: exceeds %d bytes", MaxOrientationContextSelectionBytes)
	}
	if kind, found := secretscan.DetectAlways(string(data)); found {
		return nil, fmt.Errorf("orientation context selection: unsafe %s detected", kind)
	}
	return data, nil
}

func DecodeOrientationContextSelection(data []byte) (OrientationContextSelection, error) {
	if len(data) == 0 || len(data) > MaxOrientationContextSelectionBytes {
		return OrientationContextSelection{}, fmt.Errorf(
			"orientation context selection: size must be between 1 and %d bytes",
			MaxOrientationContextSelectionBytes,
		)
	}
	if kind, found := secretscan.DetectAlways(string(data)); found {
		return OrientationContextSelection{}, fmt.Errorf("orientation context selection: unsafe %s detected", kind)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest OrientationContextSelection
	if err := decoder.Decode(&manifest); err != nil {
		return OrientationContextSelection{}, fmt.Errorf("orientation context selection: decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return OrientationContextSelection{}, fmt.Errorf("orientation context selection: multiple json values")
		}
		return OrientationContextSelection{}, fmt.Errorf("orientation context selection: trailing data: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return OrientationContextSelection{}, err
	}
	if kind, found := detectSelectionManifestSecret(reflect.ValueOf(manifest)); found {
		return OrientationContextSelection{}, fmt.Errorf("orientation context selection: unsafe %s detected", kind)
	}
	return manifest, nil
}

func detectSelectionManifestSecret(value reflect.Value) (string, bool) {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", false
		}
		return detectSelectionManifestSecret(value.Elem())
	}
	switch value.Kind() {
	case reflect.String:
		return secretscan.DetectAlways(value.String())
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if kind, found := detectSelectionManifestSecret(value.Field(index)); found {
				return kind, true
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if kind, found := detectSelectionManifestSecret(value.Index(index)); found {
				return kind, true
			}
		}
	}
	return "", false
}

func (manifest OrientationContextSelection) Validate() error {
	if manifest.Version != OrientationContextSelectionVersion {
		return fmt.Errorf("orientation context selection: unsupported version")
	}
	if !validSelectionSHA256(manifest.CanonicalBundleSHA256) ||
		!validSelectionSHA256(manifest.PersistedBundleSHA256) ||
		!validSelectionSHA256(manifest.TypedWireSHA256) ||
		manifest.CanonicalBundleBytes <= 0 || manifest.PersistedBundleBytes <= 0 || manifest.TypedWireBytes <= 0 {
		return fmt.Errorf("orientation context selection: invalid bundle or wire identity")
	}
	if err := validateSelectionCaps(manifest.ConfiguredCaps); err != nil {
		return fmt.Errorf("orientation context selection: configured caps: %w", err)
	}
	if err := validateSelectionCaps(manifest.EffectiveCaps); err != nil {
		return fmt.Errorf("orientation context selection: effective caps: %w", err)
	}
	if !selectionCapsDoNotExpand(manifest.ConfiguredCaps, manifest.EffectiveCaps) {
		return fmt.Errorf("orientation context selection: effective caps exceed configured caps")
	}
	if manifest.ByteFit.Attempts <= 0 || manifest.ByteFit.InitialBytes <= 0 || manifest.ByteFit.FittedBytes <= 0 ||
		manifest.ByteFit.FittedBytes != manifest.CanonicalBundleBytes ||
		manifest.ByteFit.Applied != !equalSelectionCaps(manifest.ConfiguredCaps, manifest.EffectiveCaps) ||
		(manifest.ByteFit.Applied && manifest.ByteFit.Attempts <= 1) ||
		(!manifest.ByteFit.Applied && manifest.ByteFit.Attempts != 1) ||
		(manifest.ConfiguredCaps.BundleBytes == 0 && !manifest.ByteFit.Fit) ||
		(manifest.ByteFit.Fit && manifest.ConfiguredCaps.BundleBytes > 0 &&
			manifest.ByteFit.FittedBytes > manifest.ConfiguredCaps.BundleBytes) {
		return fmt.Errorf("orientation context selection: invalid byte-fit trace")
	}
	if err := validateSelectionCounts(manifest.Counts); err != nil {
		return err
	}
	if len(manifest.SelectedCandidates) != manifest.Counts.Candidates.After {
		return fmt.Errorf("orientation context selection: selected candidate count mismatch")
	}
	if len(manifest.Cutoffs) > maxSelectionCutoffs || len(manifest.OmittedCandidateSamples) > maxSelectionCutoffSamples {
		return fmt.Errorf("orientation context selection: cutoff evidence is not bounded")
	}
	for index, cutoff := range manifest.Cutoffs {
		if strings.TrimSpace(cutoff.Category) == "" || strings.TrimSpace(cutoff.Stage) == "" ||
			strings.TrimSpace(cutoff.Reason) == "" || cutoff.Before < cutoff.After ||
			cutoff.Omitted != cutoff.Before-cutoff.After || len(cutoff.Samples) > maxSelectionCutoffSamples {
			return fmt.Errorf("orientation context selection: invalid cutoff %d", index)
		}
	}
	seen := make(map[string]struct{}, len(manifest.SelectedCandidates))
	for index, candidate := range manifest.SelectedCandidates {
		if err := validateCandidateSelectionRow(candidate); err != nil {
			return fmt.Errorf("orientation context selection: selected candidate %d: %w", index, err)
		}
		if _, duplicate := seen[candidate.Path]; duplicate {
			return fmt.Errorf("orientation context selection: duplicate selected candidate")
		}
		seen[candidate.Path] = struct{}{}
	}
	for index, candidate := range manifest.OmittedCandidateSamples {
		if err := validateCandidateSelectionRow(candidate); err != nil {
			return fmt.Errorf("orientation context selection: omitted candidate %d: %w", index, err)
		}
		if _, selected := seen[candidate.Path]; selected {
			return fmt.Errorf("orientation context selection: omitted candidate is selected")
		}
	}
	if err := validateSourceSignalScan(manifest.SourceSignalScan, manifest.Counts.SourceSignals.Before, manifest.ConfiguredCaps); err != nil {
		return fmt.Errorf("orientation context selection: invalid source signal scan trace")
	}
	return nil
}

func validateSourceSignalScan(trace sourcesignals.ScanTrace, selectedBeforeAllowlist int, caps SelectionCaps) error {
	if trace.MaxPerFile != caps.SourceSignalsPerFile || trace.MaxTotal != caps.SourceSignalsTotal ||
		trace.EligibleFiles < 0 || trace.ScannedFiles < 0 || trace.UnscannedFilesAtTotalLimit < 0 ||
		trace.ProvenEligibleSignals < 0 || trace.SelectedSignals < 0 ||
		trace.OmittedAtPerFileLimit < 0 || trace.OmittedAtTotalLimit < 0 ||
		trace.ScannedFiles+trace.UnscannedFilesAtTotalLimit != trace.EligibleFiles ||
		trace.SelectedSignals != selectedBeforeAllowlist ||
		trace.ProvenEligibleSignals < trace.SelectedSignals ||
		trace.ProvenEligibleSignals != trace.SelectedSignals+
			trace.OmittedAtPerFileLimit+trace.OmittedAtTotalLimit ||
		trace.SelectedSignals > trace.MaxTotal {
		return fmt.Errorf("invalid counts or caps")
	}
	omittedCount := trace.OmittedAtPerFileLimit + trace.OmittedAtTotalLimit
	if len(trace.OmittedSamples) != min(omittedCount, maxSelectionCutoffSamples) ||
		len(trace.UnscannedFileSamples) != min(trace.UnscannedFilesAtTotalLimit, maxSelectionCutoffSamples) {
		return fmt.Errorf("invalid bounded sample counts")
	}
	sampleCounts := map[string]int{"max_per_file": 0, "max_total": 0}
	for _, sample := range trace.OmittedSamples {
		if !validSelectionPath(sample.Path) || sample.Line <= 0 || strings.TrimSpace(sample.Category) == "" ||
			(sample.Cutoff != "max_per_file" && sample.Cutoff != "max_total") {
			return fmt.Errorf("invalid omitted sample")
		}
		sampleCounts[sample.Cutoff]++
	}
	if sampleCounts["max_per_file"] > trace.OmittedAtPerFileLimit ||
		sampleCounts["max_total"] > trace.OmittedAtTotalLimit {
		return fmt.Errorf("sample cutoff exceeds omitted count")
	}
	seen := make(map[string]struct{}, len(trace.UnscannedFileSamples))
	for _, path := range trace.UnscannedFileSamples {
		if !validSelectionPath(path) {
			return fmt.Errorf("invalid unscanned file sample")
		}
		if _, duplicate := seen[path]; duplicate {
			return fmt.Errorf("duplicate unscanned file sample")
		}
		seen[path] = struct{}{}
	}
	return nil
}

func completeSelectionCounts(counts *SelectionCounts) error {
	values := []*SelectionCount{
		&counts.Candidates, &counts.Entrypoints, &counts.Modules, &counts.Edges,
		&counts.SourceSignals, &counts.OrientationCandidates, &counts.KnownDocs,
		&counts.CommandTraces, &counts.ReadmeBytes,
	}
	for _, count := range values {
		if count.Before < count.After || count.Before < 0 || count.After < 0 {
			return fmt.Errorf("orientation context selection: invalid before/after count")
		}
		count.Omitted = count.Before - count.After
	}
	return nil
}

func validateSelectionCounts(counts SelectionCounts) error {
	values := []SelectionCount{
		counts.Candidates, counts.Entrypoints, counts.Modules, counts.Edges,
		counts.SourceSignals, counts.OrientationCandidates, counts.KnownDocs,
		counts.CommandTraces, counts.ReadmeBytes,
	}
	for _, count := range values {
		if count.Before < count.After || count.Before < 0 || count.After < 0 ||
			count.Omitted != count.Before-count.After {
			return fmt.Errorf("orientation context selection: invalid selection count")
		}
	}
	return nil
}

func validateSelectionCaps(caps SelectionCaps) error {
	values := []int{
		caps.ReadmeBytes, caps.Modules, caps.Entrypoints, caps.CandidateFiles,
		caps.Edges, caps.SourceSignalsTotal, caps.SourceSignalsPerFile,
		caps.KnownDocs, caps.CommandTraces,
	}
	for _, value := range values {
		if value <= 0 {
			return fmt.Errorf("non-positive cap")
		}
	}
	if caps.BundleBytes < 0 {
		return fmt.Errorf("negative bundle byte cap")
	}
	return nil
}

func selectionCapsDoNotExpand(configured, effective SelectionCaps) bool {
	return effective.ReadmeBytes <= configured.ReadmeBytes &&
		effective.Modules <= configured.Modules &&
		effective.Entrypoints <= configured.Entrypoints &&
		effective.CandidateFiles <= configured.CandidateFiles &&
		effective.Edges <= configured.Edges &&
		effective.SourceSignalsTotal <= configured.SourceSignalsTotal &&
		effective.SourceSignalsPerFile <= configured.SourceSignalsPerFile &&
		effective.KnownDocs <= configured.KnownDocs &&
		effective.CommandTraces <= configured.CommandTraces &&
		effective.BundleBytes == configured.BundleBytes
}

func validateCandidateSelectionRow(candidate CandidateSelectionRow) error {
	if !validSelectionPath(candidate.Path) || strings.TrimSpace(candidate.Kind) == "" {
		return fmt.Errorf("invalid path or kind")
	}
	if candidate.Reasons == nil || candidate.Signals == nil {
		return fmt.Errorf("reasons and signals are required")
	}
	return nil
}

func validSelectionPath(path string) bool {
	return path != "" && path != "." && fs.ValidPath(path) && !strings.ContainsRune(path, '\\')
}

func candidateSelectionRows(entries []fileIndexEntry) []CandidateSelectionRow {
	rows := make([]CandidateSelectionRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, CandidateSelectionRow{
			Path: entry.Path, Kind: entry.Kind, Score: entry.Score,
			Reasons: append([]string(nil), entry.Reasons...),
			Signals: append([]string(nil), entry.Signals...),
		})
	}
	return rows
}

func equalCandidateSelectionRows(left, right []CandidateSelectionRow) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftJSON, _ := json.Marshal(left[index])
		rightJSON, _ := json.Marshal(right[index])
		if !bytes.Equal(leftJSON, rightJSON) {
			return false
		}
	}
	return true
}

func selectionCapsFromOptions(opts Options, maxBytes int) SelectionCaps {
	return SelectionCaps{
		ReadmeBytes: opts.MaxReadmeBytes, Modules: opts.MaxModules,
		Entrypoints: opts.MaxEntrypoints, CandidateFiles: opts.MaxFiles,
		Edges: opts.MaxEdges, SourceSignalsTotal: opts.MaxSignalTotal,
		SourceSignalsPerFile: opts.MaxSignalPerFile, KnownDocs: maxKnownDocsForBundle,
		CommandTraces: maxCommandTracesForBundle, BundleBytes: maxBytes,
	}
}

func effectiveSelectionCaps(configuredOptions, fit Options, configuredCaps SelectionCaps, maxBytes int) SelectionCaps {
	caps := selectionCapsFromOptions(fit, maxBytes)
	if configuredOptions.SourceSignals != nil {
		// Supplied signals were already bounded by the caller. Byte fitting does
		// not reselect them, so a smaller attempted scanner cap is not effective.
		caps.SourceSignalsTotal = configuredCaps.SourceSignalsTotal
		caps.SourceSignalsPerFile = configuredCaps.SourceSignalsPerFile
	}
	return caps
}

func equalSelectionCaps(left, right SelectionCaps) bool { return left == right }

func appendSelectionCutoff(
	trace *BuildTrace,
	category string,
	stage string,
	reason string,
	before int,
	after int,
	samples []string,
) {
	if trace == nil || before <= after || len(trace.Cutoffs) >= maxSelectionCutoffs {
		return
	}
	if len(samples) > maxSelectionCutoffSamples {
		samples = samples[:maxSelectionCutoffSamples]
	}
	trace.Cutoffs = append(trace.Cutoffs, SelectionCutoff{
		Category: category, Stage: stage, Reason: reason,
		Before: before, After: after, Omitted: before - after,
		Samples: append([]string(nil), samples...),
	})
}

func moduleSummarySamples(values []moduleSummaryCompact) []string {
	result := make([]string, 0, min(len(values), maxSelectionCutoffSamples))
	for _, value := range values {
		name := value.ModuleDir
		if name == "" {
			name = value.ModulePath
		}
		result = append(result, name)
		if len(result) == maxSelectionCutoffSamples {
			break
		}
	}
	return result
}

func entrypointSamples(values []gofacts.Entrypoint) []string {
	result := make([]string, 0, min(len(values), maxSelectionCutoffSamples))
	for _, value := range values {
		name := value.ImportPath
		if name == "" {
			name = value.PackageDir
		}
		result = append(result, name)
		if len(result) == maxSelectionCutoffSamples {
			break
		}
	}
	return result
}

func entrypointSamplesDifference(all, selected []gofacts.Entrypoint) []string {
	selectedKeys := selectionKeySet(selected)
	var omitted []gofacts.Entrypoint
	for _, value := range all {
		if _, ok := selectedKeys[selectionJSONKey(value)]; !ok {
			omitted = append(omitted, value)
		}
	}
	return entrypointSamples(omitted)
}

func orientationCandidateSamples(values []gofacts.OrientationCandidate) []string {
	result := make([]string, 0, min(len(values), maxSelectionCutoffSamples))
	for _, value := range values {
		name := value.Name
		if name == "" {
			name = value.EntrypointPackage
		}
		result = append(result, name)
		if len(result) == maxSelectionCutoffSamples {
			break
		}
	}
	return result
}

func orientationCandidateSamplesDifference(all, selected []gofacts.OrientationCandidate) []string {
	selectedKeys := selectionKeySet(selected)
	var omitted []gofacts.OrientationCandidate
	for _, value := range all {
		if _, ok := selectedKeys[selectionJSONKey(value)]; !ok {
			omitted = append(omitted, value)
		}
	}
	return orientationCandidateSamples(omitted)
}

func edgeSamplesDifference(all, selected []gofacts.Edge) []string {
	selectedSet := make(map[gofacts.Edge]struct{}, len(selected))
	for _, value := range selected {
		selectedSet[value] = struct{}{}
	}
	result := make([]string, 0, min(len(all)-len(selected), maxSelectionCutoffSamples))
	for _, value := range all {
		if _, ok := selectedSet[value]; ok {
			continue
		}
		result = append(result, value.From+" -> "+value.To)
		if len(result) == maxSelectionCutoffSamples {
			break
		}
	}
	return result
}

func commandTraceSamplesDifference(all, selected []gofacts.CommandTrace) []string {
	selectedKeys := selectionKeySet(selected)
	result := make([]string, 0, min(len(all)-len(selected), maxSelectionCutoffSamples))
	for _, value := range all {
		if _, ok := selectedKeys[selectionJSONKey(value)]; ok {
			continue
		}
		name := value.Command
		if name == "" {
			name = value.EntrypointPackage
		}
		result = append(result, name)
		if len(result) == maxSelectionCutoffSamples {
			break
		}
	}
	return result
}

func omittedCandidateRows(all, selected []fileIndexEntry) []CandidateSelectionRow {
	selectedPaths := make(map[string]struct{}, len(selected))
	for _, value := range selected {
		selectedPaths[value.Path] = struct{}{}
	}
	result := make([]CandidateSelectionRow, 0, len(all)-len(selected))
	for _, value := range all {
		if _, ok := selectedPaths[value.Path]; ok {
			continue
		}
		result = append(result, candidateSelectionRows([]fileIndexEntry{value})[0])
	}
	return result
}

func boundedCandidateRows(values []CandidateSelectionRow, limit int) []CandidateSelectionRow {
	if len(values) > limit {
		values = values[:limit]
	}
	return append([]CandidateSelectionRow(nil), values...)
}

func candidatePaths(values []CandidateSelectionRow) []string {
	result := make([]string, 0, min(len(values), maxSelectionCutoffSamples))
	for _, value := range values {
		result = append(result, value.Path)
		if len(result) == maxSelectionCutoffSamples {
			break
		}
	}
	return result
}

func stringDifference(all, selected []string) []string {
	selectedSet := make(map[string]struct{}, len(selected))
	for _, value := range selected {
		selectedSet[value] = struct{}{}
	}
	result := make([]string, 0, min(len(all)-len(selected), maxSelectionCutoffSamples))
	for _, value := range all {
		if _, ok := selectedSet[value]; ok {
			continue
		}
		result = append(result, value)
		if len(result) == maxSelectionCutoffSamples {
			break
		}
	}
	return result
}

func sourceSignalSamplesDifference(all, selected []sourcesignals.Signal) []string {
	selectedKeys := selectionKeySet(selected)
	result := make([]string, 0, min(len(all)-len(selected), maxSelectionCutoffSamples))
	for _, value := range all {
		if _, ok := selectedKeys[selectionJSONKey(value)]; ok {
			continue
		}
		result = append(result, fmt.Sprintf("%s:%d %s", value.Path, value.Line, value.Category))
		if len(result) == maxSelectionCutoffSamples {
			break
		}
	}
	return result
}

func selectionKeySet[T any](values []T) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[selectionJSONKey(value)] = struct{}{}
	}
	return result
}

func selectionJSONKey(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func selectionSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func validSelectionSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
