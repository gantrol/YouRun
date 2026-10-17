//go:build !windows

package desktop

import (
	"os/exec"
	"runtime"
)

func openProjectDirectory(directory string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("/usr/bin/open", "-a", "Finder", directory)
	} else {
		cmd = exec.Command("xdg-open", directory)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
