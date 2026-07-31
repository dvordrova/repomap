package localization

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestBuildBatchesIsDeterministicAndPreservesEveryField(t *testing.T) {
	t.Parallel()

	specs := make([]FieldSpec, 600)
	for index := range specs {
		specs[index] = FieldSpec{
			OwnerKind: OwnerPresentationText,
			OwnerID:   fmt.Sprintf("fixture/items/%04d", index),
			Name:      FieldText,
			Text:      "English presentation text worth translating",
		}
	}
	canonical, err := NewCanonical(specs)
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	first, err := BuildBatches(canonical, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildBatches(canonical, input)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatal("identical inventories produced different batches")
	}
	if len(first) != 10 {
		t.Fatalf("batch count = %d, want 10", len(first))
	}
	next := 0
	for index, batch := range first {
		if batch.Manifest.BatchIndex != index ||
			batch.Manifest.BatchCount != len(first) ||
			batch.Manifest.StartField != next ||
			batch.Manifest.FieldCount != len(batch.Input.Fields) ||
			batch.Manifest.FieldCount > maxLocalizationBatchFields ||
			batch.Manifest.ContentSHA256 == "" ||
			batch.Manifest.ManifestSHA256 == "" ||
			batch.Manifest.PredictedOutputBytes > maxLocalizationBatchPredictedBytes {
			t.Fatalf("batch %d manifest = %#v", index, batch.Manifest)
		}
		next = batch.Manifest.EndField
	}
	if next != len(input.Fields) {
		t.Fatalf("covered fields = %d, want %d", next, len(input.Fields))
	}
}

func TestBuildBatchesEnforcesIndependentFieldCountBound(t *testing.T) {
	t.Parallel()

	specs := make([]FieldSpec, 600)
	for index := range specs {
		specs[index] = FieldSpec{
			OwnerKind: OwnerPresentationText,
			OwnerID:   fmt.Sprintf("fixture/small/%04d", index),
			Name:      FieldText,
			Text:      "English",
		}
	}
	canonical, err := NewCanonical(specs)
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	batches, err := BuildBatches(canonical, input)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]int, len(batches))
	for index, batch := range batches {
		got[index] = batch.Manifest.FieldCount
	}
	want := []int{64, 64, 64, 64, 64, 64, 64, 64, 64, 24}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batch field counts = %v, want %v", got, want)
	}
}

func TestBatchPredictionCountsPlaceholdersExactlyAndExpandsOnlyProse(t *testing.T) {
	t.Parallel()

	field := InputField{
		ID:   "fixture",
		Text: "Read {{term_01}} through {{term_02}} and {{term_02}}.",
		Placeholders: []PlaceholderExpectation{
			{Token: "{{term_01}}", Kind: ProtectedPath, Count: 1},
			{Token: "{{term_02}}", Kind: ProtectedProtocol, Count: 2},
		},
	}
	placeholderBytes := len("{{term_01}}") + 2*len("{{term_02}}")
	proseBytes := len(field.Text) - placeholderBytes
	want := placeholderBytes + (proseBytes*5+1)/2 + localizationBatchEntryOverhead
	if got := predictedLocalizationOutputBytes(field); got != want {
		t.Fatalf("predicted output bytes = %d, want %d", got, want)
	}
}

func TestBatchContentIdentityExcludesRootAndOrdinal(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical([]FieldSpec{
		{
			OwnerKind: OwnerPresentationText,
			OwnerID:   "fixture/one",
			Name:      FieldText,
			Text:      "First English presentation value",
		},
		{
			OwnerKind: OwnerPresentationText,
			OwnerID:   "fixture/two",
			Name:      FieldText,
			Text:      "Second English presentation value",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	predicted := 0
	for _, field := range input.Fields {
		predicted += predictedLocalizationOutputBytes(field)
	}
	first, err := buildBatchManifest(
		strings.Repeat("a", 64), 0, 1, 0, 2, predicted, canonical, input,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildBatchManifest(
		strings.Repeat("b", 64), 2, 4, 9, 11, predicted, canonical, input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentSHA256 != second.ContentSHA256 {
		t.Fatalf("content hashes differ: %s != %s", first.ContentSHA256, second.ContentSHA256)
	}
	if first.ManifestSHA256 == second.ManifestSHA256 {
		t.Fatal("placement-specific manifest hashes unexpectedly match")
	}
}

func TestBuildBatchesRejectsOneFieldBeyondOutputBudget(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerPresentationText,
		OwnerID:   "fixture/oversized",
		Name:      FieldText,
		Text:      strings.Repeat("English prose ", 1_000),
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildBatches(canonical, input); err == nil {
		t.Fatal("oversized single field unexpectedly fit a batch")
	}
}

func TestCombineBatchProjectionsIsAtomic(t *testing.T) {
	t.Parallel()

	specs := make([]FieldSpec, 300)
	for index := range specs {
		specs[index] = FieldSpec{
			OwnerKind: OwnerPresentationText,
			OwnerID:   fmt.Sprintf("fixture/items/%04d", index),
			Name:      FieldText,
			Text:      strings.Repeat("English prose ", 4),
		}
	}
	canonical, err := NewCanonical(specs)
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	batches, err := BuildBatches(canonical, input)
	if err != nil {
		t.Fatal(err)
	}
	projections := make([]Projection, len(batches))
	for index, batch := range batches {
		translations := make(map[string]string, len(batch.Input.Fields))
		for _, field := range batch.Input.Fields {
			translations[field.ID] = "Полностью переведённое описание"
		}
		projections[index] = Projection{
			Version: ProjectionVersion, CanonicalSHA256: batch.Canonical.SHA256,
			Locale: LocaleRussian, Translations: translations,
		}
	}
	combined, err := CombineBatchProjections(canonical, input, batches, projections)
	if err != nil {
		t.Fatal(err)
	}
	if len(combined.Translations) != len(input.Fields) ||
		combined.CanonicalSHA256 != canonical.SHA256 {
		t.Fatalf("combined projection = %#v", combined)
	}
	if _, err := CombineBatchProjections(
		canonical,
		input,
		batches,
		projections[:len(projections)-1],
	); err == nil {
		t.Fatal("partial batch set unexpectedly combined")
	}
	tampered := append([]Batch(nil), batches...)
	tampered[0].Manifest.EndField--
	if _, err := CombineBatchProjections(canonical, input, tampered, projections); err == nil {
		t.Fatal("tampered batch manifest unexpectedly combined")
	}
	tampered = append([]Batch(nil), batches...)
	tampered[0].Manifest.ContentSHA256 = strings.Repeat("0", 64)
	if _, err := CombineBatchProjections(canonical, input, tampered, projections); err == nil {
		t.Fatal("tampered batch content identity unexpectedly combined")
	}
}
