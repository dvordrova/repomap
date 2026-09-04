// Command repomap turns a repository into one static page that answers the
// first-day questions. Everything it does lives in internal/run; this file
// is the one the go tool builds into a binary.
package main

import "github.com/dvordrova/repomap/internal/run"

func main() {
	run.Main()
}
