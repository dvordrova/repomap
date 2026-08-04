package report

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
)

// BrowserErrorKind identifies fixed product copy for an HTML report-route
// failure. It does not carry an internal diagnostic or change an HTTP status.
type BrowserErrorKind string

const (
	BrowserErrorNoSavedReports       BrowserErrorKind = "no_saved_reports"
	BrowserErrorReportNotFound       BrowserErrorKind = "report_not_found"
	BrowserErrorAuthorityUnavailable BrowserErrorKind = "report_authority_unavailable"
	BrowserErrorReportUnavailable    BrowserErrorKind = "report_temporarily_unavailable"
	BrowserErrorInvalidArtifact      BrowserErrorKind = "invalid_report_artifact"
)

//go:embed templates/browser_error.html
var browserErrorTemplateHTML string

var browserErrorTemplate = template.Must(
	template.New("browser-error").Parse(browserErrorTemplateHTML),
)

// RenderBrowserErrorHTML renders a bounded browser-facing error using the
// same typed EN/RU product-copy catalog as a successful report. Internal
// diagnostics deliberately remain outside the HTML response.
func RenderBrowserErrorHTML(locale string, kind BrowserErrorKind) ([]byte, error) {
	messageID, err := browserErrorMessageID(kind)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := browserErrorTemplate.Execute(&output, map[string]any{
		"Language":     normalizedReportLanguage(locale),
		"MessageID":    messageID,
		"UIMessagesJS": template.JS(uiMessagesJS),
	}); err != nil {
		return nil, fmt.Errorf("report: render browser error: %w", err)
	}
	return output.Bytes(), nil
}

func browserErrorMessageID(kind BrowserErrorKind) (string, error) {
	switch kind {
	case BrowserErrorNoSavedReports:
		return "main.browser_error.no_saved_reports", nil
	case BrowserErrorReportNotFound:
		return "main.browser_error.report_not_found", nil
	case BrowserErrorAuthorityUnavailable:
		return "main.browser_error.report_authority_unavailable", nil
	case BrowserErrorReportUnavailable:
		return "main.browser_error.report_temporarily_unavailable", nil
	case BrowserErrorInvalidArtifact:
		return "main.browser_error.invalid_report_artifact", nil
	default:
		return "", fmt.Errorf("report: unsupported browser error kind %q", kind)
	}
}
