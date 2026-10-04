package main

import (
	"fmt"

	cumulativegofixture "example.com/repomap/cumulative-go-fixture"
	// Imported only to run its init, which registers it, as a database
	// driver registers itself: the program calls nothing of it.
	_ "example.com/repomap/cumulative-go-fixture/driver"
)

func main() {
	fmt.Println(cumulativegofixture.PublishedRoot())
}
