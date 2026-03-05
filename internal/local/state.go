package local

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const stateVersion = 1

// State holds cached file state (size, mtime, hash).
type State struct {
	Version int                `json:"version"`
	Files   map[string]FileState `json:"files"`
	Folders map[string]string   `json:"folders,omitempty"`
}

// FileState is one entry in the state cache.
type FileState struct {
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
	Hash  string `json:"hash,omitempty"`
}

// LoadState reads state from path. Missing file returns empty state.
func LoadState(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Version: stateVersion, Files: make(map[string]FileState), Folders: make(map[string]string)}, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.Files == nil {
		s.Files = make(map[string]FileState)
	}
	if s.Folders == nil {
		s.Folders = make(map[string]string)
	}
	return &s, nil
}

// SaveState writes state atomically (temp file + rename).
func SaveState(path string, s *State) error {
	if s == nil {
		s = &State{Version: stateVersion, Files: make(map[string]FileState), Folders: make(map[string]string)}
	}
	s.Version = stateVersion
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".ffsync.state.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
