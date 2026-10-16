package engine

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// Execution helpers run with the interactive user's privileges, never elevated.
// Windows ignores Unix permission bits, so protect its capability with a DACL.
func secureDirectory(directory string) error {
	token := windows.GetCurrentProcessToken()
	if token.IsElevated() {
		return errors.New("YouRun must not run as administrator")
	}
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(directory, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
