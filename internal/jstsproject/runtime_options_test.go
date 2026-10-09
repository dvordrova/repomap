package jstsproject

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func TestCumulativeJSTSNativeNodeOptionsReachRuntimeWithoutProviderCredentials(t *testing.T) {
	root := preparedCompilerProject(t)
	_, file, _, _ := runtime.Caller(0)
	fixture := filepath.Join(filepath.Dir(file), "../../testdata/repositories/jsts")
	const project = "packages/template-tool/"
	paths := []string{project + "index.tsx", project + "package.json", project + "tsconfig.json", project + "vite.config.ts"}
	for _, name := range paths {
		source, err := os.ReadFile(filepath.Join(fixture, name))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, name, string(source))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	t.Setenv("NODE_OPTIONS", "")
	baseline, err := DiscoverSelected(t.Context(), repository, root, "jsts:"+project+"package.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline.Files) != 1 || baseline.Files[0].Path != project+"vite.config.ts" || len(baseline.Calls) != 1 {
		t.Fatalf("complete native fixture did not retain its exact tool and call: %#v", baseline)
	}
	probe := t.TempDir()
	receipt := filepath.Join(probe, "runtime.json")
	preload := filepath.Join(probe, "runtime.cjs")
	source := `require("node:fs").writeFileSync(` + strconv.Quote(receipt) + `,JSON.stringify({heap:require("node:v8").getHeapStatistics().heap_size_limit,credentials:["DEEPSEEK_API_KEY","JEV_API_KEY"].some(key=>Object.hasOwn(process.env,key))}));`
	if err := os.WriteFile(preload, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEEPSEEK_API_KEY", "synthetic-native-env-test")
	t.Setenv("JEV_API_KEY", "synthetic-native-env-test")
	t.Setenv("NODE_OPTIONS", "--max-old-space-size=8192 --require "+strconv.Quote(preload))
	configured, err := DiscoverSelected(t.Context(), repository, root, "jsts:"+project+"package.json")
	if err != nil {
		t.Fatal(err)
	}
	if configured.SHA256 != baseline.SHA256 {
		t.Fatal("heap configuration changed the complete native compiler graph")
	}
	encoded, err := os.ReadFile(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var observed struct {
		Heap        int64 `json:"heap"`
		Credentials bool  `json:"credentials"`
	}
	if err := json.Unmarshal(encoded, &observed); err != nil {
		t.Fatal(err)
	}
	if observed.Heap < 8192*1024*1024 || observed.Credentials {
		t.Fatalf("native heap setting absent or provider credentials forwarded: %+v", observed)
	}
	if err := os.Remove(receipt); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NODE_OPTIONS", "--repomap-invalid-native-option")
	_, err = DiscoverSelected(t.Context(), repository, root, "jsts:"+project+"package.json")
	if err == nil || !strings.Contains(err.Error(), "not allowed in NODE_OPTIONS") {
		t.Fatalf("invalid native option did not fail at Node boundary: %v", err)
	}
	if _, err := os.Stat(receipt); !os.IsNotExist(err) {
		t.Fatalf("invalid Node option reached the native helper: %v", err)
	}
}
