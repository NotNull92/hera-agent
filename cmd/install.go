package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

	// Step 6: Add to PATH
	if err := addToPATH(installDir); err != nil {
		return printInstallError("Failed to add to PATH", err)
	}
	printDone("Added to PATH")

	// Step 7: Print success
	printInstallSuccess(installPath, exe)
	return nil
}

func getInstallPaths() (dir, bin string) {
	switch runtime.GOOS {
	case "windows":
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "AppData", "Local", "hera-agent")
		bin = filepath.Join(dir, "hera-agent.exe")
	default:
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "bin")
		bin = filepath.Join(dir, "hera-agent")
	}
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
	script := `$dir = $args[0]; $p = [Environment]::GetEnvironmentVariable("Path", "User"); if ($p -notlike ("*" + $dir + "*")) { [Environment]::SetEnvironmentVariable("Path", ($dir + ";" + $p), "User") }`
	return runPowerShellWithArgs(script, installDir)
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
	fmt.Println("  hera-agent install")
	fmt.Println("  Installing hera-agent to your system...")
	fmt.Println()
}

func printStep(label, value string) {
	fmt.Printf("  %s: %s\n", label, value)
}

func printDone(msg string) {
	fmt.Printf("  ✓ %s\n", msg)
}

func printInstallError(context string, err error) error {
	fmt.Println()
	fmt.Printf("  ✗ %s: %v\n", context, err)
	fmt.Println()
	return fmt.Errorf("%s: %w", context, err)
}

func printInstallSuccess(installedPath, originalPath string) {
	fmt.Println()
	msg := fmt.Sprintf("hera-agent installed successfully!\n\nInstalled to:\n  %s\n", installedPath)

	if runtime.GOOS == "windows" {
		msg += fmt.Sprintf("\nYou can now delete the original file:\n  %s\n", originalPath)
		msg += "\nRestart PowerShell to use 'hera-agent' from PATH."
	} else {
		msg += "\nRun 'source ~/.bashrc' (or ~/.zshrc) or restart your terminal."
	}

	fmt.Println(msg)
	fmt.Println()
}
