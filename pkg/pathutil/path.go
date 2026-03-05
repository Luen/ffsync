package pathutil

import (
	"path"
	"path/filepath"
	"strings"
)

// Normalise returns a path with forward slashes and cleaned (no . or ..).
// Use for internal keys so local and remote paths are comparable.
func Normalise(p string) string {
	return path.Clean(filepath.ToSlash(p))
}

// NormaliseTrim trims leading/trailing slashes and normalises.
// Empty or "." returns "".
func NormaliseTrim(p string) string {
	s := strings.Trim(Normalise(p), "/")
	if s == "." {
		return ""
	}
	return s
}

// MatchGlob returns true if path should be included: no exclude matches, and
// if include is non-empty at least one include pattern matches.
// Patterns use forward slashes; path should be normalised.
func MatchGlob(include, exclude []string, pathStr string) bool {
	pathStr = Normalise(pathStr)
	osPath := filepath.FromSlash(pathStr)
	for _, ex := range exclude {
		pat := filepath.FromSlash(ex)
		ok, _ := filepath.Match(pat, osPath)
		if ok {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, in := range include {
		pat := filepath.FromSlash(in)
		ok, _ := filepath.Match(pat, osPath)
		if ok {
			return true
		}
	}
	return false
}
