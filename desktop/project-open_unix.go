//go:build !windows

package desktop

import "os/exec"

func openProjectDirectory(directory string) error {
	cmd := exec.Command("/usr/bin/open", "-a", "Finder", directory)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
