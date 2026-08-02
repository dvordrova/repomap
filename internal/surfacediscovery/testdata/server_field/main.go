package main

import "net/http"

func handler(http.ResponseWriter, *http.Request) {}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/server", handler)
	server := &http.Server{Addr: ":8080", Handler: mux}
	_ = server.ListenAndServe()
}
