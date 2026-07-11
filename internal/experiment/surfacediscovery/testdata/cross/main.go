package main

import (
	"net/http"

	"example.com/cross/routes"
)

func create(http.ResponseWriter, *http.Request) {}

func main() {
	mux := http.NewServeMux()
	routes.Add(mux, "/cross", create)
	_ = http.ListenAndServe(":8080", mux)
}
