package reportserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/repoconfig"
)

func TestConfiguredEditorIsBoundOnceForTheServer(t *testing.T) {
	if os.Getenv("REPOMAP_EDITOR_HELPER") == "1" {
		cwd, _ := os.Getwd()
		data, _ := json.Marshal(struct {
			Args []string
			Cwd  string
		}{os.Args[1:], cwd})
		if err := os.WriteFile(os.Getenv("REPOMAP_EDITOR_RECORD"), data, 0o600); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	fixture := writeTestRun(t)
	root := filepath.Dir(fixture.sourcePath)
	record := filepath.Join(t.TempDir(), "args")
	t.Setenv("REPOMAP_EDITOR_HELPER", "1")
	t.Setenv("REPOMAP_EDITOR_RECORD", record)
	filename := filepath.Join(root, repoconfig.Filename)
	config := fmt.Sprintf("editor: [%q, '-test.run=TestConfiguredEditorIsBoundOnceForTheServer', '--', first-editor, '{{ .File }}:{{ .Line }}:{{ .Column }}']\n", os.Args[0])
	if err := os.WriteFile(filename, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(Options{RunsDir: fixture.runsDir, InitialRunID: fixture.runID, Capability: testCapability})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	for _, nextConfig := range []string{"editor: [second-editor]\n", "broken: [\n"} {
		if err := os.WriteFile(filename, []byte(nextConfig), 0o600); err != nil {
			t.Fatal(err)
		}
		response := postOpen(t, server.URL+capabilityURLPrefix(testCapability), openRequest{RunID: fixture.runID, SourceID: fixture.sourceID, Line: 42, Column: 7})
		readResponse(t, response, http.StatusOK)
		data, err := os.ReadFile(record)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Args []string
			Cwd  string
		}
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Args) != 4 || got.Args[2] != "first-editor" || got.Args[3] != fixture.sourcePath+":42:7" || got.Cwd != root {
			t.Fatalf("configured source open: %+v", got)
		}
	}
}

func TestHandlerServesWithoutAnEditorAndExplainsFailedOpen(t *testing.T) {
	fixture := writeTestRun(t)
	t.Setenv("PATH", t.TempDir())
	h, err := NewHandler(Options{RunsDir: fixture.runsDir, InitialRunID: fixture.runID, Capability: testCapability})
	if err != nil {
		t.Fatalf("missing editor prevented serving: %v", err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	baseURL := server.URL + capabilityURLPrefix(testCapability)
	response, err := server.Client().Get(baseURL + "/runs/" + fixture.runID + "/report.html")
	if err != nil {
		t.Fatal(err)
	}
	readResponse(t, response, http.StatusOK)
	response = postOpen(t, baseURL, openRequest{RunID: fixture.runID, SourceID: fixture.sourceID, Line: 7})
	body := readResponse(t, response, http.StatusBadGateway)
	if !strings.Contains(string(body), repoconfig.Filename) {
		t.Fatalf("open error omits editable config: %s", body)
	}
}
