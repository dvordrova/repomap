package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const standaloneBundleRevision = "0123456789abcdef0123456789abcdef01234567"

func TestStandaloneBundleTransportHTMLSectionV4IsFeatureBlindAndLazy(t *testing.T) {
	repository := []byte(`{"version":1,"repository":"shared"}`)
	target := []byte(`{"version":1,"target":"analyzed","sentinel":"TARGET_ONLY"}`)
	transport, err := prepareStandaloneBundleTransportV4(standaloneBundleTransportInputV4{
		RepositoryPayload:      repository,
		LogicalDefaultTargetID: "selected-failed",
		Targets: []standaloneBundleTransportTargetInputV4{
			{TargetID: "selected-ok", ProgramTargetID: "program-ok", State: standaloneBundleTransportTargetAnalyzed, Payload: target},
			{TargetID: "selected-failed", State: standaloneBundleTransportTargetNotAnalyzed},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	section, err := standaloneBundleTransportHTMLSectionV4(transport)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`id="rm-bundle-index"`, `id="rm-repository-payload"`, `id="rm-target-chunk-0"`} {
		if bytes.Count(section, []byte(marker)) != 1 {
			t.Fatalf("v4 section marker %q count is not one", marker)
		}
	}
	for _, forbidden := range []string{"rm-report-data", "rm-standalone-target-bootstrap", standaloneTargetBundleMarkerPrefix, "TARGET_ONLY"} {
		if bytes.Contains(section, []byte(forbidden)) {
			t.Fatalf("v4 section retained forbidden eager surface %q", forbidden)
		}
	}
	restored, err := extractStandaloneBundleTransportV4HTML(section)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored.RepositoryPayload, repository) || len(restored.TargetChunks) != 1 ||
		restored.Index.Targets[1].Chunk != nil {
		t.Fatalf("restored v4 transport = %#v", restored)
	}
	restoredTarget, err := decodeStandaloneBundleTargetChunkV4(
		restored.TargetChunks[0].Ref, restored.TargetChunks[0].Base64,
	)
	if err != nil || !bytes.Equal(restoredTarget, target) {
		t.Fatalf("restored target = %q, %v", restoredTarget, err)
	}
}

func TestStandaloneBundleV4WriterKeepsSealCorruptionOnly(t *testing.T) {
	transport, err := prepareStandaloneBundleTransportV4(standaloneBundleTransportInputV4{
		RepositoryPayload:      []byte(`{"repository":"shared"}`),
		LogicalDefaultTargetID: "selected-ok",
		Targets: []standaloneBundleTransportTargetInputV4{{
			TargetID: "selected-ok", ProgramTargetID: "program-ok",
			State: standaloneBundleTransportTargetAnalyzed, Payload: []byte(`{"payload":"exact"}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	validated := &validatedStandaloneTargetBundle{
		identity: StandaloneTargetBundleIdentity{
			Version: StandaloneTargetBundleVersion, ProgramPagePortfolioSHA256: strings.Repeat("a", 64),
			DefaultTargetIndex: 0, TargetCount: 1,
		},
		defaultTarget: &preparedStandaloneTarget{repoName: "bundle-v4"},
		transport:     transport,
	}
	runDir := t.TempDir()
	htmlPath := filepath.Join(runDir, "report.html")
	const beforeInstallSentinel = "REPORT_BEFORE_CLEANUP_FAILURE"
	if err := os.WriteFile(htmlPath, []byte(beforeInstallSentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanupAttempted := false
	if err := writeValidatedStandaloneTargetBundleAtomicBeforeInstall(
		runDir, validated, func() error {
			cleanupAttempted = true
			return errors.New("synthetic spool cleanup failure")
		},
	); err == nil || !cleanupAttempted {
		t.Fatalf("before-install cleanup failure = %v, attempted=%t", err, cleanupAttempted)
	}
	unchangedAfterCleanupFailure, err := os.ReadFile(htmlPath)
	if err != nil || string(unchangedAfterCleanupFailure) != beforeInstallSentinel {
		t.Fatalf("cleanup failure replaced report.html: %q, %v", unchangedAfterCleanupFailure, err)
	}
	if err := writeValidatedStandaloneTargetBundleAtomic(runDir, validated); err != nil {
		t.Fatal(err)
	}
	identity, found, err := InspectStandaloneTargetBundleHTML(htmlPath)
	if err != nil || !found || identity.Version != 4 {
		t.Fatalf("InspectStandaloneTargetBundleHTML = %#v, %t, %v", identity, found, err)
	}
	if err := verifyExactStandaloneTargetBundleProjection(htmlPath, validated); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("rm-report-data")) || bytes.Contains(raw, []byte("rm-standalone-target-bootstrap")) {
		t.Fatal("v4 standalone HTML retained a v3 eager/bootstrap surface")
	}
	tampered := bytes.Replace(raw, []byte("H4sI"), []byte("I4sI"), 1)
	if bytes.Equal(tampered, raw) {
		t.Fatal("compressed target sentinel is absent")
	}
	sealStart := bytes.LastIndex(tampered, []byte(standaloneTargetBundleSealPrefix))
	digest := sha256.Sum256(tampered[:sealStart])
	tampered = append(append([]byte(nil), tampered[:sealStart]...), []byte(
		standaloneTargetBundleSealPrefix+hex.EncodeToString(digest[:])+standaloneTargetBundleSealSuffix,
	)...)
	if err := os.WriteFile(htmlPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := InspectStandaloneTargetBundleHTML(htmlPath); err != nil || !found {
		t.Fatalf("resealed corruption was not structurally sealed: found=%t err=%v", found, err)
	}
	if err := verifyExactStandaloneTargetBundleProjection(htmlPath, validated); err == nil ||
		!strings.Contains(err.Error(), "manifest-derived projection") {
		t.Fatalf("resealed authority rewrite error = %v", err)
	}
}

func TestStandaloneBundleV4SpoolWritesAndVerifiesSequentially(t *testing.T) {
	repository := BrowserRepositoryPayload{
		Version: BrowserRepositoryPayloadVersion,
		Repository: BrowserRepository{
			Name: "spool-fixture", CapturedRevision: standaloneBundleRevision,
		},
		Source: BrowserSource{
			Kind: "github", RepositoryURL: "https://github.com/example/spool-fixture",
		},
		LogicalDefaultSelectedTargetID: "selected-failed",
		Targets: []BrowserTargetIndexItem{
			{
				SelectedTargetID: "selected-a", ProgramTargetID: "program-a",
				Language: "go", Kind: "application", DisplayName: "target a",
				State: "analyzed", Href: "?target=0#/program",
			},
			{
				SelectedTargetID: "selected-failed", Language: "python", Kind: "worker",
				DisplayName: "failed target", State: "not_analyzed",
				FailureStage: "target_page", FailureReason: "analysis_failed",
			},
			{
				SelectedTargetID: "selected-b", ProgramTargetID: "program-b",
				Language: "typescript", Kind: "application", DisplayName: "target b",
				State: "analyzed", Href: "?target=2#/program",
			},
		},
		OpenablePaths: []string{},
	}
	payloads := map[string][]byte{
		"program-a": bytes.Repeat([]byte(`{"target":"a"}`), 4096),
		"program-b": bytes.Repeat([]byte(`{"target":"b"}`), 4096),
	}
	loaded := make([]string, 0, len(payloads))
	spool, err := prepareStandaloneBundleSpoolV4(
		repository,
		func(_ int, row BrowserTargetIndexItem) ([]byte, error) {
			loaded = append(loaded, row.ProgramTargetID)
			return payloads[row.ProgramTargetID], nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	spoolPath := spool.path
	defer func() { _ = spool.closeAndRemove() }()
	if strings.Join(loaded, ",") != "program-a,program-b" {
		t.Fatalf("spool loader calls = %v; failed target must not call it", loaded)
	}
	if len(spool.TargetChunks) != 2 || spool.Index.Targets[1].Chunk != nil {
		t.Fatalf("spool target index = %#v", spool.Index.Targets)
	}
	validated := &validatedStandaloneTargetBundle{
		identity: StandaloneTargetBundleIdentity{
			Version: StandaloneTargetBundleVersion, ProgramPagePortfolioSHA256: strings.Repeat("b", 64),
			DefaultTargetIndex: 0, TargetCount: 2,
		},
		defaultTarget: &preparedStandaloneTarget{repoName: "spool-fixture"},
		spool:         spool,
	}
	runDir := t.TempDir()
	if err := writeValidatedStandaloneTargetBundleAtomic(runDir, validated); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(runDir, "report.html")
	if err := verifyExactStandaloneTargetBundleProjection(htmlPath, validated); err != nil {
		t.Fatal(err)
	}
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := extractStandaloneBundleTransportV4HTML(htmlBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.TargetChunks) != 2 || restored.Index.LogicalDefaultTargetID != "selected-failed" {
		t.Fatalf("restored spooled transport = %#v", restored.Index)
	}
	for _, chunk := range restored.TargetChunks {
		raw, decodeErr := decodeStandaloneBundleTargetChunkV4(chunk.Ref, chunk.Base64)
		if decodeErr != nil || !bytes.Equal(raw, payloads[restored.Index.Targets[map[string]int{
			"selected-a": 0, "selected-b": 2,
		}[chunk.TargetID]].ProgramTargetID]) {
			t.Fatalf("restored spooled target %q is invalid: %v", chunk.TargetID, decodeErr)
		}
	}
	if _, err := spool.file.WriteAt([]byte{0}, 0); err != nil {
		t.Fatal(err)
	}
	const original = "ORIGINAL_REPORT"
	if err := os.WriteFile(htmlPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeValidatedStandaloneTargetBundleAtomic(runDir, validated); err == nil {
		t.Fatal("corrupt spool was published")
	}
	unchanged, err := os.ReadFile(htmlPath)
	if err != nil || string(unchanged) != original {
		t.Fatalf("failed spooled publication replaced report.html: %q, %v", unchanged, err)
	}
	if err := spool.closeAndRemove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(spoolPath); !os.IsNotExist(err) {
		t.Fatalf("temporary spool survived cleanup: %v", err)
	}
}

func TestStandaloneBundleV4SpoolCleanupFailureIsReportedAndRetainsRetryPath(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "keep"), []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	spool := &standaloneBundleSpoolV4{path: directory}
	err := spool.closeAndRemove()
	if err == nil || !strings.Contains(err.Error(), "remove private standalone bundle spool") {
		t.Fatalf("spool cleanup error = %v", err)
	}
	if spool.path != directory {
		t.Fatalf("failed cleanup discarded retry path: %q", spool.path)
	}
	if strings.Contains(err.Error(), directory) {
		t.Fatalf("spool cleanup exposed its private path: %v", err)
	}
	closedFile, err := os.CreateTemp(t.TempDir(), "closed-spool-*")
	if err != nil {
		t.Fatal(err)
	}
	closedFilePath := closedFile.Name()
	if err := closedFile.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(closedFilePath) })
	combined := &standaloneBundleSpoolV4{file: closedFile, path: directory}
	err = combined.closeAndRemove()
	if err == nil || !strings.Contains(err.Error(), "close private standalone bundle spool") ||
		!strings.Contains(err.Error(), "remove private standalone bundle spool") {
		t.Fatalf("combined spool cleanup error = %v", err)
	}
	if combined.path != directory || strings.Contains(err.Error(), directory) {
		t.Fatalf("combined cleanup lost or exposed retry path: path=%q error=%v", combined.path, err)
	}
}

func TestStandaloneTargetHrefUsesExactOutcomeOrdinal(t *testing.T) {
	for index, want := range []string{"?target=0#/program", "?target=1#/program", "?target=2#/program"} {
		if got := standaloneTargetHref(index); got != want {
			t.Fatalf("standaloneTargetHref(%d) = %q, want %q", index, got, want)
		}
	}
}

func clonePreparedStandaloneTargets(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
	cloned := make([]PreparedStandaloneTarget, len(values))
	for index, value := range values {
		if value.prepared == nil {
			continue
		}
		copyValue := *value.prepared
		copyValue.repository.Targets = append([]BrowserTargetIndexItem(nil), value.prepared.repository.Targets...)
		copyValue.repository.OpenablePaths = append([]string(nil), value.prepared.repository.OpenablePaths...)
		copyValue.repository.Warnings = append([]string(nil), value.prepared.repository.Warnings...)
		copyValue.repositoryPayload = append([]byte(nil), value.prepared.repositoryPayload...)
		copyValue.targetPayload = append([]byte(nil), value.prepared.targetPayload...)
		copyValue.localRoots = append([]string(nil), value.prepared.localRoots...)
		cloned[index] = PreparedStandaloneTarget{prepared: &copyValue}
	}
	return cloned
}
