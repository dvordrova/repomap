package debugdump

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSemanticExchangeRecordsExactSafeBytesAndUnsafeMarker(t *testing.T) {
	t.Parallel()

	w, err := NewWriter(t.TempDir(), "semantic", false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	safeRequest := []byte(`{"model":"fixture","messages":[]}`)
	safeResponse := []byte(`{"answer":"bounded"}`)
	w.RecordSemanticExchange(SemanticExchange{
		Stage: SemanticStageOrientation, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: SemanticRequestPrepared,
		State:             SemanticStateAccepted, ValidationCode: SemanticValidationAccepted,
		SemanticCalls: 1, TransportAttempts: 2,
		Request: safeRequest, Response: safeResponse,
	})
	unsafe := []byte(`{"answer":"sk-abcdefghijklmnop"}`)
	w.RecordSemanticExchange(SemanticExchange{
		Stage: SemanticStageTargetedResearch, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: SemanticRequestPrepared,
		State:             SemanticStateRejected, ValidationCode: SemanticValidationSecret,
		SemanticCalls: 1, TransportAttempts: 1,
		Request: safeRequest, Response: unsafe,
	})

	records := readSemanticExchangeRecords(t, w.RunDir())
	if len(records) != 2 {
		t.Fatalf("semantic exchange records = %d, want 2", len(records))
	}
	byStage := make(map[string]SemanticExchangeRecord, len(records))
	for _, record := range records {
		byStage[record.Stage] = record
	}
	safe := byStage[SemanticStageOrientation]
	if safe.RequestProvenance != SemanticRequestPrepared || safe.State != SemanticStateAccepted ||
		safe.SemanticCalls != 1 || safe.TransportAttempts != 2 {
		t.Fatalf("safe semantic metadata = %#v", safe)
	}
	assertSemanticPayloadIdentity(t, w.RunDir(), safe.Request, safeRequest)
	assertSemanticPayloadIdentity(t, w.RunDir(), safe.Response, safeResponse)

	rejected := byStage[SemanticStageTargetedResearch]
	if rejected.Response.Storage != "unsafe_marker" ||
		rejected.Response.UnsafeKind != "secret_key" ||
		rejected.Response.OriginalSHA256 != sha256Text(unsafe) ||
		rejected.Response.OriginalBytes != len(unsafe) {
		t.Fatalf("unsafe semantic response = %#v", rejected.Response)
	}
	marker := readSemanticPayload(t, w.RunDir(), rejected.Response)
	if bytes.Contains(marker, unsafe) || bytes.Contains(marker, []byte("abcdefghijklmnop")) {
		t.Fatalf("unsafe marker leaked provider content: %s", marker)
	}
}

func TestSemanticExchangeRedactsSensitiveFieldsAndBoundsOversizePayload(t *testing.T) {
	t.Parallel()

	w, err := NewWriter(t.TempDir(), "semantic-redaction", false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	request := []byte(`{"api_key":"sk-abcdefghijklmnop","normal":"kept"}`)
	oversize := bytes.Repeat([]byte("x"), maxSemanticExchangePayloadSize+1)
	w.RecordSemanticExchange(SemanticExchange{
		Stage: SemanticStageOrientation, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: SemanticRequestPrepared,
		State:             SemanticStateRejected, ValidationCode: SemanticValidationResponse,
		SemanticCalls: 1, TransportAttempts: 1,
		Request: request, Response: oversize,
	})
	record := readSemanticExchangeRecords(t, w.RunDir())[0]
	requestSaved := readSemanticPayload(t, w.RunDir(), record.Request)
	if bytes.Contains(requestSaved, []byte("abcdefghijklmnop")) ||
		!bytes.Contains(requestSaved, []byte(`"normal":"kept"`)) ||
		!bytes.Contains(requestSaved, []byte("[redacted]")) {
		t.Fatalf("request redaction = %s", requestSaved)
	}
	if record.Request.OriginalSHA256 != sha256Text(request) || record.Request.OriginalBytes != len(request) {
		t.Fatalf("request original identity = %#v", record.Request)
	}
	if record.Response.Storage != "raw_unavailable" ||
		record.Response.UnavailableCode != SemanticUnavailableSize ||
		record.Response.OriginalSHA256 != sha256Text(oversize) ||
		record.Response.OriginalBytes != len(oversize) {
		t.Fatalf("oversize response marker = %#v", record.Response)
	}
}

func TestSemanticExchangePublishesMetadataLast(t *testing.T) {
	t.Parallel()

	w, err := NewWriter(t.TempDir(), "semantic-partial", false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	exchange := SemanticExchange{
		Stage: SemanticStageOrientation, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: SemanticRequestPrepared,
		State:             SemanticStateAccepted, ValidationCode: SemanticValidationAccepted,
		SemanticCalls: 1, TransportAttempts: 1,
		Request: []byte(`{"request":true}`), Response: []byte(`{"response":true}`),
	}
	if err := w.writeSemanticExchange(exchange, func() error {
		return errors.New("injected after payloads")
	}); err == nil {
		t.Fatal("injected partial write succeeded")
	}
	directory := filepath.Join(w.RunDir(), SemanticExchangesDir, semanticExchangeKey(exchange))
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial exchange directory survived cleanup: %v", err)
	}
	if records := readSemanticExchangeRecords(t, w.RunDir()); len(records) != 0 {
		t.Fatalf("partial exchange was treated as committed: %#v", records)
	}
	if err := w.writeSemanticExchange(exchange, nil); err != nil {
		t.Fatalf("identical semantic exchange was not recoverable: %v", err)
	}
	if records := readSemanticExchangeRecords(t, w.RunDir()); len(records) != 1 {
		t.Fatalf("recovered semantic records = %d, want exactly 1", len(records))
	}
}

func TestSemanticExchangeWriteWarningIsBoundedAndDeduplicated(t *testing.T) {
	t.Parallel()

	w, err := NewWriter(t.TempDir(), "semantic-warning", false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var warnings bytes.Buffer
	w.SetWarningWriter(&warnings)
	exchange := SemanticExchange{
		Stage: SemanticStageTargetedResearch, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: SemanticRequestPrepared,
		State:             SemanticStateAccepted, ValidationCode: SemanticValidationAccepted,
		SemanticCalls: 1, TransportAttempts: 1,
		Request: []byte(`{"request":true}`), Response: []byte(`{"response":true}`),
	}
	w.RecordSemanticExchange(exchange)
	w.RecordSemanticExchange(exchange)
	w.RecordSemanticExchange(exchange)
	got := warnings.String()
	if strings.Count(got, "warning:") != 1 ||
		!strings.Contains(got, "stage=targeted_research") ||
		!strings.Contains(got, "code="+SemanticExchangeWarningCode) {
		t.Fatalf("semantic warning = %q", got)
	}
	for _, forbidden := range []string{w.RunDir(), "create semantic exchange", "file exists", "response"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("semantic warning leaked unbounded detail %q: %q", forbidden, got)
		}
	}
}

func TestSemanticExchangeSupportsConcurrentDistinctOwners(t *testing.T) {
	t.Parallel()

	w, err := NewWriter(t.TempDir(), "semantic-concurrent", false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var wait sync.WaitGroup
	for ordinal := 1; ordinal <= 12; ordinal++ {
		ordinal := ordinal
		wait.Add(1)
		go func() {
			defer wait.Done()
			w.RecordSemanticExchange(SemanticExchange{
				Stage:           SemanticStageStudyReview,
				InstanceOrdinal: ordinal, SemanticAttemptOrdinal: 1,
				RequestProvenance: SemanticRequestPrepared,
				State:             SemanticStateAccepted, ValidationCode: SemanticValidationAccepted,
				SemanticCalls: 1, TransportAttempts: 1,
				Request:  []byte(fmt.Sprintf(`{"request":%d}`, ordinal)),
				Response: []byte(fmt.Sprintf(`{"response":%d}`, ordinal)),
			})
		}()
	}
	wait.Wait()
	if records := readSemanticExchangeRecords(t, w.RunDir()); len(records) != 12 {
		t.Fatalf("concurrent semantic records = %d, want 12", len(records))
	}
}

func TestOpenWriterRejectsRunSymlink(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(base, "run")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := OpenWriter(link, true); err == nil {
		t.Fatal("OpenWriter accepted a run-directory symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside directory was touched: entries=%v err=%v", entries, err)
	}
}

func TestOpenWriterRecordsInsideExistingConfinedRun(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	created, err := NewWriter(base, "existing-run", true)
	if err != nil {
		t.Fatal(err)
	}
	runDir := created.RunDir()
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWriter(runDir, true)
	if err != nil {
		t.Fatal(err)
	}
	reopened.RecordSemanticExchange(SemanticExchange{
		Stage: SemanticStageOrientation, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: SemanticRequestPrepared,
		State:             SemanticStateAccepted, ValidationCode: SemanticValidationAccepted,
		SemanticCalls: 1, TransportAttempts: 1,
		Request: []byte(`{"request":true}`), Response: []byte(`{"response":true}`),
	})
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if records := readSemanticExchangeRecords(t, runDir); len(records) != 1 {
		t.Fatalf("reopened semantic records = %d, want 1", len(records))
	}
}

func TestSemanticExchangeRejectsSymlinkEscapeWithoutChangingCaller(t *testing.T) {
	t.Parallel()

	w, err := NewWriter(t.TempDir(), "semantic-symlink", true)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(w.RunDir(), SemanticExchangesDir)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	var warnings bytes.Buffer
	w.SetWarningWriter(&warnings)
	// RecordSemanticExchange has no failure return by design; a confined write
	// failure cannot be fed back into the caller's semantic result.
	w.RecordSemanticExchange(SemanticExchange{
		Stage: SemanticStageOrientation, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: SemanticRequestPrepared,
		State:             SemanticStateAccepted, ValidationCode: SemanticValidationAccepted,
		SemanticCalls: 1, TransportAttempts: 1,
		Request: []byte(`{"request":true}`), Response: []byte(`{"response":true}`),
	})
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("semantic exchange escaped run root: entries=%v err=%v", entries, err)
	}
	if got := warnings.String(); got != "warning: semantic exchange journal unavailable: stage=orientation code=artifact_write_failed\n" {
		t.Fatalf("bounded symlink warning = %q", got)
	}
}

func TestSemanticExchangeRejectsOpenMetadataAndAvailability(t *testing.T) {
	t.Parallel()

	base := SemanticExchange{
		Stage: SemanticStageOrientation, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: SemanticRequestPrepared,
		State:             SemanticStateAccepted, ValidationCode: SemanticValidationAccepted,
		SemanticCalls: 1, TransportAttempts: 1,
		Request: []byte(`{"request":true}`), Response: []byte(`{"response":true}`),
	}
	for name, mutate := range map[string]func(*SemanticExchange){
		"stage":      func(value *SemanticExchange) { value.Stage = "repository/path" },
		"state":      func(value *SemanticExchange) { value.State = "some_error_text" },
		"validation": func(value *SemanticExchange) { value.ValidationCode = "decoder said secret prose" },
		"ordinal":    func(value *SemanticExchange) { value.InstanceOrdinal = 0 },
		"availability": func(value *SemanticExchange) {
			value.ResponseUnavailable = &SemanticUnavailable{Code: SemanticUnavailableNoContent}
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			if err := validateSemanticExchange(value); err == nil {
				t.Fatalf("invalid semantic exchange accepted: %#v", value)
			}
		})
	}
}

func readSemanticExchangeRecords(t *testing.T, runDir string) []SemanticExchangeRecord {
	t.Helper()
	directories, err := os.ReadDir(filepath.Join(runDir, SemanticExchangesDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	records := make([]SemanticExchangeRecord, 0, len(directories))
	for _, directory := range directories {
		if !directory.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(
			runDir,
			SemanticExchangesDir,
			directory.Name(),
			SemanticExchangeMetaFile,
		))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var record SemanticExchangeRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func readSemanticPayload(t *testing.T, runDir string, record SemanticPayloadRecord) []byte {
	t.Helper()
	directories, err := os.ReadDir(filepath.Join(runDir, SemanticExchangesDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range directories {
		metadata, err := os.ReadFile(filepath.Join(
			runDir, SemanticExchangesDir, directory.Name(), SemanticExchangeMetaFile,
		))
		if err != nil {
			continue
		}
		if !bytes.Contains(metadata, []byte(record.SavedSHA256)) {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(
			runDir, SemanticExchangesDir, directory.Name(), record.File,
		))
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	t.Fatalf("semantic payload %q was not found", record.File)
	return nil
}

func assertSemanticPayloadIdentity(
	t *testing.T,
	runDir string,
	record SemanticPayloadRecord,
	want []byte,
) {
	t.Helper()
	got := readSemanticPayload(t, runDir, record)
	if !bytes.Equal(got, want) || record.Storage != "raw_content" ||
		record.OriginalSHA256 != sha256Text(want) || record.OriginalBytes != len(want) ||
		record.SavedSHA256 != sha256Text(got) || record.SavedBytes != len(got) {
		t.Fatalf("semantic payload identity = %#v, bytes %q, want %q", record, got, want)
	}
}

func sha256Text(data []byte) string {
	digest := sha256.Sum256(data)
	return fmt.Sprintf("%x", digest)
}

func TestWriteLLMBundleWithSidecarBindsExactPreparedBytes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		redacted      bool
		input         []byte
		wantDifferent bool
	}{
		{
			name:          "redaction changes persisted bundle",
			redacted:      true,
			input:         []byte(`{"readme":"API_KEY=actual-secret-value"}` + "\n"),
			wantDifferent: true,
		},
		{
			name:     "non-redacted bundle stays byte exact",
			input:    []byte(`{"repo_name":"exact"}` + "\n"),
			redacted: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			writer, err := NewWriter(t.TempDir(), "bundle-sidecar", test.redacted)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			var callbackBytes []byte
			err = writer.WriteLLMBundleWithSidecar(
				test.input,
				"bundle_identity.json",
				func(saved []byte) ([]byte, error) {
					callbackBytes = append([]byte(nil), saved...)
					digest := sha256.Sum256(saved)
					return json.Marshal(struct {
						SHA256 string `json:"sha256"`
						Bytes  int    `json:"bytes"`
					}{SHA256: fmt.Sprintf("%x", digest), Bytes: len(saved)})
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := os.ReadFile(filepath.Join(writer.RunDir(), "llm_bundle.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(callbackBytes, saved) {
				t.Fatal("sidecar callback did not observe the exact persisted bundle bytes")
			}
			if test.wantDifferent == bytes.Equal(saved, test.input) {
				t.Fatalf("persisted bundle change = %v, want %v", !bytes.Equal(saved, test.input), test.wantDifferent)
			}
			if test.redacted && bytes.Contains(saved, []byte("actual-secret-value")) {
				t.Fatalf("persisted bundle leaked the redaction-changing value: %s", saved)
			}
			var sidecar struct {
				SHA256 string `json:"sha256"`
				Bytes  int    `json:"bytes"`
			}
			sidecarJSON, err := os.ReadFile(filepath.Join(writer.RunDir(), "bundle_identity.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(sidecarJSON, &sidecar); err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(saved)
			if sidecar.SHA256 != fmt.Sprintf("%x", digest) || sidecar.Bytes != len(saved) {
				t.Fatalf("sidecar identity = %#v, want exact saved bundle hash/length", sidecar)
			}
		})
	}
}

func TestNewWriterCreatesDir(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir, "test-run", true)
	if err != nil {
		t.Fatal(err)
	}

	runDir := filepath.Join(dir, "test-run")
	if _, err := os.Stat(runDir); os.IsNotExist(err) {
		t.Fatal("run dir was not created")
	}

	meta := RunMeta{
		RunID:     "test-run",
		CreatedAt: "2024-01-01T00:00:00Z",
		RepoName:  "test",
		RepoPath:  "/tmp/test",
		Command:   "orient",
	}
	if err := w.WriteMetadata(meta); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(runDir, "metadata.json")); os.IsNotExist(err) {
		t.Fatal("metadata.json was not created")
	}
}

func TestNewWriterRejectsExistingRunDirectoryWithoutReusingIt(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "test-run")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(existing, "sentinel")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := NewWriter(dir, "test-run", true); err == nil {
		t.Fatal("NewWriter accepted an existing run directory")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "keep" {
		t.Fatalf("existing run was changed: content=%q err=%v", got, err)
	}
}

func TestNewWriterRejectsSymlinkRunDirectory(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "test-run")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWriter(dir, "test-run", true); err == nil {
		t.Fatal("NewWriter accepted a symlink run directory")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside directory was touched: entries=%v err=%v", entries, err)
	}
}

func TestRunMetaPersistsSafeEffectiveRequestDiagnostics(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(RunMeta{
		RunID:         "failed-request",
		Model:         "company-model",
		Endpoint:      "https://llm.example.test/v1/chat/completions",
		AuthMode:      "bearer",
		TimeoutMillis: 45000,
		MaxTokens:     6000,
		EffectiveOptions: EffectiveOptions{
			FlowCount:        3,
			DiscoverSurfaces: true,
			NoOpen:           true,
			DebugEnabled:     true,
		},
		RequestAttempts: []RequestAttempt{{
			Stage: "orientation", State: "failed", RequestBytes: 1234, ProviderCallCount: 1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`"auth_mode":"bearer"`,
		`"timeout_ms":45000`,
		`"stage":"orientation"`,
		`"state":"failed"`,
		`"flows":3`,
		`"no_open":true`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("metadata %s does not contain %s", text, want)
		}
	}
	for _, forbidden := range []string{"api_key", "Authorization", "Bearer ", "password"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("metadata contains forbidden credential material %q: %s", forbidden, text)
		}
	}
}

func TestRedactionStripsSensitiveFields(t *testing.T) {
	input := []byte(`{
  "model": "deepseek-v4-flash",
  "messages": [{"role": "user", "content": "hello"}],
  "temperature": 0.1,
  "api_key": "sk-secret-12345",
  "authorization": "Bearer abc",
  "token": "secret-token",
  "password": "mypassword",
  "normal_field": "normal-value"
}`)
	result := redactJSON(input)
	resultStr := string(result)

	if strings.Contains(resultStr, "sk-secret-12345") {
		t.Fatal("api_key value was not redacted")
	}
	if strings.Contains(resultStr, `"api_key":`) && !strings.Contains(resultStr, "[redacted]") {
		t.Fatal("api_key field was not redacted")
	}
	if strings.Contains(resultStr, "Bearer abc") {
		t.Fatal("authorization value was not redacted")
	}
	if strings.Contains(resultStr, "secret-token") {
		t.Fatal("token value was not redacted")
	}
	if strings.Contains(resultStr, "mypassword") {
		t.Fatal("password value was not redacted")
	}
	if !strings.Contains(resultStr, "normal-value") {
		t.Fatal("normal field was incorrectly redacted")
	}
}

func TestRedactionLeavesNormalJSON(t *testing.T) {
	input := []byte(`{"name": "test", "value": 42}`)
	result := redactJSON(input)

	var v map[string]interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		t.Fatalf("redaction produced invalid JSON: %v\nresult: %s", err, string(result))
	}
}

func TestWriteFileUsesTempThenRename(t *testing.T) {
	dir := t.TempDir()
	w, _ := NewWriter(dir, "run", false)

	if err := w.WriteFile("test.json", []byte(`{"key":"value"}`)); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "run", "test.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `{"key":"value"}` {
		t.Fatalf("got %q", string(content))
	}

	tmpExists := false
	entries, _ := os.ReadDir(filepath.Join(dir, "run"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			tmpExists = true
		}
	}
	if tmpExists {
		t.Fatal("tmp file should not exist after rename")
	}
}

func TestWriteFileAtomicallyReplacesPublishedArtifact(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir, "run", false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.WriteFile("test.json", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("test.json", []byte("second")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "run", "test.json"))
	if err != nil || string(got) != "second" {
		t.Fatalf("published artifact changed: content=%q err=%v", got, err)
	}
}

func TestWriteDirErrorRedactsPlainTextCredential(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir, "run", true)
	if err != nil {
		t.Fatal(err)
	}
	w.WriteDirError("flows/startup", fmt.Errorf("provider echoed Bearer company-secret-token-value"))

	content, err := os.ReadFile(filepath.Join(dir, "run", "flows", "startup", "error.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "company-secret-token-value") || !strings.Contains(string(content), "[redacted:") {
		t.Fatalf("error artifact was not safely redacted: %q", content)
	}
}

func TestGenerateRunID(t *testing.T) {
	id := GenerateRunID("etcd")
	second := GenerateRunID("etcd")
	if !strings.Contains(id, "etcd") {
		t.Fatalf("run ID should contain repo name: %q", id)
	}
	if len(id) < 10 {
		t.Fatalf("run ID too short: %q", id)
	}
	if strings.Contains(id, " ") {
		t.Fatalf("run ID contains spaces: %q", id)
	}
	if id == second {
		t.Fatalf("independent run IDs collided: %q", id)
	}
}

func TestSanitize(t *testing.T) {
	if s := sanitize("hello world"); s != "hello-world" {
		t.Fatalf("sanitize(hello world) = %q, want hello-world", s)
	}
	if s := sanitize("a/b/c"); s != "a-b-c" {
		t.Fatalf("sanitize(a/b/c) = %q, want a-b-c", s)
	}
}

func TestRedactionRemovesAuthorizationHeader(t *testing.T) {
	input := []byte(`{
  "model": "deepseek-v4-flash",
  "messages": [
    {"role": "user", "content": "hello"}
  ],
  "authorization": "Bearer sk-abc123",
  "Authorization": "Bearer sk-xyz789",
  "api_key": "secret-apikey",
  "token": "my-secret-token",
  "password": "hunter2",
  "secret": "shhh",
  "access_key": "AKIA12345",
  "refresh_token": "rt-secret",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----",
  "normal": "ok"
}`)
	result := redactJSON(input)
	resultStr := string(result)

	if strings.Contains(resultStr, "sk-abc123") || strings.Contains(resultStr, "sk-xyz789") {
		t.Fatal("Authorization/Bearer values were not redacted")
	}
	if strings.Contains(resultStr, "secret-apikey") {
		t.Fatal("api_key was not redacted")
	}
	if strings.Contains(resultStr, "my-secret-token") {
		t.Fatal("token was not redacted")
	}
	if strings.Contains(resultStr, "hunter2") {
		t.Fatal("password was not redacted")
	}
	if strings.Contains(resultStr, "shhh") {
		t.Fatal("secret was not redacted")
	}
	if strings.Contains(resultStr, "AKIA12345") {
		t.Fatal("access_key was not redacted")
	}
	if strings.Contains(resultStr, "rt-secret") {
		t.Fatal("refresh_token was not redacted")
	}
	if strings.Contains(resultStr, "BEGIN RSA PRIVATE KEY") {
		t.Fatal("private_key was not redacted")
	}
	if !strings.Contains(resultStr, `"normal": "ok"`) {
		t.Fatal("normal field was incorrectly redacted")
	}
	if !strings.Contains(resultStr, "[redacted]") {
		t.Fatal("should contain [redacted] markers")
	}
}
