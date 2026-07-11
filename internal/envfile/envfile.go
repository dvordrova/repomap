// Package envfile loads simple KEY=VALUE files without shell evaluation.
package envfile

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

func Load(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("envfile: open: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("envfile: set %q: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("envfile: scan: %w", err)
	}
	return nil
}
