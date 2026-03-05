package local

// File represents a local file for planning.
type File struct {
	Path   string
	Size   int64
	Mtime  int64
	Hash   string
}
