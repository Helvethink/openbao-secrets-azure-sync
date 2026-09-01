package httpauth

import "testing"

func TestTokenMatches(t *testing.T) {
	tests := []struct {
		name     string
		provided string
		expected string
		want     bool
	}{
		{name: "matching", provided: "secret-token", expected: "secret-token", want: true},
		{name: "different value", provided: "other-token", expected: "secret-token", want: false},
		{name: "different length", provided: "short", expected: "secret-token", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := TokenMatches(test.provided, test.expected); got != test.want {
				t.Fatalf("TokenMatches() = %t, want %t", got, test.want)
			}
		})
	}
}
