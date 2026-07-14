package reportserver

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

var ErrEditorUnavailable = errors.New("VS Code launcher is unavailable")

func NewVSCodeLauncher(loggers ...func(string, ...any)) (OpenFileFunc, error) {
	name, prefix, err := resolveVSCodeCommand(runtime.GOOS, exec.LookPath)
	if err != nil {
		return nil, err
	}
	var logf func(string, ...any)
	if len(loggers) > 0 {
		logf = loggers[0]
	}
	mechanism := "code_cli"
	if filepath.Base(name) == "open" {
		mechanism = "macos_launch_services"
	}
	return editorLauncherWithLog(name, prefix, mechanism, logf), nil
}

func editorLauncher(name string, prefix []string) OpenFileFunc {
	return editorLauncherWithLog(name, prefix, "test_launcher", nil)
}

func editorLauncherWithLog(name string, prefix []string, mechanism string, logf func(string, ...any)) OpenFileFunc {
	return func(_ context.Context, absolutePath string, line, column int) error {
		target := absolutePath
		if line > 0 {
			target += ":" + strconv.Itoa(line)
			if column > 0 {
				target += ":" + strconv.Itoa(column)
			}
		}
		args := append(append([]string(nil), prefix...), target)
		cmd := exec.Command(name, args...)
		started := time.Now()
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start VS Code open command: %w", err)
		}
		if logf != nil {
			logf("editor dispatch mechanism=%s executable=%s editor_state=unknown spawn_ms=%d", mechanism, filepath.Base(name), time.Since(started).Milliseconds())
		}
		go func() {
			err := cmd.Wait()
			if logf == nil {
				return
			}
			outcome := "exited"
			if err != nil {
				outcome = "exit_error"
			}
			logf("editor dispatch mechanism=%s executable=%s outcome=%s elapsed_ms=%d", mechanism, filepath.Base(name), outcome, time.Since(started).Milliseconds())
		}()
		return nil
	}
}

func unavailableEditorLauncher(cause error) OpenFileFunc {
	return func(context.Context, string, int, int) error {
		return fmt.Errorf("%w: %v", ErrEditorUnavailable, cause)
	}
}

func resolveVSCodeCommand(goos string, lookPath func(string) (string, error)) (string, []string, error) {
	if codePath, err := lookPath("code"); err == nil {
		return codePath, []string{"--goto"}, nil
	}
	if goos == "darwin" {
		openPath, err := lookPath("open")
		if err == nil {
			return openPath, []string{"-a", "Visual Studio Code", "--args", "--goto"}, nil
		}
	}
	return "", nil, ErrEditorUnavailable
}

func vsCodeCommand(goos, target string, lookPath func(string) (string, error)) (string, []string) {
	name, args, _ := resolveVSCodeCommand(goos, lookPath)
	return name, append(args, target)
}
