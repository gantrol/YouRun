package engine

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func shellCommand(c Command, request Launch) (*exec.Cmd, string, error) {
	shell := c.Shell
	if shell == "default" {
		if runtime.GOOS == "windows" {
			shell = "PowerShell"
		} else {
			shell = "zsh"
		}
	}
	text := c.Text
	environment := []string{}
	for _, entry := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if strings.HasPrefix(key, "COMMAND_MANAGER_UPDATE_") || key == "MYGO_GENERATE" {
			continue
		}
		environment = append(environment, entry)
	}
	for _, field := range c.Fields {
		value := request.Values[field]
		if strings.TrimSpace(value) == "" || len(value) > 8192 || strings.ContainsRune(value, 0) {
			return nil, "", errors.New("invalidInput")
		}
		// Expand an environment variable, never splice user input into shell syntax.
		name := "COMMAND_MANAGER_" + strings.ToUpper(field)
		environment = append(environment, name+"="+value)
		quoted := "\"${" + name + "}\""
		if shell == "PowerShell" {
			quoted = "$env:" + name
		}
		if shell == "cmd" {
			return nil, "", errors.New("cmdParametersUnsupported")
		}
		placeholder := "{{" + field + "}}"
		if !strings.Contains(text, placeholder) {
			return nil, "", errors.New("missingPlaceholder")
		}
		text = strings.ReplaceAll(text, placeholder, quoted)
	}
	var cmd *exec.Cmd
	switch shell {
	case "PowerShell":
		exe, err := exec.LookPath("pwsh")
		if err != nil {
			exe, err = exec.LookPath("powershell.exe")
		}
		if err != nil {
			return nil, "", errors.New("shellMissing")
		}
		prefix := "$ErrorActionPreference='Stop'; [Console]::OutputEncoding=[System.Text.UTF8Encoding]::new(); $OutputEncoding=[Console]::OutputEncoding; "
		cmd = exec.Command(exe, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", prefix+text+"\nif ($null -ne $LASTEXITCODE) { exit $LASTEXITCODE }")
	case "zsh", "bash":
		cmd = exec.Command(shell, "-l", "-c", text)
	case "cmd":
		if runtime.GOOS != "windows" {
			return nil, "", errors.New("shellMissing")
		}
		cmd = exec.Command("cmd.exe", "/d", "/s", "/c", text)
	default:
		return nil, "", errors.New("shellMissing")
	}
	cmd.Dir = request.Directory
	cmd.Env = environment
	prepareChild(cmd)
	return cmd, text, nil
}
