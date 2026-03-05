package cli

import (
	"log/slog"
	"os"
)

// Global flags (set by main or commands).
var (
	configPath string
	verbose    bool
)

// RootFlags returns common flags for the root command (config path, verbose).
func RootFlags(config *string, verboseFlag *bool) {
	*config = os.Getenv("FOLDERFORT_CONFIG")
	if *config == "" {
		*config = ""
		// will be resolved in config.Load
	}
	*verboseFlag = false
}

// SetVerbose sets slog level to debug when true.
func SetVerbose(v bool) {
	verbose = v
	if v {
		slog.SetDefault(slog.Default().With("verbose", true))
	}
}
