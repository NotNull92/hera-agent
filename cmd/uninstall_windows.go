//go:build windows

package cmd

import (
	"os"
	"os/exec"
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
	// Verify the legacy directory actually exists before trying to clean PATH.
	if _, err := os.Stat(legacyDir); os.IsNotExist(err) {
		return nil
	}
	// Use a single-line script so -Command treats the entire expression as one
	// statement. Multi-line strings cause PowerShell to parse subsequent lines as
	// separate commands, producing "GetFullPath" and "CommandNotFound" errors.
	script := `$legacy = $args[0]; if (-not $legacy) { exit 0 }; $norm = [System.IO.Path]::GetFullPath($legacy).TrimEnd('\'); $p = [Environment]::GetEnvironmentVariable("Path", "User"); $entries = if ($p) { $p -split ';' } else { @() }; $filtered = $entries | Where-Object { if (-not $_) { return $false }; try { return ([System.IO.Path]::GetFullPath($_).TrimEnd('\')) -ne $norm } catch { return $true } }; $new = $filtered -join ';'; if ($new -ne $p) { [Environment]::SetEnvironmentVariable("Path", $new, "User") }`
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

	// Try to remove the binary in installDir (WindowsApps).
	// If it's the currently running executable, Windows locks it.
	// Use a deferred deletion via cmd.exe so the file disappears after we exit.
	if _, err := os.Stat(binPath); err == nil {
		if rmErr := os.Remove(binPath); rmErr != nil {
			// Self-deletion on Windows fails with "Access is denied".
			// Schedule a delayed delete that runs after this process terminates.
			quoted := `"` + binPath + `"`
			cmd := exec.Command("cmd", "/c", "timeout /t 1 >nul && del /f "+quoted)
			_ = cmd.Start()
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

	// Also remove the running binary itself if it's outside installDir.
	// Again, use deferred deletion if direct removal fails.
	if exe != "" && exe != binPath {
		if rmErr := os.Remove(exe); rmErr != nil {
			quoted := `"` + exe + `"`
			cmd := exec.Command("cmd", "/c", "timeout /t 1 >nul && del /f "+quoted)
			_ = cmd.Start()
		}
	}
	return nil
}
