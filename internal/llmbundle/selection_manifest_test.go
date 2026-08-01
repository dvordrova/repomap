package llmbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

func TestOrientationContextSelectionMatchesActualBundleAndIsDeterministic(t *testing.T) {
	files := make([]string, 0, 24)
	for index := 0; index < 24; index++ {
		files = append(files, fmt.Sprintf("internal/service/handler_%02d.go", index))
	}
	files = append(files, "private/full-tree-marker.bin")
	opts := Options{MaxFiles: 8, MaxBytes: 8 << 10}
	bundle, trace := BuildWithTrace(snapshot.Snapshot{
		RepoName: "selection-fixture",
		FileTree: []string{"private/full-tree-marker.bin"},
	}, files, opts)
	ordinary := Build(snapshot.Snapshot{
		RepoName: "selection-fixture",
		FileTree: []string{"private/full-tree-marker.bin"},
	}, files, opts)
	if !reflect.DeepEqual(bundle, ordinary) {
		t.Fatal("traced build changed ordinary bundle selection")
	}
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	wireJSON := []byte(`{"typed_wire":true}`)
	scanTrace := sourcesignals.ScanTrace{MaxPerFile: 5, MaxTotal: 200}
	manifest, err := FinalizeOrientationContextSelection(trace, bundle, bundleJSON, bundleJSON, wireJSON, scanTrace)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Counts.Candidates.Before != 24 || manifest.Counts.Candidates.After != len(bundle.CandidateFileIndex) ||
		manifest.Counts.Candidates.Omitted != 24-len(bundle.CandidateFileIndex) {
		t.Fatalf("candidate counts = %#v", manifest.Counts.Candidates)
	}
	if len(manifest.SelectedCandidates) != len(bundle.CandidateFileIndex) {
		t.Fatalf("selected rows = %d, bundle candidates = %d", len(manifest.SelectedCandidates), len(bundle.CandidateFileIndex))
	}
	for index, candidate := range bundle.CandidateFileIndex {
		row := manifest.SelectedCandidates[index]
		if row.Path != candidate.Path || row.Kind != candidate.Kind || row.Score != candidate.Score ||
			!reflect.DeepEqual(row.Reasons, candidate.Reasons) || !reflect.DeepEqual(row.Signals, candidate.Signals) {
			t.Fatalf("selected candidate %d differs: row=%#v bundle=%#v", index, row, candidate)
		}
	}
	first, err := EncodeOrientationContextSelection(manifest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncodeOrientationContextSelection(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("identical selection manifests encoded differently")
	}
	if bytes.Contains(first, []byte("full-tree-marker")) || bytes.Contains(first, []byte(`"file_tree"`)) ||
		bytes.Contains(first, []byte(`"snippet"`)) {
		t.Fatalf("selection manifest leaked full-tree or source-content data: %s", first)
	}
	decoded, err := DecodeOrientationContextSelection(first)
	if err != nil || !reflect.DeepEqual(decoded, manifest) {
		t.Fatalf("decoded manifest = %#v, %v", decoded, err)
	}
}

func TestOrientationContextSelectionSeparatesCanonicalAndPersistedBundleIdentities(t *testing.T) {
	t.Parallel()

	bundle, trace := BuildWithTrace(
		snapshot.Snapshot{RepoName: "redacted-selection"},
		[]string{"main.go"},
		Options{MaxFiles: 8},
	)
	canonical, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	persisted := []byte("[redacted: credential assignment detected]\n")
	manifest, err := FinalizeOrientationContextSelection(
		trace,
		bundle,
		canonical,
		persisted,
		[]byte(`{"wire":true}`),
		sourcesignals.ScanTrace{MaxPerFile: 5, MaxTotal: 200},
	)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CanonicalBundleSHA256 != selectionSHA256(canonical) ||
		manifest.CanonicalBundleBytes != len(canonical) ||
		manifest.PersistedBundleSHA256 != selectionSHA256(persisted) ||
		manifest.PersistedBundleBytes != len(persisted) ||
		manifest.CanonicalBundleSHA256 == manifest.PersistedBundleSHA256 ||
		manifest.ByteFit.FittedBytes != len(canonical) {
		t.Fatalf("canonical/persisted identities were conflated: %#v", manifest)
	}
}

func TestOrientationContextSelectionExposesByteFitAndBoundedCutoffs(t *testing.T) {
	files := make([]string, 0, 180)
	for index := 0; index < 180; index++ {
		files = append(files, fmt.Sprintf("internal/service/long_descriptive_handler_%03d.go", index))
	}
	bundle, trace := BuildWithTrace(snapshot.Snapshot{RepoName: "byte-fit"}, files, Options{
		MaxFiles:      180,
		MaxBytes:      12 << 10,
		SourceSignals: []sourcesignals.Signal{},
	})
	if !trace.ByteFit.Applied || !trace.ByteFit.Fit || trace.ByteFit.Attempts < 2 ||
		trace.EffectiveCaps.CandidateFiles >= trace.ConfiguredCaps.CandidateFiles {
		t.Fatalf("byte fit trace = %#v, caps %#v -> %#v", trace.ByteFit, trace.ConfiguredCaps, trace.EffectiveCaps)
	}
	if trace.EffectiveCaps.SourceSignalsTotal != trace.ConfiguredCaps.SourceSignalsTotal ||
		trace.EffectiveCaps.SourceSignalsPerFile != trace.ConfiguredCaps.SourceSignalsPerFile {
		t.Fatalf("caller-bounded source signal caps were reported as effective byte-fit cuts: %#v -> %#v", trace.ConfiguredCaps, trace.EffectiveCaps)
	}
	if trace.Counts.Candidates.Before != len(files) || trace.Counts.Candidates.After != len(bundle.CandidateFileIndex) ||
		trace.Counts.Candidates.Omitted == 0 || len(trace.OmittedCandidateSamples) == 0 ||
		len(trace.OmittedCandidateSamples) > maxSelectionCutoffSamples {
		t.Fatalf("candidate cutoff trace = counts %#v samples %d", trace.Counts.Candidates, len(trace.OmittedCandidateSamples))
	}
	foundByteFit := false
	for _, cutoff := range trace.Cutoffs {
		if len(cutoff.Samples) > maxSelectionCutoffSamples {
			t.Fatalf("unbounded cutoff = %#v", cutoff)
		}
		if cutoff.Stage == "byte_fit" && cutoff.Reason == "request_byte_budget" {
			foundByteFit = true
		}
	}
	if !foundByteFit {
		t.Fatalf("byte-fit cutoff missing: %#v", trace.Cutoffs)
	}
}

func TestByteFitTraceUsesExactReturnedBytesWhenFitWarningCrossesBudget(t *testing.T) {
	files := make([]string, 0, 300)
	for index := 0; index < 300; index++ {
		files = append(files, fmt.Sprintf("internal/service/long_descriptive_handler_%03d.go", index))
	}
	s := snapshot.Snapshot{RepoName: "warning-boundary"}
	configured := defaults(Options{MaxFiles: len(files), SourceSignals: []sourcesignals.Signal{}})
	fit := configured
	if !shrinkForByteBudget(&fit) {
		t.Fatal("fixture options did not shrink")
	}
	initialBundle, _ := buildWithTrace(s, files, configured)
	initialJSON, err := json.Marshal(initialBundle)
	if err != nil {
		t.Fatal(err)
	}
	preWarningBundle, _ := buildWithTrace(s, files, fit)
	preWarningJSON, err := json.Marshal(preWarningBundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(initialJSON) <= len(preWarningJSON) {
		t.Fatalf("fixture did not reduce bundle bytes: %d -> %d", len(initialJSON), len(preWarningJSON))
	}

	bundle, trace := BuildWithTrace(s, files, Options{
		MaxFiles:      len(files),
		MaxBytes:      len(preWarningJSON),
		SourceSignals: []sourcesignals.Signal{},
	})
	actualJSON, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !trace.ByteFit.Applied || trace.ByteFit.Fit ||
		trace.ByteFit.FittedBytes != len(actualJSON) || len(actualJSON) <= len(preWarningJSON) {
		t.Fatalf("warning-boundary byte fit = %#v, actual=%d cap=%d", trace.ByteFit, len(actualJSON), len(preWarningJSON))
	}
	if !containsString(bundle.Warnings, "provider bundle fitted to request-byte context budget") {
		t.Fatalf("existing returned bundle warning changed: %#v", bundle.Warnings)
	}
	foundExhausted := false
	for _, cutoff := range trace.Cutoffs {
		if cutoff.Stage == "byte_fit" && cutoff.Reason == "request_byte_budget_exhausted" {
			foundExhausted = true
		}
	}
	if !foundExhausted {
		t.Fatalf("truthful exhausted cutoff missing: %#v", trace.Cutoffs)
	}
	manifest, err := FinalizeOrientationContextSelection(
		trace,
		bundle,
		actualJSON,
		actualJSON,
		[]byte(`{}`),
		sourcesignals.ScanTrace{MaxPerFile: 5, MaxTotal: 200},
	)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ByteFit.FittedBytes != manifest.CanonicalBundleBytes {
		t.Fatalf("final byte identity mismatch: %#v", manifest.ByteFit)
	}
}

func TestFinalizeSelectionMeasuresWarningsAppendedAfterBundleSelection(t *testing.T) {
	s := snapshot.Snapshot{RepoName: "post-selection-warning"}
	base, _ := BuildWithTrace(s, nil, Options{SourceSignals: []sourcesignals.Signal{}})
	baseJSON, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	bundle, trace := BuildWithTrace(s, nil, Options{
		MaxBytes:      len(baseJSON),
		SourceSignals: []sourcesignals.Signal{},
	})
	bundle.Warnings = append(bundle.Warnings, "local operational evidence was unavailable")
	finalJSON, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalJSON) <= len(baseJSON) {
		t.Fatalf("fixture warning did not cross cap: base=%d final=%d", len(baseJSON), len(finalJSON))
	}
	manifest, err := FinalizeOrientationContextSelection(
		trace,
		bundle,
		finalJSON,
		finalJSON,
		[]byte(`{}`),
		sourcesignals.ScanTrace{MaxPerFile: 5, MaxTotal: 200},
	)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ByteFit.Fit || manifest.ByteFit.FittedBytes != len(finalJSON) ||
		manifest.CanonicalBundleBytes != len(finalJSON) {
		t.Fatalf("post-selection final bytes = %#v", manifest.ByteFit)
	}
	foundExhausted := false
	for _, cutoff := range manifest.Cutoffs {
		if cutoff.Stage == "byte_fit" && cutoff.Reason == "request_byte_budget_exhausted" {
			foundExhausted = true
		}
	}
	if !foundExhausted {
		t.Fatalf("post-selection warning cutoff missing: %#v", manifest.Cutoffs)
	}
}

func TestOrientationContextSelectionRejectsUnsafeUnknownAndOversizedData(t *testing.T) {
	bundle, trace := BuildWithTrace(snapshot.Snapshot{RepoName: "safe"}, []string{"main.go"}, Options{MaxFiles: 8})
	bundleJSON, _ := json.Marshal(bundle)
	manifest, err := FinalizeOrientationContextSelection(
		trace,
		bundle,
		bundleJSON,
		bundleJSON,
		[]byte(`{"wire":true}`),
		sourcesignals.ScanTrace{MaxPerFile: 5, MaxTotal: 200},
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Warnings = append(manifest.Warnings, `api_key := "company-secret-value"`)
	if _, err := EncodeOrientationContextSelection(manifest); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unsafe encode error = %v", err)
	}
	manifest.Warnings = nil
	data, err := EncodeOrientationContextSelection(manifest)
	if err != nil {
		t.Fatal(err)
	}
	unsupported := manifest
	unsupported.Version++
	if _, err := EncodeOrientationContextSelection(unsupported); err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("unsupported-version encode error = %v", err)
	}
	legacy := manifest
	legacy.Version = 1
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeOrientationContextSelection(legacyJSON); err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("legacy-version decode error = %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	value["unknown"] = true
	unknown, _ := json.Marshal(value)
	if _, err := DecodeOrientationContextSelection(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field decode error = %v", err)
	}
	oversized := bytes.Repeat([]byte("x"), MaxOrientationContextSelectionBytes+1)
	if _, err := DecodeOrientationContextSelection(oversized); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("oversized decode error = %v", err)
	}
}
