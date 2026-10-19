package nativeproc

import (
	"os"
	"strings"
	"testing"
)

func TestSourceFileHoldsTheSourceForOneInvocationAndRemovesIt(t *testing.T) {
	source := strings.Repeat("x", 200_000)
	path, remove, err := SourceFile("repomap-test-*.py", source)
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != source {
		t.Fatalf("source file holds %d bytes, want %d", len(written), len(source))
	}
	if !strings.HasSuffix(path, ".py") {
		t.Fatalf("pattern suffix lost: %s", path)
	}
	remove()
	remove()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("source file survives remove: %v", err)
	}
}
