package jstsproject

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Linux caps one argv string at 128 KiB; the embedded helper (180 KB) reaches
// Node as a module file that the call removes afterwards.
func TestHelperSourceReachesNodeAsAFileNotAnArgument(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled executable uses a POSIX shell")
	}
	if len(nodeHelper) < 131072 {
		t.Logf("helper source is %d bytes; the argv limit is not exercised", len(nodeHelper))
	}
	bin := t.TempDir()
	body := "#!/bin/sh\n" + strings.Join([]string{
		`for a in "$@"; do [ "${#a}" -lt 131072 ] || { printf 'argument too long' >&2; exit 23; }; done`,
		`eval last=\${$#}`,
		`[ -f "$last" ] && printf 'source:%s' "$last" >&2`,
		`exit 23`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	_, err := invokeHelper(t.Context(), t.TempDir(), helperRequest{})
	if err == nil || !strings.Contains(err.Error(), "source:") {
		t.Fatalf("helper did not receive its source as a file: %v", err)
	}
	path := err.Error()[strings.Index(err.Error(), "source:")+len("source:"):]
	if !strings.HasSuffix(path, ".mjs") {
		t.Fatalf("source path has no .mjs suffix: %s", path)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("source file %s survives the call: %v", path, statErr)
	}
}
