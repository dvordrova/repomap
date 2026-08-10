package main

import "net/http"

type router struct{}

func (*router) Handle(path string, handler func(http.ResponseWriter, *http.Request)) {
	_, _ = path, handler
}

func RunInNetNS(path string, callback func() error) error {
	_ = path
	return callback()
}

func serve(http.ResponseWriter, *http.Request) {}

func main() {
	routes := &router{}
	routes.Handle("/exact", serve)
	_ = RunInNetNS("/not-a-route", func() error { return nil })
}
