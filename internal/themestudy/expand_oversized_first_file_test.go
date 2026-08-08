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
// applies to the FIRST file as well: the oversized file is retained only as a
// typed closed record under OmittedRefs and a readable sibling continues.
func TestExpandFilesOversizedFirstFileClosesAndContinuesSibling(t *testing.T) {
	t.Parallel()
	// ~1200 lines of 500+ chars — beyond MaxExpansionBytes on its own
	// (1200 × 500 = 600 KiB raw).
	bigLines := make([]string, 1200)
	for i := range bigLines {
		bigLines[i] = strings.Repeat("x", 500)
	}
	reader := func(path string, from, to int) ([]string, error) {
		if path == "small.go" {
			return []string{"package small"}, nil
		}
		if to > len(bigLines) {
			to = len(bigLines)
		}
		if from < 1 {
			from = 1
		}
		return bigLines[from-1 : to], nil
	}
	totalLines := func(path string) (int, error) {
		if path == "small.go" {
			return 1, nil
		}
		return len(bigLines), nil
	}

	expansion, err := ExpandFiles([]FileRef{
		{Ref: "f1", Path: "big.go"},
		{Ref: "f2", Path: "small.go"},
	}, reader, totalLines)
	if err != nil {
		t.Fatalf("ExpandFiles returned terminal error for oversized first file: %v", err)
	}
	if len(expansion.Files) != 2 {
		t.Fatalf("files = %#v, want one closed file plus one accepted sibling", expansion.Files)
	}
	closed := expansion.Files[0]
	if !closed.Closed || closed.ClosedReason != ExpansionClosedReasonOversized ||
		len(closed.Objects) != 0 || closed.ExpandedLines != 0 {
		t.Fatalf("oversized file closure = %#v", closed)
	}
	accepted := expansion.Files[1]
	if accepted.Closed || accepted.Ref != "f2" || len(accepted.Objects) != 1 ||
		len(accepted.Objects[0].Lines) != 1 || accepted.Objects[0].Lines[0] != "package small" {
		t.Fatalf("accepted sibling = %#v", accepted)
	}
	if len(expansion.OmittedRefs) != 1 || expansion.OmittedRefs[0] != "f1" {
		t.Fatalf("expected f1 under OmittedRefs, got %v", expansion.OmittedRefs)
	}
	if _, err := EncodeExpansion(expansion); err != nil {
		t.Fatalf("closed oversized source must leave an encodable expansion: %v", err)
	}
}
