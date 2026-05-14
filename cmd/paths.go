package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// getInstallPaths returns the canonical install directory and binary path
// used by the install.ps1 / install.sh bootstrap scripts. uninstall reads
// these to know where to delete from.
func getInstallPaths() (dir, bin string) {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		// WindowsApps is on the default user PATH in Windows 10+,
		// so install.ps1 places the binary here.
		dir = filepath.Join(home, "AppData", "Local", "Microsoft", "WindowsApps")
		bin = filepath.Join(dir, "hera-agent.exe")
	default:
		dir = filepath.Join(home, ".local", "bin")
		bin = filepath.Join(dir, "hera-agent")
	}
	return
}

// legacyInstallPaths returns the pre-WindowsApps install location
// (%LOCALAPPDATA%\hera-agent). uninstall scrubs leftover binaries and PATH
// entries from this location for users who installed before v0.0.6.
// Returns empty strings on non-Windows platforms.
func legacyInstallPaths() (dir, bin string) {
	if runtime.GOOS != "windows" {
		return "", ""
	}
	home, _ := os.UserHomeDir()
	dir = filepath.Join(home, "AppData", "Local", "hera-agent")
	bin = filepath.Join(dir, "hera-agent.exe")
	return
}

// runPowerShellWithArgs invokes powershell.exe with -Command "<script>" and
// the supplied positional args. The script can reference $args[0], $args[1],
// etc. to read them.
func runPowerShellWithArgs(script string, args ...string) error {
	psArgs := []string{"-Command", script}
	psArgs = append(psArgs, args...)
	cmd := exec.Command("powershell.exe", psArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
