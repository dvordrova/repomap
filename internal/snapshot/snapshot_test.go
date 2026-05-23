package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldSkipPath(t *testing.T) {
	cases := []struct {
		path string
		skip bool
	}{
		{"vendor/a.go", true},
		{"node_modules/pkg/index.js", true},
		{".github/workflows/ci.yml", true},
		{"configs/.env", true},
		{"certs/tls.key", true},
		{"images/logo.png", true},
		{"cmd/repomap/main.go", false},
	}

	for _, tc := range cases {
		if got := shouldSkipPath(tc.path); got != tc.skip {
			t.Fatalf("shouldSkipPath(%q)=%v, want %v", tc.path, got, tc.skip)
		}
	}
}

func TestParseModuleName(t *testing.T) {
	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module github.com/example/demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := parseModuleName(goModPath)
	if got != "github.com/example/demo" {
		t.Fatalf("parseModuleName()=%q, want %q", got, "github.com/example/demo")
	}
}

func TestTruncateUTF8Bytes(t *testing.T) {
	in := "ábc"
	got, truncated := truncateUTF8Bytes(in, 1)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if got != "" {
		t.Fatalf("got %q, want empty valid UTF-8 string", got)
	}

	got, truncated = truncateUTF8Bytes(in, 2)
	if !truncated {
		t.Fatal("expected truncation at 2")
	}
	if got != "á" {
		t.Fatalf("got %q, want %q", got, "á")
	}
}
