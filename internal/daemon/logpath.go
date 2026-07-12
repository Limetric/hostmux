package daemon

import (
	"os"
	"path/filepath"
)

// LogFileName is the basename of the detached daemon's combined stdout/stderr
// log inside the hostmux home directory.
const LogFileName = "hostmux.log"

// LogPath returns the path of the detached daemon's log file
// (~/.hostmux/hostmux.log), where SpawnDetached appends the daemon's
// stdout/stderr. It does not create the file.
func LogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".hostmux", LogFileName), nil
}
