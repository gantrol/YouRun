//go:build !windows

package engine

import (
	"os"
	"os/exec"
)

type independentProcess struct {
	pid int
	cmd *exec.Cmd
}

func startIndependent(cmd *exec.Cmd, log *os.File, _ ...string) (*independentProcess, *processGroup, error) {
	input, err := os.Open(os.DevNull)
	if err != nil {
		return nil, nil, err
	}
	defer input.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = input, log, log
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	group, err := attachProcess(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, nil, err
	}
	return &independentProcess{pid: cmd.Process.Pid, cmd: cmd}, group, nil
}
func (p *independentProcess) wait() (int, error) {
	err := p.cmd.Wait()
	if p.cmd.ProcessState == nil {
		return -1, err
	}
	return p.cmd.ProcessState.ExitCode(), err
}
func (p *independentProcess) close() {}
