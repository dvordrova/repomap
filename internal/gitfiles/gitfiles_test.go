package gitfiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestListWithModesContextTerminatesGitCommand(t *testing.T) {
	fakeBin := t.TempDir()
	fakeGit := filepath.Join(fakeBin, "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nexec /bin/sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := ListWithModesContext(ctx, t.TempDir())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ListWithModesContext error = %v, want context deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("canceled Git subprocess returned after %v", elapsed)
	}
}

func TestSplitNull(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want []string
	}{
		{"empty", nil, nil},
		{"single with trailing null", []byte("README.md\x00"), []string{"README.md"}},
		{"two with trailing null", []byte("cmd/main.go\x00pkg/foo.go\x00"), []string{"cmd/main.go", "pkg/foo.go"}},
		{"no trailing null", []byte("a.go\x00b.go"), []string{"a.go", "b.go"}},
		{"single no trailing null", []byte("README.md"), []string{"README.md"}},
		{"only null", []byte{0}, nil},
		{"three files", []byte("a\x00b\x00c\x00"), []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitNull(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("splitNull(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseIndexListingSeparatesRegularStageZeroPaths(t *testing.T) {
	listing, err := parseIndexListing([]byte(
		"100644 aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa 0\tmain.go\x00" +
			"100755 bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb 0\tscript.sh\x00" +
			"120000 cccccccccccccccccccccccccccccccccccccccc 0\tlinked.go\x00" +
			"160000 dddddddddddddddddddddddddddddddddddddddd 0\tvendor/submodule\x00" +
			"100644 eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee 1\tconflict.go\x00" +
			"100644 ffffffffffffffffffffffffffffffffffffffff 2\tconflict.go\x00",
	))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(listing.Paths, []string{
		"main.go", "script.sh", "linked.go", "vendor/submodule", "conflict.go",
	}) {
		t.Fatalf("paths = %#v", listing.Paths)
	}
	if !reflect.DeepEqual(listing.RegularPaths, []string{"main.go", "script.sh"}) {
		t.Fatalf("regular paths = %#v", listing.RegularPaths)
	}
	if !reflect.DeepEqual(listing.ExecutablePaths, []string{"script.sh"}) {
		t.Fatalf("executable paths = %#v", listing.ExecutablePaths)
	}
	if !reflect.DeepEqual(listing.Gitlinks, []Gitlink{{
		Path: "vendor/submodule", ObjectID: "dddddddddddddddddddddddddddddddddddddddd",
	}}) {
		t.Fatalf("gitlinks = %#v", listing.Gitlinks)
	}
}

func TestIsolatedEnvironmentDropsGitConfigInjectionAndNeutralizesGlobalConfig(t *testing.T) {
	got := strings.Join(isolatedEnvironment([]string{
		"PATH=/usr/bin",
		"GIT_CONFIG=/tmp/injected",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.bare",
		"GIT_CONFIG_VALUE_0=true",
		"GIT_CONFIG_PARAMETERS='core.worktree'='/tmp/other'",
		"GIT_CONFIG_GLOBAL=/tmp/global",
		"GIT_CONFIG_SYSTEM=/tmp/system",
	}), "\n")
	for _, forbidden := range []string{
		"GIT_CONFIG=/tmp/injected", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=",
		"GIT_CONFIG_VALUE_0=", "GIT_CONFIG_PARAMETERS=", "GIT_CONFIG_GLOBAL=/tmp/global",
		"GIT_CONFIG_SYSTEM=/tmp/system",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("isolated environment retained %q: %q", forbidden, got)
		}
	}
	for _, required := range []string{
		"PATH=/usr/bin", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("isolated environment lacks %q: %q", required, got)
		}
	}
}
