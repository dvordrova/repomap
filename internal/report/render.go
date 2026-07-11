package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"

	_ "embed"
)

//go:embed templates/report.html
var templateHTML string

//go:embed templates/style.css
var styleCSS string

//go:embed templates/architecture_canvas.css
var architectureCanvasCSS string

//go:embed templates/architecture_canvas.js
var architectureCanvasJS string

//go:embed templates/script.js
var scriptJS string

var reportTmpl *template.Template

func init() {
	reportTmpl = template.Must(template.New("report").Parse(templateHTML))
}

func WriteReportJSON(data *ReportData, path string) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func WriteReportHTML(data *ReportData, path string) error {
	html, err := RenderHTML(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, html, 0o644)
}

// RenderHTML renders report data with repomap's embedded trusted template and
// assets. Preview and report servers use it without executing a saved HTML
// artifact.
func RenderHTML(data *ReportData) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	return buildHTML(data)
}

func buildHTML(data *ReportData) ([]byte, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	title := data.RepoName
	if data.ProjectGuess != "" {
		title = title + " — " + data.ProjectGuess
	}

	var buf bytes.Buffer
	err = reportTmpl.Execute(&buf, map[string]any{
		"Title":                 title,
		"CSS":                   template.CSS(styleCSS),
		"HasArchitectureCanvas": data.ArchitectureCanvas != nil,
		"ArchitectureCanvasCSS": template.CSS(architectureCanvasCSS),
		"ELKJS":                 template.JS(elkJSBundledJS),
		"ArchitectureCanvasJS":  template.JS(architectureCanvasJS),
		"DataJSON":              template.JS(dataJSON),
		"JS":                    template.JS(scriptJS),
	})
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func Generate(runDir string) error {
	data, err := ReadRunDir(runDir)
	if err != nil {
		return err
	}
	feedbackPath := runDir + "/onboarding-feedback.md"
	if err := ensureFeedbackTemplate(data, feedbackPath); err != nil {
		return err
	}
	data.FeedbackPath = feedbackPath

	jsonPath := runDir + "/report.json"
	if err := WriteReportJSON(data, jsonPath); err != nil {
		return err
	}

	htmlPath := runDir + "/report.html"
	if err := WriteReportHTML(data, htmlPath); err != nil {
		return err
	}
	return nil
}

func ensureFeedbackTemplate(data *ReportData, path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create onboarding feedback template: %w", err)
	}
	defer file.Close()

	content := fmt.Sprintf(`# repomap onboarding feedback

Repository: %s
Direction followed:
First useful file:
Time to useful orientation:

## Correct

-

## Missing

-

## Misleading

-
`, data.RepoName)
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("write onboarding feedback template: %w", err)
	}
	return nil
}
