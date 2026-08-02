package main

import "net/http"

func handler(http.ResponseWriter, *http.Request) {}

type owner struct {
	mux *http.ServeMux
}

type adminHandler interface {
	ServeHTTP(http.ResponseWriter, *http.Request) error
}

type adminHandlerFunc func(http.ResponseWriter, *http.Request) error

func (fn adminHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	return fn(w, r)
}

func main() {
	current := owner{mux: http.NewServeMux()}
	register := func(path string, h http.Handler) {
		current.mux.Handle(path, h)
	}
	add := func(path string) {
		register(path, http.HandlerFunc(handler))
	}
	add("/nested")
	addAdmin := func(path string, h adminHandler) {
		wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = h.ServeHTTP(w, r)
		})
		register(path, wrapped)
	}
	addAdmin("/wrapped", adminHandlerFunc(func(http.ResponseWriter, *http.Request) error { return nil }))
}
