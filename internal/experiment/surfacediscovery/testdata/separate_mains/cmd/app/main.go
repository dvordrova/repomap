package main

import (
	"net/http"

	"example.com/separate-mains/shared"
)

func primary() {
	http.HandleFunc("/primary", func(http.ResponseWriter, *http.Request) {})
}

func main() {
	primary()
	shared.Set(primary)
	shared.Run()
}
