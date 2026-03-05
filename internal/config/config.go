package config

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultBaseURL is the default FolderFort server.
const DefaultBaseURL = "https://na.folderfort.com"

// KnownBaseURLs are alternative FolderFort server locations.
var KnownBaseURLs = []string{
	DefaultBaseURL,
	"https://na2.folderfort.com",
	"https://na3.folderfort.com",
	"https://eu.folderfort.com",
	"https://eu2.folderfort.com",
}

// Config holds ffsync configuration (file + env overrides).
type Config struct {
	BaseURL   string
	Email     string
	Password  string
	ConfigPath string
	StateDir  string
}

// Load reads config from file and applies env overrides.
// Config file: FOLDERFORT_CONFIG or ~/.config/ffsync/ffsync.conf or ./.ffsync.
// Env: FOLDERFORT_BASE_URL, FOLDERFORT_EMAIL, FOLDERFORT_PASSWORD.
func Load() (*Config, error) {
	cfg := &Config{
		BaseURL: DefaultBaseURL,
	}
	configPath := os.Getenv("FOLDERFORT_CONFIG")
	if configPath == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			configPath = filepath.Join(dir, "ffsync", "ffsync.conf")
		}
	}
	if configPath == "" {
		configPath = ".ffsync"
	}
	cfg.ConfigPath = configPath
	if stateDir := os.Getenv("FOLDERFORT_STATE_DIR"); stateDir != "" {
		cfg.StateDir = stateDir
	} else {
		cfg.StateDir = filepath.Dir(configPath)
	}

	// File overrides
	if b, err := os.ReadFile(configPath); err == nil {
		applyKeyValue(b, cfg)
	}

	// Env overrides (never log these)
	if v := os.Getenv("FOLDERFORT_BASE_URL"); v != "" {
		cfg.BaseURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("FOLDERFORT_EMAIL"); v != "" {
		cfg.Email = v
	}
	if v := os.Getenv("FOLDERFORT_PASSWORD"); v != "" {
		cfg.Password = v
	}
	return cfg, nil
}

// applyKeyValue parses simple key=value lines and sets Config.
func applyKeyValue(b []byte, cfg *Config) {
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "=")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		val = strings.Trim(val, "\"")
		switch key {
		case "base_url":
			if val != "" {
				cfg.BaseURL = strings.TrimRight(val, "/")
			}
		case "email":
			cfg.Email = val
		case "password":
			cfg.Password = val
		}
	}
}

// Save writes the config to ConfigPath (creates dir if needed).
// Passwords are written; caller must ensure file permissions.
func (c *Config) Save() error {
	dir := filepath.Dir(c.ConfigPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	content := "# ffsync config\n"
	content += "base_url = \"" + c.BaseURL + "\"\n"
	content += "email = \"" + c.Email + "\"\n"
	content += "password = \"" + c.Password + "\"\n"
	return os.WriteFile(c.ConfigPath, []byte(content), 0600)
}

// Redacted returns a copy suitable for logging (password/email redacted).
func (c *Config) Redacted() *Config {
	r := *c
	if r.Email != "" {
		r.Email = "[redacted]"
	}
	if r.Password != "" {
		r.Password = "[redacted]"
	}
	return &r
}
