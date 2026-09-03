package update_test

import (
	"testing"

	"github.com/nabrahma/lathe/internal/update"
)

func TestIsNewerComparesVersionsNumerically(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.1.0", "1.0.9", true},
		{"2.0.0", "1.9.9", true},
		// The lexical trap: "10" sorts before "9" as text.
		{"0.10.0", "0.9.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.0.1", false},
		{"v1.2.0", "1.1.0", true},
		{"1.2.0-rc.1", "1.1.0", true},

		// A development build must never be told it is out of date.
		{"1.0.0", "dev", false},
		{"nonsense", "1.0.0", false},
	}

	for _, tc := range cases {
		if got := update.IsNewer(tc.candidate, tc.current); got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.candidate, tc.current, got, tc.want)
		}
	}
}
