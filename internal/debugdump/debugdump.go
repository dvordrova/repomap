package debugdump

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/secretscan"
)

type RunMeta struct {
	RunID                   string           `json:"run_id"`
	CreatedAt               string           `json:"created_at"`
	RepoName                string           `json:"repo_name"`
	RepoPath                string           `json:"repo_path"`
	Command                 string           `json:"command"`
	Model                   string           `json:"model"`
	Endpoint                string           `json:"endpoint"`
	PromptVersion           string           `json:"prompt_version,omitempty"`
	CompactContextBytes     int              `json:"compact_context_bytes,omitempty"`
	ExternalRequestBytes    int              `json:"external_request_bytes,omitempty"`
	ProviderRequestCount    int              `json:"provider_request_count,omitempty"`
	CandidateDirectionCount int              `json:"candidate_direction_count,omitempty"`
	AcceptedDirectionCount  int              `json:"accepted_direction_count,omitempty"`
	RejectedDirectionCount  int              `json:"rejected_direction_count,omitempty"`
	ProviderLatencyMillis   *int64           `json:"provider_latency_ms,omitempty"`
	SurfaceDiscoveryRan     bool             `json:"surface_discovery_ran,omitempty"`
	SurfaceDiscoveryCount   int              `json:"surface_discovery_count,omitempty"`
	SurfaceDiscoveryMillis  *int64           `json:"surface_discovery_ms,omitempty"`
	Warnings                []string         `json:"warnings,omitempty"`
	SnapshotOnly            bool             `json:"snapshot_only"`
	LLMBundleOnly           bool             `json:"llm_bundle_only"`
	AuthMode                string           `json:"auth_mode,omitempty"`
	TimeoutMillis           int64            `json:"timeout_ms,omitempty"`
	MaxTokens               int              `json:"max_tokens,omitempty"`
	EffectiveOptions        EffectiveOptions `json:"effective_options,omitempty"`
	RequestAttempts         []RequestAttempt `json:"request_attempts,omitempty"`
}

type EffectiveOptions struct {
	Offline          bool `json:"offline"`
	FlowCount        int  `json:"flows"`
	DiscoverSurfaces bool `json:"discover_surfaces"`
	DumpLLM          bool `json:"dump_llm"`
	OutputJSON       bool `json:"json_output"`
	PreviewRequest   bool `json:"preview_request"`
	NoOpen           bool `json:"no_open"`
	NoServe          bool `json:"no_serve"`
	Port             int  `json:"port"`
	DebugEnabled     bool `json:"debug_enabled"`
}

type RequestAttempt struct {
	Stage             string `json:"stage"`
	State             string `json:"state"`
	RequestBytes      int    `json:"request_bytes,omitempty"`
	ProviderCallCount int    `json:"provider_call_count,omitempty"`
	LatencyMillis     *int64 `json:"latency_ms,omitempty"`
}

type Writer struct {
	BaseDir  string
	RunID    string
	Redacted bool
	runDir   string
}

func NewWriter(baseDir, runID string, redacted bool) (*Writer, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("debug dir is empty")
	}
	runDir := filepath.Join(baseDir, runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return nil, fmt.Errorf("create debug run dir: %w", err)
	}
	fmt.Fprintf(os.Stderr, "debug artifacts: %s\n", runDir)
	return &Writer{BaseDir: baseDir, RunID: runID, Redacted: redacted, runDir: runDir}, nil
}

func (w *Writer) WriteFile(name string, data []byte) error {
	path := filepath.Join(w.runDir, name)
	if w.Redacted {
		data = redactJSON(data)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s: %w", name, err)
	}
	return nil
}

func (w *Writer) WriteMetadata(meta RunMeta) error {
	data, _ := json.MarshalIndent(meta, "", "  ")
	return w.WriteFile("metadata.json", append(data, '\n'))
}

func (w *Writer) WriteSnapshot(snapshotJSON []byte) error {
	return w.WriteFile("snapshot.json", snapshotJSON)
}

func (w *Writer) WriteLLMBundle(bundleJSON []byte) error {
	return w.WriteFile("llm_bundle.json", bundleJSON)
}

func (w *Writer) WriteLLMRequest(requestJSON []byte) error {
	return w.WriteFile("llm_request.redacted.json", requestJSON)
}

func (w *Writer) WriteLLMResponse(responseJSON []byte) error {
	return w.WriteFile("llm_response.raw.json", responseJSON)
}

func (w *Writer) WriteOrientationReport(reportJSON []byte) error {
	return w.WriteFile("orientation_report.json", reportJSON)
}

func (w *Writer) WriteOrientationValidation(validationJSON []byte) error {
	return w.WriteFile("orientation_validation.json", validationJSON)
}

func (w *Writer) WriteDirFile(subdir, name string, data []byte) error {
	dir := filepath.Join(w.runDir, subdir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if w.Redacted {
		data = redactJSON(data)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write %s/%s: %w", subdir, name, err)
	}
	return os.Rename(tmpPath, path)
}

func (w *Writer) WriteDirError(subdir string, err error) {
	dir := filepath.Join(w.runDir, subdir)
	os.MkdirAll(dir, 0o700)
	data := []byte(fmt.Sprintf("error: %v\n", err))
	if w.Redacted {
		data = redactJSON(data)
	}
	os.WriteFile(filepath.Join(dir, "error.txt"), data, 0o600)
}

func (w *Writer) RunDir() string {
	return w.runDir
}

func (w *Writer) WriteError(err error) {
	data := []byte(fmt.Sprintf("error: %v\n", err))
	_ = w.WriteFile("error.txt", data)
}

var sensitiveKeyPattern = regexp.MustCompile(
	`(?i)"(` + strings.Join([]string{
		`api_key`,
		`apikey`,
		`api-key`,
		`authorization`,
		`bearer`,
		`token`,
		`secret`,
		`password`,
		`passwd`,
		`private_key`,
		`private-key`,
		`access_key`,
		`access-key`,
		`refresh_token`,
		`refresh-token`,
	}, "|") + `)"\s*:\s*"[^"]*"`,
)

func redactJSON(data []byte) []byte {
	redacted := sensitiveKeyPattern.ReplaceAll(data, []byte(`"$1": "[redacted]"`))
	if kind, found := secretscan.Detect(string(redacted)); found {
		return []byte(fmt.Sprintf("[redacted: %s detected]\n", kind))
	}
	return redacted
}

func GenerateRunID(repoName string) string {
	ts := time.Now().UTC().Format("20060102-150405")
	return fmt.Sprintf("%s-%s", ts, sanitize(repoName))
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
}
