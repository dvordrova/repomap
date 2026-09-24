package run

// background is one stage computed beside the caller because nothing it reads
// is produced by the work it overlaps. wait blocks until the stage finished
// and returns the same result to every caller; the stage's own artifacts and
// console lines stay where its serial predecessor wrote them.
type background[T any] struct {
	done  chan struct{}
	value T
	err   error
}

func startBackground[T any](compute func() (T, error)) *background[T] {
	work := &background[T]{done: make(chan struct{})}
	go func() {
		defer close(work.done)
		work.value, work.err = compute()
	}()
	return work
}

func (work *background[T]) wait() (T, error) {
	<-work.done
	return work.value, work.err
}
