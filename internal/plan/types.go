package plan

import (
	"github.com/ffsync/ffsync/internal/local"
	"github.com/ffsync/ffsync/internal/remote"
)

// Plan holds the result of planning a sync (upload, update, delete).
type Plan struct {
	Upload       []local.File
	Update       []UpdateAction   // upload new then delete old
	DeleteFiles  []remote.Object
	DeleteFolders []remote.Folder // deepest first
}

// UpdateAction is a local file to upload, replacing the given remote ID.
type UpdateAction struct {
	Local    local.File
	RemoteID string
}
