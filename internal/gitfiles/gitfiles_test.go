package gitfiles

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

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
