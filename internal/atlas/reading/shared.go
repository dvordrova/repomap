package reading

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// readerShared is what concurrent work of one reading shares behind one
// lock: the knowledge maps and the request-keyed response caches, whose
// keys never collide across stages, the loads in progress and the version
// of the knowledge records, which grows with every record written.
type readerShared struct {
	mu               sync.Mutex
	loading          map[string]chan struct{}
	knowledgeVersion int
}

// lock takes the shared lock and returns its release. The first call, made
// before any concurrent work starts, creates the shared state.
func (r *reader) lock() func() {
	if r.shared == nil {
		r.shared = &readerShared{}
	}
	r.shared.mu.Lock()
	return r.shared.mu.Unlock
}

// loadOnce returns what a request-keyed cache holds for key, loading it at
// most once: another goroutine asking for the same key waits for that load.
// A cache holds the same value whichever goroutine loaded it.
func loadOnce[V any](r *reader, cache *map[string]V, space, key string, load func() V) V {
	pending := space + "\x00" + key
	for {
		unlock := r.lock()
		if value, ok := (*cache)[key]; ok {
			unlock()
			return value
		}
		if wait, ok := r.shared.loading[pending]; ok {
			unlock()
			<-wait
			continue
		}
		if r.shared.loading == nil {
			r.shared.loading = make(map[string]chan struct{})
		}
		done := make(chan struct{})
		r.shared.loading[pending] = done
		unlock()
		value := load()
		unlock = r.lock()
		if *cache == nil {
			*cache = make(map[string]V)
		}
		(*cache)[key] = value
		delete(r.shared.loading, pending)
		unlock()
		close(done)
		return value
	}
}

// eachIndex runs work for every index below n on as many goroutines as
// there are processors, and returns when all are done.
func eachIndex(n int, work func(int)) {
	workers := min(runtime.GOMAXPROCS(0), n)
	if workers <= 1 {
		for i := 0; i < n; i++ {
			work(i)
		}
		return
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				work(i)
			}
		}()
	}
	wg.Wait()
}
