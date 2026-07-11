package main

import "net/http"

type Router interface {
	Route(*http.ServeMux)
}

type firstRouter struct{}
type secondRouter struct{}

func (firstRouter) Route(mux *http.ServeMux) {
	mux.HandleFunc("/first", first)
}

func (secondRouter) Route(mux *http.ServeMux) {
	mux.HandleFunc("/second", second)
}

func first(http.ResponseWriter, *http.Request)  {}
func second(http.ResponseWriter, *http.Request) {}

func install(router Router, mux *http.ServeMux) {
	router.Route(mux)
}

func main() {
	mux := http.NewServeMux()
	install(firstRouter{}, mux)
	install(secondRouter{}, mux)
	_ = http.ListenAndServe(":8080", mux)
}
