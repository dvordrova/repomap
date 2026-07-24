package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"

	"github.com/dvordrova/repomap/internal/freshness"

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

//go:embed templates/surface_catalog.css
var surfaceCatalogCSS string

//go:embed templates/surface_catalog.js
var surfaceCatalogJS string

//go:embed templates/semantic_search.css
var semanticSearchCSS string

//go:embed templates/semantic_search.js
var semanticSearchJS string

//go:embed templates/script.js
var scriptJS string

var reportTmpl *template.Template

func init() {
	reportTmpl = template.Must(template.New("report").Parse(templateHTML))
}

func WriteReportJSON(data *ReportData, path string) error {
	if data == nil {
		return fmt.Errorf("report: data is required")
	}
	persisted := reportDataForRendering(data)
	// SourceIDs are issued by the local report server after manifest
	// verification. They are session navigation IDs, not persistent evidence.
	persisted.SourceIDs = nil
	persisted.SourceContextIDs = nil
	b, err := json.MarshalIndent(persisted, "", "  ")
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
// assets. Servers should use this instead of executing a saved HTML artifact.
func RenderHTML(data *ReportData) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	return buildHTML(data)
}

func buildHTML(data *ReportData) ([]byte, error) {
	rendered := reportDataForRendering(data)
	dataJSON, err := json.Marshal(rendered)
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
		"HasDiscoveredSurfaces": data.DiscoveredSurfaces != nil,
		"SurfaceCatalogCSS":     template.CSS(surfaceCatalogCSS),
		"SurfaceCatalogJS":      template.JS(surfaceCatalogJS),
		"HasSemanticSearch":     rendered.SemanticSearch != nil && !rendered.SemanticSearchDisabled,
		"SemanticSearchCSS":     template.CSS(semanticSearchCSS),
		"SemanticSearchJS":      template.JS(semanticSearchJS),
		"DataJSON":              template.JS(dataJSON),
		"JS":                    template.JS(scriptJS),
	})
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func reportDataForRendering(data *ReportData) *ReportData {
	rendered := *data
	if rendered.SemanticSearchDisabled {
		rendered.SemanticSearch = nil
		return &rendered
	}
	if rendered.SemanticSearch != nil {
		if err := rendered.SemanticSearch.Validate(&rendered); err == nil {
			return &rendered
		}
	}
	rendered.SemanticSearch = nil
	_ = attachSemanticSearchIndex(&rendered)
	return &rendered
}

func Generate(runDir string) error {
	return generate(runDir, nil)
}

// GenerateAuthorized renders a report and binds its exact generated JSON to
// repository authority confirmed stable across orientation.
func GenerateAuthorized(runDir string, authority RunAuthority) error {
	return generate(runDir, &authority)
}

func generate(runDir string, authority *RunAuthority) error {
	if err := RemoveRunManifest(runDir); err != nil {
		return err
	}
	if authority != nil {
		if err := authority.validate(); err != nil {
			return err
		}
	}
	studyDocumentSourceRoot := ""
	if authority != nil {
		studyDocumentSourceRoot = authority.analysisRoot
	}
	data, err := readRunDir(runDir, studyDocumentSourceRoot)
	if err != nil {
		return err
	}
	feedbackPath := runDir + "/onboarding-feedback.md"
	if err := ensureFeedbackTemplate(data, feedbackPath); err != nil {
		return err
	}
	data.FeedbackPath = feedbackPath
	if authority != nil {
		freshnessResult := authority.freshness
		data.Freshness = &freshnessResult
		data.CapturedRevision = authority.repository.Head
		bindOperationalRevision(data.Operations, data.CapturedRevision)
		if err := bindTaskInvestigationAuthority(data.TaskInvestigation, authority.repository); err != nil {
			return err
		}
		data.CapturedInputCount = len(authority.inputs)
		data.RepositorySubmodules = append([]freshness.SubmoduleState(nil), authority.repository.Submodules...)
		catalog, available, catalogErr := authorizedExactSearchCatalog(data, *authority)
		if catalogErr != nil {
			return catalogErr
		}
		if available {
			if err := AttachExactWorkspaceSearch(data, catalog); err != nil {
				return err
			}
		}
	}

	jsonPath := runDir + "/report.json"
	if err := WriteReportJSON(data, jsonPath); err != nil {
		return err
	}
	var reportJSON []byte
	if authority != nil {
		reportJSON, err = os.ReadFile(jsonPath)
		if err != nil {
			return fmt.Errorf("read generated report json: %w", err)
		}
	}

	htmlPath := runDir + "/report.html"
	if err := WriteReportHTML(data, htmlPath); err != nil {
		return err
	}
	if authority != nil {
		if err := writeAuthorizedRunManifest(runDir, data, reportJSON, *authority); err != nil {
			return err
		}
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
