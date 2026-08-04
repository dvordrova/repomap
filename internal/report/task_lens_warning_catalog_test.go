package report

import (
	"os"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/tasklens"
)

func TestTaskInvestigationWarningProjectionIsRenderOnly(t *testing.T) {
	canonicalWarning := "Evidence join 2 was rejected locally: document support lacks document evidence."
	data := &ReportData{
		FormatVersion: CurrentFormatVersion,
		RepoName:      "example.test/repository",
		TaskInvestigation: &TaskInvestigationWorkspace{
			TaskID:   "task-1234567890abcdef",
			Warnings: []string{canonicalWarning},
			warningDiagnostics: []tasklens.WarningDiagnostic{{
				Code:  tasklens.WarningJoinRejected,
				Index: 2,
			}},
		},
	}

	jsonPath := t.TempDir() + "/report.json"
	if err := WriteReportJSON(data, jsonPath); err != nil {
		t.Fatal(err)
	}
	canonicalJSON, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(canonicalJSON), canonicalWarning) ||
		strings.Contains(string(canonicalJSON), "presentation_warnings") ||
		strings.Contains(string(canonicalJSON), string(tasklens.WarningJoinRejected)) {
		t.Fatalf("canonical report warning changed:\n%s", canonicalJSON)
	}

	rendered := reportDataForRendering(data)
	if rendered.TaskInvestigation == data.TaskInvestigation ||
		len(rendered.TaskInvestigation.PresentationWarnings) != 1 ||
		rendered.TaskInvestigation.PresentationWarnings[0].MessageID != "main.task_lens.warning.join_rejected" ||
		rendered.TaskInvestigation.PresentationWarnings[0].Index != 2 {
		t.Fatalf("render-only warning projection = %#v", rendered.TaskInvestigation)
	}
	if len(data.TaskInvestigation.PresentationWarnings) != 0 ||
		data.TaskInvestigation.Warnings[0] != canonicalWarning {
		t.Fatalf("render projection mutated canonical workspace = %#v", data.TaskInvestigation)
	}

	localizedClone, err := cloneReportData(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(localizedClone.TaskInvestigation.warningDiagnostics) != 1 ||
		localizedClone.TaskInvestigation.warningDiagnostics[0].Code != tasklens.WarningJoinRejected {
		t.Fatalf(
			"localization clone lost typed warning diagnostics = %#v",
			localizedClone.TaskInvestigation,
		)
	}
}
