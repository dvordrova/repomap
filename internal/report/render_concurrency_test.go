package report

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// Publication renders the page the local server serves beside report.html.
// Both renders start from shallow copies of one ReportData, so neither may
// write anything the other reads, and each must produce the page it produces
// alone. The race detector is what makes this test meaningful.
func TestStampedAndServedRendersOfOneReportDataRunSideBySide(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	options := reportSingleTargetRenderOptionsFixture(t, &data)
	stamped, stampedOptions := data, options
	stampedOptions.ReportSHA256 = strings.Repeat("f", 64)
	served, servedOptions := data, options
	served.SourceIDs = make(map[string]string, len(data.OpenablePaths))
	for position, sourcePath := range data.OpenablePaths {
		served.SourceIDs[sourcePath] = fmt.Sprintf("%043d", position)
	}
	servedOptions.LocalRoots = []string{"/tmp/report-run", "/tmp/repository"}

	wantStamped, err := RenderHTMLWithOptions(&stamped, stampedOptions)
	if err != nil {
		t.Fatal(err)
	}
	wantServed, err := RenderHTMLWithOptions(&served, servedOptions)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(wantStamped, wantServed) {
		t.Fatal("the stamped and served pages must differ by their stamp and source ids")
	}
	for round := 0; round < 3; round++ {
		var rendering sync.WaitGroup
		var gotStamped, gotServed []byte
		var stampedErr, servedErr error
		rendering.Add(2)
		go func() {
			defer rendering.Done()
			copied := data
			gotStamped, stampedErr = RenderHTMLWithOptions(&copied, stampedOptions)
		}()
		go func() {
			defer rendering.Done()
			copied := served
			gotServed, servedErr = RenderHTMLWithOptions(&copied, servedOptions)
		}()
		rendering.Wait()
		if stampedErr != nil || servedErr != nil {
			t.Fatalf("concurrent renders: %v, %v", stampedErr, servedErr)
		}
		if !bytes.Equal(gotStamped, wantStamped) || !bytes.Equal(gotServed, wantServed) {
			t.Fatalf("round %d: a concurrent render differs from its render alone", round)
		}
	}
}
