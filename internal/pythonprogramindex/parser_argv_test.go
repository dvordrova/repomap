package pythonprogramindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Linux caps one argv string at 128 KiB; the embedded parser is larger, so it
// must reach the interpreter as a file that the call removes afterwards.
func TestIndexParserSourceReachesTheInterpreterAsAFileNotAnArgument(t *testing.T) {
	if len(parserHelper) < 131072 {
		t.Logf("parser source is %d bytes; the argv limit is not exercised", len(parserHelper))
	}
	executable := controlledIndexParserExecutable(t, strings.Join([]string{
		`for a in "$@"; do [ "${#a}" -lt 131072 ] || { printf 'argument too long' >&2; exit 23; }; done`,
		`eval last=\${$#}`,
		`[ -f "$last" ] && printf 'source:%s' "$last" >&2`,
		`exit 23`,
	}, "\n")+"\n")
	t.Setenv("PATH", filepath.Dir(executable))
	_, err := runParser(t.Context(), parserRequest{})
	if err == nil || !strings.Contains(err.Error(), "source:") {
		t.Fatalf("parser did not receive its source as a file: %v", err)
	}
	path := err.Error()[strings.Index(err.Error(), "source:")+len("source:"):]
	if !strings.HasSuffix(path, ".py") {
		t.Fatalf("source path has no .py suffix: %s", path)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("source file %s survives the call: %v", path, statErr)
	}
}
