package programindex

import (
	"reflect"
	"testing"
)

func TestTestSourcesPreserveNativeIdentityAndAllDeclarations(t *testing.T) {
	input := representativeInput()
	base, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Target.TestSources = []string{"checks/ready.py", "checks/another.py", "checks/ready.py"}
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatal(err)
	}
	if index.Target.ID != base.Target.ID || !reflect.DeepEqual(index.Objects, base.Objects) || !reflect.DeepEqual(index.Relations, base.Relations) {
		t.Fatal("testing metadata changed target identity or original declarations/relations")
	}
	if !reflect.DeepEqual(index.Target.TestSources, []string{"checks/another.py", "checks/ready.py"}) || input.Target.TestSources[0] != "checks/ready.py" {
		t.Fatal("test paths were not canonicalized without mutating input")
	}
	snapshot := index.Snapshot()
	snapshot.Target.TestSources[0] = "checks/changed.py"
	if index.Target.TestSources[0] != "checks/another.py" {
		t.Fatal("snapshot aliases testing metadata")
	}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("changed test metadata did not invalidate sealed index")
	}
	input.Target.TestSources = []string{"../outside.py"}
	if _, err := newMeasuredProgramIndex(input); err == nil {
		t.Fatal("invalid test path accepted")
	}
}
