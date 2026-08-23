package main

import "testing"

func TestAutomaticGoTargetAuthorityHonorsExplicitCLIAndEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		env      map[string]string
		want     bool
	}{
		{name: "host default", want: true},
		{name: "explicit flag", explicit: "darwin/amd64"},
		{name: "GOOS environment", env: map[string]string{"GOOS": "darwin"}},
		{name: "GOARCH environment", env: map[string]string{"GOARCH": "amd64"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := automaticGoTargetAllowed(test.explicit, func(name string) string {
				return test.env[name]
			})
			if got != test.want {
				t.Fatalf("automaticGoTargetAllowed() = %t, want %t", got, test.want)
			}
		})
	}
}
