//go:build !darwin && !linux

package jstsproject

// physicalMemory is unknown here: the helper keeps V8's own heap default.
func physicalMemory() uint64 { return 0 }
