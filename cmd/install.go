package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/NotNull92/hera-agent/internal/tui"
)

func installCmd() error {
	printInstallHeader()

	// Step 1: Detect current binary
	exe, err := os.Executable()
	if err != nil {
		return printInstallError("Cannot locate current binary", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return printInstallError("Cannot resolve binary path", err)
	}

	// Step 2: Determine install path
	installDir, installPath := getInstallPaths()
	printStep("Install directory", installDir)
	printStep("Binary path", installPath)

	// Step 3: Create install directory
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return printInstallError("Failed to create install directory", err)
	}
	printDone("Created install directory")

	// Step 4: Copy self to install path
	if err := copyFile(exe, installPath); err != nil {
		return printInstallError("Failed to copy binary", err)
	}
	printDone("Copied binary to install path")

	// Step 5: Make executable (Unix)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(installPath, 0755); err != nil {
			return printInstallError("Failed to set executable permission", err)
		}
		printDone("Set executable permission")
	}

	// Step 6: Clean up legacy install (older versions used %LOCALAPPDATA%\hera-agent)
	if runtime.GOOS == "windows" {
		cleanupLegacyInstall(installPath)
	}

	// Step 7: Migrate PATH (no registration needed on Windows — WindowsApps is auto-PATH)
	if err := addToPATH(installDir); err != nil {
		return printInstallError("Failed to update PATH", err)
	}
	if runtime.GOOS == "windows" {
		printDone("Cleaned legacy PATH entries (if any)")
	} else {
		printDone("Added to PATH")
	}

	// Step 8: Print success
	printInstallSuccess(installPath, exe)
	return nil
}

// cleanupLegacyInstall removes the pre-WindowsApps binary and directory if present.
// Skips the running binary itself (can't delete an executable that's currently running).
func cleanupLegacyInstall(_ string) {
	legacyDir, legacyBin := legacyInstallPaths()
	if legacyBin == "" {
		return
	}
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	if _, err := os.Stat(legacyBin); err == nil && legacyBin != exe {
		_ = os.Remove(legacyBin)
	}
	// Remove legacy dir if empty
	_ = os.Remove(legacyDir)
}

func getInstallPaths() (dir, bin string) {
	switch runtime.GOOS {
	case "windows":
		home, _ := os.UserHomeDir()
		// WindowsApps is on the default user PATH in Windows 10+,
		// so installing here avoids the need to touch the PATH registry
		// (and the env-block-staleness problems that come with it).
		dir = filepath.Join(home, "AppData", "Local", "Microsoft", "WindowsApps")
		bin = filepath.Join(dir, "hera-agent.exe")
	default:
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "bin")
		bin = filepath.Join(dir, "hera-agent")
	}
	return
}

// legacyInstallPaths returns the pre-WindowsApps install location.
// Used for migration cleanup.
func legacyInstallPaths() (dir, bin string) {
	if runtime.GOOS != "windows" {
		return "", ""
	}
	home, _ := os.UserHomeDir()
	dir = filepath.Join(home, "AppData", "Local", "hera-agent")
	bin = filepath.Join(dir, "hera-agent.exe")
	return
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func addToPATH(installDir string) error {
	if runtime.GOOS == "windows" {
		return addToPATHWindows(installDir)
	}
	return addToPATHUnix(installDir)
}

func addToPATHWindows(installDir string) error {
	// installDir is %LOCALAPPDATA%\Microsoft\WindowsApps which Windows already
	// keeps on the default user PATH. We don't need to register it.
	// We DO want to clean any leftover legacy entries from older installs.
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
	return runPowerShellWithArgs(script, legacyDir)
}

func addToPATHUnix(installDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	exportLine := fmt.Sprintf(`export PATH="%s:$PATH"`, installDir)

	// Update .bashrc
	bashrc := filepath.Join(home, ".bashrc")
	_ = appendLineIfNotExists(bashrc, exportLine)

	// Update .zshrc if exists
	zshrc := filepath.Join(home, ".zshrc")
	if _, err := os.Stat(zshrc); err == nil {
		_ = appendLineIfNotExists(zshrc, exportLine)
	}

	return nil
}

func appendLineIfNotExists(path, line string) error {
	data, _ := os.ReadFile(path)
	content := string(data)
	if strings.Contains(content, line) {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

func runPowerShellWithArgs(script string, args ...string) error {
	psArgs := []string{"-Command", script}
	psArgs = append(psArgs, args...)
	cmd := exec.Command("powershell.exe", psArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printInstallHeader() {
	fmt.Println()
	fmt.Println(tui.TitleStyle.Render("  Hera Agent — Commissioning"))
	fmt.Println(tui.InfoStyle.Render("  Establishing your estate..."))
	fmt.Println()
}

func printStep(label, value string) {
	fmt.Printf("  %s %s\n", tui.LabelStyle.Render(label+":"), tui.PathStyle.Render(value))
}

func printDone(msg string) {
	fmt.Printf("  %s %s\n", tui.CheckStyle.Render("✓"), msg)
}

func printInstallError(context string, err error) error {
	fmt.Println()
	fmt.Printf("  %s %s: %v\n", tui.ErrorStyle.Render("✗"), tui.LabelStyle.Render(context), err)
	fmt.Println()
	return fmt.Errorf("%s: %w", context, err)
}

func printInstallSuccess(installedPath, originalPath string) {
	fmt.Println()
	msg := fmt.Sprintf("Your instrument has been commissioned.\n\nEstablished at:\n  %s\n", installedPath)

	if runtime.GOOS == "windows" {
		msg += fmt.Sprintf("\nYou can now delete the original file:\n  %s\n", originalPath)
		msg += "\nAny NEW terminal or IDE will recognize 'hera-agent' immediately"
		msg += "\n(WindowsApps resides on the default user PATH)."
		msg += "\n\nShould an open terminal not yet recognize it, refresh with:"
		msg += "\n  $env:Path = [Environment]::GetEnvironmentVariable(\"Path\",\"User\")"
		msg += " + \";\" + [Environment]::GetEnvironmentVariable(\"Path\",\"Machine\")"
	} else {
		msg += "\nRun 'source ~/.bashrc' (or ~/.zshrc), or restart your terminal."
	}

	msg += "\n\nNext, instruct your agent to employ it:"
	msg += "\n  - Discover: inquire of Claude Code CLI or Codex in any terminal:"
	msg += "\n      \"Verify that hera-agent is installed and survey its capabilities.\""
	msg += "\n  - Commission (recommended): add to your project's CLAUDE.md / AGENTS.md:"
	msg += "\n      \"For all Unity endeavours, employ hera-agent.\""

	fmt.Println(tui.BoxAccent.Render(tui.SuccessStyle.Render(msg)))
	fmt.Println()
}
