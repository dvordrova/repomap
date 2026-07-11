package main

import (
	"log"
	"strings"
	"time"
)

func middleware(callback func()) func() { return callback }

func main() {
	value := strings.TrimSpace(" value ")
	callback := middleware(func() {})
	callback()
	log.Print(value, time.Second)
}
