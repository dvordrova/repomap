package main

import "net/http"

func handler(http.ResponseWriter, *http.Request) {}

func main() {
	http.HandleFunc("/default", handler)
	_ = http.ListenAndServe(":8080", nil)
}
