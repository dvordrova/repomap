package main

import "testing"

func TestNormalizeReportLanguage(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		input string
		want  string
	}{
		{input: "", want: "en"},
		{input: "en", want: "en"},
		{input: "EN", want: "en"},
		{input: " ru ", want: "ru"},
	} {
		got, err := normalizeReportLanguage(test.input)
		if err != nil || got != test.want {
			t.Errorf("normalizeReportLanguage(%q) = %q, %v, want %q", test.input, got, err, test.want)
		}
	}
	if _, err := normalizeReportLanguage("de"); err == nil {
		t.Fatal("normalizeReportLanguage(de) error = nil")
	}
}

func TestStoredReportLanguageKeepsOnlyRequestedPresentationLocale(t *testing.T) {
	t.Parallel()

	if storedReportLanguage("en") != "" || storedReportLanguage("ru") != "ru" {
		t.Fatal("stored report language does not preserve the optional English default")
	}
}
