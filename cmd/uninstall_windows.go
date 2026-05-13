//go:build windows

package cmd

import (
	"os"
	"path/filepath"
)

func removeFromPATH(installDir string) error {
	// installDir is %LOCALAPPDATA%\Microsoft\WindowsApps — a Windows-managed
	// directory that's on the default user PATH. Removing it would break every
	// Store-app alias on the system. Leave it alone.
	//
	// Only legacy hera-agent PATH entries (from pre-WindowsApps installs)
	// need cleanup.
	legacyDir, _ := legacyInstallPaths()
	if legacyDir == "" {
		return nil
	}
	script := `
$legacy = $args[0]
$norm = [System.IO.Path]::GetFullPath($legacy).TrimEnd('\')
$p = [Environment]::GetEnvironmentVariable("Path", "User")
$entries = if ($p) { $p -split ';' } else { @() }
$filtered = $entries | Where-Object {
    if (-not $_) { return $false }
    try { return ([System.IO.Path]::GetFullPath($_).TrimEnd('\')) -ne $norm }
    catch { return $true }
}
$new = $filtered -join ';'
if ($new -ne $p) { [Environment]::SetEnvironmentVariable("Path", $new, "User") }
`
	if err := runPowerShellWithArgs(script, legacyDir); err != nil {
		return err
	}

	// Update current session for good measure
	currentPath := os.Getenv("PATH")
	newCurrentPath := removePathEntry(currentPath, legacyDir)
	_ = os.Setenv("PATH", newCurrentPath)
	return nil
}

func removeBinaryAndDir(exe, installDir string) error {
	binPath := filepath.Join(installDir, "hera-agent.exe")
	if _, err := os.Stat(binPath); err == nil {
		if rmErr := os.Remove(binPath); rmErr != nil {
			return rmErr
		}
	}
	// installDir is WindowsApps (a shared OS directory) — never remove it.
	// We only own the binary inside.

	// Also clean up the legacy install location (%LOCALAPPDATA%\hera-agent)
	// in case the user migrated and we never cleaned it up.
	legacyDir, legacyBin := legacyInstallPaths()
	if legacyBin != "" {
		if _, err := os.Stat(legacyBin); err == nil && legacyBin != exe {
			_ = os.Remove(legacyBin)
		}
		_ = os.Remove(legacyDir)
	}

	// Also remove the running binary itself if it's outside installDir
	if exe != "" && exe != binPath {
		_ = os.Remove(exe)
	}
	return nil
}
