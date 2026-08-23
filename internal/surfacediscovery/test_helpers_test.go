package surfacediscovery

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func defaultHostOptions(repository string) Options {
	return DefaultOptions(repository, runtime.GOOS+"/"+runtime.GOARCH)
}

func analyzeForTest(options Options, input Input) (Result, error) {
	return AnalyzeContextWithInput(context.Background(), options, input)
}

func writeFixtureFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeTargetScopeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	writeFixtureFile(t, filepath.Join(root, filepath.FromSlash(relative)), content)
}
