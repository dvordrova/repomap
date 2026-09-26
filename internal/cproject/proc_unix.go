//go:build unix

package cproject

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup lets a timeout stop make together with the shells and
// $(shell ...) commands it started.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
