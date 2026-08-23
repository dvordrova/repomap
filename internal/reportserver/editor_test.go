package reportserver

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestResolveVSCodeCommandRequiresInstalledCLI(t *testing.T) {
	requested := ""
	_, _, err := resolveVSCodeCommand(func(name string) (string, error) {
		requested = name
		return "", errors.New("not found")
	})
	if requested != "code" || !errors.Is(err, ErrEditorUnavailable) {
		t.Fatalf("requested=%q error=%v", requested, err)
	}
}

func TestEditorLauncherWaitsForSuccessAndPassesExactTarget(t *testing.T) {
	if os.Getenv("REPOMAP_EDITOR_HELPER") == "1" {
		if err := os.WriteFile(os.Getenv("REPOMAP_EDITOR_RECORD"), []byte(strings.Join(os.Args[1:], "\n")), 0o600); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}

	record := t.TempDir() + "/args"
	t.Setenv("REPOMAP_EDITOR_HELPER", "1")
	t.Setenv("REPOMAP_EDITOR_RECORD", record)
	launcher := editorLauncherWithLog(
		os.Args[0],
		[]string{"-test.run=TestEditorLauncherWaitsForSuccessAndPassesExactTarget", "--", "--goto"},
		"test_launcher",
		nil,
	)
	if err := launcher(context.Background(), "/repo/main.go", 42, 7); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "--goto\n/repo/main.go:42:7") {
		t.Fatalf("helper arguments = %q", data)
	}
}

func TestEditorLauncherReportsDispatchFailure(t *testing.T) {
	launcher := editorLauncherWithLog(
		"/repomap-test/missing-code-cli", []string{"--goto"}, "test_launcher", nil,
	)
	if err := launcher(context.Background(), "/repo/main.go", 42, 7); err == nil {
		t.Fatal("launcher reported success for a command that could not start")
	}
}

func TestResolveVSCodeCommandUsesInstalledCLI(t *testing.T) {
	name, args, err := resolveVSCodeCommand(func(string) (string, error) {
		return "/custom/bin/code", nil
	})
	if err != nil || name != "/custom/bin/code" || !reflect.DeepEqual(args, []string{"--goto"}) {
		t.Fatalf("command = %q %v error=%v", name, args, err)
	}
}
