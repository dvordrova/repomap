package main

import "net/http"

func healthHandler(http.ResponseWriter, *http.Request) {}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	_ = http.ListenAndServe(":8080", mux)
}
