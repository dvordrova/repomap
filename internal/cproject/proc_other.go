//go:build !unix

package cproject

import "os/exec"

// ownProcessGroup leaves the default cancellation, which stops make itself.
func ownProcessGroup(*exec.Cmd) {}
