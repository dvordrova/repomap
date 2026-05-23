package gitfiles

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func List(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoPath, "ls-files")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git ls-files failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git ls-files failed: %w", err)
	}

	lines := bytes.Split(out, []byte{'\n'})
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		s := strings.TrimSpace(string(line))
		if s == "" {
			continue
		}
		files = append(files, s)
	}
	return files, nil
}
