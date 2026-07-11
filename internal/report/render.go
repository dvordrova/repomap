package report

import (
	"bytes"
	"encoding/json"
	"os"
	"text/template"

	_ "embed"
)

//go:embed templates/report.html
var templateHTML string

//go:embed templates/style.css
var styleCSS string

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
	html, err := buildHTML(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, html, 0o644)
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
		"Title":    title,
		"CSS":      styleCSS,
		"DataJSON": string(dataJSON),
		"JS":       scriptJS,
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
