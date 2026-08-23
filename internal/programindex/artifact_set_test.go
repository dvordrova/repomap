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
	if got := ArtifactFilenameForTarget("ignored", true); got != ArtifactFilename {
		t.Fatalf("default filename = %q, want %q", got, ArtifactFilename)
	}
	if got, want := ArtifactFilenameForTarget("pyt-example", false), "program-index.pyt-example.json"; got != want {
		t.Fatalf("target filename = %q, want %q", got, want)
	}

	set, err := BuildArtifactSet(index.Target.ID, []Index{index}, []string{ArtifactFilename})
	if err != nil {
		t.Fatalf("BuildArtifactSet: %v", err)
	}
	exact, err := ExactIndexByTargetID([]Index{index}, index.Target.ID)
	if err != nil {
		t.Fatalf("ExactIndexByTargetID: %v", err)
	}
	if exact.SHA256 != index.SHA256 {
		t.Fatal("exact index selection changed authority")
	}
	exact.Objects[0].Name = "consumer-owned"
	if index.Objects[0].Name == "consumer-owned" {
		t.Fatal("exact index selection aliases the source inventory")
	}
	if _, err := ExactIndexByTargetID(nil, index.Target.ID); err == nil ||
		!strings.Contains(err.Error(), "no exact default target") {
		t.Fatalf("missing ExactIndexByTargetID error = %v", err)
	}
	if _, err := ExactIndexByTargetID([]Index{index, index}, index.Target.ID); err == nil ||
		!strings.Contains(err.Error(), "repeats default target") {
		t.Fatalf("duplicate ExactIndexByTargetID error = %v", err)
	}
	if _, err := BuildArtifactSet(index.Target.ID, []Index{index}, nil); err == nil ||
		!strings.Contains(err.Error(), "inventories do not match") {
		t.Fatalf("mismatched BuildArtifactSet error = %v", err)
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

func TestArtifactSetCanonicalizesMultipleSelectedViewsAndSeals(t *testing.T) {
	set, err := NewArtifactSet("python-target-api", []ArtifactSetEntry{
		{TargetID: "python-target-worker", Filename: "program-index.worker.json", IndexSHA256: strings.Repeat("c", 64)},
		{TargetID: "python-target-api", Filename: ArtifactFilename, IndexSHA256: strings.Repeat("a", 64)},
		{TargetID: "python-target-cli", Filename: "program-index.cli.json", IndexSHA256: strings.Repeat("b", 64)},
	})
	if err != nil {
		t.Fatalf("NewArtifactSet: %v", err)
	}
	if err := set.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := set.Entries[0].TargetID, "python-target-api"; got != want {
		t.Fatalf("first target = %q, want %q", got, want)
	}
	if got := len(set.SHA256); got != 64 {
		t.Fatalf("sha256 length = %d", got)
	}

	reordered, err := NewArtifactSet("python-target-api", []ArtifactSetEntry{
		set.Entries[2], set.Entries[0], set.Entries[1],
	})
	if err != nil {
		t.Fatalf("NewArtifactSet reordered: %v", err)
	}
	if !reflect.DeepEqual(reordered, set) {
		t.Fatalf("input order changed canonical set:\nfirst=%#v\nsecond=%#v", set, reordered)
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
	if _, err := DecodeArtifactSet(tampered); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered DecodeArtifactSet error = %v", err)
	}
}

func TestArtifactSetRejectsInvalidBindings(t *testing.T) {
	shaA := strings.Repeat("a", 64)
	shaB := strings.Repeat("b", 64)
	tests := []struct {
		name      string
		defaultID string
		entries   []ArtifactSetEntry
		want      string
	}{
		{
			name: "default is absent", defaultID: "target-missing",
			entries: []ArtifactSetEntry{{TargetID: "target-a", Filename: ArtifactFilename, IndexSHA256: shaA}},
			want:    "default target",
		},
		{
			name: "duplicate target", defaultID: "target-a",
			entries: []ArtifactSetEntry{
				{TargetID: "target-a", Filename: ArtifactFilename, IndexSHA256: shaA},
				{TargetID: "target-a", Filename: "program-index.a.json", IndexSHA256: shaA},
			},
			want: "not canonical",
		},
		{
			name: "unsafe parent filename", defaultID: "target-a",
			entries: []ArtifactSetEntry{{TargetID: "target-a", Filename: "../program-index.json", IndexSHA256: shaA}},
			want:    "invalid entry",
		},
		{
			name: "unsafe windows filename", defaultID: "target-a",
			entries: []ArtifactSetEntry{{TargetID: "target-a", Filename: `scope\\program-index.json`, IndexSHA256: shaA}},
			want:    "invalid entry",
		},
		{
			name: "invalid index digest", defaultID: "target-a",
			entries: []ArtifactSetEntry{{TargetID: "target-a", Filename: ArtifactFilename, IndexSHA256: "not-a-sha"}},
			want:    "invalid entry",
		},
		{
			name: "same filename cannot represent two sealed targets", defaultID: "target-a",
			entries: []ArtifactSetEntry{
				{TargetID: "target-a", Filename: ArtifactFilename, IndexSHA256: shaA},
				{TargetID: "target-b", Filename: ArtifactFilename, IndexSHA256: shaB},
			},
			want: "more than one target",
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
		{TargetID: "target-b", Filename: "program-index.b.json", IndexSHA256: strings.Repeat("b", 64)},
		{TargetID: "target-a", Filename: ArtifactFilename, IndexSHA256: strings.Repeat("a", 64)},
	})
	if err != nil {
		t.Fatalf("NewArtifactSet: %v", err)
	}

	nonCanonical := set.Snapshot()
	nonCanonical.Entries[0], nonCanonical.Entries[1] = nonCanonical.Entries[1], nonCanonical.Entries[0]
	if err := nonCanonical.Validate(); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("non-canonical Validate error = %v", err)
	}

	tampered := set.Snapshot()
	tampered.Entries[0].IndexSHA256 = strings.Repeat("c", 64)
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered Validate error = %v", err)
	}
}
