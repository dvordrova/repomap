//go:build darwin

package jstsproject

import (
	"encoding/binary"
	"syscall"
)

// physicalMemory is hw.memsize, a little-endian uint64 that syscall.Sysctl
// returns as bytes with a trailing zero byte trimmed; 0 when unknown.
func physicalMemory() uint64 {
	value, err := syscall.Sysctl("hw.memsize")
	if err != nil || len(value) == 0 || len(value) > 8 {
		return 0
	}
	var raw [8]byte
	copy(raw[:], value)
	return binary.LittleEndian.Uint64(raw[:])
}
