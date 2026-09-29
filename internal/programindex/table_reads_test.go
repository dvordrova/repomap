package programindex

import "testing"

// A table read's shared form sits on a read of variables, one form per
// site, and a keys witness names another variable.
func TestTableReadFormsBelongToReadsOfVariables(t *testing.T) {
	input := shapeInput()
	input.Objects = append(input.Objects,
		ObjectInput{SourceRef: "args", Kind: ObjectVariable, Name: "ARGS", Visibility: VisibilityInternal},
		ObjectInput{SourceRef: "options", Kind: ObjectVariable, Name: "OPTIONS", Visibility: VisibilityInternal})
	read := func(witnesses ...Witness) RelationInput {
		return RelationInput{SourceRef: "read", Kind: RelationReads, FromRef: "caller", ToRefs: []string{"args"}, Resolution: ResolutionExact, Witnesses: witnesses}
	}
	for name, relation := range map[string]RelationInput{
		"keys":       read(Witness{Kind: "variable_read"}, Witness{Kind: WitnessKeys, ObjectRef: "options"}),
		"membership": read(Witness{Kind: WitnessMembership}),
	} {
		input.Relations = []RelationInput{relation}
		if _, err := newMeasuredProgramIndex(input); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, relation := range map[string]RelationInput{
		"keys of itself":      read(Witness{Kind: WitnessKeys, ObjectRef: "args"}),
		"keys of a function":  read(Witness{Kind: WitnessKeys, ObjectRef: "target-a"}),
		"keys naming nothing": read(Witness{Kind: WitnessKeys}),
		"both forms":          read(Witness{Kind: WitnessKeys, ObjectRef: "options"}, Witness{Kind: WitnessMembership}),
		"a call": {SourceRef: "call", Kind: RelationCalls, FromRef: "caller", ToRefs: []string{"target-a"}, Resolution: ResolutionExact,
			Witnesses: []Witness{{Kind: WitnessMembership}}},
	} {
		input.Relations = []RelationInput{relation}
		if _, err := newMeasuredProgramIndex(input); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}
