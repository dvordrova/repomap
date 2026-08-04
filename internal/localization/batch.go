package localization

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const (
	BatchManifestVersion               = 2
	maxLocalizationBatchFields         = 64
	maxLocalizationBatchPredictedBytes = 30 << 10
	localizationBatchEntryOverhead     = 16
)

// BatchManifest binds one deterministic contiguous slice of the complete
// presentation inventory. ContentSHA256 is its root- and ordinal-independent
// cache identity; ManifestSHA256 binds the complete run-local placement.
type BatchManifest struct {
	Version              int    `json:"version"`
	RootCanonicalSHA256  string `json:"root_canonical_sha256"`
	BatchIndex           int    `json:"batch_index"`
	BatchCount           int    `json:"batch_count"`
	StartField           int    `json:"start_field"`
	EndField             int    `json:"end_field"`
	FieldCount           int    `json:"field_count"`
	FirstFieldID         string `json:"first_field_id"`
	LastFieldID          string `json:"last_field_id"`
	FieldIDsSHA256       string `json:"field_ids_sha256"`
	BatchCanonicalSHA256 string `json:"batch_canonical_sha256"`
	BatchInputSHA256     string `json:"batch_input_sha256"`
	PredictedOutputBytes int    `json:"predicted_output_bytes"`
	// ContentSHA256 intentionally excludes root, ordinal, and batch-count
	// metadata. Identical exact batch contents therefore retain one cache
	// identity when unrelated root fields change.
	ContentSHA256  string `json:"content_sha256"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type Batch struct {
	Manifest  BatchManifest
	Canonical CanonicalArtifact
	Input     Input
}

// BuildBatches partitions the already sorted complete inventory without
// filtering or re-ranking fields. Both a predicted translated-output bound and
// an independent field-count bound are deterministic safety limits.
func BuildBatches(canonical CanonicalArtifact, input Input) ([]Batch, error) {
	if err := validateCanonical(canonical); err != nil {
		return nil, err
	}
	if err := validateInput(canonical, input); err != nil {
		return nil, err
	}
	if input.SourceLocale != LocaleEnglish || input.TargetLocale != LocaleRussian {
		return nil, fmt.Errorf("localization: unsupported batch direction")
	}
	if len(input.Fields) == 0 {
		return nil, fmt.Errorf("localization: empty batch inventory")
	}

	type boundary struct {
		start, end, predicted int
	}
	boundaries := make([]boundary, 0, 4)
	start := 0
	predicted := 0
	for index, field := range input.Fields {
		fieldBytes := predictedLocalizationOutputBytes(field)
		if fieldBytes > maxLocalizationBatchPredictedBytes {
			return nil, fmt.Errorf("localization: field exceeds batch output budget")
		}
		if index > start &&
			(index-start == maxLocalizationBatchFields ||
				predicted+fieldBytes > maxLocalizationBatchPredictedBytes) {
			boundaries = append(boundaries, boundary{start: start, end: index, predicted: predicted})
			start = index
			predicted = 0
		}
		predicted += fieldBytes
	}
	boundaries = append(boundaries, boundary{start: start, end: len(input.Fields), predicted: predicted})

	batches := make([]Batch, 0, len(boundaries))
	for index, bounds := range boundaries {
		batchCanonical := canonicalBatchSlice(canonical, bounds.start, bounds.end)
		batchInput := inputBatchSlice(input, batchCanonical.SHA256, bounds.start, bounds.end)
		if err := validateInput(batchCanonical, batchInput); err != nil {
			return nil, err
		}
		manifest, err := buildBatchManifest(
			canonical.SHA256,
			index,
			len(boundaries),
			bounds.start,
			bounds.end,
			bounds.predicted,
			batchCanonical,
			batchInput,
		)
		if err != nil {
			return nil, err
		}
		batches = append(batches, Batch{
			Manifest: manifest, Canonical: batchCanonical, Input: batchInput,
		})
	}
	return batches, nil
}

func CombineBatchProjections(
	canonical CanonicalArtifact,
	input Input,
	batches []Batch,
	projections []Projection,
) (Projection, error) {
	if err := validateCanonical(canonical); err != nil {
		return Projection{}, err
	}
	if err := validateInput(canonical, input); err != nil {
		return Projection{}, err
	}
	expectedBatches, err := BuildBatches(canonical, input)
	if err != nil {
		return Projection{}, err
	}
	if len(batches) != len(expectedBatches) || len(batches) != len(projections) {
		return Projection{}, fmt.Errorf("localization: incomplete batch projection")
	}
	translations := make(map[string]string, len(input.Fields))
	for index, batch := range batches {
		if err := validateBatch(batch, expectedBatches[index]); err != nil {
			return Projection{}, err
		}
		result, err := Apply(batch.Canonical, batch.Input, projections[index])
		if err != nil || result.Fallback || len(result.Diagnostics) != 0 {
			return Projection{}, fmt.Errorf("localization: invalid batch projection")
		}
		for _, field := range batch.Input.Fields {
			translated, exists := projections[index].Translations[field.ID]
			if !exists {
				return Projection{}, fmt.Errorf("localization: incomplete batch projection")
			}
			if _, exists := translations[field.ID]; exists {
				return Projection{}, fmt.Errorf("localization: duplicate batch translation")
			}
			translations[field.ID] = translated
		}
	}
	if len(translations) != len(input.Fields) {
		return Projection{}, fmt.Errorf("localization: incomplete batch projection")
	}
	projection := Projection{
		Version: ProjectionVersion, CanonicalSHA256: canonical.SHA256,
		Locale: input.TargetLocale, Translations: translations,
	}
	result, err := Apply(canonical, input, projection)
	if err != nil || result.Fallback || len(result.Diagnostics) != 0 {
		return Projection{}, fmt.Errorf("localization: invalid combined projection")
	}
	return projection, nil
}

func predictedLocalizationOutputBytes(field InputField) int {
	// Placeholder bytes are copied exactly. The remaining prose uses a 2.5x
	// UTF-8 expansion estimate for Russian, rounded up. The fixed compact tuple
	// overhead covers its positional index, quotes, separators, and escaping.
	placeholderBytes := int64(0)
	for _, placeholder := range field.Placeholders {
		if placeholder.Count <= 0 {
			return int(^uint(0) >> 1)
		}
		placeholderBytes += int64(len(placeholder.Token)) * int64(placeholder.Count)
	}
	proseBytes := int64(len(field.Text)) - placeholderBytes
	if proseBytes < 0 {
		return int(^uint(0) >> 1)
	}
	maxInt := int64(^uint(0) >> 1)
	if proseBytes > (maxInt-1)/5 {
		return int(maxInt)
	}
	predicted := placeholderBytes + (proseBytes*5+1)/2 + localizationBatchEntryOverhead
	if predicted <= 0 || predicted > maxInt {
		return int(maxInt)
	}
	return int(predicted)
}

func canonicalBatchSlice(canonical CanonicalArtifact, start, end int) CanonicalArtifact {
	fields := make([]CanonicalField, end-start)
	for index, field := range canonical.Fields[start:end] {
		field.ProtectedTerms = append([]ProtectedTerm(nil), field.ProtectedTerms...)
		fields[index] = field
	}
	batch := CanonicalArtifact{Version: CanonicalVersion, Locale: LocaleEnglish, Fields: fields}
	batch.SHA256 = canonicalHash(batch)
	return batch
}

func inputBatchSlice(input Input, canonicalSHA string, start, end int) Input {
	fields := make([]InputField, end-start)
	for index, field := range input.Fields[start:end] {
		field.Placeholders = append([]PlaceholderExpectation(nil), field.Placeholders...)
		fields[index] = field
	}
	return Input{
		Version: InputVersion, CanonicalSHA256: canonicalSHA,
		SourceLocale: input.SourceLocale, TargetLocale: input.TargetLocale,
		Fields: fields,
	}
}

func buildBatchManifest(
	rootSHA string,
	index,
	count,
	start,
	end,
	predicted int,
	canonical CanonicalArtifact,
	input Input,
) (BatchManifest, error) {
	if rootSHA == "" || len(input.Fields) == 0 ||
		index < 0 || count <= 0 || index >= count ||
		start < 0 || end <= start || end-start != len(input.Fields) ||
		predicted <= 0 || predicted > maxLocalizationBatchPredictedBytes {
		return BatchManifest{}, fmt.Errorf("localization: invalid batch manifest")
	}
	fieldIDs := make([]string, len(input.Fields))
	for index, field := range input.Fields {
		fieldIDs[index] = field.ID
	}
	fieldIDsJSON, err := json.Marshal(fieldIDs)
	if err != nil {
		return BatchManifest{}, err
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return BatchManifest{}, err
	}
	manifest := BatchManifest{
		Version: BatchManifestVersion, RootCanonicalSHA256: rootSHA,
		BatchIndex: index, BatchCount: count, StartField: start, EndField: end,
		FieldCount: len(input.Fields), FirstFieldID: fieldIDs[0],
		LastFieldID:          fieldIDs[len(fieldIDs)-1],
		FieldIDsSHA256:       sha256String(fieldIDsJSON),
		BatchCanonicalSHA256: canonical.SHA256,
		BatchInputSHA256:     sha256String(inputJSON),
		PredictedOutputBytes: predicted,
	}
	contentJSON, err := json.Marshal(batchContentIdentity{
		Version:              manifest.Version,
		FieldCount:           manifest.FieldCount,
		FirstFieldID:         manifest.FirstFieldID,
		LastFieldID:          manifest.LastFieldID,
		FieldIDsSHA256:       manifest.FieldIDsSHA256,
		BatchCanonicalSHA256: manifest.BatchCanonicalSHA256,
		BatchInputSHA256:     manifest.BatchInputSHA256,
		PredictedOutputBytes: manifest.PredictedOutputBytes,
	})
	if err != nil {
		return BatchManifest{}, err
	}
	manifest.ContentSHA256 = sha256String(contentJSON)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return BatchManifest{}, err
	}
	manifest.ManifestSHA256 = sha256String(manifestJSON)
	return manifest, nil
}

type batchContentIdentity struct {
	Version              int    `json:"version"`
	FieldCount           int    `json:"field_count"`
	FirstFieldID         string `json:"first_field_id"`
	LastFieldID          string `json:"last_field_id"`
	FieldIDsSHA256       string `json:"field_ids_sha256"`
	BatchCanonicalSHA256 string `json:"batch_canonical_sha256"`
	BatchInputSHA256     string `json:"batch_input_sha256"`
	PredictedOutputBytes int    `json:"predicted_output_bytes"`
}

func validateBatch(batch, expected Batch) error {
	manifest := batch.Manifest
	if manifest != expected.Manifest ||
		manifest.Version != BatchManifestVersion ||
		manifest.FieldCount <= 0 ||
		manifest.FieldCount > maxLocalizationBatchFields ||
		manifest.EndField <= manifest.StartField ||
		manifest.FieldCount != manifest.EndField-manifest.StartField ||
		manifest.ContentSHA256 == "" || manifest.ManifestSHA256 == "" ||
		manifest.PredictedOutputBytes <= 0 ||
		manifest.PredictedOutputBytes > maxLocalizationBatchPredictedBytes {
		return fmt.Errorf("localization: invalid batch manifest")
	}
	if err := validateCanonical(batch.Canonical); err != nil {
		return fmt.Errorf("localization: invalid batch manifest")
	}
	if err := validateInput(batch.Canonical, batch.Input); err != nil {
		return fmt.Errorf("localization: invalid batch manifest")
	}
	if batch.Canonical.SHA256 != expected.Canonical.SHA256 ||
		batch.Input.CanonicalSHA256 != expected.Input.CanonicalSHA256 {
		return fmt.Errorf("localization: invalid batch manifest")
	}
	return nil
}

func sha256String(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
