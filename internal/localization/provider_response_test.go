package localization

import (
	"encoding/json"
	"testing"
)

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
