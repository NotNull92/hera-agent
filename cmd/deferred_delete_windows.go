//go:build windows

package cmd

import (
	"os"
	"os/exec"
)

// scheduleDelete tries os.Remove first; on failure (typically a Windows file
// lock because the file is the running .exe or its .bak still memory-mapped),
// it schedules a delayed cmd.exe delete that runs after this process exits.
// Returns nil if removal happened or was scheduled, error only if scheduling
// itself failed.
func scheduleDelete(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	if err := os.Remove(path); err == nil {
		return nil
	}
	quoted := `"` + path + `"`
	cmd := exec.Command("cmd", "/c", "timeout /t 1 >nul && del /f "+quoted)
	return cmd.Start()
}
