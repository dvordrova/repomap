package debugdump

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
