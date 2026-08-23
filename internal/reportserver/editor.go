package reportserver

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

var ErrEditorUnavailable = errors.New("VS Code launcher is unavailable")

func NewVSCodeLauncher(loggers ...func(string, ...any)) (OpenFileFunc, error) {
	name, prefix, err := resolveVSCodeCommand(exec.LookPath)
	if err != nil {
		return nil, err
	}
	var logf func(string, ...any)
	if len(loggers) > 0 {
		logf = loggers[0]
	}
	return editorLauncherWithLog(name, prefix, "code_cli", logf), nil
}

func editorLauncherWithLog(name string, prefix []string, mechanism string, logf func(string, ...any)) OpenFileFunc {
	return func(ctx context.Context, absolutePath string, line, column int) error {
		target := absolutePath
		if line > 0 {
			target += ":" + strconv.Itoa(line)
			if column > 0 {
				target += ":" + strconv.Itoa(column)
			}
		}
		args := append(append([]string(nil), prefix...), target)
		cmd := exec.CommandContext(ctx, name, args...)
		started := time.Now()
		if err := cmd.Run(); err != nil {
			if logf != nil {
				logf("editor dispatch mechanism=%s executable=%s outcome=exit_error elapsed_ms=%d", mechanism, filepath.Base(name), time.Since(started).Milliseconds())
			}
			return fmt.Errorf("VS Code open command failed: %w", err)
		}
		if logf != nil {
			logf("editor dispatch mechanism=%s executable=%s outcome=opened elapsed_ms=%d", mechanism, filepath.Base(name), time.Since(started).Milliseconds())
		}
		return nil
	}
}

func resolveVSCodeCommand(lookPath func(string) (string, error)) (string, []string, error) {
	if codePath, err := lookPath("code"); err == nil {
		return codePath, []string{"--goto"}, nil
	}
	return "", nil, ErrEditorUnavailable
}
