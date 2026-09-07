package reading

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

func TestSavedInputRunsThroughFilesWithoutLaterStages(t *testing.T) {
	cache := t.TempDir()
	first := readOptions(t, testGraph(t), &tableProvider{}, cache)
	result, err := Read(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete {
		t.Fatal("full reading not complete")
	}
	input, err := LoadInput(filepath.Join(first.OwnerRunDir, InputFilename))
	if err != nil {
		t.Fatal(err)
	}
	provider := &tableProvider{}
	opts := input.Options()
	opts.Executor, opts.Provider = first.Executor, provider
	opts.OwnerRunDir, opts.Through = t.TempDir(), lines.StageFiles
	again, err := Read(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if again.Complete || len(again.Atlas.Targets) != 0 || again.Through != lines.StageFiles {
		t.Fatal("partial reading advertised a complete atlas")
	}
	if provider.calls != 0 {
		t.Fatalf("same input caused %d live calls", provider.calls)
	}
	for _, use := range again.Uses {
		if use.Stage != lines.StageDirectories && use.Stage != lines.StageFiles {
			t.Fatalf("ran later stage %s", use.Stage)
		}
	}
	// A prompt edit must ask only the changed stage. Its predecessors stay cached.
	provider = &tableProvider{}
	opts.Provider, opts.OwnerRunDir, opts.Prompt = provider, t.TempDir(), "A different file prompt"
	edited, err := Read(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, use := range edited.Uses {
		if use.Stage == lines.StageDirectories && use.Live != 0 {
			t.Fatal("prompt edit invalidated predecessor")
		}
		if use.Stage == lines.StageFiles && use.Live == 0 {
			t.Fatal("prompt edit reused old response")
		}
	}
	results, _ := filepath.Glob(filepath.Join(opts.OwnerRunDir, atlas.TablesDir, "atlas_files-*.result.json"))
	if len(results) == 0 {
		t.Fatal("no normalized results")
	}
	raw, _ := os.ReadFile(results[0])
	if !strings.Contains(string(raw), `"id": "file:pkg/a/x.go"`) || !strings.Contains(string(raw), `"path": "pkg/a/x.go"`) || !strings.Contains(string(raw), `"cells"`) {
		t.Fatalf("missing source binding: %s", raw)
	}
}

func TestSavedInputHasOneVersionAndRequiresCompleteTargets(t *testing.T) {
	opts := readOptions(t, testGraph(t), nil, "")
	if _, err := SaveInput(opts); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(opts.OwnerRunDir, InputFilename)
	raw, _ := os.ReadFile(filename)
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(){func() { input["version"] = InputVersion - 1 }, func() { input["version"] = InputVersion; input["targets"] = []any{} }} {
		change()
		data, _ := json.Marshal(input)
		os.WriteFile(filename, data, 0o600)
		if _, err := LoadInput(filename); err == nil {
			t.Fatal("incompatible or incomplete input accepted")
		}
	}
}
