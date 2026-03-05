package remote

import (
	"testing"
)

func TestParseRemote(t *testing.T) {
	tests := []struct {
		spec      string
		wantName  string
		wantPath  string
		wantError bool
	}{
		{"remote:", "remote", "", false},
		{"remote:foo", "remote", "foo", false},
		{"remote:foo/bar", "remote", "foo/bar", false},
		{"remote:foo/bar/", "remote", "foo/bar", false},
		{"r:x", "r", "x", false},
		{"no-colon", "", "", true},
		{"", "", "", true},
	}
	for _, tt := range tests {
		name, path, err := ParseRemote(tt.spec)
		if (err != nil) != tt.wantError {
			t.Errorf("ParseRemote(%q) err = %v", tt.spec, err)
			continue
		}
		if !tt.wantError && (name != tt.wantName || path != tt.wantPath) {
			t.Errorf("ParseRemote(%q) = %q, %q; want %q, %q", tt.spec, name, path, tt.wantName, tt.wantPath)
		}
	}
}
