package pathutil

import (
	"testing"
)

func TestNormalise(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"a/b/c", "a/b/c"},
		{"a\\b\\c", "a/b/c"},
		{"a/../b", "b"},
		{"./a", "a"},
		{"", "."},
	}
	for _, tt := range tests {
		got := Normalise(tt.in)
		if got != tt.want {
			t.Errorf("Normalise(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormaliseTrim(t *testing.T) {
	if got := NormaliseTrim("/a/b/"); got != "a/b" {
		t.Errorf("NormaliseTrim = %q", got)
	}
	if got := NormaliseTrim(""); got != "" {
		t.Errorf("NormaliseTrim empty = %q", got)
	}
}
