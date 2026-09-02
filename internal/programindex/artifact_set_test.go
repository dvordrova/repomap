package programindex

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestArtifactPersistenceAndInventoryHelpers(t *testing.T) {
	index, err := newMeasuredProgramIndex(shapeInput())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	set, err := BuildArtifactSet(index)
	if err != nil {
		t.Fatalf("BuildArtifactSet: %v", err)
	}
	if len(set.Entries) != 1 || set.DefaultTargetID != index.Target.ID ||
		set.Entries[0].TargetID != index.Target.ID ||
		set.Entries[0].Filename != ArtifactFilename ||
		set.Entries[0].IndexSHA256 != index.SHA256 {
		t.Fatalf("page-local ProgramIndex set = %#v", set)
	}

	runDir := t.TempDir()
	if err := Persist(runDir, "", index); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if err := PersistArtifactSet(runDir, set); err != nil {
		t.Fatalf("PersistArtifactSet: %v", err)
	}
	indexBytes, err := os.ReadFile(filepath.Join(runDir, ArtifactFilename))
	if err != nil {
		t.Fatalf("read persisted index: %v", err)
	}
	restoredIndex, err := Decode(indexBytes)
	if err != nil {
		t.Fatalf("decode persisted index: %v", err)
	}
	if restoredIndex.SHA256 != index.SHA256 || restoredIndex.Target.ID != index.Target.ID {
		t.Fatal("persisted index changed exact authority")
	}
	setBytes, err := os.ReadFile(filepath.Join(runDir, ArtifactSetFilename))
	if err != nil {
		t.Fatalf("read persisted artifact set: %v", err)
	}
	restoredSet, err := DecodeArtifactSet(setBytes)
	if err != nil {
		t.Fatalf("decode persisted artifact set: %v", err)
	}
	if restoredSet.SHA256 != set.SHA256 || restoredSet.DefaultTargetID != set.DefaultTargetID {
		t.Fatal("persisted artifact set changed exact authority")
	}
}

func TestArtifactSetSealsOnePageLocalBinding(t *testing.T) {
	set, err := NewArtifactSet("python-target-api", []ArtifactSetEntry{
		{TargetID: "python-target-api", Filename: ArtifactFilename, IndexSHA256: strings.Repeat("a", 64)},
	})
	if err != nil {
		t.Fatalf("NewArtifactSet: %v", err)
	}
	if err := set.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := set.Entries[0].TargetID, "python-target-api"; got != want {
		t.Fatalf("page-local target = %q, want %q", got, want)
	}
	if got := len(set.SHA256); got != 64 {
		t.Fatalf("sha256 length = %d", got)
	}

	snapshot := set.Snapshot()
	snapshot.Entries[0].Filename = "changed.json"
	if set.Entries[0].Filename == "changed.json" {
		t.Fatal("Snapshot aliases entry storage")
	}
	if ArtifactSetFilename != "program-index-set.json" || ArtifactSetVersion != 1 {
		t.Fatalf("artifact identity drift: filename=%q version=%d", ArtifactSetFilename, ArtifactSetVersion)
	}
}

func TestArtifactSetCodecIsStrictAndValidatesSeal(t *testing.T) {
	set, err := NewArtifactSet("go-target-main", []ArtifactSetEntry{{
		TargetID: "go-target-main", Filename: ArtifactFilename, IndexSHA256: strings.Repeat("b", 64),
	}})
	if err != nil {
		t.Fatalf("NewArtifactSet: %v", err)
	}
	encoded, err := EncodeArtifactSet(set)
	if err != nil {
		t.Fatalf("EncodeArtifactSet: %v", err)
	}
	decoded, err := DecodeArtifactSet(encoded)
	if err != nil {
		t.Fatalf("DecodeArtifactSet: %v", err)
	}
	if !reflect.DeepEqual(decoded, set) {
		t.Fatalf("codec changed set:\nencoded=%s\ndecoded=%#v", encoded, decoded)
	}
	if _, err := DecodeArtifactSet(append(append([]byte(nil), encoded...), []byte(` {}`)...)); err == nil {
		t.Fatal("DecodeArtifactSet accepted trailing JSON")
	}
	unknown := append([]byte(`{"unknown":true,`), encoded[1:]...)
	if _, err := DecodeArtifactSet(unknown); err == nil {
		t.Fatal("DecodeArtifactSet accepted an unknown field")
	}
	tampered := []byte(strings.Replace(string(encoded), ArtifactFilename, "changed.json", 1))
	if _, err := DecodeArtifactSet(tampered); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("tampered DecodeArtifactSet error = %v", err)
	}
}

func TestArtifactSetRejectsInvalidBindings(t *testing.T) {
	shaA := strings.Repeat("a", 64)
	tests := []struct {
		name      string
		defaultID string
		entries   []ArtifactSetEntry
		want      string
	}{
		{
			name: "default is absent", defaultID: "target-missing",
			entries: []ArtifactSetEntry{{
				TargetID: "target-a", Filename: ArtifactFilename, IndexSHA256: shaA,
			}},
			want: "default target",
		},
		{
			name: "multiple entries", defaultID: "target-a",
			entries: []ArtifactSetEntry{
				{TargetID: "target-a", Filename: ArtifactFilename, IndexSHA256: shaA},
				{TargetID: "target-b", Filename: ArtifactFilename, IndexSHA256: strings.Repeat("b", 64)},
			},
			want: "exactly one page-local entry",
		},
		{
			name: "empty entries", defaultID: "target-a",
			want: "exactly one page-local entry",
		},
		{
			name: "non-canonical target filename", defaultID: "target-a",
			entries: []ArtifactSetEntry{{TargetID: "target-a", Filename: "program-index.target-a.json", IndexSHA256: shaA}},
			want:    "not canonical",
		},
		{
			name: "invalid index digest", defaultID: "target-a",
			entries: []ArtifactSetEntry{{TargetID: "target-a", Filename: ArtifactFilename, IndexSHA256: "not-a-sha"}},
			want:    "invalid entry",
		},
		{
			name: "legacy JSTS filename is never canonical", defaultID: "target-a",
			entries: []ArtifactSetEntry{{
				TargetID: "target-a", Filename: "program-index-jsts.json", IndexSHA256: shaA,
			}},
			want: "not canonical",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewArtifactSet(test.defaultID, test.entries)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewArtifactSet error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestArtifactSetValidateRejectsNonCanonicalAndTamperedContent(t *testing.T) {
	set, err := NewArtifactSet("target-a", []ArtifactSetEntry{
		{TargetID: "target-a", Filename: ArtifactFilename, IndexSHA256: strings.Repeat("a", 64)},
	})
	if err != nil {
		t.Fatalf("NewArtifactSet: %v", err)
	}

	wrongTarget := set.Snapshot()
	wrongTarget.Entries[0].TargetID = "target-b"
	if err := wrongTarget.Validate(); err == nil || !strings.Contains(err.Error(), "default target") {
		t.Fatalf("wrong-target Validate error = %v", err)
	}

	tampered := set.Snapshot()
	tampered.Entries[0].IndexSHA256 = strings.Repeat("c", 64)
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered Validate error = %v", err)
	}
}

func TestArtifactSetRetainsArtifactBeyondFormerLocalByteThreshold(t *testing.T) {
	targetID := strings.Repeat("t", AdvisoryArtifactSetBytes+1)
	set, err := NewArtifactSet(targetID, []ArtifactSetEntry{{
		TargetID: targetID, Filename: ArtifactFilename, IndexSHA256: strings.Repeat("a", 64),
	}})
	if err != nil {
		t.Fatalf("NewArtifactSet above former byte threshold: %v", err)
	}
	encoded, err := EncodeArtifactSet(set)
	if err != nil {
		t.Fatalf("EncodeArtifactSet above former byte threshold: %v", err)
	}
	if len(encoded) <= AdvisoryArtifactSetBytes {
		t.Fatalf("fixture artifact = %d bytes", len(encoded))
	}
	warnings := ArtifactSetScaleWarnings(set)
	found := false
	for _, warning := range warnings {
		if warning.Kind == ArtifactSetScaleWarningBytes && warning.Retained == len(encoded) {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing artifact-byte warning: %#v", warnings)
	}
	decoded, err := DecodeArtifactSet(encoded)
	if err != nil || decoded.SHA256 != set.SHA256 {
		t.Fatalf("DecodeArtifactSet above former byte threshold: %#v, %v", decoded, err)
	}
}
