package main

import "net/http"

type spec struct {
	Path string
	Name string
}

func loadSpecs() []spec { return nil }

func main() {
	mux := http.NewServeMux()
	handlers := map[string]http.Handler{}
	for _, item := range loadSpecs() {
		mux.Handle(item.Path, handlers[item.Name])
	}
	_ = http.ListenAndServe(":8080", mux)
}
