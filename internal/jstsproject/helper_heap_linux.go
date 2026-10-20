//go:build linux

package jstsproject

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// physicalMemory is the machine's RAM, or the cgroup's memory limit when it
// is lower (a container), as Node's own default heeds it; 0 when unknown.
func physicalMemory() uint64 {
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return 0
	}
	memory := uint64(info.Totalram) * uint64(info.Unit)
	for _, path := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		limit, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
		if err == nil && limit > 0 && limit < memory {
			memory = limit
		}
	}
	return memory
}
