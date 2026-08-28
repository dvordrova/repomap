package main

import (
	"log"
	"net/http"

	"example.invalid/fulfilment/internal/bootstrap"
	"example.invalid/fulfilment/internal/httpapi"
)

func main() {
	runtime, err := bootstrap.AssembleFromEnvironment()
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: ":" + runtime.Port, Handler: httpapi.Handler(runtime.Checkout)}
	log.Printf("fulfilment service listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
