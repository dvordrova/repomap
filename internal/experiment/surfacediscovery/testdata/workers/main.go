package main

import "golang.org/x/sync/errgroup"

var jobs <-chan string

func process(string) {}

func runWorker() error {
	for job := range jobs {
		process(job)
	}
	return nil
}

func oneShot() error {
	process("once")
	return nil
}

func registerTasks(group *errgroup.Group) {
	group.Go(runWorker)
	group.Go(oneShot)
}

func main() {
	registerTasks(&errgroup.Group{})
}
