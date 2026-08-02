package localization

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/dvordrova/repomap/internal/modelresearch"
)

func TestDecodeRussianProviderResponseDiscardsOnlyTrailingUnrequestedIndices(t *testing.T) {
	t.Parallel()

	specs := make([]FieldSpec, 64)
	for index := range specs {
		specs[index] = FieldSpec{
			OwnerKind: OwnerComponent,
			OwnerID:   fmt.Sprintf("component-%02d", index),
			Name:      FieldSummary,
			Text:      fmt.Sprintf("English text %d", index),
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
	entries := make([]ProviderTranslation, 0, len(input.Fields)+1)
	for index := range input.Fields {
		entries = append(entries, NewProviderTranslation(index, fmt.Sprintf("Русский текст %d", index)))
	}
	entries = append(entries, NewProviderTranslation(64, "Незапрошенный хвост"))
	encoded, err := json.Marshal(ProviderResponse{
		Version: ProviderResponseVersion, CanonicalSHA256: canonical.SHA256,
		Locale: LocaleRussian, Translations: entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, diagnostics, err := DecodeRussianProviderResponseDetailed(canonical, input, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.UnrequestedTranslations != 1 || len(projection.Translations) != 64 {
		t.Fatalf("decoded shape = diagnostics %#v translations %d", diagnostics, len(projection.Translations))
	}
	for _, translated := range projection.Translations {
		if translated == "Незапрошенный хвост" {
			t.Fatal("unrequested translation entered the stable projection")
		}
	}
	if _, err := DecodeRussianProviderResponse(canonical, input, encoded); err != nil {
		t.Fatalf("source-compatible decoder rejected safe unrequested tail: %v", err)
	}

	for _, test := range []struct {
		name  string
		index int
	}{
		{name: "duplicate requested", index: 63},
		{name: "negative", index: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalidEntries := append([]ProviderTranslation(nil), entries[:64]...)
			invalidEntries = append(invalidEntries, NewProviderTranslation(test.index, "Недопустимый хвост"))
			invalid, err := json.Marshal(ProviderResponse{
				Version: ProviderResponseVersion, CanonicalSHA256: canonical.SHA256,
				Locale: LocaleRussian, Translations: invalidEntries,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := DecodeRussianProviderResponseDetailed(canonical, input, invalid); err == nil {
				t.Fatal("invalid trailing translation unexpectedly decoded")
			}
		})
	}
}

func TestDecodeRussianProviderResponseRestoresStableFieldIDs(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]ProviderTranslation, len(input.Fields))
	for index, field := range input.Fields {
		text := "Русское описание"
		for _, placeholder := range field.Placeholders {
			for count := 0; count < placeholder.Count; count++ {
				text += " " + placeholder.Token
			}
		}
		entries[index] = NewProviderTranslation(index, text)
	}
	encoded, err := json.Marshal(ProviderResponse{
		Version:         ProviderResponseVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations:    entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := DecodeRussianProviderResponse(canonical, input, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Translations) != len(input.Fields) {
		t.Fatalf("translation count = %d, want %d", len(projection.Translations), len(input.Fields))
	}
	for index, field := range input.Fields {
		if got := projection.Translations[field.ID]; got != *entries[index].Text {
			t.Fatalf("translation for %q = %q, want %q", field.ID, got, *entries[index].Text)
		}
	}
}

func TestDecodeRussianProviderResponseUsesSharedEnvelope(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]ProviderTranslation, len(input.Fields))
	for index := range entries {
		entries[index] = NewProviderTranslation(index, "Русский текст")
	}
	valid, err := json.Marshal(ProviderResponse{
		Version: ProviderResponseVersion, CanonicalSHA256: canonical.SHA256,
		Locale: LocaleRussian, Translations: entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	aboveFormerCap := append(bytes.Repeat([]byte(" "), (2<<20)+1), valid...)
	if _, err := DecodeRussianProviderResponse(canonical, input, aboveFormerCap); err != nil {
		t.Fatalf("response above former stage cap rejected: %v", err)
	}

	oversize := bytes.Repeat([]byte("x"), maxProviderResponseBytes+1)
	_, err = DecodeRussianProviderResponse(canonical, input, oversize)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) ||
		limitErr.Stage != "localization" ||
		limitErr.Kind != modelresearch.ResourceLimitResponseBytes ||
		limitErr.Limit != maxProviderResponseBytes ||
		limitErr.Observed != len(oversize) {
		t.Fatalf("terminal response limit = %#v", err)
	}
}

func TestDecodeRussianProviderResponseRejectsIncompleteOrUnboundOutput(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]ProviderTranslation, len(input.Fields))
	for index := range entries {
		entries[index] = NewProviderTranslation(index, "Русский текст")
	}
	tests := []struct {
		name   string
		mutate func(*ProviderResponse)
	}{
		{name: "missing", mutate: func(response *ProviderResponse) {
			response.Translations = response.Translations[:len(response.Translations)-1]
		}},
		{name: "reordered", mutate: func(response *ProviderResponse) { index := 1; response.Translations[0].Index = &index }},
		{name: "wrong hash", mutate: func(response *ProviderResponse) { response.CanonicalSHA256 = "invalid" }},
		{name: "wrong locale", mutate: func(response *ProviderResponse) { response.Locale = LocaleEnglish }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := ProviderResponse{
				Version: ProviderResponseVersion, CanonicalSHA256: canonical.SHA256,
				Locale:       LocaleRussian,
				Translations: append([]ProviderTranslation(nil), entries...),
			}
			test.mutate(&response)
			encoded, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeRussianProviderResponse(canonical, input, encoded); err == nil {
				t.Fatal("invalid provider response unexpectedly decoded")
			}
		})
	}

	valid, err := json.Marshal(ProviderResponse{
		Version: ProviderResponseVersion, CanonicalSHA256: canonical.SHA256,
		Locale: LocaleRussian, Translations: entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{
		append(valid, []byte("\n{}")...),
		[]byte(`{"version":1,"canonical_sha256":"x","locale":"ru","translations":[],"unknown":true}`),
	} {
		if _, err := DecodeRussianProviderResponse(canonical, input, invalid); err == nil {
			t.Fatal("non-strict provider response unexpectedly decoded")
		}
	}
	for _, invalid := range []string{
		`[0]`,
		`[null,"Русский текст"]`,
		`[0,null]`,
		`{"index":0,"text":"Русский текст"}`,
	} {
		var translation ProviderTranslation
		if err := json.Unmarshal([]byte(invalid), &translation); err == nil {
			t.Fatalf("invalid provider translation %s unexpectedly decoded", invalid)
		}
	}
}
