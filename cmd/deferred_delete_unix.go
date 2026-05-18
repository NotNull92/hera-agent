//go:build !windows

package cmd

import "os"

// scheduleDelete removes the file immediately. Unix does not lock running
// executables, so deferred deletion is unnecessary.
func scheduleDelete(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}
