package workspacecontent

import (
	"testing"
)

// Decision 231 (Archive 9, ghz live run 20260806-231526): an exact read of
// one short line must survive the ±10 context padding even when the file
// contains an oversized generated line (statik.go embeds a 1.4MB string).
// The padding may skip oversized lines OUTSIDE the requested span; only an
// oversized line INSIDE the requested span is a hard LimitLine.
func TestContextPaddingSkipsOversizedLinesOutsideRequestedSpan(t *testing.T) {
	t.Parallel()
	// 13 lines; line 11 is 1.4MB (oversized), every other line is short.
	content := make([]byte, 0, 1_500_000)
	for line := 1; line <= 13; line++ {
		if line == 11 {
			content = append(content, make([]byte, 1_400_000)...)
		} else {
			content = append(content, byte('a'+line%26))
		}
		content = append(content, '\n')
	}
	start, end, focus, truncated, ok := selectRange(
		Range{StartLine: 4, EndLine: 4, FocusLine: 4},
		13,
		maxLines,
	)
	if !ok {
		t.Fatal("selectRange failed")
	}
	if start == 1 && end == 13 {
		t.Logf("padding expanded 4-4 to %d-%d", start, end)
	}
	lines, _, err := selectLines(
		content,
		start, end,
		4, 4,
		maxTextBytes,
		maxLineBytes,
		maxLines,
	)
	if err != nil {
		t.Fatalf("exact line 4 with oversized context: %v", err)
	}
	foundLine4 := false
	for _, line := range lines {
		if line.Number == 4 {
			foundLine4 = true
		}
		if line.Number == 11 {
			t.Fatal("oversized padding line was served as content")
		}
	}
	if !foundLine4 {
		t.Fatalf("exact requested line 4 missing from result: %#v", lines)
	}
	_ = focus
	_ = truncated
}

// The requested span itself still fails hard when it contains an oversized
// line — an exact read that cannot be served faithfully must not guess.
func TestRequestedSpanOversizedLineStaysHardLimit(t *testing.T) {
	t.Parallel()
	content := append([]byte("one\ntwo\n"), make([]byte, 1_400_000)...)
	content = append(content, '\n')
	start, end, _, _, ok := selectRange(Range{StartLine: 3, EndLine: 3, FocusLine: 3}, 3, maxLines)
	if !ok {
		t.Fatal("selectRange failed")
	}
	_, _, err := selectLines(
		content,
		start, end,
		3, 3,
		maxTextBytes,
		maxLineBytes,
		maxLines,
	)
	if ErrorKindOf(err) != ErrorLimitExceeded || LimitKindOf(err) != LimitLine {
		t.Fatalf("oversized requested line: err=%v kind=%q limit=%q", err, ErrorKindOf(err), LimitKindOf(err))
	}
}
