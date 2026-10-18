package desktop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/projectmeta"
	"commandmanager.local/core/internal/workspace"
)

type ProcessItem struct {
	PID                int                    `json:"pid"`
	PPID               int                    `json:"ppid"`
	Name               string                 `json:"name"`
	DisplayName        string                 `json:"displayName,omitempty"`
	Icon               string                 `json:"icon,omitempty"`
	Executable         string                 `json:"executable,omitempty"`
	CommandLine        string                 `json:"commandLine,omitempty"`
	CommandLineState   string                 `json:"commandLineState"`
	User               string                 `json:"user,omitempty"`
	Status             string                 `json:"status"`
	StartedAt          string                 `json:"startedAt,omitempty"`
	CPUPercent         *float64               `json:"cpuPercent"`
	CPUSampling        bool                   `json:"cpuSampling,omitempty"`
	MemoryBytes        uint64                 `json:"memoryBytes"`
	GPUPercent         *float64               `json:"gpuPercent"`
	GPUSampling        bool                   `json:"gpuSampling,omitempty"`
	GPUMemoryBytes     uint64                 `json:"gpuMemoryBytes,omitempty"`
	NetworkBytesPerSec *float64               `json:"networkBytesPerSec"`
	NetworkSampling    bool                   `json:"networkSampling,omitempty"`
	NetworkConnections int                    `json:"networkConnections,omitempty"`
	Ports              []int                  `json:"ports,omitempty"`
	Ownership          string                 `json:"ownership"`
	RunID              string                 `json:"runId,omitempty"`
	RunStatus          string                 `json:"runStatus,omitempty"`
	CommandID          string                 `json:"commandId,omitempty"`
	CommandTitle       string                 `json:"commandTitle,omitempty"`
	ProjectID          string                 `json:"projectId,omitempty"`
	ProjectName        string                 `json:"projectName,omitempty"`
	WorkspaceID        string                 `json:"workspaceId,omitempty"`
	WorkspaceName      string                 `json:"workspaceName,omitempty"`
	Directory          string                 `json:"directory,omitempty"`
	LaunchSource       *engine.SourceSnapshot `json:"launchSource,omitempty"`
}

type PortItem struct {
	ID                 string                 `json:"id"`
	Protocol           string                 `json:"protocol"`
	IPVersion          string                 `json:"ipVersion"`
	ProtocolLabel      string                 `json:"protocolLabel"`
	LocalAddress       string                 `json:"localAddress"`
	IP                 string                 `json:"ip"`
	Port               int                    `json:"port"`
	RemoteAddress      string                 `json:"remoteAddress,omitempty"`
	State              string                 `json:"state"`
	PID                int                    `json:"pid"`
	ProcessName        string                 `json:"processName"`
	ProcessDisplayName string                 `json:"processDisplayName,omitempty"`
	ProcessIcon        string                 `json:"processIcon,omitempty"`
	Executable         string                 `json:"executable,omitempty"`
	CommandLine        string                 `json:"commandLine,omitempty"`
	CommandLineState   string                 `json:"commandLineState"`
	User               string                 `json:"user,omitempty"`
	ProcessExited      bool                   `json:"processExited,omitempty"`
	Ownership          string                 `json:"ownership"`
	RunID              string                 `json:"runId,omitempty"`
	RunStatus          string                 `json:"runStatus,omitempty"`
	CommandID          string                 `json:"commandId,omitempty"`
	CommandTitle       string                 `json:"commandTitle,omitempty"`
	ProjectID          string                 `json:"projectId,omitempty"`
	ProjectName        string                 `json:"projectName,omitempty"`
	WorkspaceID        string                 `json:"workspaceId,omitempty"`
	WorkspaceName      string                 `json:"workspaceName,omitempty"`
	Directory          string                 `json:"directory,omitempty"`
	LaunchSource       *engine.SourceSnapshot `json:"launchSource,omitempty"`
}

type FileHandleItem struct {
	ID                 string                 `json:"id"`
	PID                int                    `json:"pid"`
	ProcessName        string                 `json:"processName"`
	ProcessDisplayName string                 `json:"processDisplayName,omitempty"`
	ProcessIcon        string                 `json:"processIcon,omitempty"`
	User               string                 `json:"user,omitempty"`
	Executable         string                 `json:"executable,omitempty"`
	CommandLine        string                 `json:"commandLine,omitempty"`
	CommandLineState   string                 `json:"commandLineState"`
	ReferenceType      string                 `json:"referenceType"`
	BlockingDelete     string                 `json:"blockingDelete"`
	TargetFile         string                 `json:"targetFile"`
	ProcessExited      bool                   `json:"processExited,omitempty"`
	Ownership          string                 `json:"ownership"`
	RunID              string                 `json:"runId,omitempty"`
	RunStatus          string                 `json:"runStatus,omitempty"`
	CommandID          string                 `json:"commandId,omitempty"`
	CommandTitle       string                 `json:"commandTitle,omitempty"`
	ProjectID          string                 `json:"projectId,omitempty"`
	ProjectName        string                 `json:"projectName,omitempty"`
	WorkspaceID        string                 `json:"workspaceId,omitempty"`
	WorkspaceName      string                 `json:"workspaceName,omitempty"`
	Directory          string                 `json:"directory,omitempty"`
	LaunchSource       *engine.SourceSnapshot `json:"launchSource,omitempty"`
}

type rawProcess struct {
	PID                int
	PPID               int
	Name               string
	DisplayName        string
	Icon               string
	Executable         string
	CommandLine        string
	CommandLineState   string
	User               string
	StartedAt          time.Time
	CPUPercent         *float64
	CPUSampling        bool
	MemoryBytes        uint64
	GPUPercent         *float64
	GPUSampling        bool
	GPUMemoryBytes     uint64
	NetworkBytesPerSec *float64
	NetworkSampling    bool
	AccessDenied       bool
}

type rawPort struct {
	Protocol      string
	IPVersion     string
	ProtocolLabel string
	LocalAddress  string
	IP            string
	Port          int
	RemoteAddress string
	State         string
	PID           int
}

type cpuSampleKey struct {
	pid       int
	startedNs int64
}

type cpuSampleVal struct {
	total100ns   uint64
	ioBytes      uint64
	lastCPUPct   float64
	lastNetRate  float64
	hasLastRates bool
	sampledAt    time.Time
}

var (
	cpuSamplesMu sync.Mutex
	cpuSamples   = map[cpuSampleKey]cpuSampleVal{}
)

type skipIconsKey struct{}
type processMetricsKey struct{}

// WithSkipProcessIcons instructs OS resource queries to skip extracting and
// returning base64 process icons (used by CLI/MCP unless --include-images is set).
func WithSkipProcessIcons(ctx context.Context, skip bool) context.Context {
	return context.WithValue(ctx, skipIconsKey{}, skip)
}

func shouldSkipProcessIcons(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(skipIconsKey{}).(bool)
	return v
}

func withProcessMetrics(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, processMetricsKey{}, enabled)
}

func shouldCollectProcessMetrics(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(processMetricsKey{}).(bool)
	return v
}

type runAttribution struct {
	Ownership     string
	RunID         string
	RunStatus     string
	CommandID     string
	CommandTitle  string
	ProjectID     string
	ProjectName   string
	WorkspaceID   string
	WorkspaceName string
	Directory     string
	LaunchSource  *engine.SourceSnapshot
}

func (h *Host) buildRunAttributions(ctx context.Context, procs map[int]rawProcess) map[int]runAttribution {
	return buildRunAttributionsFrom(ctx, h.dataDir, h.workspace, procs)
}

func buildRunAttributionsFrom(ctx context.Context, dataDir string, ws *workspace.Service, procs map[int]rawProcess) map[int]runAttribution {
	result := map[int]runAttribution{}
	if ws == nil {
		return result
	}
	status, data, err := ws.Request(ctx, "GET", "/api/state", nil)
	if err != nil || status != http.StatusOK {
		return result
	}
	var snap engine.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return result
	}
	meta, _ := projectmeta.LoadContext(ctx, dataDir)
	projectsByID := map[string]engine.Project{}
	for _, p := range snap.Definitions.Projects {
		projectsByID[p.ID] = p
	}
	commandsByID := map[string]engine.Command{}
	projectCmdDirs := map[string][]string{}
	for _, c := range snap.Definitions.Commands {
		commandsByID[c.ID] = c
		if c.Project != nil && c.Directory != "" {
			projectCmdDirs[*c.Project] = append(projectCmdDirs[*c.Project], c.Directory)
		}
	}
	projectLocations := map[string][]projectmeta.Location{}
	for pid := range projectsByID {
		locs, _ := projectmeta.ResolveLocations(meta[pid], projectCmdDirs[pid])
		projectLocations[pid] = locs
	}

	rootRuns := map[int]runAttribution{}
	for _, run := range snap.Runs {
		if run.PID <= 0 || (run.Status != "running" && run.Status != "stopping" && run.Status != "unknown") {
			continue
		}
		proc, alive := procs[run.PID]
		if !alive {
			continue
		}
		if runStarted, parseErr := time.Parse(time.RFC3339Nano, run.Time); parseErr == nil && !proc.StartedAt.IsZero() {
			if proc.StartedAt.Before(runStarted.Add(-5 * time.Second)) {
				continue
			}
		}
		ownership := "managed"
		if run.Status == "unknown" {
			ownership = "unconfirmed"
		}
		cmd, hasCmd := commandsByID[run.CommandID]
		cmdTitle := run.Title
		var projID, projName, wsID, wsName string
		if hasCmd {
			cmdTitle = cmd.Title
			if cmd.Project != nil {
				projID = *cmd.Project
				if p, ok := projectsByID[projID]; ok {
					projName = p.Name
				}
				locs := projectLocations[projID]
				cleanRunDir := filepath.Clean(run.Directory)
				for _, loc := range locs {
					rel, relErr := filepath.Rel(filepath.Clean(loc.Directory), cleanRunDir)
					if relErr == nil && (rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))) {
						wsID, wsName = loc.ID, loc.Name
						break
					}
				}
			}
		}
		launchSrc := run.LaunchSource
		if launchSrc != nil {
			if wsID == "" {
				wsID = launchSrc.WorkspaceID
			}
			if wsName == "" {
				wsName = launchSrc.WorkspaceName
			}
		} else if run.Directory != "" {
			inspected := engine.InspectSource(ctx, run.Directory, wsID, wsName, false)
			launchSrc = &inspected
		}
		rootRuns[run.PID] = runAttribution{
			Ownership:     ownership,
			RunID:         run.ID,
			RunStatus:     run.Status,
			CommandID:     run.CommandID,
			CommandTitle:  cmdTitle,
			ProjectID:     projID,
			ProjectName:   projName,
			WorkspaceID:   wsID,
			WorkspaceName: wsName,
			Directory:     run.Directory,
			LaunchSource:  launchSrc,
		}
	}

	if len(rootRuns) == 0 {
		return result
	}

	for pid, proc := range procs {
		if attr, ok := rootRuns[pid]; ok {
			result[pid] = attr
			continue
		}
		curr := proc
		for depth := 0; depth < 64 && curr.PPID > 0 && curr.PPID != curr.PID; depth++ {
			parent, ok := procs[curr.PPID]
			if !ok {
				break
			}
			if !curr.StartedAt.IsZero() && !parent.StartedAt.IsZero() && curr.StartedAt.Before(parent.StartedAt.Add(-1*time.Second)) {
				break
			}
			if attr, matched := rootRuns[parent.PID]; matched {
				result[pid] = attr
				break
			}
			curr = parent
		}
	}
	return result
}

func (h *Host) SetFilePicker(picker func() (string, error)) {
	h.pickerMu.Lock()
	h.chooseFile = picker
	h.pickerMu.Unlock()
}

func (h *Host) serveChooseFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}
	h.pickerMu.RLock()
	picker := h.chooseFile
	h.pickerMu.RUnlock()
	if picker == nil {
		h.updates.mu.Lock()
		picker = h.updates.pick
		h.updates.mu.Unlock()
	}
	if picker == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "directoryPickerFailed"})
		return
	}
	if !h.choosing.CompareAndSwap(false, true) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "avatarPickerBusy"})
		return
	}
	defer h.choosing.Store(false)
	path, err := picker()
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "directoryPickerFailed"})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"path": path})
}

func QueryProcesses(ctx context.Context, dataDir string, ws *workspace.Service) (map[string]any, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	procs, err := collectOSProcesses(withProcessMetrics(ctx, true))
	if err != nil {
		return nil, err
	}
	rawPorts, _ := collectOSPorts(ctx)
	portsByPID := map[int][]int{}
	seenPortByPID := map[int]map[int]bool{}
	connCountByPID := map[int]int{}
	for _, p := range rawPorts {
		if p.PID <= 0 || p.Port <= 0 {
			continue
		}
		connCountByPID[p.PID]++
		if seenPortByPID[p.PID] == nil {
			seenPortByPID[p.PID] = map[int]bool{}
		}
		if !seenPortByPID[p.PID][p.Port] {
			seenPortByPID[p.PID][p.Port] = true
			portsByPID[p.PID] = append(portsByPID[p.PID], p.Port)
		}
	}
	for pid := range portsByPID {
		sort.Ints(portsByPID[pid])
	}

	attributions := buildRunAttributionsFrom(ctx, dataDir, ws, procs)
	items := make([]ProcessItem, 0, len(procs))
	unreadableCount := 0
	for _, p := range procs {
		if p.AccessDenied {
			unreadableCount++
		}
		attr, hasAttr := attributions[p.PID]
		ownership := "external"
		if hasAttr {
			ownership = attr.Ownership
		}
		startedStr := ""
		if !p.StartedAt.IsZero() {
			startedStr = p.StartedAt.UTC().Format(time.RFC3339Nano)
		}
		connCount := connCountByPID[p.PID]
		netRate := p.NetworkBytesPerSec
		netSampling := p.NetworkSampling
		if connCount == 0 {
			zero := 0.0
			netRate = &zero
			netSampling = false
		}
		items = append(items, ProcessItem{
			PID:                p.PID,
			PPID:               p.PPID,
			Name:               p.Name,
			DisplayName:        p.DisplayName,
			Icon:               p.Icon,
			Executable:         p.Executable,
			CommandLine:        p.CommandLine,
			CommandLineState:   p.CommandLineState,
			User:               p.User,
			Status:             "running",
			StartedAt:          startedStr,
			CPUPercent:         p.CPUPercent,
			CPUSampling:        p.CPUSampling,
			MemoryBytes:        p.MemoryBytes,
			GPUPercent:         p.GPUPercent,
			GPUSampling:        p.GPUSampling,
			GPUMemoryBytes:     p.GPUMemoryBytes,
			NetworkBytesPerSec: netRate,
			NetworkSampling:    netSampling,
			NetworkConnections: connCount,
			Ports:              portsByPID[p.PID],
			Ownership:          ownership,
			RunID:              attr.RunID,
			RunStatus:          attr.RunStatus,
			CommandID:          attr.CommandID,
			CommandTitle:       attr.CommandTitle,
			ProjectID:          attr.ProjectID,
			ProjectName:        attr.ProjectName,
			WorkspaceID:        attr.WorkspaceID,
			WorkspaceName:      attr.WorkspaceName,
			Directory:          attr.Directory,
			LaunchSource:       attr.LaunchSource,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Ownership != items[j].Ownership {
			if items[i].Ownership == "managed" {
				return true
			}
			if items[j].Ownership == "managed" {
				return false
			}
		}
		if items[i].MemoryBytes != items[j].MemoryBytes {
			return items[i].MemoryBytes > items[j].MemoryBytes
		}
		return items[i].PID < items[j].PID
	})
	coverage := "complete"
	if unreadableCount > 0 {
		coverage = "partial"
	}
	return map[string]any{
		"observedAt":             now,
		"coverage":               coverage,
		"unreadableProcessCount": unreadableCount,
		"items":                  items,
	}, nil
}

func (h *Host) serveResourcesProcesses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	res, err := QueryProcesses(ctx, h.dataDir, h.workspace)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requestFailed"})
		return
	}
	json.NewEncoder(w).Encode(res)
}

func QueryPorts(ctx context.Context, dataDir string, ws *workspace.Service) (map[string]any, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	procs, _ := collectOSProcesses(ctx)
	unreadableCount := 0
	for _, p := range procs {
		if p.AccessDenied {
			unreadableCount++
		}
	}
	rawPorts, err := collectOSPorts(ctx)
	if err != nil {
		return nil, err
	}
	attributions := buildRunAttributionsFrom(ctx, dataDir, ws, procs)
	items := make([]PortItem, 0, len(rawPorts))
	seen := map[string]bool{}
	for _, rp := range rawPorts {
		proc, hasProc := procs[rp.PID]
		attr, hasAttr := attributions[rp.PID]
		ownership := "external"
		if hasAttr {
			ownership = attr.Ownership
		}
		procName := proc.Name
		cmdState := proc.CommandLineState
		if !hasProc {
			procName = ""
			cmdState = "unavailable"
		}
		rowID := rp.Protocol + "-" + rp.IPVersion + "-" + rp.LocalAddress + "-" + rp.State
		if rp.RemoteAddress != "" {
			rowID += "-" + rp.RemoteAddress
		}
		if seen[rowID] {
			continue
		}
		seen[rowID] = true
		items = append(items, PortItem{
			ID:                 rowID,
			Protocol:           rp.Protocol,
			IPVersion:          rp.IPVersion,
			ProtocolLabel:      rp.ProtocolLabel,
			LocalAddress:       rp.LocalAddress,
			IP:                 rp.IP,
			Port:               rp.Port,
			RemoteAddress:      rp.RemoteAddress,
			State:              rp.State,
			PID:                rp.PID,
			ProcessName:        procName,
			ProcessDisplayName: proc.DisplayName,
			ProcessIcon:        proc.Icon,
			Executable:         proc.Executable,
			CommandLine:        proc.CommandLine,
			CommandLineState:   cmdState,
			User:               proc.User,
			ProcessExited:      !hasProc && rp.PID > 0,
			Ownership:          ownership,
			RunID:              attr.RunID,
			RunStatus:          attr.RunStatus,
			CommandID:          attr.CommandID,
			CommandTitle:       attr.CommandTitle,
			ProjectID:          attr.ProjectID,
			ProjectName:        attr.ProjectName,
			WorkspaceID:        attr.WorkspaceID,
			WorkspaceName:      attr.WorkspaceName,
			Directory:          attr.Directory,
			LaunchSource:       attr.LaunchSource,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Ownership != items[j].Ownership {
			if items[i].Ownership == "managed" {
				return true
			}
			if items[j].Ownership == "managed" {
				return false
			}
		}
		if items[i].Port != items[j].Port {
			return items[i].Port < items[j].Port
		}
		return items[i].LocalAddress < items[j].LocalAddress
	})
	coverage := "complete"
	if unreadableCount > 0 {
		coverage = "partial"
	}
	return map[string]any{
		"observedAt":             now,
		"coverage":               coverage,
		"unreadableProcessCount": unreadableCount,
		"items":                  items,
	}, nil
}

func (h *Host) serveResourcesPorts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	res, err := QueryPorts(ctx, h.dataDir, h.workspace)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requestFailed"})
		return
	}
	json.NewEncoder(w).Encode(res)
}

func QueryFiles(ctx context.Context, dataDir string, ws *workspace.Service, cleanTarget string, recursive bool) (map[string]any, error) {
	info, statErr := os.Stat(cleanTarget)
	isDir := statErr == nil && info.IsDir()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	procs, _ := collectOSProcesses(ctx)
	attributions := buildRunAttributionsFrom(ctx, dataDir, ws, procs)
	rawHandles, unreadableCount, err := collectOSFileHandles(ctx, cleanTarget, isDir, recursive, procs, attributions)
	if err != nil {
		return nil, err
	}

	coverage := "complete"
	if unreadableCount > 0 {
		coverage = "partial"
	}

	items := make([]FileHandleItem, 0, len(rawHandles))
	for _, rh := range rawHandles {
		proc, hasProc := procs[rh.PID]
		attr, hasAttr := attributions[rh.PID]
		// Critical v2 rule: external processes are never attributed to a project/workspace
		// merely because their open file lies inside a project workspace directory.
		ownership := "external"
		if hasAttr {
			ownership = attr.Ownership
		}
		procName := rh.ProcessName
		if hasProc && proc.Name != "" {
			procName = proc.Name
		}
		cmdState := proc.CommandLineState
		if !hasProc {
			cmdState = "unavailable"
		}
		items = append(items, FileHandleItem{
			ID:                 rh.ID,
			PID:                rh.PID,
			ProcessName:        procName,
			ProcessDisplayName: proc.DisplayName,
			ProcessIcon:        proc.Icon,
			User:               proc.User,
			Executable:         proc.Executable,
			CommandLine:        proc.CommandLine,
			CommandLineState:   cmdState,
			ReferenceType:      rh.ReferenceType,
			BlockingDelete:     "unconfirmed",
			TargetFile:         rh.TargetFile,
			ProcessExited:      !hasProc && rh.PID > 0,
			Ownership:          ownership,
			RunID:              attr.RunID,
			RunStatus:          attr.RunStatus,
			CommandID:          attr.CommandID,
			CommandTitle:       attr.CommandTitle,
			ProjectID:          attr.ProjectID,
			ProjectName:        attr.ProjectName,
			WorkspaceID:        attr.WorkspaceID,
			WorkspaceName:      attr.WorkspaceName,
			Directory:          attr.Directory,
			LaunchSource:       attr.LaunchSource,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Ownership != items[j].Ownership {
			if items[i].Ownership == "managed" {
				return true
			}
			if items[j].Ownership == "managed" {
				return false
			}
		}
		return items[i].PID < items[j].PID
	})

	return map[string]any{
		"targetPath":             cleanTarget,
		"isDirectory":            isDir,
		"recursive":              recursive,
		"observedAt":             now,
		"coverage":               coverage,
		"unreadableProcessCount": unreadableCount,
		"items":                  items,
	}, nil
}

func (h *Host) serveResourcesFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}
	var request struct {
		TargetPath string `json:"targetPath"`
		Recursive  bool   `json:"recursive,omitempty"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}
	trimmed := strings.TrimSpace(request.TargetPath)
	if trimmed == "" || !filepath.IsAbs(trimmed) || strings.Contains(strings.ToLower(trimmed), "trash") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidDirectory"})
		return
	}
	cleanTarget := filepath.Clean(trimmed)

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	res, err := QueryFiles(ctx, h.dataDir, h.workspace, cleanTarget, request.Recursive)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "requestFailed"})
		return
	}
	json.NewEncoder(w).Encode(res)
}
