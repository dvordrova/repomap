package reportserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenInvestigationStoreSupportsAtomicReplacement(t *testing.T) {
	_, runsDir, _ := writeAnalysisRun(t)
	h := handler{runsDir: runsDir}
	root, _, err := h.openInvestigationStore("20260711-220000-pebble", true)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	const temporary = ".checkpoint.tmp"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(
		filepath.Join(root.Name(), temporary),
		filepath.Join(root.Name(), "checkpoint.json"),
	); err != nil {
		t.Fatalf("atomic replacement below investigation root %q: %v", root.Name(), err)
	}
	if info, err := root.Lstat("checkpoint.json"); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("checkpoint info = %#v, err=%v", info, err)
	}
}
