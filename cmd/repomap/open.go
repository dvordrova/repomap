package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func openReport(location string) error {
	target := location
	if !strings.HasPrefix(location, "http://") && !strings.HasPrefix(location, "https://") {
		absPath, err := filepath.Abs(location)
		if err != nil {
			return fmt.Errorf("resolve report path: %w", err)
		}
		target = absPath
	}
	name, args, err := reportOpenCommand(runtime.GOOS, target)
	if err != nil {
		return err
	}
	if output, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("run %s: %w: %s", name, err, output)
	}
	return nil
}

func reportOpenCommand(goos, path string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{path}, nil
	case "linux":
		return "xdg-open", []string{path}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", path}, nil
	default:
		return "", nil, fmt.Errorf("opening reports is not supported on %s", goos)
	}
}
