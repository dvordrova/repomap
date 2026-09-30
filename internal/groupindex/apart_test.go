package groupindex

import (
	"slices"
	"testing"
)

// Entries sharing a name are told apart by the first spelling that differs
// for each of them; the rest keep their names.
func TestTellApartQualifiesOnlyTheNamesAListShares(t *testing.T) {
	names := []string{"DB", "ReplicaClient", "ReplicaClient", "ReplicaClient", "run", "run", "handle", "handle"}
	spellings := [][]string{
		append([]string{""}, Where("DB", "db.go", "Databases")...),
		append([]string{""}, Where("ReplicaClient", "s3/replica_client.go", "Storage")...),
		append([]string{""}, Where("ReplicaClient", "gs/replica_client.go", "Storage")...),
		append([]string{""}, Where("ReplicaClient", "replica_client.go", "Storage")...),
		// Methods of two types, in one file.
		append([]string{"Worker.run"}, Where("run", "app/worker.py", "Workers")...),
		append([]string{"Loop.run"}, Where("run", "app/worker.py", "Workers")...),
		// One folder, one file: only their parts tell them apart.
		append([]string{""}, Where("handle", "src/app.c", "Commands")...),
		append([]string{""}, Where("handle", "src/app.c", "Events")...),
	}
	want := []string{"DB", "s3.ReplicaClient", "gs.ReplicaClient", "replica_client.ReplicaClient", "Worker.run", "Loop.run", "handle in Commands", "handle in Events"}
	if got := TellApart(names, spellings); !slices.Equal(got, want) {
		t.Fatalf("told apart = %q\nwant %q", got, want)
	}
	// Nothing tells two apart: they keep their names.
	if got := TellApart([]string{"f", "f"}, [][]string{Where("f", "a.go", ""), Where("f", "a.go", "")}); !slices.Equal(got, []string{"f", "f"}) {
		t.Fatalf("untold = %q", got)
	}
}
