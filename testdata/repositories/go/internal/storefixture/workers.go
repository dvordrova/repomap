package storefixture

// Workers built and run the way freqtrade's trade command runs its Worker,
// beside Python's workers.py. Go runs no constructor: NewWorker is an
// ordinary call, and the struct literal it returns is no call. A call on its
// result is the method of the result's type, one promoted from an embedded
// struct included.

type baseWorker struct{ name string }

func (w *baseWorker) Run() string  { return w.name }
func (w *baseWorker) Step() string { return w.name }

type Worker struct {
	baseWorker
	retries int
}

func NewWorker(name string, retries int) *Worker {
	return &Worker{baseWorker: baseWorker{name: name}, retries: retries}
}

func (w *Worker) Step() string { return w.name + w.name }

func StartWorker(name string) int {
	worker := NewWorker(name, 2)
	worker.Run()
	worker.Step()
	return 0
}
