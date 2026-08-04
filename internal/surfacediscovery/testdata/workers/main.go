package main

import "golang.org/x/sync/errgroup"

var jobs <-chan string

func process(string) {}

type scanner struct{}

func (*scanner) Scan() error { return nil }

func runWorker() error {
	for job := range jobs {
		process(job)
	}
	return nil
}

func oneShot() error {
	for _, item := range []string{"once", "twice"} {
		process(item)
	}
	return nil
}

func registerTasks(group *errgroup.Group) {
	group.Go(runWorker)
	group.Go(oneShot)
	scan := &scanner{}
	group.Go(func() error { return scan.Scan() })
}

func main() {
	registerTasks(&errgroup.Group{})
}
