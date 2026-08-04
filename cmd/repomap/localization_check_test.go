package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunLocalizationCheckCLIRequiresOneRunDirectory(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		nil,
		{"one", "two"},
		{"--run-dir"},
	} {
		var stdout bytes.Buffer
		err := runLocalizationCheckCLI(args, &stdout)
		if err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("runLocalizationCheckCLI(%q) error = %v, want usage", args, err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("runLocalizationCheckCLI(%q) stdout = %q", args, stdout.String())
		}
	}
}
