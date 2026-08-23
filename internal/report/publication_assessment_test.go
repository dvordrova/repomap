package report

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAssessRunPublicationFailsClosedWithoutManifestAuthority(t *testing.T) {
	runDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(runDir, "report.json"),
		[]byte(`{}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	assessment, err := AssessRunPublication(runDir)
	if err == nil {
		t.Fatal("report existence without a verified manifest was accepted")
	}
	want := FailedPublicationAssessment()
	if !reflect.DeepEqual(assessment, want) {
		t.Fatalf("assessment = %#v, want %#v", assessment, want)
	}
}
