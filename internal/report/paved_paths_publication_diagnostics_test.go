package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/pavedpath"
)

type pavedPathPublicationDiagnosticsFixture struct {
	Version         int               `json:"version"`
	RecordRawSHA256 string            `json:"record_raw_sha256"`
	BundleSHA256    string            `json:"bundle_sha256"`
	RecordIssues    []pavedpath.Issue `json:"record_issues"`
	ReplayIssues    []pavedpath.Issue `json:"replay_issues"`
}

func TestGenerateAuthorizedWritesBoundPavedPathPublicationDiagnostics(t *testing.T) {
	repository := newPavedPathPublicationDiagnosticsRepository(t)
	initial, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	current, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ConfirmRunAuthority(repository, initial, current)
	if err != nil {
		t.Fatal(err)
	}

	runDir := t.TempDir()
	record := pavedPathPublicationDiagnosticsRecord(t)
	recordRaw := writePavedPathPublicationDiagnosticsRun(t, runDir, repository, record)
	recordDigest := sha256.Sum256(recordRaw)

	victimPath := filepath.Join(runDir, "diagnostics-victim")
	const victimContent = "must not be overwritten through the sidecar symlink\n"
	if err := os.WriteFile(victimPath, []byte(victimContent), 0o600); err != nil {
		t.Fatal(err)
	}
	sidecarPath := filepath.Join(runDir, pavedPathPublicationDiagnosticsFile)
	if err := os.Symlink(victimPath, sidecarPath); err != nil {
		t.Fatal(err)
	}
	oldTargetInfo, err := os.Stat(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := GenerateAuthorized(runDir, authority); err != nil {
		t.Fatalf("GenerateAuthorized: %v", err)
	}

	victimRaw, err := os.ReadFile(victimPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(victimRaw) != victimContent {
		t.Fatalf("sidecar replacement followed its old symlink: %q", victimRaw)
	}
	sidecarInfo, err := os.Lstat(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	if !sidecarInfo.Mode().IsRegular() || sidecarInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("sidecar mode = %v, want regular file", sidecarInfo.Mode())
	}
	if sidecarInfo.Mode().Perm() != 0o600 {
		t.Fatalf("sidecar permissions = %o, want 600", sidecarInfo.Mode().Perm())
	}
	newSidecarInfo, err := os.Stat(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(oldTargetInfo, newSidecarInfo) {
		t.Fatal("sidecar replacement retained the symlink target inode")
	}
	assertNoPavedPathPublicationDiagnosticsTemps(t, runDir)

	sidecarRaw, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics pavedPathPublicationDiagnosticsFixture
	decoder := json.NewDecoder(bytes.NewReader(sidecarRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&diagnostics); err != nil {
		t.Fatalf("decode diagnostics: %v\n%s", err, sidecarRaw)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("diagnostics trailing JSON error = %v", err)
	}
	if diagnostics.Version != pavedPathPublicationDiagnosticsVersion {
		t.Fatalf("diagnostics version = %d", diagnostics.Version)
	}
	if diagnostics.RecordRawSHA256 != hex.EncodeToString(recordDigest[:]) {
		t.Fatalf(
			"record raw sha = %q, want %q",
			diagnostics.RecordRawSHA256,
			hex.EncodeToString(recordDigest[:]),
		)
	}
	if diagnostics.BundleSHA256 != record.BundleSHA256 {
		t.Fatalf(
			"bundle sha = %q, want %q",
			diagnostics.BundleSHA256,
			record.BundleSHA256,
		)
	}
	if len(diagnostics.RecordIssues) != 1 ||
		diagnostics.RecordIssues[0].PathIndex != 1 ||
		diagnostics.RecordIssues[0].Code != pavedpath.PublicationIssueMissingResult {
		t.Fatalf("record issues = %#v", diagnostics.RecordIssues)
	}
	if len(diagnostics.ReplayIssues) != 1 ||
		diagnostics.ReplayIssues[0].PathIndex != 1 ||
		diagnostics.ReplayIssues[0].Code != pavedpath.PublicationIssueMissingPrerequisite {
		t.Fatalf("replay issues = %#v", diagnostics.ReplayIssues)
	}

	afterRecordRaw, err := os.ReadFile(filepath.Join(runDir, pavedpath.RecordFile))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterRecordRaw, recordRaw) {
		t.Fatal("report generation mutated the historical paved path record")
	}

	for _, name := range []string{"report.json", "report.html", RunManifestFilename} {
		raw, readErr := os.ReadFile(filepath.Join(runDir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, forbidden := range []string{
			pavedPathPublicationDiagnosticsFile,
			"record_raw_sha256",
			"record_issues",
			"replay_issues",
			pavedpath.PublicationIssueMissingPrerequisite,
			pavedpath.PublicationIssueMissingResult,
		} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Fatalf("%s exposes internal diagnostics token %q", name, forbidden)
			}
		}
	}
	if _, err := ReadRunManifest(runDir); err != nil {
		t.Fatalf("ReadRunManifest before sidecar tamper: %v", err)
	}
	if err := os.WriteFile(sidecarPath, []byte("{diagnostic-only tamper"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRunManifest(runDir); err != nil {
		t.Fatalf("sidecar-only tamper affected report authority: %v", err)
	}
}

func TestGenerateRemovesStalePavedPathPublicationDiagnostics(t *testing.T) {
	t.Run("absent record", func(t *testing.T) {
		runDir := t.TempDir()
		writePavedPathPublicationDiagnosticsBaseRun(t, runDir, "")
		sidecarPath := filepath.Join(runDir, pavedPathPublicationDiagnosticsFile)
		if err := os.WriteFile(sidecarPath, []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := Generate(runDir); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if _, err := os.Lstat(sidecarPath); !os.IsNotExist(err) {
			t.Fatalf("stale sidecar survived absent record: %v", err)
		}
	})

	t.Run("clean record", func(t *testing.T) {
		runDir := t.TempDir()
		record := pavedPathPublicationDiagnosticsCleanRecord(t)
		writePavedPathPublicationDiagnosticsRun(t, runDir, "", record)
		sidecarPath := filepath.Join(runDir, pavedPathPublicationDiagnosticsFile)
		if err := os.WriteFile(sidecarPath, []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := Generate(runDir); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if _, err := os.Lstat(sidecarPath); !os.IsNotExist(err) {
			t.Fatalf("stale sidecar survived clean replay: %v", err)
		}
	})
}

func TestGeneratePropagatesPavedPathPublicationDiagnosticsFilesystemFailures(t *testing.T) {
	t.Run("atomic replacement failure", func(t *testing.T) {
		runDir := t.TempDir()
		writePavedPathPublicationDiagnosticsRun(
			t,
			runDir,
			"",
			pavedPathPublicationDiagnosticsRecord(t),
		)
		sidecarPath := filepath.Join(runDir, pavedPathPublicationDiagnosticsFile)
		if err := os.Mkdir(sidecarPath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sidecarPath, "block"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}

		err := Generate(runDir)
		if err == nil || !strings.Contains(err.Error(), "publication diagnostics") {
			t.Fatalf("Generate error = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(runDir, "report.json")); !os.IsNotExist(statErr) {
			t.Fatalf("report.json exists after diagnostics write failure: %v", statErr)
		}
		assertNoPavedPathPublicationDiagnosticsTemps(t, runDir)
	})

	t.Run("stale removal failure", func(t *testing.T) {
		runDir := t.TempDir()
		writePavedPathPublicationDiagnosticsBaseRun(t, runDir, "")
		sidecarPath := filepath.Join(runDir, pavedPathPublicationDiagnosticsFile)
		if err := os.Mkdir(sidecarPath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sidecarPath, "block"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}

		err := Generate(runDir)
		if err == nil || !strings.Contains(err.Error(), "publication diagnostics") {
			t.Fatalf("Generate error = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(runDir, "report.json")); !os.IsNotExist(statErr) {
			t.Fatalf("report.json exists after diagnostics removal failure: %v", statErr)
		}
		assertNoPavedPathPublicationDiagnosticsTemps(t, runDir)
	})
}

func pavedPathPublicationDiagnosticsRecord(t *testing.T) pavedpath.Record {
	t.Helper()

	record := pavedPathPublicationDiagnosticsBuiltRecord(t, true)
	historical := pavedpath.Path{
		Title: "Run historical cluster",
		Goal:  "Start the historical local cluster.",
		Actions: []pavedpath.Action{{
			EvidenceID:  "historical-action",
			Instruction: "Start the historical cluster.",
			Command:     "goreman start",
			SafeToCopy:  true,
		}},
		OrderingBasis: pavedpath.OrderingDocumented,
	}
	historical.ID = pavedPathPublicationDiagnosticsStablePathID(historical)
	record.Paths = append(record.Paths, historical)
	if err := record.Validate(); err != nil {
		t.Fatalf("validate historical record: %v", err)
	}
	return record
}

func pavedPathPublicationDiagnosticsCleanRecord(t *testing.T) pavedpath.Record {
	t.Helper()
	return pavedPathPublicationDiagnosticsBuiltRecord(t, false)
}

func pavedPathPublicationDiagnosticsBuiltRecord(
	t *testing.T,
	includeRejected bool,
) pavedpath.Record {
	t.Helper()

	bundle := pavedpath.Bundle{
		Version:      pavedpath.BundleVersion,
		RepoName:     "publication-diagnostics",
		AllowedPaths: []string{"README.md", "OPERATE.md"},
		Evidence: []pavedpath.Evidence{
			{
				ID: "complete-prerequisite", Role: pavedpath.RoleDocumentedProcedure,
				Path: "README.md", StartLine: 1, EndLine: 3, Label: "Run tool",
				Excerpt: []string{
					"First install tool before running this procedure.",
					"",
					"$ tool run -o result.txt",
				},
			},
			{
				ID: "complete-action", Role: pavedpath.RoleDocumentedProcedure,
				Path: "README.md", StartLine: 3, EndLine: 4, Label: "Run tool",
				Excerpt: []string{"$ tool run -o result.txt", "completed"},
				Commands: []pavedpath.Command{{
					Value: "tool run -o result.txt", Basis: pavedpath.CommandExact, SafeToCopy: true,
				}},
			},
			{
				ID: "verify-prerequisite", Role: pavedpath.RoleDocumentedProcedure,
				Path: "OPERATE.md", StartLine: 10, EndLine: 12, Label: "Verify tool",
				Excerpt: []string{
					"First install verifier before this procedure.",
					"",
					"$ tool verify",
				},
			},
			{
				ID: "verify-action", Role: pavedpath.RoleDocumentedProcedure,
				Path: "OPERATE.md", StartLine: 12, EndLine: 12, Label: "Verify tool",
				Excerpt: []string{"$ tool verify"},
				Commands: []pavedpath.Command{{
					Value: "tool verify", Basis: pavedpath.CommandExact, SafeToCopy: true,
				}},
			},
			{
				ID: "historical-action", Role: pavedpath.RoleDocumentedProcedure,
				Path: "README.md", StartLine: 20, EndLine: 20, Label: "Run historical cluster",
				Excerpt: []string{"goreman start"},
				Commands: []pavedpath.Command{{
					Value: "goreman start", Basis: pavedpath.CommandExact, SafeToCopy: true,
				}},
			},
		},
	}
	proposal := pavedpath.Proposal{
		Version: pavedpath.ProposalVersion,
		Paths: []pavedpath.ProposedPath{{
			Title: "Run tool", Goal: "Run the tool and observe its exact result.",
			PrerequisiteEvidenceIDs: []string{"complete-prerequisite"},
			Actions: []pavedpath.ProposedAction{{
				EvidenceID: "complete-action", Instruction: "Run the tool.",
				Command: "tool run -o result.txt",
			}},
			OrderingBasis: pavedpath.OrderingDocumented,
		}},
	}
	if includeRejected {
		proposal.Paths = append(proposal.Paths, pavedpath.ProposedPath{
			Title: "Verify tool", Goal: "Run the verifier.",
			PrerequisiteEvidenceIDs: []string{"verify-prerequisite"},
			Actions: []pavedpath.ProposedAction{{
				EvidenceID: "verify-action", Instruction: "Run the verifier.",
				Command: "tool verify",
			}},
			OrderingBasis: pavedpath.OrderingDocumented,
		})
	}
	record, err := pavedpath.BuildRecord(bundle, proposal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if includeRejected {
		if len(record.Issues) != 1 ||
			record.Issues[0].Code != pavedpath.PublicationIssueMissingResult {
			t.Fatalf("record issues = %#v", record.Issues)
		}
	} else if len(record.Issues) != 0 {
		t.Fatalf("clean record issues = %#v", record.Issues)
	}
	return record
}

func pavedPathPublicationDiagnosticsStablePathID(item pavedpath.Path) string {
	parts := make([]string, 0, len(item.Actions)*3)
	for _, action := range item.Actions {
		parts = append(parts, action.EvidenceID, action.Command, action.Endpoint)
	}
	actionIdentity := strings.Join(parts, "\x00")
	digest := sha256.Sum256([]byte(strings.Join(
		[]string{item.Title, actionIdentity},
		"\x00",
	)))
	return "operate-" + hex.EncodeToString(digest[:8])
}

func writePavedPathPublicationDiagnosticsRun(
	t *testing.T,
	runDir string,
	repository string,
	record pavedpath.Record,
) []byte {
	t.Helper()

	writePavedPathPublicationDiagnosticsBaseRun(t, runDir, repository)
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(runDir, pavedpath.RecordFile), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func writePavedPathPublicationDiagnosticsBaseRun(
	t *testing.T,
	runDir string,
	repository string,
) {
	t.Helper()

	snapshot := `{
		"repo_name": "publication-diagnostics",
		"file_tree": ["README.md", "OPERATE.md"],
		"files_considered": 2
	}`
	if err := os.WriteFile(
		filepath.Join(runDir, "snapshot.json"),
		snapshotJSONWithAnalysisTarget(t, []byte(snapshot)),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	orientation := `{
		"project_guess": "publication diagnostics",
		"candidate_flows": [],
		"warnings": []
	}`
	if err := os.WriteFile(
		filepath.Join(runDir, "orientation_report.json"),
		[]byte(orientation),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if repository != "" {
		writeRunManifestMetadata(t, runDir, repository)
	}
}

func newPavedPathPublicationDiagnosticsRepository(t *testing.T) string {
	t.Helper()

	repository := t.TempDir()
	readme := strings.Join([]string{
		"First install tool before running this procedure.",
		"",
		"$ tool run -o result.txt",
		"completed",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"goreman start",
	}, "\n") + "\n"
	if err := os.WriteFile(
		filepath.Join(repository, "README.md"),
		[]byte(readme),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	operate := strings.Join([]string{
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"First install verifier before this procedure.",
		"",
		"$ tool verify",
	}, "\n") + "\n"
	if err := os.WriteFile(
		filepath.Join(repository, "OPERATE.md"),
		[]byte(operate),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runManifestGit(t, repository, "init", "--quiet")
	runManifestGit(t, repository, "add", "README.md", "OPERATE.md")
	runManifestGit(
		t,
		repository,
		"-c", "user.name=repomap test",
		"-c", "user.email=repomap@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "fixture",
	)
	state, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	return state.Identity
}

func assertNoPavedPathPublicationDiagnosticsTemps(t *testing.T, runDir string) {
	t.Helper()

	entries, err := os.ReadDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == pavedPathPublicationDiagnosticsFile {
			continue
		}
		if strings.Contains(name, "paved") &&
			strings.Contains(name, "publication") &&
			strings.Contains(name, "diagnostic") {
			t.Fatalf("temporary diagnostics artifact survived: %s", name)
		}
	}
}
