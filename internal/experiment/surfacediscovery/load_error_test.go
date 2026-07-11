package surfacediscovery

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestSurfacePackageLoadErrorReturnsOneBoundedSummary(t *testing.T) {
	loaded := []*packages.Package{{
		Errors: []packages.Error{
			{Pos: "z.go:9:1", Msg: "requires newer Go version"},
			{Pos: "a.go:2:1", Msg: "undefined: dependency"},
		},
	}}

	err := surfacePackageLoadError(loaded)
	if err == nil {
		t.Fatal("error = nil, want a package-load summary")
	}
	message := err.Error()
	for _, want := range []string{"2 error(s)", "a.go:2:1: undefined: dependency"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want %q", message, want)
		}
	}
	if strings.Contains(message, "z.go:9:1") {
		t.Fatalf("error leaked more than the first deterministic detail: %q", message)
	}
}

func TestCheckSurfaceGoVersionRejectsNewerTargetBeforePackageLoading(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/future\n\ngo 99.0\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	err := checkSurfaceGoVersion(root)
	if err == nil {
		t.Fatal("error = nil, want an incompatible-toolchain error")
	}
	for _, want := range []string{"requires go99.0", runtime.Version()} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err, want)
		}
	}
}
