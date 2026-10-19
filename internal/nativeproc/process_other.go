//go:build !unix

package nativeproc

import "os/exec"

// OwnProcessGroup leaves the default cancellation, which stops the command
// itself.
func OwnProcessGroup(*exec.Cmd) {}
