package reportserver

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
)

func OpenInVSCode(ctx context.Context, absolutePath string, line int) error {
	target := absolutePath
	if line > 0 {
		target += ":" + strconv.Itoa(line)
	}
	name, args := vsCodeCommand(runtime.GOOS, target, exec.LookPath)
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("run VS Code open command: %w: %s", err, output)
	}
	return nil
}

func vsCodeCommand(goos, target string, lookPath func(string) (string, error)) (string, []string) {
	if codePath, err := lookPath("code"); err == nil {
		return codePath, []string{"--goto", target}
	}
	if goos == "darwin" {
		// LaunchServices can find a registered VS Code app even when the user
		// has not installed the optional `code` shell command.
		return "open", []string{"-a", "Visual Studio Code", "--args", "--goto", target}
	}
	return "code", []string{"--goto", target}
}
