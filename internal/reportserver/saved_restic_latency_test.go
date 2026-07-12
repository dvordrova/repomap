package reportserver

import (
	"io"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestSavedResticReportLatency(t *testing.T) {
	runsDir := os.Getenv("REPOMAP_SAVED_RESTIC_RUNS")
	if runsDir == "" {
		t.Skip("set REPOMAP_SAVED_RESTIC_RUNS to exercise the owner-provided offline run")
	}
	const runID = "20260712-210947-restic"
	started := time.Now()
	handler, err := NewHandler(Options{
		RunsDir: runsDir, InitialRunID: runID, Capability: testCapability,
	})
	if err != nil {
		t.Fatal(err)
	}
	startup := time.Since(started)
	server := httptest.NewServer(handler)
	defer server.Close()

	requestStarted := time.Now()
	response, err := server.Client().Get(server.URL + capabilityURLPrefix(testCapability) + "/runs/" + runID + "/report.html")
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read/close report: %v / %v", readErr, closeErr)
	}
	requestLatency := time.Since(requestStarted)
	t.Logf("saved restic startup=%s report_request=%s", startup, requestLatency)
	if startup > 5*time.Second || requestLatency > 5*time.Second {
		t.Fatalf("saved restic report exceeded latency budget: startup=%s request=%s", startup, requestLatency)
	}
}
