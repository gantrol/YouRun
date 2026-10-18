//go:build !windows

package desktop

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type rawFileHandle struct {
	ID            string
	PID           int
	ProcessName   string
	ReferenceType string
	TargetFile    string
}

func collectOSProcesses(ctx context.Context) (map[int]rawProcess, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,pcpu=,rss=,user=,comm=,args=")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	numCPU := float64(runtime.NumCPU())
	if numCPU < 1 {
		numCPU = 1
	}
	currentUser := os.Getenv("USER")
	result := map[int]rawProcess{}
	for _, line := range strings.Split(out.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil || pid <= 0 {
			continue
		}
		cpuRaw, _ := strconv.ParseFloat(fields[2], 64)
		pct := cpuRaw / numCPU
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		rssKB, _ := strconv.ParseUint(fields[3], 10, 64)
		user := fields[4]
		comm := fields[5]
		name := filepath.Base(comm)
		displayName := ""
		if idx := strings.Index(comm, ".app/Contents/MacOS/"); idx > 0 {
			appBundle := filepath.Base(comm[:idx])
			if appBundle != "" && appBundle != "." && appBundle != "/" {
				displayName = appBundle
			}
		}
		args := strings.Join(fields[6:], " ")
		cmdState := "ok"
		if args == "" {
			cmdState = "unavailable"
		}
		result[pid] = rawProcess{
			PID:              pid,
			PPID:             ppid,
			Name:             name,
			DisplayName:      displayName,
			Executable:       comm,
			CommandLine:      args,
			CommandLineState: cmdState,
			User:             user,
			CPUPercent:       &pct,
			MemoryBytes:      rssKB * 1024,
			AccessDenied:     currentUser != "" && user != currentUser,
		}
	}
	return result, nil
}

func collectOSPorts(ctx context.Context) ([]rawPort, error) {
	cmd := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-iUDP")
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()
	var result []rawPort
	lines := strings.Split(out.String(), "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		ipVer := "IPv4"
		if strings.Contains(fields[4], "6") {
			ipVer = "IPv6"
		}
		proto := "TCP"
		if strings.HasPrefix(strings.ToUpper(fields[7]), "UDP") {
			proto = "UDP"
		}
		nameField := fields[8]
		state := "bound"
		if proto == "TCP" {
			state = "other"
			if len(fields) >= 10 {
				st := strings.ToUpper(strings.Trim(fields[9], "()"))
				switch st {
				case "LISTEN":
					state = "listening"
				case "ESTABLISHED":
					state = "established"
				case "CLOSE_WAIT":
					state = "close_wait"
				case "TIME_WAIT":
					state = "time_wait"
				}
			}
		}
		localPart := nameField
		remotePart := ""
		if parts := strings.SplitN(nameField, "->", 2); len(parts) == 2 {
			localPart = parts[0]
			remotePart = parts[1]
		}
		idx := strings.LastIndex(localPart, ":")
		if idx <= 0 {
			continue
		}
		ip := localPart[:idx]
		port, err := strconv.Atoi(localPart[idx+1:])
		if err != nil {
			continue
		}
		protoLabel := proto + " / " + ipVer
		if proto == "UDP" {
			protoLabel = "UDP"
		}
		result = append(result, rawPort{
			Protocol:      proto,
			IPVersion:     ipVer,
			ProtocolLabel: protoLabel,
			LocalAddress:  localPart,
			IP:            ip,
			Port:          port,
			RemoteAddress: remotePart,
			State:         state,
			PID:           pid,
		})
	}
	return result, nil
}

func collectOSFileHandles(ctx context.Context, targetPath string, isDir, recursive bool, procs map[int]rawProcess, attributions map[int]runAttribution) ([]rawFileHandle, int, error) {
	unreadable := 0
	for _, p := range procs {
		if p.AccessDenied {
			unreadable++
		}
	}
	args := []string{"-F", "pcn"}
	if isDir {
		if recursive {
			args = append(args, "+D", targetPath)
		} else {
			args = append(args, "+d", targetPath)
		}
	} else {
		args = append(args, "--", targetPath)
	}
	cmd := exec.CommandContext(ctx, "lsof", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run()

	seenPID := map[int]bool{}
	var results []rawFileHandle
	var currPID int
	var currComm string
	for _, line := range strings.Split(out.String(), "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			currPID, _ = strconv.Atoi(line[1:])
		case 'c':
			currComm = line[1:]
		case 'n':
			if currPID > 0 && !seenPID[currPID] {
				seenPID[currPID] = true
				results = append(results, rawFileHandle{
					ID:            "lsof-" + strconv.Itoa(currPID),
					PID:           currPID,
					ProcessName:   currComm,
					ReferenceType: "open_file",
					TargetFile:    line[1:],
				})
			}
		}
	}
	for pid, attr := range attributions {
		if !seenPID[pid] && attr.Directory != "" && strings.HasPrefix(filepath.Clean(attr.Directory), filepath.Clean(targetPath)) {
			seenPID[pid] = true
			results = append(results, rawFileHandle{
				ID:            "cwd-" + strconv.Itoa(pid),
				PID:           pid,
				ProcessName:   procs[pid].Name,
				ReferenceType: "open_file",
				TargetFile:    targetPath,
			})
		}
	}
	return results, unreadable, nil
}
