//go:build profile

package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

// A profiling build (go build -tags profile) writes CPU profiles of every
// minute of the run as $REPOMAP_CPUPROFILE.NNN (run.Main exits without
// returning, so a run-long profile would never be flushed; `go tool pprof`
// reads them together), and a heap profile to $REPOMAP_HEAPPROFILE on
// SIGUSR1. The ordinary build has neither.
func init() {
	if prefix := os.Getenv("REPOMAP_CPUPROFILE"); prefix != "" {
		go func() {
			for n := 1; ; n++ {
				file, err := os.Create(fmt.Sprintf("%s.%03d", prefix, n))
				if err != nil || pprof.StartCPUProfile(file) != nil {
					return
				}
				time.Sleep(time.Minute)
				pprof.StopCPUProfile()
				file.Close()
			}
		}()
	}
	if path := os.Getenv("REPOMAP_HEAPPROFILE"); path != "" {
		dump := make(chan os.Signal, 1)
		signal.Notify(dump, syscall.SIGUSR1)
		go func() {
			for range dump {
				runtime.GC()
				if file, err := os.Create(path); err == nil {
					pprof.WriteHeapProfile(file)
					file.Close()
				}
			}
		}()
	}
}
