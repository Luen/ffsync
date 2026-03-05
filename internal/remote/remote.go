package remote

import (
	"errors"
	"strings"

	"github.com/ffsync/ffsync/pkg/pathutil"
)

// ErrInvalidRemote is returned when the remote spec is not "remote:path".
var ErrInvalidRemote = errors.New("invalid remote: expected remote:path")

// ParseRemote parses a spec like "remote:" or "remote:foo/bar" into remote name and path.
// Path is normalised (forward slashes, trimmed).
func ParseRemote(spec string) (name, path string, err error) {
	i := strings.Index(spec, ":")
	if i < 0 {
		return "", "", ErrInvalidRemote
	}
	name = strings.TrimSpace(spec[:i])
	path = pathutil.NormaliseTrim(spec[i+1:])
	if path == "." {
		path = ""
	}
	if name == "" {
		return "", "", ErrInvalidRemote
	}
	return name, path, nil
}
