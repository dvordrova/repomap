package debugdump

import (
	"crypto/rand"
	"encoding/hex"
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
	Offline          bool   `json:"offline"`
	NoCache          bool   `json:"no_cache"`
	FlowCount        int    `json:"flows"`
	DiscoverSurfaces bool   `json:"discover_surfaces"`
	GuidedTour       bool   `json:"guided_tour"`
	NoSearch         bool   `json:"no_search"`
	ReportLanguage   string `json:"report_language,omitempty"`
	DumpLLM          bool   `json:"dump_llm"`
	OutputJSON       bool   `json:"json_output"`
	PreviewRequest   bool   `json:"preview_request"`
	NoOpen           bool   `json:"no_open"`
	NoServe          bool   `json:"no_serve"`
	Port             int    `json:"port"`
	DebugEnabled     bool   `json:"debug_enabled"`
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
	root     *os.Root
}

func NewWriter(baseDir, runID string, redacted bool) (*Writer, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("debug dir is empty")
	}
	if runID == "" || !filepath.IsLocal(runID) || filepath.Clean(runID) != runID || filepath.Base(runID) != runID {
		return nil, fmt.Errorf("debug run id must be one local path component")
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("create debug base dir: %w", err)
	}
	baseRoot, err := os.OpenRoot(baseDir)
	if err != nil {
		return nil, fmt.Errorf("open debug base dir: %w", err)
	}
	defer baseRoot.Close()
	// The caller binds metadata, report routing, and browser authority to this
	// exact ID. Fail closed on collision instead of reusing or silently
	// suffixing a directory that those callers did not authorize.
	if err := baseRoot.Mkdir(runID, 0o700); err != nil {
		return nil, fmt.Errorf("create unique debug run dir: %w", err)
	}
	runRoot, err := baseRoot.OpenRoot(runID)
	if err != nil {
		return nil, fmt.Errorf("open debug run dir: %w", err)
	}
	runDir := filepath.Join(baseDir, runID)
	fmt.Fprintf(os.Stderr, "debug artifacts: %s\n", runDir)
	return &Writer{
		BaseDir: baseDir, RunID: runID, Redacted: redacted,
		runDir: runDir, root: runRoot,
	}, nil
}

func (w *Writer) WriteFile(name string, data []byte) error {
	return w.writeRootFile(name, data)
}

func (w *Writer) writeRootFile(name string, data []byte) error {
	if w == nil || w.root == nil {
		return fmt.Errorf("debug writer is closed")
	}
	localName := filepath.FromSlash(name)
	if name == "" || !filepath.IsLocal(localName) || filepath.Clean(localName) != localName {
		return fmt.Errorf("invalid debug artifact path %q", name)
	}
	if w.Redacted {
		data = redactJSON(data)
	}
	tmpName := localName + ".tmp"
	file, err := w.root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	written := false
	defer func() {
		if !written {
			_ = w.root.Remove(tmpName)
		}
	}()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", name, err)
	}
	if err := w.root.Rename(tmpName, localName); err != nil {
		return fmt.Errorf("publish %s: %w", name, err)
	}
	written = true
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
	localSubdir := filepath.FromSlash(subdir)
	if w == nil || w.root == nil || subdir == "" || !filepath.IsLocal(localSubdir) ||
		filepath.Clean(localSubdir) != localSubdir || localSubdir == "." ||
		name == "" || filepath.Base(name) != name {
		return fmt.Errorf("invalid debug artifact subpath")
	}
	if err := w.root.MkdirAll(localSubdir, 0o700); err != nil {
		return err
	}
	return w.writeRootFile(filepath.Join(localSubdir, name), data)
}

func (w *Writer) WriteDirError(subdir string, err error) {
	data := []byte(fmt.Sprintf("error: %v\n", err))
	_ = w.WriteDirFile(subdir, "error.txt", data)
}

func (w *Writer) RunDir() string {
	return w.runDir
}

// Close releases the directory handle that confines artifact writes.
func (w *Writer) Close() error {
	if w == nil || w.root == nil {
		return nil
	}
	err := w.root.Close()
	w.root = nil
	return err
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
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Sprintf("%s-%s-%x", ts, sanitize(repoName), time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("%s-%s-%s", ts, sanitize(repoName), hex.EncodeToString(nonce[:]))
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
}
