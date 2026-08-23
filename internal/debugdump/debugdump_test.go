package debugdump

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSemanticExchangeRecordsLivePayloads(t *testing.T) {
	t.Parallel()

	writer, err := NewWriter(t.TempDir(), "semantic", false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	request := []byte(`{"request":true}`)
	response := []byte(`{"response":true}`)
	reference := writer.RecordSemanticExchange(SemanticExchange{
		Stage: SemanticStageTargetPortfolio, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: SemanticRequestExactSent,
		State:             SemanticStateAccepted, ValidationCode: SemanticValidationAccepted,
		SemanticCalls: 1, TransportAttempts: 2,
		Request: request, Response: response,
	})
	if !strings.HasSuffix(reference, "/"+SemanticExchangeMetaFile) {
		t.Fatalf("semantic exchange reference = %q", reference)
	}
	runDir := filepath.Join(writer.BaseDir, writer.RunID)
	record := readOnlySemanticExchange(t, runDir, reference)
	if record.Stage != SemanticStageTargetPortfolio ||
		record.Outcome != (SemanticOutcome{Phase: "complete", Code: "accepted"}) {
		t.Fatalf("semantic exchange record = %#v", record)
	}
	assertSavedPayload(t, runDir, reference, record.Request, request)
	assertSavedPayload(t, runDir, reference, record.Response, response)
}

func TestSemanticExchangeAcceptsOnlyLiveStages(t *testing.T) {
	t.Parallel()

	live := []string{
		SemanticStageReadmeFileClassifier,
		SemanticStageTargetPortfolio,
		SemanticStageTargetViewChoice,
		SemanticStageCoreMapBaseline,
		SemanticStageCoreMapRefined,
		SemanticStageActivityEntrypoints,
		SemanticStageIntegrationDependencies,
		SemanticStageIntegrationUsage,
		SemanticStageCubemapActivities,
		SemanticStageCubemapEntrypoints,
		SemanticStageCubemapDependencies,
		SemanticStageCubemapSymbols,
		SemanticStageCubemapBindings,
	}
	for _, stage := range live {
		exchange := validExchange(stage)
		if err := validateSemanticExchange(exchange); err != nil {
			t.Fatalf("live stage %q rejected: %v", stage, err)
		}
	}
	exchange := validExchange("architecture_synthesis")
	if err := validateSemanticExchange(exchange); err == nil {
		t.Fatal("removed architecture stage was accepted")
	}
}

func TestSemanticExchangeMarksPersistenceSensitiveResponse(t *testing.T) {
	t.Parallel()

	writer, err := NewWriter(t.TempDir(), "unsafe", false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	unsafe := []byte(`{"answer":"sk-abcdefghijklmnop"}`)
	exchange := validExchange(SemanticStageCubemapEntrypoints)
	exchange.State = SemanticStateRejected
	exchange.ValidationCode = SemanticValidationSecret
	exchange.Response = unsafe
	reference := writer.RecordSemanticExchange(exchange)
	if reference == "" {
		t.Fatal("semantic exchange was not recorded")
	}
	runDir := filepath.Join(writer.BaseDir, writer.RunID)
	record := readOnlySemanticExchange(t, runDir, reference)
	if record.Response.Storage != "unsafe_marker" || record.Response.UnsafeKind != "secret_key" {
		t.Fatalf("unsafe response record = %#v", record.Response)
	}
	markerPath := filepath.Join(runDir, filepath.Dir(reference), record.Response.File)
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(marker, unsafe) || bytes.Contains(marker, []byte("abcdefghijklmnop")) {
		t.Fatalf("unsafe marker leaked response: %s", marker)
	}
}

func TestSemanticExchangeWarningIsBoundedAndDeduplicated(t *testing.T) {
	t.Parallel()

	writer, err := NewWriter(t.TempDir(), "warning", false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	var warnings bytes.Buffer
	writer.SetWarningWriter(&warnings)

	exchange := validExchange(SemanticStageCoreMapBaseline)
	exchange.Response = nil
	writer.RecordSemanticExchange(exchange)
	writer.RecordSemanticExchange(exchange)
	if got := strings.Count(warnings.String(), SemanticExchangeWarningCode); got != 1 {
		t.Fatalf("warning count = %d, text = %q", got, warnings.String())
	}
	if strings.Contains(warnings.String(), filepath.Join(writer.BaseDir, writer.RunID)) {
		t.Fatalf("warning leaked local path: %q", warnings.String())
	}
}

func TestWriteValidatedFileChecksPersistedBytes(t *testing.T) {
	t.Parallel()

	writer, err := NewWriter(t.TempDir(), "validated", true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	original := []byte(`{"api_key":"top-secret-value","safe":true}`)
	if err := writer.WriteValidatedFile("artifact.json", original, func(saved []byte) error {
		if bytes.Contains(saved, []byte("top-secret-value")) ||
			!bytes.Contains(saved, []byte(`"api_key": "[redacted]"`)) {
			t.Fatalf("validator received unexpected bytes: %s", saved)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(writer.BaseDir, writer.RunID, "artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(saved, []byte("top-secret-value")) {
		t.Fatalf("persisted artifact leaked secret: %s", saved)
	}
}

func TestMetadataUsesOnlyLiveRequestStates(t *testing.T) {
	t.Parallel()

	writer, err := NewWriter(t.TempDir(), "metadata", false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	meta := RunMeta{
		RunID: "metadata",
		RequestAttempts: []RequestAttempt{{
			Stage: SemanticStageTargetPortfolio, State: SemanticStateCacheHit,
		}},
	}
	if err := writer.WriteMetadata(meta); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(writer.BaseDir, writer.RunID, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved RunMeta
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.RequestAttempts) != 1 || saved.RequestAttempts[0].Outcome == nil ||
		*saved.RequestAttempts[0].Outcome != (SemanticOutcome{Phase: "cache", Code: "cache_hit"}) {
		t.Fatalf("saved request attempt = %#v", saved.RequestAttempts)
	}
	meta.RequestAttempts[0].State = "removed_state"
	if err := writer.WriteMetadata(meta); err == nil {
		t.Fatal("removed request state was accepted")
	}
}

func TestOpenWriterPreservesBuildIdentityAndConfinement(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	created, err := NewWriter(base, "existing", false)
	if err != nil {
		t.Fatal(err)
	}
	modified := false
	created.buildIdentity = &BuildIdentity{
		Available: true, GoVersion: "go1.25", ModulePath: "example.com/repomap",
		VCSRevision: strings.Repeat("a", 40), VCSModified: &modified,
	}
	if err := created.WriteMetadata(RunMeta{RunID: "existing"}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(created.BaseDir, created.RunID)
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}

	opened, err := OpenWriter(runDir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if opened.buildIdentity == nil || opened.buildIdentity.VCSRevision != strings.Repeat("a", 40) {
		t.Fatalf("preserved build identity = %#v", opened.buildIdentity)
	}
	if err := opened.WriteFile("../escape", []byte("x")); err == nil {
		t.Fatal("writer accepted an escaping path")
	}
}

func TestNewAndOpenWriterRejectAmbiguousRunDirectories(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	writer, err := NewWriter(base, "run", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWriter(base, "run", false); err == nil {
		t.Fatal("existing run directory was reused")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "run"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWriter(filepath.Join(base, "alias"), false); err == nil {
		t.Fatal("run-directory symlink was accepted")
	}
}

func validExchange(stage string) SemanticExchange {
	return SemanticExchange{
		Stage: stage, InstanceOrdinal: 1, SemanticAttemptOrdinal: 1,
		RequestProvenance: SemanticRequestExactSent,
		State:             SemanticStateAccepted, ValidationCode: SemanticValidationAccepted,
		SemanticCalls: 1, TransportAttempts: 1,
		Request: []byte(`{"request":true}`), Response: []byte(`{"response":true}`),
	}
}

func readOnlySemanticExchange(t *testing.T, runDir, reference string) SemanticExchangeRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, filepath.FromSlash(reference)))
	if err != nil {
		t.Fatal(err)
	}
	var record SemanticExchangeRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func assertSavedPayload(
	t *testing.T,
	runDir string,
	reference string,
	record SemanticPayloadRecord,
	want []byte,
) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, filepath.Dir(reference), record.File))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("saved payload = %s, want %s", data, want)
	}
}
