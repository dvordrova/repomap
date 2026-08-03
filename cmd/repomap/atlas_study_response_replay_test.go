package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	atlasStudyReplayRequestFixtureSHA  = "cf42141cda77aabf3db5ab3ae6ba0023bee87218c83af63b011c36ccfdab0563"
	atlasStudyReplayResponseFixtureSHA = "abdeb1c0738bb2fe0457d5e8d3662bcd05b3e1c6beccfd1283cb74102f0655eb"
)

func TestAtlasStudyResponseReplayCLIExactSavedResponse(t *testing.T) {
	runDir, responsePath := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	oldResult := []byte("old result must not be read or changed\n")
	oldStatus := []byte("old status must not be read or changed\n")
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(runDir, "atlas_study_result.v5.json"), oldResult)
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(runDir, "atlas_study_status.v5.json"), oldStatus)

	var stdout bytes.Buffer
	err := runAtlasStudyResponseReplayCLI([]string{
		"--run-dir", runDir,
		"--request-sha256", atlasStudyReplayRequestFixtureSHA,
		"--response", responsePath,
		"--response-sha256", atlasStudyReplayResponseFixtureSHA,
	}, &stdout)
	if err != nil {
		t.Fatalf("runAtlasStudyResponseReplayCLI: %v", err)
	}
	for _, want := range []string{
		"request_sha256: " + atlasStudyReplayRequestFixtureSHA,
		"response_sha256: " + atlasStudyReplayResponseFixtureSHA,
		"result_sha256: ",
		"status_sha256: ",
		"directions: 5",
		"provider_calls: 0",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}

	resultRaw, err := os.ReadFile(filepath.Join(runDir, atlasstudy.ResultArtifactFilename))
	if err != nil {
		t.Fatalf("read v6 result: %v", err)
	}
	result, err := atlasstudy.DecodeResultRecord(resultRaw)
	if err != nil {
		t.Fatalf("decode v6 result: %v", err)
	}
	readingCounts := make([]int, len(result.Directions))
	for index, direction := range result.Directions {
		readingCounts[index] = len(direction.Reading)
	}
	if !slices.Equal(readingCounts, []int{1, 3, 4, 1, 1}) {
		t.Fatalf("reading cardinalities = %v", readingCounts)
	}
	statusRaw, err := os.ReadFile(filepath.Join(runDir, atlasstudy.StatusArtifactFilename))
	if err != nil {
		t.Fatalf("read v6 status: %v", err)
	}
	status, err := atlasstudy.DecodeStatus(statusRaw)
	if err != nil {
		t.Fatalf("decode v6 status: %v", err)
	}
	if status.State != atlasstudy.ProductStateAccepted || status.DirectionCount != 5 {
		t.Fatalf("status = %+v", status)
	}
	assertAtlasStudyReplayTestFile(t, filepath.Join(runDir, "atlas_study_result.v5.json"), oldResult)
	assertAtlasStudyReplayTestFile(t, filepath.Join(runDir, "atlas_study_status.v5.json"), oldStatus)
}

func TestAtlasStudyResponseReplayCLIRequiresExactHashes(t *testing.T) {
	for _, test := range []struct {
		name        string
		requestSHA  string
		responseSHA string
		want        string
	}{
		{name: "wrong request", requestSHA: strings.Repeat("0", 64), responseSHA: atlasStudyReplayResponseFixtureSHA, want: "request SHA-256 mismatch"},
		{name: "wrong response", requestSHA: atlasStudyReplayRequestFixtureSHA, responseSHA: strings.Repeat("0", 64), want: "response SHA-256 mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runDir, responsePath := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
			var stdout bytes.Buffer
			err := runAtlasStudyResponseReplayCLI([]string{
				"--run-dir", runDir,
				"--request-sha256", test.requestSHA,
				"--response", responsePath,
				"--response-sha256", test.responseSHA,
			}, &stdout)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("failure wrote stdout: %q", stdout.String())
			}
			assertAtlasStudyReplayOutputsAbsent(t, runDir)
		})
	}
}

func TestAtlasStudyResponseReplayCLIRejectsUnsafeResponseFiles(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		runDir, responsePath := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
		link := filepath.Join(t.TempDir(), "response-link.json")
		if err := os.Symlink(responsePath, link); err != nil {
			t.Fatal(err)
		}
		err := runAtlasStudyReplayTestCLI(runDir, link, atlasStudyReplayResponseFixtureSHA, nil)
		if err == nil || !strings.Contains(err.Error(), "regular non-symlink") {
			t.Fatalf("symlink error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, runDir)
	})

	t.Run("nonregular", func(t *testing.T) {
		runDir, _ := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
		responseDir := filepath.Join(t.TempDir(), "response-dir")
		if err := os.Mkdir(responseDir, 0o700); err != nil {
			t.Fatal(err)
		}
		err := runAtlasStudyReplayTestCLI(runDir, responseDir, strings.Repeat("0", 64), nil)
		if err == nil || !strings.Contains(err.Error(), "regular non-symlink") {
			t.Fatalf("nonregular error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, runDir)
	})

	t.Run("inside target run", func(t *testing.T) {
		runDir, responsePath := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
		inside := filepath.Join(runDir, "semantic_exchanges", "exact-response.json")
		if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
			t.Fatal(err)
		}
		responseRaw, err := os.ReadFile(responsePath)
		if err != nil {
			t.Fatal(err)
		}
		mustWriteAtlasStudyReplayTestFile(t, inside, responseRaw)
		err = runAtlasStudyReplayTestCLI(runDir, inside, atlasStudyReplayResponseFixtureSHA, nil)
		if err == nil || !strings.Contains(err.Error(), "outside the target run") {
			t.Fatalf("inside-run error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, runDir)
	})
}

func TestAtlasStudyResponseReplayCLIMandatorySecretScanDoesNotEcho(t *testing.T) {
	runDir, _ := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	credential := "Bearer replaycredentialvalue123456789"
	responsePath := filepath.Join(t.TempDir(), "unsafe-response.json")
	mustWriteAtlasStudyReplayTestFile(t, responsePath, []byte(credential))
	restore := secretscan.SetDisabled(true)
	defer restore()

	err := runAtlasStudyReplayTestCLI(runDir, responsePath, atlasStudyReplaySHA256([]byte(credential)), nil)
	if err == nil || !strings.Contains(err.Error(), "bearer_credential") {
		t.Fatalf("secret response error = %v", err)
	}
	if strings.Contains(err.Error(), credential) || strings.Contains(err.Error(), "replaycredentialvalue") {
		t.Fatalf("secret response error echoed credential: %v", err)
	}
	assertAtlasStudyReplayOutputsAbsent(t, runDir)
}

func TestAtlasStudyResponseReplayCLIRejectsOriginalAndPreexistingOutputs(t *testing.T) {
	t.Run("original canonical run", func(t *testing.T) {
		runDir, responsePath := writeAtlasStudyResponseReplayFixture(t, "canonical-run", "canonical-run")
		err := runAtlasStudyReplayTestCLI(runDir, responsePath, atlasStudyReplayResponseFixtureSHA, nil)
		if err == nil || !strings.Contains(err.Error(), "original canonical run") {
			t.Fatalf("original-run error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, runDir)
	})

	for _, output := range []string{atlasstudy.ResultArtifactFilename, atlasstudy.StatusArtifactFilename} {
		t.Run(output, func(t *testing.T) {
			runDir, responsePath := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
			sentinel := []byte("must not overwrite\n")
			mustWriteAtlasStudyReplayTestFile(t, filepath.Join(runDir, output), sentinel)
			err := runAtlasStudyReplayTestCLI(runDir, responsePath, atlasStudyReplayResponseFixtureSHA, nil)
			if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
				t.Fatalf("preexisting-output error = %v", err)
			}
			assertAtlasStudyReplayTestFile(t, filepath.Join(runDir, output), sentinel)
		})
	}
}

func TestAtlasStudyResponseReplayCLIRejectsCanonicalRequestMaterialMismatch(t *testing.T) {
	runDir, responsePath := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	requestRaw, err := os.ReadFile(filepath.Join(runDir, atlasstudy.RequestArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	request, err := atlasstudy.DecodeRequestRecord(requestRaw)
	if err != nil {
		t.Fatal(err)
	}
	request.CatalogSHA256 = strings.Repeat("0", 64)
	request.CatalogRef = "atlas-study-v5-" + request.CatalogSHA256
	tampered, err := atlasstudy.EncodeRequestRecord(request)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(runDir, atlasstudy.RequestArtifactFilename), tampered)

	var stdout bytes.Buffer
	err = runAtlasStudyResponseReplayCLI([]string{
		"--run-dir", runDir,
		"--request-sha256", atlasStudyReplaySHA256(tampered),
		"--response", responsePath,
		"--response-sha256", atlasStudyReplayResponseFixtureSHA,
	}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "catalog hash mismatch") {
		t.Fatalf("request-material mismatch error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure wrote stdout: %q", stdout.String())
	}
	assertAtlasStudyReplayOutputsAbsent(t, runDir)
}

func writeAtlasStudyResponseReplayFixture(t *testing.T, dirName, metadataRunID string) (string, string) {
	t.Helper()
	parent := t.TempDir()
	runDir := filepath.Join(parent, dirName)
	writer, err := debugdump.NewWriter(parent, dirName, false)
	if err != nil {
		t.Fatalf("create test run: %v", err)
	}
	if err := writer.WriteMetadata(debugdump.RunMeta{
		RunID:   metadataRunID,
		Command: "atlas-first",
	}); err != nil {
		writer.Close()
		t.Fatalf("write metadata: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close test writer: %v", err)
	}
	requestRaw, err := os.ReadFile("../../internal/atlasstudy/testdata/casdoor_20260803_075743_request_v5.json")
	if err != nil {
		t.Fatalf("read request fixture: %v", err)
	}
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(runDir, atlasstudy.RequestArtifactFilename), requestRaw)
	responseRaw, err := os.ReadFile("../../internal/atlasstudy/testdata/casdoor_20260803_075743_response.json")
	if err != nil {
		t.Fatalf("read response fixture: %v", err)
	}
	responsePath := filepath.Join(parent, "saved-response.json")
	mustWriteAtlasStudyReplayTestFile(t, responsePath, responseRaw)
	return runDir, responsePath
}

func runAtlasStudyReplayTestCLI(runDir, responsePath, responseSHA string, stdout *bytes.Buffer) error {
	if stdout == nil {
		stdout = &bytes.Buffer{}
	}
	return runAtlasStudyResponseReplayCLI([]string{
		"--run-dir", runDir,
		"--request-sha256", atlasStudyReplayRequestFixtureSHA,
		"--response", responsePath,
		"--response-sha256", responseSHA,
	}, stdout)
}

func mustWriteAtlasStudyReplayTestFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func assertAtlasStudyReplayOutputsAbsent(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range []string{atlasstudy.ResultArtifactFilename, atlasstudy.StatusArtifactFilename} {
		if _, err := os.Lstat(filepath.Join(runDir, name)); !os.IsNotExist(err) {
			t.Fatalf("output %s exists or cannot be inspected: %v", name, err)
		}
	}
}

func assertAtlasStudyReplayTestFile(t *testing.T, name string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s changed: got %q want %q", name, got, want)
	}
}
