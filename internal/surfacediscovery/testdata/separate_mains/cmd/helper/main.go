package main

import (
	"net/http"

	"example.com/separate-mains/shared"
)

func helper() {
	http.HandleFunc("/helper", func(http.ResponseWriter, *http.Request) {})
}

func main() {
	shared.Set(helper)
}
