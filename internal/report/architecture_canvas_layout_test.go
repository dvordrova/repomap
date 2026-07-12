package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestArchitectureCanvasLayoutModes(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}

	assetPath, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const window = { __REPOMAP_LAYOUT_TEST__: {} };
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
const api = window.__REPOMAP_LAYOUT_TEST__;
const mode = (groupCount, primaryCount) => api.landscapeLayoutMode({
  groups: Array.from({ length: groupCount }, (_, index) => ({ id: String(index) })),
  primaryRegion: primaryCount == null ? null : {
    groupIDs: Array.from({ length: primaryCount }, (_, index) => String(index)),
  },
});
process.stdout.write(JSON.stringify({
  modes: [mode(5, null), mode(5, 5), mode(5, 3)],
  columns: [1204, 1044, 884, 600].map((width) => api.boardProfileForWidth(width).columns),
  tieOrder: [
    api.shortestColumnIndex([0, 0, 0, 0], [1, 2, 0, 3]),
    api.shortestColumnIndex([10, 20, 5, 30], [1, 2, 0, 3]),
  ],
}));
`
	runnerPath := filepath.Join(t.TempDir(), "layout-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run Landscape layout contract: %v\n%s", err, output)
	}
	var result struct {
		Modes    []string `json:"modes"`
		Columns  []int    `json:"columns"`
		TieOrder []int    `json:"tieOrder"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode Landscape layout contract: %v\n%s", err, output)
	}
	if want := []string{"board", "graph", "hybrid"}; !reflect.DeepEqual(result.Modes, want) {
		t.Errorf("layout modes = %v, want %v", result.Modes, want)
	}
	if want := []int{4, 3, 2, 1}; !reflect.DeepEqual(result.Columns, want) {
		t.Errorf("responsive columns = %v, want %v", result.Columns, want)
	}
	if want := []int{1, 2}; !reflect.DeepEqual(result.TieOrder, want) {
		t.Errorf("shortest-column choices = %v, want %v", result.TieOrder, want)
	}
}
