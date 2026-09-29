//go:build fixturevfs

package cumulativegofixture

// OpenReplica exists only in the fixturevfs build, as litestream's vfs.go
// does: the vfs program is analysed with its tag, so it reaches this file.
func OpenReplica(url string) string { return "replica " + url }
