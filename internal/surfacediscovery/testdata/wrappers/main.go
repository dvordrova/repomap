package main

import "net/http"

type API struct{}

func (API) CreateRun(http.ResponseWriter, *http.Request) {}

func auth(handler http.Handler) http.Handler {
	return handler
}

func addRoute(mux *http.ServeMux, path string, handler http.Handler) {
	mux.Handle(path, handler)
}

func registerAdmin(mux *http.ServeMux, path string, handler http.Handler) {
	addRoute(mux, path, handler)
}

func registerAPI(mux *http.ServeMux, prefix string, api API) {
	registerAdmin(mux, prefix+"/runs", auth(http.HandlerFunc(api.CreateRun)))
}

func makeMux() *http.ServeMux {
	return http.NewServeMux()
}

func main() {
	mux := makeMux()
	registerAPI(mux, "/v1", API{})
	_ = http.ListenAndServe(":8080", mux)
}
