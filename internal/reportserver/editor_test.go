package reportserver

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestVSCodeCommandFallsBackToMacOSApplication(t *testing.T) {
	name, args := vsCodeCommand("darwin", "/repo/main.go:42", func(name string) (string, error) {
		if name == "open" {
			return "/usr/bin/open", nil
		}
		return "", errors.New("not found")
	})
	if name != "/usr/bin/open" || !reflect.DeepEqual(args, []string{"-a", "Visual Studio Code", "--args", "--goto", "/repo/main.go:42"}) {
		t.Fatalf("command = %q %v", name, args)
	}
}

func TestEditorLauncherStartsWithoutWaitingAndPassesExactTarget(t *testing.T) {
	if os.Getenv("REPOMAP_EDITOR_HELPER") == "1" {
		if err := os.WriteFile(os.Getenv("REPOMAP_EDITOR_RECORD"), []byte(strings.Join(os.Args[1:], "\n")), 0o600); err != nil {
			os.Exit(2)
		}
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}

	record := t.TempDir() + "/args"
	t.Setenv("REPOMAP_EDITOR_HELPER", "1")
	t.Setenv("REPOMAP_EDITOR_RECORD", record)
	launcher := editorLauncher(os.Args[0], []string{"-test.run=TestEditorLauncherStartsWithoutWaitingAndPassesExactTarget", "--", "--goto"})
	started := time.Now()
	if err := launcher(context.Background(), "/repo/main.go", 42, 7); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("launcher waited %v for child", elapsed)
	}
	deadline := time.Now().Add(time.Second)
	for {
		data, err := os.ReadFile(record)
		if err == nil {
			if !strings.Contains(string(data), "--goto\n/repo/main.go:42:7") {
				t.Fatalf("helper arguments = %q", data)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("editor helper did not record arguments")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestVSCodeCommandPrefersInstalledCLI(t *testing.T) {
	name, args := vsCodeCommand("darwin", "/repo/main.go:42", func(string) (string, error) {
		return "/custom/bin/code", nil
	})
	if name != "/custom/bin/code" || !reflect.DeepEqual(args, []string{"--goto", "/repo/main.go:42"}) {
		t.Fatalf("command = %q %v", name, args)
	}
}
