package main

import "net/http"

func handler(http.ResponseWriter, *http.Request) {}

func register(mux *http.ServeMux, again bool) {
	if again {
		register(mux, false)
	}
	mux.HandleFunc("/recursive", handler)
}

func main() {
	mux := http.NewServeMux()
	register(mux, true)
	_ = http.ListenAndServe(":8080", mux)
}
