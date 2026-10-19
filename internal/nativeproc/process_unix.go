//go:build unix

package nativeproc

import (
	"os/exec"
	"syscall"
)

// OwnProcessGroup puts the command in its own process group and makes
// cancellation kill the whole group, so a stop reaches the shells and
// children a tool started; otherwise a child holding the pipes keeps Wait
// waiting after the tool itself is dead.
func OwnProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
