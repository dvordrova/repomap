package themestudy

import (
	"strings"
	"testing"
)

// Long-horizon incident regression (casdoor live run 2026-08-07, pre-12e0b9e):
// the expansion budget was measured on RAW source bytes while the persisted
// artifact limit applies to the ENCODED JSON (escaping + per-object envelope
// grow raw ~1.3-2x). A raw budget at MaxExpansionBytes let the encoded
// artifact exceed MaxExpansionArtifactBytes and EncodeExpansion terminated
// the whole Study stage with "theme source expansion artifact exceeds 393216
// bytes" — despite accepted Scout state and with no durable adjudication
// status. The budget is now measured on encoded bytes, so EncodeExpansion is
// guaranteed to succeed: the encoded artifact stays inside its bound and the
// only mechanism for excess is OmittedRefs (bounded, never terminal).
func TestExpandFilesEncodedBudgetGuaranteesEncodeExpansion(t *testing.T) {
	t.Parallel()
	// 40 files x 200 lines x 200 chars of JSON-hostile content (quotes and
	// backslashes inflate encoded size well beyond raw). Raw total:
	// 40 * 200 * 200 = 1.6 MiB — far beyond any raw budget; the encoded
	// budget must admit the first files and omit the rest, and
	// EncodeExpansion must succeed on whatever was admitted.
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = strings.Repeat(`\"\\{x}`, 40) // 240 chars, escape-heavy
	}
	reader := func(path string, from, to int) ([]string, error) {
		return lines[from-1 : to], nil
	}
	totalLines := func(path string) (int, error) { return len(lines), nil }
	var files []FileRef
	for i := 0; i < 40; i++ {
		files = append(files, FileRef{Ref: "f" + itoa(i+1), Path: "p/f" + itoa(i+1) + ".go"})
	}
	expansion, err := ExpandFiles(files, reader, totalLines)
	if err != nil {
		t.Fatalf("ExpandFiles: %v", err)
	}
	if len(expansion.Files) == 0 {
		t.Fatalf("encoded budget must admit at least the first files")
	}
	if len(expansion.OmittedRefs) == 0 {
		t.Fatalf("escape-heavy set must exceed the encoded budget at some point")
	}
	encoded, err := EncodeExpansion(expansion)
	if err != nil {
		t.Fatalf("EncodeExpansion must never fail under the encoded budget: %v", err)
	}
	if len(encoded) > MaxExpansionArtifactBytes {
		t.Fatalf("encoded artifact %d exceeds bound %d", len(encoded), MaxExpansionArtifactBytes)
	}
	// Every admitted file must also decode cleanly (round trip).
	decoded, err := DecodeExpansion(encoded)
	if err != nil {
		t.Fatalf("DecodeExpansion: %v", err)
	}
	if len(decoded.Files) != len(expansion.Files) {
		t.Fatalf("round-trip file count %d != %d", len(decoded.Files), len(expansion.Files))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
