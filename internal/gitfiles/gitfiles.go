package gitfiles

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func List(repoPath string) ([]string, error) {
	if err := verifyGitRepo(repoPath); err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "-C", repoPath, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git ls-files failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git ls-files failed: %w", err)
	}

	return splitNull(out), nil
}

func verifyGitRepo(repoPath string) error {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%s does not appear to be a git repository: %w", repoPath, err)
	}
	if strings.TrimSpace(string(out)) != "true" {
		return fmt.Errorf("%s is not inside a git working tree", repoPath)
	}
	return nil
}

func splitNull(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	if data[len(data)-1] == 0 {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		return nil
	}
	parts := bytes.Split(data, []byte{0})
	files := make([]string, len(parts))
	for i, p := range parts {
		files[i] = string(p)
	}
	return files
}
