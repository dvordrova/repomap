package main

import (
	"fmt"
	"net/http"
	"strings"
)

// serveStatus answers the worker's status page. A HEAD request asks for the
// headers alone and X-Verbose for every pending job: words only this
// handler compares with the request it was handed, sub-arguments of its
// route. What it writes to w is its answer, no word it checks.
func serveStatus(w http.ResponseWriter, r *http.Request) {
	if strings.EqualFold(r.Method, "HEAD") {
		return
	}
	verbose := r.Header.Get("X-Verbose") != ""
	fmt.Fprintf(w, "pending jobs (verbose %t)\n", verbose)
}

func init() { http.HandleFunc("/status", serveStatus) }
