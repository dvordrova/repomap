package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	atlasStudyD209ReplayRequestFixtureSHA  = "cf42141cda77aabf3db5ab3ae6ba0023bee87218c83af63b011c36ccfdab0563"
	atlasStudyD209ReplayResponseFixtureSHA = "abdeb1c0738bb2fe0457d5e8d3662bcd05b3e1c6beccfd1283cb74102f0655eb"
)

type atlasStudyReplayFixture struct {
	runDir       string
	responsePath string
	requestSHA   string
	responseSHA  string
}

func TestAtlasStudyResponseReplayCLIExactSavedResponse(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	oldResult := []byte("old result must not be read or changed\n")
	oldStatus := []byte("old status must not be read or changed\n")
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, "atlas_study_result.v6.json"), oldResult)
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, "atlas_study_status.v6.json"), oldStatus)

	var stdout bytes.Buffer
	err := runAtlasStudyResponseReplayCLI([]string{
		"--run-dir", fixture.runDir,
		"--request-sha256", fixture.requestSHA,
		"--response", fixture.responsePath,
		"--response-sha256", fixture.responseSHA,
	}, &stdout)
	if err != nil {
		t.Fatalf("runAtlasStudyResponseReplayCLI: %v", err)
	}
	for _, want := range []string{
		"request_sha256: " + fixture.requestSHA,
		"response_sha256: " + fixture.responseSHA,
		"result_sha256: ",
		"status_sha256: ",
		"directions: 1",
		"provider_calls: 0",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}

	resultRaw, err := os.ReadFile(filepath.Join(fixture.runDir, atlasstudy.ResultArtifactFilename))
	if err != nil {
		t.Fatalf("read current result: %v", err)
	}
	result, err := atlasstudy.DecodeResultRecord(resultRaw)
	if err != nil {
		t.Fatalf("decode current result: %v", err)
	}
	if result.Version != atlasstudy.ResultVersion ||
		len(result.Directions) != 1 || len(result.Directions[0].Reading) != 1 {
		t.Fatalf("D210 direction cardinalities = %#v", result.Directions)
	}
	statusRaw, err := os.ReadFile(filepath.Join(fixture.runDir, atlasstudy.StatusArtifactFilename))
	if err != nil {
		t.Fatalf("read current status: %v", err)
	}
	status, err := atlasstudy.DecodeStatus(statusRaw)
	if err != nil {
		t.Fatalf("decode current status: %v", err)
	}
	if status.Version != atlasstudy.ResultVersion || status.State != atlasstudy.ProductStateAccepted ||
		status.DirectionCount != 1 || !status.CoverageComplete {
		t.Fatalf("status = %+v", status)
	}
	assertAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, "atlas_study_result.v6.json"), oldResult)
	assertAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, "atlas_study_status.v6.json"), oldStatus)
}

func TestAtlasStudyResponseReplayCLIRejectsD209RequestAndResponse(t *testing.T) {
	parent := t.TempDir()
	runDir := filepath.Join(parent, "copied-review")
	writer, err := debugdump.NewWriter(parent, "copied-review", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteMetadata(debugdump.RunMeta{RunID: "original-canonical-run", Command: "atlas-first"}); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	requestRaw, err := os.ReadFile("../../internal/atlasstudy/testdata/casdoor_20260803_075743_request_v5.json")
	if err != nil {
		t.Fatal(err)
	}
	responseRaw, err := os.ReadFile("../../internal/atlasstudy/testdata/casdoor_20260803_075743_response.json")
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(runDir, atlasstudy.RequestArtifactFilename)
	responsePath := filepath.Join(parent, "d209-response.json")
	mustWriteAtlasStudyReplayTestFile(t, requestPath, requestRaw)
	mustWriteAtlasStudyReplayTestFile(t, responsePath, responseRaw)

	var stdout bytes.Buffer
	err = runAtlasStudyResponseReplayCLI([]string{
		"--run-dir", runDir,
		"--request-sha256", atlasStudyD209ReplayRequestFixtureSHA,
		"--response", responsePath,
		"--response-sha256", atlasStudyD209ReplayResponseFixtureSHA,
	}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "canonical v6 request") {
		t.Fatalf("D209 replay error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("D209 rejection wrote stdout: %q", stdout.String())
	}
	assertAtlasStudyReplayOutputsAbsent(t, runDir)
}

func TestAtlasStudyResponseReplayCLIRequiresExactHashes(t *testing.T) {
	for _, test := range []struct {
		name        string
		requestSHA  string
		responseSHA string
		want        string
	}{
		{name: "wrong request", requestSHA: strings.Repeat("0", 64), want: "request SHA-256 mismatch"},
		{name: "wrong response", responseSHA: strings.Repeat("0", 64), want: "response SHA-256 mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
			requestSHA := test.requestSHA
			if requestSHA == "" {
				requestSHA = fixture.requestSHA
			}
			responseSHA := test.responseSHA
			if responseSHA == "" {
				responseSHA = fixture.responseSHA
			}
			var stdout bytes.Buffer
			err := runAtlasStudyResponseReplayCLI([]string{
				"--run-dir", fixture.runDir,
				"--request-sha256", requestSHA,
				"--response", fixture.responsePath,
				"--response-sha256", responseSHA,
			}, &stdout)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("failure wrote stdout: %q", stdout.String())
			}
			assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
		})
	}
}

func TestAtlasStudyResponseReplayCLIRejectsUnsafeResponseFiles(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
		link := filepath.Join(t.TempDir(), "response-link.json")
		if err := os.Symlink(fixture.responsePath, link); err != nil {
			t.Fatal(err)
		}
		err := runAtlasStudyReplayTestCLI(fixture, link, fixture.responseSHA, nil)
		if err == nil || !strings.Contains(err.Error(), "regular non-symlink") {
			t.Fatalf("symlink error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
	})

	t.Run("nonregular", func(t *testing.T) {
		fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
		responseDir := filepath.Join(t.TempDir(), "response-dir")
		if err := os.Mkdir(responseDir, 0o700); err != nil {
			t.Fatal(err)
		}
		err := runAtlasStudyReplayTestCLI(fixture, responseDir, strings.Repeat("0", 64), nil)
		if err == nil || !strings.Contains(err.Error(), "regular non-symlink") {
			t.Fatalf("nonregular error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
	})

	t.Run("inside target run", func(t *testing.T) {
		fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
		inside := filepath.Join(fixture.runDir, "semantic_exchanges", "exact-response.json")
		if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
			t.Fatal(err)
		}
		responseRaw, err := os.ReadFile(fixture.responsePath)
		if err != nil {
			t.Fatal(err)
		}
		mustWriteAtlasStudyReplayTestFile(t, inside, responseRaw)
		err = runAtlasStudyReplayTestCLI(fixture, inside, fixture.responseSHA, nil)
		if err == nil || !strings.Contains(err.Error(), "outside the target run") {
			t.Fatalf("inside-run error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
	})
}

func TestAtlasStudyResponseReplayCLIMandatorySecretScanDoesNotEcho(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	credential := "Bearer replaycredentialvalue123456789"
	responsePath := filepath.Join(t.TempDir(), "unsafe-response.json")
	mustWriteAtlasStudyReplayTestFile(t, responsePath, []byte(credential))
	restore := secretscan.SetDisabled(true)
	defer restore()

	err := runAtlasStudyReplayTestCLI(fixture, responsePath, atlasStudyReplaySHA256([]byte(credential)), nil)
	if err == nil || !strings.Contains(err.Error(), "bearer_credential") {
		t.Fatalf("secret response error = %v", err)
	}
	if strings.Contains(err.Error(), credential) || strings.Contains(err.Error(), "replaycredentialvalue") {
		t.Fatalf("secret response error echoed credential: %v", err)
	}
	assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
}

func TestAtlasStudyResponseReplayCLIRejectsOriginalAndPreexistingOutputs(t *testing.T) {
	t.Run("original canonical run", func(t *testing.T) {
		fixture := writeAtlasStudyResponseReplayFixture(t, "canonical-run", "canonical-run")
		err := runAtlasStudyReplayTestCLI(fixture, fixture.responsePath, fixture.responseSHA, nil)
		if err == nil || !strings.Contains(err.Error(), "original canonical run") {
			t.Fatalf("original-run error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
	})

	for _, output := range []string{atlasstudy.ResultArtifactFilename, atlasstudy.StatusArtifactFilename} {
		t.Run(output, func(t *testing.T) {
			fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
			sentinel := []byte("must not overwrite\n")
			mustWriteAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, output), sentinel)
			err := runAtlasStudyReplayTestCLI(fixture, fixture.responsePath, fixture.responseSHA, nil)
			if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
				t.Fatalf("preexisting-output error = %v", err)
			}
			assertAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, output), sentinel)
		})
	}
}

func TestAtlasStudyResponseReplayCLIRejectsCanonicalRequestMaterialMismatch(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	requestRaw, err := os.ReadFile(filepath.Join(fixture.runDir, atlasstudy.RequestArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	request, err := atlasstudy.DecodeRequestRecord(requestRaw)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.ReplaceAll(requestRaw, []byte(request.CatalogSHA256), []byte(strings.Repeat("0", 64)))
	if bytes.Equal(tampered, requestRaw) {
		t.Fatal("request catalog identity was not changed")
	}
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, atlasstudy.RequestArtifactFilename), tampered)

	var stdout bytes.Buffer
	err = runAtlasStudyResponseReplayCLI([]string{
		"--run-dir", fixture.runDir,
		"--request-sha256", atlasStudyReplaySHA256(tampered),
		"--response", fixture.responsePath,
		"--response-sha256", fixture.responseSHA,
	}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "catalog identity mismatch") {
		t.Fatalf("request-material mismatch error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure wrote stdout: %q", stdout.String())
	}
	assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
}

func writeAtlasStudyResponseReplayFixture(t *testing.T, dirName, metadataRunID string) atlasStudyReplayFixture {
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
	product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatalf("build D210 request fixture: %v", err)
	}
	requestRaw, err := atlasstudy.EncodeRequestRecord(request)
	if err != nil {
		t.Fatalf("encode D210 request fixture: %v", err)
	}
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(runDir, atlasstudy.RequestArtifactFilename), requestRaw)
	responseRaw := atlasStudyRuntimeResponse(t, product, false)
	responsePath := filepath.Join(parent, "saved-response.json")
	mustWriteAtlasStudyReplayTestFile(t, responsePath, responseRaw)
	return atlasStudyReplayFixture{
		runDir: runDir, responsePath: responsePath,
		requestSHA:  atlasStudyReplaySHA256(requestRaw),
		responseSHA: atlasStudyReplaySHA256(responseRaw),
	}
}

func runAtlasStudyReplayTestCLI(fixture atlasStudyReplayFixture, responsePath, responseSHA string, stdout *bytes.Buffer) error {
	if stdout == nil {
		stdout = &bytes.Buffer{}
	}
	return runAtlasStudyResponseReplayCLI([]string{
		"--run-dir", fixture.runDir,
		"--request-sha256", fixture.requestSHA,
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
