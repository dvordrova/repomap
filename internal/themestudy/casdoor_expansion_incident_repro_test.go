package themestudy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Incident repro (casdoor live run 2026-08-07 11:53, pre-12e0b9e): 53 f*
// requested files expanded to raw ~346 KiB, then EncodeExpansion failed with
// "theme source expansion artifact exceeds 393216 bytes" — a terminal error
// that killed the whole run despite accepted Scout state. The real question:
// is the encoded artifact bound enforced against the RAW byte budget (which
// it is not — JSON escaping and the per-file envelope grow raw bytes ~1.5x),
// and is the hard failure even necessary (contract D artifact is persisted
// locally, never sent to the provider)?
func TestCasdoorExpansionEncodeBoundRepro(t *testing.T) {
	repo := "/Users/dvordrova/git/casdoor"
	if _, err := os.Stat(repo); err != nil {
		t.Skipf("casdoor repo not present: %v", err)
	}
	data, err := os.ReadFile("/tmp/v11-live-casdoor-v20/20260807-115328-casdoor-05cbf07364f9/theme_scout_request.v1.json")
	if err != nil {
		t.Skipf("incident scout request not present: %v", err)
	}
	var request struct {
		WireJSON string `json:"wire_json"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatalf("decode scout request: %v", err)
	}
	var wire struct {
		Vocabulary struct {
			Files []struct {
				Ref  string `json:"ref"`
				Path string `json:"path"`
			} `json:"files"`
		} `json:"vocabulary"`
	}
	if err := json.Unmarshal([]byte(request.WireJSON), &wire); err != nil {
		t.Fatalf("decode wire: %v", err)
	}
	resultData, err := os.ReadFile("/tmp/v11-live-casdoor-v20/20260807-115328-casdoor-05cbf07364f9/theme_scout_result.v1.json")
	if err != nil {
		t.Skipf("incident scout result not present: %v", err)
	}
	var result struct {
		Candidates []struct {
			ExpansionFileRefs []string `json:"expansion_file_refs"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("decode scout result: %v", err)
	}
	refs := map[string]string{}
	for _, f := range wire.Vocabulary.Files {
		refs[f.Ref] = f.Path
	}
	requested := map[string]struct{}{}
	for _, c := range result.Candidates {
		for _, f := range c.ExpansionFileRefs {
			requested[f] = struct{}{}
		}
	}
	var files []FileRef
	for ref := range requested {
		files = append(files, FileRef{Ref: ref, Path: refs[ref]})
	}
	reader := func(path string, from, to int) ([]string, error) {
		lines, err := readRealLines(filepath.Join(repo, path), from, to)
		return lines, err
	}
	totalLines := func(path string) (int, error) {
		return countRealLines(filepath.Join(repo, path))
	}
	expansion, err := ExpandFiles(files, reader, totalLines)
	if err != nil {
		t.Fatalf("ExpandFiles: %v", err)
	}
	t.Logf("requested=%d included=%d omitted=%d raw=%d",
		len(requested), len(expansion.Files), len(expansion.OmittedRefs), expansion.ExpandedBytes)
	encoded, err := EncodeExpansion(expansion)
	if err != nil {
		t.Fatalf("EncodeExpansion: %v", err)
	}
	t.Logf("encoded=%d (limit=%d)", len(encoded), MaxExpansionArtifactBytes)
}

func readRealLines(path string, from, to int) ([]string, error) {
	all, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(all)
	lines := []string{}
	start := 0
	for i := 0; i <= len(text); i++ {
		if i == len(text) || text[i] == '\n' {
			lines = append(lines, text[start:i])
			start = i + 1
			if len(lines) == to {
				break
			}
		}
	}
	if from > 1 && from <= len(lines) {
		lines = lines[from-1:]
	}
	return lines, nil
}

func countRealLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	count := 1
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	return count, nil
}
