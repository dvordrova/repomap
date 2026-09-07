package run

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/repoconfig"
)

func TestConfCreatesAndOpensWithoutGitOrAnalysis(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	bin := t.TempDir()
	record := filepath.Join(t.TempDir(), "opened")
	if err := os.WriteFile(filepath.Join(bin, "code"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$REPOMAP_CONF_RECORD\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("REPOMAP_CONF_RECORD", record)
	var stdout, stderr bytes.Buffer
	if err := runConfWithContext(context.Background(), nil, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, repoconfig.Filename)
	data, err := os.ReadFile(record)
	if err != nil || string(data) != "--goto\n"+filename+":1:1\n" {
		t.Fatalf("editor arguments: %q %v", data, err)
	}
	if !strings.Contains(stdout.String(), "Created "+filename) {
		t.Fatalf("output: %s", &stdout)
	}
	custom := "# Owner comment\neditor: [code, '{{ .File }}', '+{{ .Line }}']\n"
	if err := os.WriteFile(filename, []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runConfWithContext(context.Background(), []string{root}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filename)
	if err != nil || string(data) != custom {
		t.Fatalf("config overwritten: %q %v", data, err)
	}
	data, err = os.ReadFile(record)
	if err != nil || string(data) != filename+"\n+1\n" {
		t.Fatalf("custom editor arguments: %q %v", data, err)
	}
}

func TestConfKeepsCreatedFileWhenEditorCannotStart(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	var output bytes.Buffer
	err := runConfWithContext(context.Background(), []string{root}, &output, &output)
	if err == nil || !strings.Contains(err.Error(), repoconfig.Filename) {
		t.Fatalf("missing editor error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, repoconfig.Filename)); err != nil {
		t.Fatal(err)
	}
}
