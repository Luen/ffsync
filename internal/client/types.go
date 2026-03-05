package client

import "encoding/json"

// FlexID unmarshals from JSON number or string (BeDrive/FolderFort may return either).
type FlexID string

// UnmarshalJSON accepts either a number or a quoted string.
func (s *FlexID) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if data[0] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*s = FlexID(str)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*s = FlexID(n.String())
	return nil
}

// String returns the ID as a string.
func (s FlexID) String() string { return string(s) }

// LoginRequest is sent to POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

// FileEntryResponse is one entry from GET /api/v1/drive/file-entries.
type FileEntryResponse struct {
	ID        FlexID `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // "folder" or file
	Size      int64  `json:"size"`
	Mime      string `json:"mime"`
	Extension string `json:"extension"`
	UpdatedAt string `json:"updated_at"`
}

// FileEntriesResponse is the response from file-entries API.
type FileEntriesResponse struct {
	Data []FileEntryResponse `json:"data"`
}

// CreateFolderRequest is sent to POST /api/v1/folders.
type CreateFolderRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parentId"`
}

// CreateFolderResponse is the response from create folder.
type CreateFolderResponse struct {
	Folder struct {
		ID FlexID `json:"id"`
	} `json:"folder"`
}

// PresignRequest is sent to POST /api/v1/s3/simple/presign.
type PresignRequest struct {
	TotalFileCount int    `json:"totalFileCount"`
	Filename       string `json:"filename"`
	Mime           string `json:"mime"`
	Disk           string `json:"disk"`
	Size           int64  `json:"size"`
	Extension      string `json:"extension"`
	WorkspaceID    int    `json:"workspaceId"`
	ParentID       string `json:"parentId"`
	RelativePath   string `json:"relativePath"`
}

// PresignResponse is the response from presign.
type PresignResponse struct {
	URL string `json:"url"`
	Key string `json:"key"`
	ACL string `json:"acl"`
}

// CreateEntryRequest is sent to POST /api/v1/s3/entries.
type CreateEntryRequest struct {
	WorkspaceID     int    `json:"workspaceId"`
	ParentID       string `json:"parentId"`
	RelativePath   string `json:"relativePath"`
	Disk           string `json:"disk"`
	ClientMime     string `json:"clientMime"`
	ClientName     string `json:"clientName"`
	Filename       string `json:"filename"`
	Size           int64  `json:"size"`
	ClientExtension string `json:"clientExtension"`
}

// CreateEntryResponse contains the created file entry.
type CreateEntryResponse struct {
	FileEntry struct {
		ID FlexID `json:"id"`
	} `json:"fileEntry"`
}
