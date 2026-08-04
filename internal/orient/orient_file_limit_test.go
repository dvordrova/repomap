package orient

import "testing"

func TestOrientationFileLimitUsesCompleteInputUnlessExplicitlyCapped(t *testing.T) {
	tests := []struct {
		name          string
		explicitLimit int
		inputCount    int
		want          int
	}{
		{name: "ordinary complete catalog", inputCount: 834, want: 834},
		{name: "explicit debug cap", explicitLimit: 250, inputCount: 834, want: 250},
		{name: "empty input remains valid", want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := orientationFileLimit(test.explicitLimit, test.inputCount); got != test.want {
				t.Fatalf("orientationFileLimit(%d, %d) = %d, want %d", test.explicitLimit, test.inputCount, got, test.want)
			}
		})
	}
}
