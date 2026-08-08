package themestudy

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestD239ScoutRequestV3CompactsWireAndRestoresPrivateMetadata(t *testing.T) {
	request := buildTestScoutRequest(t)
	request.Vocabulary.Complete = false
	request.Vocabulary.Considered++
	request.Vocabulary.Omissions = []Omission{{
		Reason: "vocabulary_budget", Count: 1, Representatives: []string{"f6"},
	}}
	request.SeedPacks.Omissions = []Omission{{
		Reason: "seed_budget", Count: 2, Representatives: []string{"a6", "a7"},
	}}

	encoded, err := EncodeScoutRequest(request)
	if err != nil {
		t.Fatalf("EncodeScoutRequest: %v", err)
	}
	var artifact scoutRequestArtifactV3
	if err := json.Unmarshal(encoded, &artifact); err != nil {
		t.Fatalf("decode compact artifact shape: %v", err)
	}
	if artifact.Version != 3 || len(artifact.WireJSON) == 0 || artifact.WireJSON[0] != '{' {
		t.Fatalf("compact artifact identity/wire = version %d wire %q", artifact.Version, artifact.WireJSON)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	var vocabularyFields map[string]json.RawMessage
	if err := json.Unmarshal(raw["vocabulary"], &vocabularyFields); err != nil {
		t.Fatal(err)
	}
	if _, duplicated := vocabularyFields["files"]; duplicated {
		t.Fatal("v3 artifact duplicated model-visible vocabulary files")
	}
	var seedFields struct {
		Packs []map[string]json.RawMessage `json:"packs"`
	}
	if err := json.Unmarshal(raw["seed_packs"], &seedFields); err != nil {
		t.Fatal(err)
	}
	for index, pack := range seedFields.Packs {
		if _, duplicated := pack["objects"]; duplicated {
			t.Fatalf("v3 artifact pack %d duplicated model-visible source objects", index)
		}
	}

	decoded, err := DecodeScoutRequest(encoded)
	if err != nil {
		t.Fatalf("DecodeScoutRequest: %v", err)
	}
	if !reflect.DeepEqual(decoded, request) {
		t.Fatalf("lossless round trip changed request\n got: %#v\nwant: %#v", decoded, request)
	}
	foundPrivateBinding := false
	for _, pack := range decoded.SeedPacks.Packs {
		if pack.Seed.CanonicalSpanID != "" && pack.Seed.Provenance != "" {
			foundPrivateBinding = true
			break
		}
	}
	if !foundPrivateBinding || len(decoded.Vocabulary.Omissions) != 1 ||
		len(decoded.SeedPacks.Omissions) != 1 {
		t.Fatalf("private metadata was not restored: %#v", decoded)
	}
}

func TestD239ScoutRequestCompactionFitsWhenDuplicatedShapeWouldOverflow(t *testing.T) {
	vocabulary := BuildFileVocabulary([]string{"main.go"}, 0, nil)
	line := strings.Repeat("\"", 85<<10)
	seed := SeedSpec{
		Ref: "a1", Path: "main.go", Line: 1, Symbol: "main",
		Provenance: "d211_span_reading_target", Kind: "focused", Role: RoleProductionSource,
		CanonicalSpanID: "span-private-1",
	}
	packs := SeedPackResult{
		Packs: []SeedPack{{
			Seed: seed,
			Objects: []SourceObject{{
				Role: SourceRoleDeclaration, Path: seed.Path, Line: seed.Line,
				Symbol: seed.Symbol, Provenance: seed.Provenance, FullBody: true,
				Lines: []string{line}, ContentSHA256: strings.Repeat("a", 64),
			}},
			TotalBytes: len(line),
		}},
		TotalBytes: len(line),
	}
	request, err := CompileScout(
		LanguageEnglish, vocabulary, packs,
		ScoutContext{RepositoryName: "large-exact-source"}, "",
	)
	if err != nil {
		t.Fatalf("CompileScout: %v", err)
	}
	duplicated, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(duplicated) <= MaxScoutRequestArtifactBytes {
		t.Fatalf("duplicated fixture = %d bytes, want > %d", len(duplicated), MaxScoutRequestArtifactBytes)
	}
	compact, err := EncodeScoutRequest(request)
	if err != nil {
		t.Fatalf("compact EncodeScoutRequest: %v", err)
	}
	if len(compact) >= MaxScoutRequestArtifactBytes {
		t.Fatalf("compact artifact = %d bytes, want < %d", len(compact), MaxScoutRequestArtifactBytes)
	}
	if len(compact) >= len(duplicated) {
		t.Fatalf("compact artifact %d did not reduce duplicated artifact %d", len(compact), len(duplicated))
	}
	decoded, err := DecodeScoutRequest(compact)
	if err != nil {
		t.Fatalf("DecodeScoutRequest: %v", err)
	}
	if !reflect.DeepEqual(decoded, request) {
		t.Fatal("large compact request did not round-trip losslessly")
	}
}

func TestD239ScoutRequestV3RejectsWirePrivateCatalogDisagreement(t *testing.T) {
	request := buildTestScoutRequest(t)
	encoded, err := EncodeScoutRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	var artifact scoutRequestArtifactV3
	if err := json.Unmarshal(encoded, &artifact); err != nil {
		t.Fatal(err)
	}
	artifact.SeedPacks.Packs[0].Seed.Path = "different.go"
	tampered, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeScoutRequest(tampered); err == nil ||
		!strings.Contains(err.Error(), "seed metadata and wire disagree") {
		t.Fatalf("DecodeScoutRequest error = %v", err)
	}
}

func TestD239ScoutRequestV2HistoricalArtifactStillReplaysExactly(t *testing.T) {
	request := buildTestScoutRequest(t)
	request.Version = legacyScoutRequestVersion
	// Historical v2 persisted the complete in-memory request directly. Current
	// production encoding is v3-only; this fixture exercises decode/replay of
	// the old canonical bytes without reopening v2 writes.
	legacy, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal historical v2 fixture: %v", err)
	}
	if !bytes.Contains(legacy, []byte(`"wire_json":"{`)) {
		t.Fatal("historical v2 artifact no longer uses its original full shape")
	}
	decoded, err := DecodeScoutRequest(legacy)
	if err != nil {
		t.Fatalf("decode historical v2: %v", err)
	}
	if !reflect.DeepEqual(decoded, request) {
		t.Fatal("historical v2 decode changed the request")
	}
	if _, err := EncodeScoutRequest(decoded); err == nil {
		t.Fatal("current encoder unexpectedly accepted historical v2 request")
	}
	mock, err := MockScoutResponse(decoded)
	if err != nil {
		t.Fatalf("mock historical response: %v", err)
	}
	if _, _, err := ReplayScoutResponse(decoded, mock); err != nil {
		t.Fatalf("historical v2 replay: %v", err)
	}
}
