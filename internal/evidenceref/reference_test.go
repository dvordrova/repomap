package evidenceref

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestCanonicalizeGroundedSourceLines(t *testing.T) {
	t.Parallel()

	paths := []string{"batch.go", "batchrepr/reader.go", "internal/private/batch.go"}
	locations := []evidence.Location{
		{Path: "batch.go", Line: 395},
		{Path: "batch.go", Line: 952},
		{Path: "batch.go", Line: 1042},
		{Path: "batchrepr/reader.go", Line: 49},
		{Path: "internal/private/batch.go", Line: 12},
	}
	tests := map[string]string{
		"batch.go line 395: fsync call":                           "batch.go:395: fsync call",
		"batch.go references compaction at line 952":              "batch.go:952 references compaction",
		"batch.go defines DeleteRange method at line 1042":        "batch.go:1042 defines DeleteRange method",
		"batchrepr/reader.go lines 49-50: committed sequence":     "batchrepr/reader.go:49: committed sequence",
		"batch.go:395 contains fsync":                             "batch.go:395 contains fsync",
		"batch.go#395 contains fsync":                             "batch.go:395 contains fsync",
		"internal/private/batch.go line 12: helper":               "internal/private/batch.go:12: helper",
		"batch.go and batchrepr/reader.go are related components": "batch.go and batchrepr/reader.go are related components",
	}
	for input, want := range tests {
		input, want := input, want
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			got, grounded := Canonicalize(input, paths, locations)
			if !grounded || got != want {
				t.Fatalf("Canonicalize() = %q, %v; want %q, true", got, grounded, want)
			}
		})
	}
}

func TestExtractGroupsPathAndLineReferences(t *testing.T) {
	t.Parallel()

	got := Extract(
		"batch.go and batch.go:395 lead to wal/wal.go; not-batch.go2 stays prose",
		[]string{"batch.go", "wal/wal.go"},
	)
	want := []evidence.Location{
		{Path: "batch.go"},
		{Path: "batch.go", Line: 395},
		{Path: "wal/wal.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Extract() = %#v, want %#v", got, want)
	}
}

func TestCanonicalizeRemovesUngroundedLine(t *testing.T) {
	t.Parallel()

	got, grounded := Canonicalize(
		"batch.go contains fsync at line 999",
		[]string{"batch.go"},
		[]evidence.Location{{Path: "batch.go", Line: 395}},
	)
	if grounded || got != "batch.go contains fsync" {
		t.Fatalf("Canonicalize() = %q, %v", got, grounded)
	}
}
