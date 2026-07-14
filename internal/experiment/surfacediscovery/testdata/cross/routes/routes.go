package routes

import "net/http"

func Add(mux *http.ServeMux, path string, handler http.HandlerFunc) {
	mux.HandleFunc(path, handler)
}
