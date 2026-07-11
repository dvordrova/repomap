package main

import "net/http"

type Router interface {
	Route(*http.ServeMux)
}

type onlyRouter struct{}

func (onlyRouter) Route(mux *http.ServeMux) {
	mux.HandleFunc("/one", one)
}

func one(http.ResponseWriter, *http.Request) {}

func install(router Router, mux *http.ServeMux) {
	router.Route(mux)
}

func main() {
	mux := http.NewServeMux()
	install(onlyRouter{}, mux)
	_ = http.ListenAndServe(":8080", mux)
}
