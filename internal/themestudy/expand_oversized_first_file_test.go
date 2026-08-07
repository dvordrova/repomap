package themestudy

import (
	"strings"
	"testing"
)

// Archive 12 P0 regression (casdoor live run 2026-08-07): a requested file
// whose expansion alone exceeds the byte budget was appended unconditionally
// (first-append exception), overflowing the encoded artifact
// (MaxExpansionArtifactBytes) and terminating the whole Study stage with
// "theme source expansion artifact exceeds 393216 bytes". The budget now
// applies to the FIRST file as well: the oversized file is recorded under
// OmittedRefs and the stage continues.
func TestExpandFilesOversizedFirstFileIsOmittedNotTerminal(t *testing.T) {
	t.Parallel()
	// ~1200 lines of 500+ chars — beyond MaxExpansionBytes on its own
	// (1200 × 500 = 600 KiB raw).
	bigLines := make([]string, 1200)
	for i := range bigLines {
		bigLines[i] = strings.Repeat("x", 500)
	}
	reader := func(path string, from, to int) ([]string, error) {
		if to > len(bigLines) {
			to = len(bigLines)
		}
		if from < 1 {
			from = 1
		}
		return bigLines[from-1 : to], nil
	}
	totalLines := func(path string) (int, error) { return len(bigLines), nil }

	expansion, err := ExpandFiles([]FileRef{{Ref: "f1", Path: "big.go"}}, reader, totalLines)
	if err != nil {
		t.Fatalf("ExpandFiles returned terminal error for oversized first file: %v", err)
	}
	if len(expansion.Files) != 0 {
		t.Fatalf("expected the oversized first file to be omitted, got %d files", len(expansion.Files))
	}
	if len(expansion.OmittedRefs) != 1 || expansion.OmittedRefs[0] != "f1" {
		t.Fatalf("expected f1 under OmittedRefs, got %v", expansion.OmittedRefs)
	}
}
