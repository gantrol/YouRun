package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func openProjectDirectory(directory string) error {
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), directory)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
