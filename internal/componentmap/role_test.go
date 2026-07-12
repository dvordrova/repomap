package componentmap

import (
	"encoding/json"
	"testing"
)

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  Role
		ok    bool
	}{
		{" Domain ", RoleDomain, true},
		{"BOUNDARY", RoleBoundary, true},
		{"", RoleUnknown, true},
		{"storage-ish", RoleUnknown, false},
	}
	for _, test := range tests {
		got, ok := Normalize(test.input)
		if got != test.want || ok != test.ok {
			t.Errorf("Normalize(%q) = %q, %v; want %q, %v", test.input, got, ok, test.want, test.ok)
		}
	}
}

func TestRoleUnmarshalKeepsMalformedOptionalValueRecoverable(t *testing.T) {
	t.Parallel()

	var role Role
	if err := json.Unmarshal([]byte(`{"unexpected":true}`), &role); err != nil {
		t.Fatal(err)
	}
	if normalized, ok := Normalize(string(role)); normalized != RoleUnknown || ok {
		t.Fatalf("Normalize(%q) = %q, %v; want unknown, false", role, normalized, ok)
	}
}
