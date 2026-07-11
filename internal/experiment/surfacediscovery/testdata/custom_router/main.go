package main

import "net/http"

type router struct {
	handlers map[string]http.HandlerFunc
}

func (r *router) Register(path string, handler http.HandlerFunc) {
	r.handlers[path] = handler
}

func (r *router) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	r.handlers[request.URL.Path](writer, request)
}

func handler(http.ResponseWriter, *http.Request) {}

func main() {
	router := &router{handlers: map[string]http.HandlerFunc{}}
	router.Register("/custom", handler)
	_ = http.ListenAndServe(":8080", router)
}
