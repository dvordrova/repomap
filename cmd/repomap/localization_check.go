package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/report"
)

func runLocalizationCheckCLI(args []string, stdout io.Writer) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: repomap dev localization-check <run-dir>")
	}
	absDir, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("localization check: resolve run dir: %w", err)
	}
	artifactDir, err := report.MaterializeArchitectureLocalizationIdentity(absDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Architecture localization identity: %s\n", artifactDir)
	return nil
}
