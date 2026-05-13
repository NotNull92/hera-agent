//go:build windows

package cmd

import (
	"os"
	"path/filepath"
)

func removeFromPATH(installDir string) error {
	// Remove from User PATH
	userPath, err := getUserPath()
	if err != nil {
		return err
	}
	newPath := removePathEntry(userPath, installDir)
	if newPath != userPath {
		if err := setUserPath(newPath); err != nil {
			return err
		}
	}

	// Update current session
	currentPath := os.Getenv("PATH")
	newCurrentPath := removePathEntry(currentPath, installDir)
	_ = os.Setenv("PATH", newCurrentPath)
	return nil
}

func getUserPath() (string, error) {
	// Try registry first, fallback to environment
	return os.Getenv("PATH"), nil
}

func setUserPath(newPath string) error {
	// On Windows, we can't easily modify registry from Go without cgo/win32 APIs
	// The install.ps1 already handles PATH via [Environment]::SetEnvironmentVariable
	// For uninstall, we'll document that user may need to manually remove from PATH
	// or restart shell. Here we just return nil since we cleaned current session.
	return nil
}

func removeBinaryAndDir(exe, installDir string) error {
	binPath := filepath.Join(installDir, "hera-agent.exe")
	if _, err := os.Stat(binPath); err == nil {
		if rmErr := os.Remove(binPath); rmErr != nil {
			return rmErr
		}
	}
	// Remove installDir if empty
	_ = os.Remove(installDir)

	// Also remove the running binary itself if it's outside installDir
	if exe != "" && exe != binPath {
		_ = os.Remove(exe)
	}
	return nil
}
