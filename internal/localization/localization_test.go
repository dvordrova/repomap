package localization

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalIdentityProjectionIsDeterministicAndExact(t *testing.T) {
	t.Parallel()

	specs := fixtureSpecs()
	first, err := NewCanonical(specs)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCanonical([]FieldSpec{specs[1], specs[0]})
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := MarshalCanonical(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := MarshalCanonical(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("canonical bytes differ:\n%s\n%s", firstBytes, secondBytes)
	}
	const wantSHA256 = "0916b6136334691462d740449331cbe8596a765178529aeabf0f43aac1197529"
	if first.SHA256 != wantSHA256 {
		t.Fatalf("canonical SHA-256 = %q, want %q", first.SHA256, wantSHA256)
	}

	projection, err := IdentityProjection(first)
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(first, LocaleEnglish)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Apply(first, input, projection)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf("identity result = %#v", result)
	}
	for index, field := range first.Fields {
		if result.Fields[index].ID != field.ID || result.Fields[index].Text != field.Text {
			t.Fatalf("identity field %d = %#v, want %#v", index, result.Fields[index], field)
		}
	}
}

func TestRussianProjectionChangesOnlyAllowlistedProse(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	encodedInput, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, protected := range []string{
		"cmd/сервер main.go:42",
		"StartReplication",
		"PostgreSQL",
		"API/v2",
	} {
		if bytes.Contains(encodedInput, []byte(protected)) {
			t.Fatalf("localization input exposed protected term %q: %s", protected, encodedInput)
		}
	}

	translations := make(map[string]string, len(input.Fields))
	for _, field := range input.Fields {
		switch {
		case strings.HasSuffix(field.ID, ".summary"):
			translations[field.ID] = "Открывает {{term_01}} через {{term_02}}; CJK: {{term_03}}."
		case strings.HasSuffix(field.ID, ".question"):
			translations[field.ID] = "Как {{term_01}} использует {{term_02}}?"
		default:
			t.Fatalf("unexpected fixture field %q", field.ID)
		}
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations:    translations,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 || result.Locale != LocaleRussian {
		t.Fatalf("Russian result = %#v", result)
	}
	joined := result.Fields[0].Text + "\n" + result.Fields[1].Text
	for _, protected := range []string{
		"cmd/сервер main.go:42",
		"StartReplication",
		"PostgreSQL",
		"API/v2",
	} {
		if !strings.Contains(joined, protected) {
			t.Fatalf("projected text lost protected term %q: %s", protected, joined)
		}
	}
	if !strings.Contains(joined, "東京") {
		t.Fatalf("projected text lost CJK prose: %s", joined)
	}
}

func TestProjectionFallsBackPerFieldWithDeterministicDiagnostics(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	translations := map[string]string{
		"component:unknown.summary": "unknown",
		input.Fields[0].ID:          "Потерян placeholder.",
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations:    translations,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback {
		t.Fatal("invalid projection did not report fallback")
	}
	wantCodes := []Diagnostic{
		{Code: "unknown_field_id", FieldID: "component:unknown.summary"},
		{Code: "placeholder_mismatch", FieldID: input.Fields[0].ID},
		{Code: "missing_translation", FieldID: input.Fields[1].ID},
	}
	if !equalDiagnostics(result.Diagnostics, wantCodes) {
		t.Fatalf("diagnostics = %#v, want %#v", result.Diagnostics, wantCodes)
	}
	for index, field := range canonical.Fields {
		if result.Fields[index].Text != field.Text {
			t.Fatalf("field %q did not fall back to canonical text", field.ID)
		}
	}
}

func TestProjectionHashMismatchFallsBackAtomically(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: strings.Repeat("0", 64),
		Locale:          LocaleRussian,
		Translations: map[string]string{
			canonical.Fields[0].ID: "Не должно примениться.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback ||
		!equalDiagnostics(result.Diagnostics, []Diagnostic{{Code: "canonical_hash_mismatch"}}) {
		t.Fatalf("hash mismatch result = %#v", result)
	}
	for index, field := range canonical.Fields {
		if result.Fields[index].Text != field.Text {
			t.Fatalf("hash mismatch changed field %q", field.ID)
		}
	}
}

func TestProjectionRejectsInvalidUTF8AndAddedPlaceholder(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations: map[string]string{
			input.Fields[0].ID: string([]byte{0xff}),
			input.Fields[1].ID: "Как {{term_01}} использует {{term_02}} и {{term_99}}?",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Diagnostic{
		{Code: "invalid_utf8", FieldID: input.Fields[0].ID},
		{Code: "placeholder_mismatch", FieldID: input.Fields[1].ID},
	}
	if !equalDiagnostics(result.Diagnostics, want) {
		t.Fatalf("diagnostics = %#v, want %#v", result.Diagnostics, want)
	}
}

func TestProjectionIsBoundToRequestedLocale(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := IdentityProjection(canonical)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Apply(canonical, input, projection)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback || result.Locale != LocaleEnglish ||
		!equalDiagnostics(result.Diagnostics, []Diagnostic{{Code: "projection_locale_mismatch"}}) {
		t.Fatalf("locale mismatch result = %#v", result)
	}
}

func TestProjectionProcessingIsBoundedByCanonicalInput(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}

	tooMany := map[string]string{
		input.Fields[0].ID: "one",
		input.Fields[1].ID: "two",
		"unknown":          strings.Repeat("x", 1<<20),
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations:    tooMany,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !equalDiagnostics(result.Diagnostics, []Diagnostic{{Code: "projection_field_count_exceeded"}}) {
		t.Fatalf("field-count diagnostics = %#v", result.Diagnostics)
	}

	tooLarge := strings.Repeat("x", translationByteLimit(input.Fields[0].Text)+1)
	result, err = Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations: map[string]string{
			input.Fields[0].ID: tooLarge,
			input.Fields[1].ID: "Как {{term_01}} использует {{term_02}}?",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !equalDiagnostics(result.Diagnostics, []Diagnostic{{
		Code:    "translation_too_large",
		FieldID: input.Fields[0].ID,
	}}) {
		t.Fatalf("scalar diagnostics = %#v", result.Diagnostics)
	}
}

func TestCanonicalValidationRederivesProtectedMetadata(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	canonical.Fields[0].ProtectedTerms[0].Count++
	canonical.SHA256 = canonicalHash(canonical)
	if _, err := BuildInput(canonical, LocaleRussian); err == nil {
		t.Fatal("self-hashed malformed placeholder count unexpectedly accepted")
	}

	canonical, err = NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	canonical.Fields[0].ProtectedTerms[0].Value = string([]byte{0xff})
	canonical.SHA256 = canonicalHash(canonical)
	if _, err := BuildInput(canonical, LocaleRussian); err == nil {
		t.Fatal("self-hashed invalid UTF-8 placeholder unexpectedly accepted")
	}
}

func TestFieldIDsAreAllowlistedAndStable(t *testing.T) {
	t.Parallel()

	id, err := FieldID(OwnerComponent, "component-01", FieldSummary)
	if err != nil {
		t.Fatal(err)
	}
	if id != "component:component-01.summary" {
		t.Fatalf("FieldID = %q", id)
	}
	if _, err := FieldID(OwnerComponent, "component-01", FieldQuestion); err == nil {
		t.Fatal("component question unexpectedly entered the localization allowlist")
	}
	if _, err := FieldID(OwnerComponent, " translated text ", FieldSummary); err == nil {
		t.Fatal("unstable owner id unexpectedly accepted")
	}
}

func TestCanonicalRejectsReservedPlaceholderAndHandlesNestedTerms(t *testing.T) {
	t.Parallel()

	if _, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "unsafe",
		Name:      FieldSummary,
		Text:      "Already contains {{term_01}}.",
	}}); err == nil {
		t.Fatal("reserved placeholder unexpectedly accepted")
	}

	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "nested",
		Name:      FieldSummary,
		Text:      "PostgreSQL accepts SQL and SQL.",
		ProtectedTerms: []ProtectedValue{
			{Kind: ProtectedProduct, Value: "PostgreSQL"},
			{Kind: ProtectedProtocol, Value: "SQL"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Fields) != 1 || len(input.Fields[0].Placeholders) != 2 ||
		input.Fields[0].Placeholders[1].Count != 2 {
		t.Fatalf("nested placeholders = %#v", input.Fields)
	}
}

func fixtureSpecs() []FieldSpec {
	return []FieldSpec{
		{
			OwnerKind: OwnerComponent,
			OwnerID:   "startup",
			Name:      FieldSummary,
			Text:      "Opens cmd/сервер main.go:42 through StartReplication; CJK: 東京.",
			ProtectedTerms: []ProtectedValue{
				{Kind: ProtectedPath, Value: "cmd/сервер main.go:42"},
				{Kind: ProtectedSymbol, Value: "StartReplication"},
				{Kind: ProtectedIdentifier, Value: "東京"},
			},
		},
		{
			OwnerKind: OwnerStudyDirection,
			OwnerID:   "replication",
			Name:      FieldQuestion,
			Text:      "How does PostgreSQL use API/v2?",
			ProtectedTerms: []ProtectedValue{
				{Kind: ProtectedProduct, Value: "PostgreSQL"},
				{Kind: ProtectedAPI, Value: "API/v2"},
			},
		},
	}
}

func equalDiagnostics(left, right []Diagnostic) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
