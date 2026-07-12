package reportserver

import (
	"errors"
	"reflect"
	"testing"
)

func TestVSCodeCommandFallsBackToMacOSApplication(t *testing.T) {
	name, args := vsCodeCommand("darwin", "/repo/main.go:42", func(string) (string, error) {
		return "", errors.New("not found")
	})
	if name != "open" || !reflect.DeepEqual(args, []string{"-a", "Visual Studio Code", "--args", "--goto", "/repo/main.go:42"}) {
		t.Fatalf("command = %q %v", name, args)
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
