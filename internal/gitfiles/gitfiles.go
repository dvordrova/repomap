package gitfiles

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func List(repoPath string) ([]string, error) {
	if err := verifyGitRepo(repoPath); err != nil {
		return nil, err
	}

	cmd := safeCommand(repoPath, "ls-files", "-z")
	cmd.Env = isolatedEnvironment(os.Environ())
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
	cmd := safeCommand(repoPath, "rev-parse", "--is-inside-work-tree")
	cmd.Env = isolatedEnvironment(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%s does not appear to be a git repository: %w", repoPath, err)
	}
	if strings.TrimSpace(string(out)) != "true" {
		return fmt.Errorf("%s is not inside a git working tree", repoPath)
	}
	return nil
}

func safeCommand(repoPath string, args ...string) *exec.Cmd {
	commandArgs := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", repoPath,
	}
	commandArgs = append(commandArgs, args...)
	return exec.Command("git", commandArgs...)
}

// A caller's alternate-index/worktree variables must not override the
// repository path passed to this package. This is especially important for
// historical detached-worktree analysis, where a shared bare-repository HEAD
// would otherwise masquerade as the requested checkout.
func isolatedEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+4)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
			"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS",
			"GIT_CONFIG_SYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM",
			"GIT_EXTERNAL_DIFF", "GIT_PAGER", "PAGER":
			continue
		}
		if strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		result = append(result, entry)
	}
	return append(result,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
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
