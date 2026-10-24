// Package xdg resolves glview directories following the XDG base
// directory spec on every OS (including macOS, where ~/.config is used
// instead of ~/Library/Application Support).
package xdg

import (
	"os"
	"path/filepath"
)

const app = "glview"

// ConfigDir returns $XDG_CONFIG_HOME/glview or ~/.config/glview.
func ConfigDir() string { return dir("XDG_CONFIG_HOME", ".config") }

// CacheDir returns $XDG_CACHE_HOME/glview or ~/.cache/glview.
func CacheDir() string { return dir("XDG_CACHE_HOME", ".cache") }

// StateDir returns $XDG_STATE_HOME/glview or ~/.local/state/glview.
func StateDir() string { return dir("XDG_STATE_HOME", filepath.Join(".local", "state")) }

func dir(env, fallback string) string {
	base := os.Getenv(env)
	if base == "" || !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, fallback)
	}
	return filepath.Join(base, app)
}
