package contracttest

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func assertGoRuntimeRegistrations(t *testing.T, index programindex.Index) {
	t.Helper()
	const path = "internal/storefixture/runtime_registrations.go"
	worker := programIndexObjectNamed(t, index, programindex.ObjectFunction, "RunCommitWorker", path)
	startup := programIndexObjectNamed(t, index, programindex.ObjectFunction, "StartCommitWorker", path)
	retry := programIndexObjectNamed(t, index, programindex.ObjectFunction, "RetryCommit", path)
	commit := programIndexObjectNamed(t, index, programindex.ObjectFunction, "commitPendingBatches", path)
	var launched, scheduled, periodic, finite bool
	for _, relation := range index.Relations {
		if relation.FromID == startup.ID && relation.Invocation == "goroutine" {
			for _, id := range relation.ToIDs {
				launched = launched || id == worker.ID
			}
		}
		if relation.Kind == programindex.RelationPassesCallback && relation.SourceArgumentID != "" {
			for _, id := range relation.ToIDs {
				if id == commit.ID {
					for _, witness := range relation.Witnesses {
						scheduled = scheduled || strings.Contains(witness.Detail, "AfterFunc")
					}
				}
			}
		}
		for _, pattern := range relation.Patterns {
			if relation.FromID == worker.ID && pattern.Selector == "NewTicker" {
				periodic = true
			}
		}
		if relation.FromID == retry.ID {
			if relation.Invocation == "goroutine" {
				t.Fatal("finite retry became a launched goroutine")
			}
			for _, id := range relation.ToIDs {
				finite = finite || id == commit.ID
			}
		}
	}
	if !launched || !scheduled || !periodic || !finite {
		t.Fatalf("runtime observations: goroutine=%v scheduled=%v ticker=%v finite=%v", launched, scheduled, periodic, finite)
	}
}
