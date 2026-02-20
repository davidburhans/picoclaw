package utils

import (
	"os"
	"path/filepath"
)

// ExpandHome replaces "~" at the start of a path with the current user's home directory.
// It includes specific logic to handle Docker environments where HOME might be
// incorrectly set to /root even when running as a non-root user (like picoclaw).
func ExpandHome(path string) string {
	if path == "" {
		return path
	}

	if path[0] == '~' {
		home := os.Getenv("HOME")
		if home == "" {
			home, _ = os.UserHomeDir()
		}

		// Docker safety: many systems map volumes to /home/picoclaw but the environment
		// might report HOME=/root if running as root or if it's a generic Alpine image.
		if home == "/root" || home == "" {
			// Check if /home/picoclaw exists (priority for Docker mapped volumes)
			if _, err := os.Stat("/home/picoclaw"); err == nil {
				home = "/home/picoclaw"
			} else {
				// Fallback: check if we can see /home/picoclaw/.picoclaw
				if _, err := os.Stat("/home/picoclaw/.picoclaw"); err == nil {
					home = "/home/picoclaw"
				}
			}
		}

		if home == "" {
			return path // fallback to literal
		}

		if len(path) > 1 && (path[1] == '/' || path[1] == '\\') {
			return filepath.Join(home, path[2:])
		}
		return home
	}
	return path
}
