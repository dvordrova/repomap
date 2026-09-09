package main

import (
	"example.com/repomap/cumulative-go-fixture/pkg/servicecfg"
	"net/http"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	_ = http.ListenAndServe(servicecfg.Address(), nil)
}
