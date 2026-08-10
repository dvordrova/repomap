package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/gotarget"
)

func TestGoTargetEvidenceUsesOnlyPlatformConstraints(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		prefix string
		want   string
	}{
		{name: "filename", path: "daemon_linux.go", want: "linux"},
		{name: "filename pair", path: "daemon_linux_amd64.go", want: "linux"},
		{name: "build expression", path: "daemon.go", prefix: "//go:build linux || freebsd\n\npackage daemon\n", want: "linux,freebsd"},
		{name: "negative is not a vote", path: "daemon.go", prefix: "//go:build !windows\n\npackage daemon\n"},
		{name: "custom tag is not a platform vote", path: "daemon.go", prefix: "//go:build linux && integration\n\npackage daemon\n"},
		{name: "other architecture", path: "daemon_linux_arm64.go", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := goTargetEvidenceForFile(test.path, []byte(test.prefix), "amd64")
			var joined string
			for _, candidate := range []string{"linux", "freebsd"} {
				if _, ok := got[candidate]; ok {
					if joined != "" {
						joined += ","
					}
					joined += candidate
				}
			}
			if joined != test.want {
				t.Fatalf("evidence = %q, want %q", joined, test.want)
			}
		})
	}
}

func TestDetectGoTargetAdvisoryRequiresUniqueStrongAlternative(t *testing.T) {
	repository := t.TempDir()
	paths := []string{"daemon/a_linux.go", "daemon/b_linux.go", "daemon/c_linux.go"}
	for _, path := range paths {
		absolute := filepath.Join(repository, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte("package daemon\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	advisory := detectGoTargetAdvisory(repository, paths, gotarget.Target{GOOS: "darwin", GOARCH: "amd64"})
	if advisory == nil || advisory.Suggested != "linux/amd64" || advisory.EvidenceFiles != 3 || len(advisory.Examples) != 3 {
		t.Fatalf("advisory = %#v", advisory)
	}

	for _, name := range []string{"a_windows.go", "b_windows.go", "c_windows.go"} {
		path := filepath.Join(repository, name)
		if err := os.WriteFile(path, []byte("package daemon\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, name)
	}
	if got := detectGoTargetAdvisory(repository, paths, gotarget.Target{GOOS: "darwin", GOARCH: "amd64"}); got != nil {
		t.Fatalf("tied advisory = %#v, want nil", got)
	}
}

func TestGoTargetAdvisoryExcludesNonProductPaths(t *testing.T) {
	for _, path := range []string{
		"daemon_linux_test.go",
		"vendor/example.com/p_linux.go",
		"testdata/p_linux.go",
		"examples/p_linux.go",
		"tools/p_linux.go",
	} {
		if goTargetAdvisoryEligiblePath(path) {
			t.Fatalf("%q is eligible", path)
		}
	}
}

func TestAutomaticGoTargetSelectionRemainsBoundToExactAdvisory(t *testing.T) {
	advisory := GoTargetAdvisory{
		Suggested: "linux/amd64", EvidenceFiles: 3,
		Examples: []string{"daemon/a_linux.go", "daemon/b_linux.go", "daemon/c_linux.go"},
	}
	selection, err := newAutomaticGoTargetSelection(
		gotarget.Target{GOOS: "darwin", GOARCH: "amd64"}, advisory,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := selection.ValidateAgainstAdvisory(&advisory); err != nil {
		t.Fatal(err)
	}
	tampered := advisory
	tampered.Examples = append([]string(nil), advisory.Examples...)
	tampered.Examples[0] = "daemon/other_linux.go"
	if err := selection.ValidateAgainstAdvisory(&tampered); err == nil {
		t.Fatal("automatic selection accepted different advisory evidence")
	}
}
