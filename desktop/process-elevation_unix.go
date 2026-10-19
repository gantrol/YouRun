//go:build !windows

package desktop

import "errors"

func canElevateProcessControl() bool { return false }
func launchElevatedControl(string, string) (func(), error) {
	return nil, errors.New("processElevationUnsupported")
}
