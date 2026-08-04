package report

import (
	"bytes"
	"testing"
)

func TestRenderBrowserErrorHTMLUsesTypedCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind      BrowserErrorKind
		messageID string
		russian   string
	}{
		{
			BrowserErrorNoSavedReports,
			"main.browser_error.no_saved_reports",
			"Сохранённых отчётов нет.",
		},
		{
			BrowserErrorReportNotFound,
			"main.browser_error.report_not_found",
			"Сохранённый отчёт не найден.",
		},
		{
			BrowserErrorAuthorityUnavailable,
			"main.browser_error.report_authority_unavailable",
			"Сохранённый отчёт нельзя открыть: подтверждённые локальные данные недоступны.",
		},
		{
			BrowserErrorReportUnavailable,
			"main.browser_error.report_temporarily_unavailable",
			"Сейчас не удалось открыть сохранённый отчёт.",
		},
		{
			BrowserErrorInvalidArtifact,
			"main.browser_error.invalid_report_artifact",
			"Сохранённый отчёт повреждён или превышает допустимый размер.",
		},
	}
	for _, locale := range []string{"en", "ru"} {
		locale := locale
		for _, test := range tests {
			test := test
			t.Run(locale+"/"+string(test.kind), func(t *testing.T) {
				t.Parallel()
				got, err := RenderBrowserErrorHTML(locale, test.kind)
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range [][]byte{
					[]byte(`<html lang="` + locale + `">`),
					[]byte(`data-rm-message="` + test.messageID + `"`),
					[]byte("window.RepomapUI.message"),
				} {
					if !bytes.Contains(got, want) {
						t.Fatalf("browser error HTML is missing %q", want)
					}
				}
				if locale == "ru" && !bytes.Contains(got, []byte(test.russian)) {
					t.Fatalf("browser error catalog is missing Russian copy %q", test.russian)
				}
			})
		}
	}
}

func TestRenderBrowserErrorHTMLRejectsUnknownKind(t *testing.T) {
	t.Parallel()

	if _, err := RenderBrowserErrorHTML("ru", BrowserErrorKind("unknown")); err == nil {
		t.Fatal("unknown browser error kind was accepted")
	}
}
