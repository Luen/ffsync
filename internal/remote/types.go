package remote

// Object represents a file on the remote (FolderFort).
type Object struct {
	ID        string
	Path     string // normalised key with "/"
	Name     string
	Size     int64
	Mime     string
	Extension string
	UpdatedAt string // if API provides it
}

// Folder represents a folder on the remote.
type Folder struct {
	ID   string
	Path string
	Name string
}
