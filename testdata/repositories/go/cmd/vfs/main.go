//go:build fixturevfs

// Command vfs builds only with the fixturevfs tag, as the Makefile's vfs
// recipe builds it: without the tag this directory holds no program.
package main

import (
	"fmt"
	"os"

	cumulativegofixture "example.com/repomap/cumulative-go-fixture"
)

func main() {
	fmt.Println(cumulativegofixture.OpenReplica(os.Getenv("FIXTURE_VFS_REPLICA_URL")))
}
