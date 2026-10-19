package desktop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"commandmanager.local/core/internal/database"
	"commandmanager.local/core/internal/engine"
)

const (
	bountySettingKey     = "bounty/state"
	bountyRingMaxAge     = 10 * time.Minute
	bountyRingMaxSamples = 120
	bountyWindowDuration = 30 * time.Second
	bountyMaxEvents      = 48
	bountyMaxWatches     = 64
)

type BountyGroupSummary struct {
	GroupKey           string   `json:"groupKey"`
	WindowCPU          *float64 `json:"windowCpu"`
	WindowMemory       uint64   `json:"windowMemory"`
	WindowGPU          *float64 `json:"windowGpu"`
	WindowGPUMemory    uint64   `json:"windowGpuMemory"`
	WindowNetwork      *float64 `json:"windowNetwork"`
	CumulativeNetBytes uint64   `json:"cumulativeNetBytes"`
	CPUDelta           float64  `json:"cpuDelta,omitempty"`
	MemoryDeltaBytes   int64    `json:"memoryDeltaBytes,omitempty"`
	GPUDelta           float64  `json:"gpuDelta,omitempty"`
	NetworkDeltaBps    float64  `json:"networkDeltaBps,omitempty"`
	RisingCategories   []string `json:"risingCategories,omitempty"`
	Watched            bool     `json:"watched,omitempty"`
	WatchTriggered     bool     `json:"watchTriggered,omitempty"`
}

type BountyEvent struct {
	ID                 string   `json:"id"`
	GroupKey           string   `json:"groupKey"`
	Category           string   `json:"category"`   // "cpu" | "memory" | "gpu" | "network"
	ReasonKind         string   `json:"reasonKind"` // "rapid_rise" | "peak_high" | "watch_triggered"
	WatchID            string   `json:"watchId,omitempty"`
	Title              string   `json:"title"`
	ProcessName        string   `json:"processName"`
	Icon               string   `json:"icon,omitempty"`
	PID                int      `json:"pid"`
	ProcessIdentity    string   `json:"processIdentity,omitempty"`
	ControlRunID       string   `json:"controlRunId,omitempty"`
	Executable         string   `json:"executable,omitempty"`
	CommandLine        string   `json:"commandLine,omitempty"`
	CommandLineState   string   `json:"commandLineState,omitempty"`
	Ownership          string   `json:"ownership"`
	RunID              string   `json:"runId,omitempty"`
	RunStatus          string   `json:"runStatus,omitempty"`
	CommandID          string   `json:"commandId,omitempty"`
	CommandTitle       string   `json:"commandTitle,omitempty"`
	ProjectID          string   `json:"projectId,omitempty"`
	ProjectName        string   `json:"projectName,omitempty"`
	WorkspaceID        string   `json:"workspaceId,omitempty"`
	WorkspaceName      string   `json:"workspaceName,omitempty"`
	Directory          string   `json:"directory,omitempty"`
	Ports              []int    `json:"ports,omitempty"`
	PeakValue          float64  `json:"peakValue"`
	BaselineValue      float64  `json:"baselineValue,omitempty"`
	DeltaValue         float64  `json:"deltaValue,omitempty"`
	CumulativeBytes    uint64   `json:"cumulativeBytes,omitempty"`
	MemoryBytes        uint64   `json:"memoryBytes,omitempty"`
	CPUPercent         *float64 `json:"cpuPercent,omitempty"`
	GPUPercent         *float64 `json:"gpuPercent,omitempty"`
	GPUMemoryBytes     uint64   `json:"gpuMemoryBytes,omitempty"`
	NetworkBytesPerSec *float64 `json:"networkBytesPerSec,omitempty"`
	NetworkConnections int      `json:"networkConnections,omitempty"`
	FirstObservedAt    string   `json:"firstObservedAt"`
	PeakAt             string   `json:"peakAt"`
	LastObservedAt     string   `json:"lastObservedAt"`
	DurationSec        int      `json:"durationSec"`
	ProcessExited      bool     `json:"processExited,omitempty"`
	Acknowledged       bool     `json:"acknowledged,omitempty"`
}

type BountyWatch struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	MatchKind       string  `json:"matchKind"` // "process" | "command"
	TargetName      string  `json:"targetName"`
	Executable      string  `json:"executable,omitempty"`
	CommandID       string  `json:"commandId,omitempty"`
	ProjectID       string  `json:"projectId,omitempty"`
	ProjectName     string  `json:"projectName,omitempty"`
	Category        string  `json:"category"`      // "network" | "cpu" | "memory" | "gpu"
	ConditionKind   string  `json:"conditionKind"` // "threshold" | "rapid_rise"
	Threshold       float64 `json:"threshold"`
	NotifyToast     bool    `json:"notifyToast"`
	Enabled         bool    `json:"enabled"`
	Note            string  `json:"note,omitempty"`
	CreatedAt       string  `json:"createdAt"`
	HitCount        int     `json:"hitCount"`
	LastTriggeredAt string  `json:"lastTriggeredAt,omitempty"`
	LastPeakValue   float64 `json:"lastPeakValue,omitempty"`
	LastPID         int     `json:"lastPid,omitempty"`
	UnreadHit       bool    `json:"unreadHit,omitempty"`
}

type bountyPersistedState struct {
	Events  []BountyEvent `json:"events"`
	Watches []BountyWatch `json:"watches"`
}

type BountySnapshot struct {
	Groups           map[string]BountyGroupSummary `json:"groups"`
	Events           []BountyEvent                 `json:"events"`
	Watches          []BountyWatch                 `json:"watches"`
	UnreadAlertCount int                           `json:"unreadAlertCount"`
}

type bountySamplePoint struct {
	at             time.Time
	cpu            float64
	hasCPU         bool
	memory         uint64
	gpu            float64
	hasGPU         bool
	gpuMemory      uint64
	network        float64
	hasNetwork     bool
	netConnections int
}

type bountyGroupHistory struct {
	samples            []bountySamplePoint
	cumulativeNetBytes uint64
	lastSampleAt       time.Time
	latestRep          ProcessItem
	latestTitle        string
	latestIcon         string
	latestPorts        []int
}

var (
	bountyMu          sync.Mutex
	bountyRingByGroup = map[string]*bountyGroupHistory{}
	bountyLoadedDir   string
	bountyStateCache  bountyPersistedState
)

func friendlyProcessLabelGo(name, displayName string) string {
	dn := strings.TrimSpace(displayName)
	if dn != "" {
		return dn
	}
	n := strings.TrimSpace(name)
	lower := strings.ToLower(n)
	for _, ext := range []string{".exe", ".app", ".bin", ".cmd", ".bat", ".sh"} {
		if strings.HasSuffix(lower, ext) && len(n) > len(ext) {
			return n[:len(n)-len(ext)]
		}
	}
	if n != "" {
		return n
	}
	return "process"
}

func processGroupKeyGo(item ProcessItem) string {
	label := strings.ToLower(friendlyProcessLabelGo(item.Name, item.DisplayName))
	if item.Ownership == "managed" {
		id := item.RunID
		if id == "" {
			id = strconv.Itoa(item.PID)
		}
		return "managed:" + id + ":" + label
	}
	return "ext:" + label
}

func ensureBountyStateLoadedLocked(ctx context.Context, dataDir string) {
	if dataDir == "" || bountyLoadedDir == dataDir {
		return
	}
	store, err := database.Open(dataDir)
	if err != nil {
		return
	}
	var st bountyPersistedState
	if err := store.ReadSetting(ctx, bountySettingKey, &st); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return
	}
	if st.Events == nil {
		st.Events = []BountyEvent{}
	}
	if st.Watches == nil {
		st.Watches = []BountyWatch{}
	}
	bountyStateCache = st
	bountyLoadedDir = dataDir
}

func saveBountyStateLocked(ctx context.Context, dataDir string) {
	if dataDir == "" {
		return
	}
	store, err := database.Open(dataDir)
	if err != nil {
		return
	}
	_ = store.WriteSetting(ctx, bountySettingKey, bountyStateCache)
}

func recordBountyObservation(ctx context.Context, dataDir string, items []ProcessItem, now time.Time) BountySnapshot {
	bountyMu.Lock()
	defer bountyMu.Unlock()

	ensureBountyStateLoadedLocked(ctx, dataDir)

	type aggGroup struct {
		key         string
		title       string
		icon        string
		rep         ProcessItem
		cpu         float64
		hasCPU      bool
		memory      uint64
		gpu         float64
		hasGPU      bool
		gpuMemory   uint64
		network     float64
		hasNetwork  bool
		netConns    int
		ports       map[int]bool
		executables map[string]bool
	}

	activeMap := map[string]*aggGroup{}
	for _, item := range items {
		key := processGroupKeyGo(item)
		g, exists := activeMap[key]
		if !exists {
			g = &aggGroup{
				key:         key,
				title:       friendlyProcessLabelGo(item.Name, item.DisplayName),
				icon:        item.Icon,
				rep:         item,
				ports:       map[int]bool{},
				executables: map[string]bool{},
			}
			activeMap[key] = g
		}
		if g.icon == "" && item.Icon != "" {
			g.icon = item.Icon
		}
		if item.MemoryBytes > g.rep.MemoryBytes {
			g.rep = item
		}
		g.memory += item.MemoryBytes
		if item.CPUPercent != nil {
			g.cpu += *item.CPUPercent
			g.hasCPU = true
		}
		if item.GPUPercent != nil {
			g.gpu += *item.GPUPercent
			g.hasGPU = true
		}
		g.gpuMemory += item.GPUMemoryBytes
		if item.NetworkBytesPerSec != nil {
			g.network += *item.NetworkBytesPerSec
			g.hasNetwork = true
		}
		g.netConns += item.NetworkConnections
		for _, p := range item.Ports {
			g.ports[p] = true
		}
		if item.Executable != "" {
			g.executables[strings.ToLower(item.Executable)] = true
		}
	}

	cutoffRing := now.Add(-bountyRingMaxAge)
	cutoffWin := now.Add(-bountyWindowDuration)
	nowISO := now.UTC().Format(time.RFC3339Nano)
	stateDirty := false

	summaries := make(map[string]BountyGroupSummary, len(activeMap))

	for key, g := range activeMap {
		if g.hasCPU && g.cpu > 100 {
			g.cpu = 100
		}
		if g.hasGPU && g.gpu > 100 {
			g.gpu = 100
		}
		hist, ok := bountyRingByGroup[key]
		if !ok {
			hist = &bountyGroupHistory{}
			bountyRingByGroup[key] = hist
		}
		if !hist.lastSampleAt.IsZero() && g.hasNetwork && g.network > 0 {
			dt := now.Sub(hist.lastSampleAt).Seconds()
			if dt > 0 && dt <= 15 {
				hist.cumulativeNetBytes += uint64(g.network * dt)
			}
		}
		hist.lastSampleAt = now
		hist.latestRep = g.rep
		hist.latestTitle = g.title
		hist.latestIcon = g.icon
		portList := make([]int, 0, len(g.ports))
		for p := range g.ports {
			portList = append(portList, p)
		}
		sort.Ints(portList)
		hist.latestPorts = portList

		pt := bountySamplePoint{
			at:             now,
			cpu:            g.cpu,
			hasCPU:         g.hasCPU,
			memory:         g.memory,
			gpu:            g.gpu,
			hasGPU:         g.hasGPU,
			gpuMemory:      g.gpuMemory,
			network:        g.network,
			hasNetwork:     g.hasNetwork,
			netConnections: g.netConns,
		}
		kept := hist.samples[:0]
		for _, s := range hist.samples {
			if !s.at.Before(cutoffRing) {
				kept = append(kept, s)
			}
		}
		kept = append(kept, pt)
		if len(kept) > bountyRingMaxSamples {
			kept = kept[len(kept)-bountyRingMaxSamples:]
		}
		hist.samples = kept

		// Compute 30s rolling window averages
		var (
			winCPUSum, winGPUSum, winNetSum          float64
			winCPUCount, winGPUCount, winNetCount    int
			winMemSum, winGPUMemSum                  uint64
			winSampleCount                           int
			recentCPUSum, recentGPUSum, recentNetSum float64
			recentCPUCnt, recentGPUCnt, recentNetCnt int
			recentMemSum                             uint64
			recentCnt                                int
			baseCPUSum, baseGPUSum, baseNetSum       float64
			baseCPUCnt, baseGPUCnt, baseNetCnt       int
			baseMemMin                               uint64
			baseCnt                                  int
		)
		recentCutoff := now.Add(-10 * time.Second)
		baseCutoff := now.Add(-180 * time.Second)

		for _, s := range hist.samples {
			if !s.at.Before(cutoffWin) {
				winSampleCount++
				winMemSum += s.memory
				winGPUMemSum += s.gpuMemory
				if s.hasCPU {
					winCPUSum += s.cpu
					winCPUCount++
				}
				if s.hasGPU {
					winGPUSum += s.gpu
					winGPUCount++
				}
				if s.hasNetwork {
					winNetSum += s.network
					winNetCount++
				}
			}
			if !s.at.Before(recentCutoff) {
				recentCnt++
				recentMemSum += s.memory
				if s.hasCPU {
					recentCPUSum += s.cpu
					recentCPUCnt++
				}
				if s.hasGPU {
					recentGPUSum += s.gpu
					recentGPUCnt++
				}
				if s.hasNetwork {
					recentNetSum += s.network
					recentNetCnt++
				}
			} else if !s.at.Before(baseCutoff) {
				baseCnt++
				if baseMemMin == 0 || s.memory < baseMemMin {
					baseMemMin = s.memory
				}
				if s.hasCPU {
					baseCPUSum += s.cpu
					baseCPUCnt++
				}
				if s.hasGPU {
					baseGPUSum += s.gpu
					baseGPUCnt++
				}
				if s.hasNetwork {
					baseNetSum += s.network
					baseNetCnt++
				}
			}
		}

		// Fallback baseline if history is between 4s and 10s old
		if baseCnt == 0 && len(hist.samples) >= 2 {
			first := hist.samples[0]
			if now.Sub(first.at) >= 4*time.Second {
				baseCnt = 1
				baseMemMin = first.memory
				if first.hasCPU {
					baseCPUSum = first.cpu
					baseCPUCnt = 1
				}
				if first.hasGPU {
					baseGPUSum = first.gpu
					baseGPUCnt = 1
				}
				if first.hasNetwork {
					baseNetSum = first.network
					baseNetCnt = 1
				}
			}
		}

		var winCPU, winGPU, winNet *float64
		if winCPUCount > 0 {
			v := winCPUSum / float64(winCPUCount)
			winCPU = &v
		}
		if winGPUCount > 0 {
			v := winGPUSum / float64(winGPUCount)
			winGPU = &v
		}
		if winNetCount > 0 {
			v := winNetSum / float64(winNetCount)
			winNet = &v
		}
		winMem := g.memory
		winGPUMem := g.gpuMemory
		if winSampleCount > 0 {
			winMem = winMemSum / uint64(winSampleCount)
			winGPUMem = winGPUMemSum / uint64(winSampleCount)
		}

		var cpuDelta, gpuDelta, netDelta float64
		var memDelta int64
		var rising []string

		if recentCPUCnt > 0 && baseCPUCnt > 0 {
			recAvg := recentCPUSum / float64(recentCPUCnt)
			baseAvg := baseCPUSum / float64(baseCPUCnt)
			if recAvg-baseAvg >= 20 && recAvg >= 25 {
				cpuDelta = recAvg - baseAvg
				rising = append(rising, "cpu")
			}
		}
		if recentCnt > 0 && baseCnt > 0 && baseMemMin > 0 {
			recMem := recentMemSum / uint64(recentCnt)
			if recMem > baseMemMin {
				diff := int64(recMem - baseMemMin)
				// Rapid rise: +128 MB and at least +20% over baseline
				if diff >= 128*1024*1024 && float64(diff)/float64(baseMemMin) >= 0.20 {
					memDelta = diff
					rising = append(rising, "memory")
				}
			}
		}
		if recentGPUCnt > 0 && baseGPUCnt > 0 {
			recAvg := recentGPUSum / float64(recentGPUCnt)
			baseAvg := baseGPUSum / float64(baseGPUCnt)
			if recAvg-baseAvg >= 20 && recAvg >= 25 {
				gpuDelta = recAvg - baseAvg
				rising = append(rising, "gpu")
			}
		}
		if recentNetCnt > 0 && baseNetCnt > 0 {
			recAvg := recentNetSum / float64(recentNetCnt)
			baseAvg := baseNetSum / float64(baseNetCnt)
			if recAvg-baseAvg >= 512*1024 && recAvg >= 512*1024 {
				netDelta = recAvg - baseAvg
				rising = append(rising, "network")
			}
		}

		summary := BountyGroupSummary{
			GroupKey:           key,
			WindowCPU:          winCPU,
			WindowMemory:       winMem,
			WindowGPU:          winGPU,
			WindowGPUMemory:    winGPUMem,
			WindowNetwork:      winNet,
			CumulativeNetBytes: hist.cumulativeNetBytes,
			CPUDelta:           cpuDelta,
			MemoryDeltaBytes:   memDelta,
			GPUDelta:           gpuDelta,
			NetworkDeltaBps:    netDelta,
			RisingCategories:   rising,
		}

		// Check user watch rules against this group
		for wi := range bountyStateCache.Watches {
			w := &bountyStateCache.Watches[wi]
			if !w.Enabled {
				continue
			}
			if !watchMatchesGroup(*w, g.title, g.rep, g.executables) {
				continue
			}
			summary.Watched = true
			triggered, currentVal, deltaVal := evaluateWatchRule(*w, summary, g.cpu, g.memory, g.gpu, g.network)
			if triggered {
				summary.WatchTriggered = true
				shouldBumpHit := true
				if w.LastTriggeredAt != "" {
					if lastT, err := time.Parse(time.RFC3339Nano, w.LastTriggeredAt); err == nil {
						if now.Sub(lastT) < 45*time.Second {
							shouldBumpHit = false
						}
					}
				}
				if shouldBumpHit {
					w.HitCount++
					w.UnreadHit = true
				}
				if currentVal > w.LastPeakValue || shouldBumpHit {
					w.LastPeakValue = currentVal
				}
				w.LastTriggeredAt = nowISO
				w.LastPID = g.rep.PID
				upsertBountyEventLocked(
					key,
					w.Category,
					"watch_triggered",
					w.ID,
					g.title,
					g.icon,
					g.rep,
					portList,
					currentVal,
					deltaVal,
					hist.cumulativeNetBytes,
					g.memory,
					winCPU,
					winGPU,
					g.gpuMemory,
					winNet,
					g.netConns,
					now,
					nowISO,
				)
				stateDirty = true
			}
		}

		// Automatic peak / rapid-rise recording
		// 1. Network peak (>= 1 MB/s or cumulative >= 25 MB or rapid rise)
		if (g.hasNetwork && g.network >= 1024*1024) || hist.cumulativeNetBytes >= 25*1024*1024 || netDelta >= 512*1024 {
			reason := "peak_high"
			if netDelta >= 512*1024 {
				reason = "rapid_rise"
			}
			val := g.network
			if winNet != nil && *winNet > val {
				val = *winNet
			}
			upsertBountyEventLocked(
				key, "network", reason, "", g.title, g.icon, g.rep, portList,
				val, netDelta, hist.cumulativeNetBytes, g.memory, winCPU, winGPU, g.gpuMemory, winNet, g.netConns, now, nowISO,
			)
			stateDirty = true
		}
		// 2. CPU peak (>= 35% window/current or rapid rise >= 20%)
		if (winCPU != nil && *winCPU >= 35) || cpuDelta >= 20 {
			reason := "peak_high"
			if cpuDelta >= 20 {
				reason = "rapid_rise"
			}
			val := g.cpu
			if winCPU != nil && *winCPU > val {
				val = *winCPU
			}
			upsertBountyEventLocked(
				key, "cpu", reason, "", g.title, g.icon, g.rep, portList,
				val, cpuDelta, hist.cumulativeNetBytes, g.memory, winCPU, winGPU, g.gpuMemory, winNet, g.netConns, now, nowISO,
			)
			stateDirty = true
		}
		// 3. Memory rapid rise (>= +128 MB) or high memory (>= 1.2 GB)
		if memDelta >= 128*1024*1024 || g.memory >= 1200*1024*1024 {
			reason := "peak_high"
			if memDelta >= 128*1024*1024 {
				reason = "rapid_rise"
			}
			upsertBountyEventLocked(
				key, "memory", reason, "", g.title, g.icon, g.rep, portList,
				float64(g.memory), float64(memDelta), hist.cumulativeNetBytes, g.memory, winCPU, winGPU, g.gpuMemory, winNet, g.netConns, now, nowISO,
			)
			stateDirty = true
		}
		// 4. GPU peak (>= 30% or VRAM >= 512 MB or rapid rise >= 20%)
		if (winGPU != nil && *winGPU >= 30) || g.gpuMemory >= 512*1024*1024 || gpuDelta >= 20 {
			reason := "peak_high"
			if gpuDelta >= 20 {
				reason = "rapid_rise"
			}
			val := g.gpu
			if winGPU != nil && *winGPU > val {
				val = *winGPU
			}
			upsertBountyEventLocked(
				key, "gpu", reason, "", g.title, g.icon, g.rep, portList,
				val, gpuDelta, hist.cumulativeNetBytes, g.memory, winCPU, winGPU, g.gpuMemory, winNet, g.netConns, now, nowISO,
			)
			stateDirty = true
		}

		summaries[key] = summary
	}

	// Mark exited processes on stored events
	for i := range bountyStateCache.Events {
		ev := &bountyStateCache.Events[i]
		_, alive := activeMap[ev.GroupKey]
		if ev.ProcessExited == alive {
			ev.ProcessExited = !alive
			stateDirty = true
		}
	}

	// Prune stale ring buffer groups not seen in 10 minutes
	for k, h := range bountyRingByGroup {
		if now.Sub(h.lastSampleAt) > bountyRingMaxAge {
			delete(bountyRingByGroup, k)
		}
	}

	if stateDirty {
		saveBountyStateLocked(ctx, dataDir)
	}

	return buildBountySnapshotLocked(summaries)
}

func watchMatchesGroup(w BountyWatch, groupTitle string, rep ProcessItem, executables map[string]bool) bool {
	if w.MatchKind == "command" && w.CommandID != "" {
		return rep.CommandID == w.CommandID
	}
	if w.Executable != "" {
		exeLower := strings.ToLower(strings.TrimSpace(w.Executable))
		if executables[exeLower] || strings.EqualFold(rep.Executable, exeLower) {
			return true
		}
	}
	target := strings.ToLower(strings.TrimSpace(w.TargetName))
	if target == "" {
		return false
	}
	if strings.ToLower(groupTitle) == target || strings.ToLower(rep.Name) == target || strings.ToLower(rep.DisplayName) == target {
		return true
	}
	return strings.Contains(strings.ToLower(groupTitle), target) || strings.Contains(strings.ToLower(rep.Name), target)
}

func evaluateWatchRule(
	w BountyWatch,
	s BountyGroupSummary,
	curCPU float64,
	curMem uint64,
	curGPU float64,
	curNet float64,
) (bool, float64, float64) {
	switch w.Category {
	case "network":
		val := curNet
		if s.WindowNetwork != nil && *s.WindowNetwork > val {
			val = *s.WindowNetwork
		}
		if w.ConditionKind == "rapid_rise" {
			return s.NetworkDeltaBps >= w.Threshold, val, s.NetworkDeltaBps
		}
		if w.ConditionKind == "cumulative" {
			return float64(s.CumulativeNetBytes) >= w.Threshold, float64(s.CumulativeNetBytes), s.NetworkDeltaBps
		}
		return val >= w.Threshold, val, s.NetworkDeltaBps
	case "memory":
		val := float64(curMem)
		if float64(s.WindowMemory) > val {
			val = float64(s.WindowMemory)
		}
		if w.ConditionKind == "rapid_rise" {
			return float64(s.MemoryDeltaBytes) >= w.Threshold, val, float64(s.MemoryDeltaBytes)
		}
		return val >= w.Threshold, val, float64(s.MemoryDeltaBytes)
	case "cpu":
		val := curCPU
		if s.WindowCPU != nil && *s.WindowCPU > val {
			val = *s.WindowCPU
		}
		if w.ConditionKind == "rapid_rise" {
			return s.CPUDelta >= w.Threshold, val, s.CPUDelta
		}
		return val >= w.Threshold, val, s.CPUDelta
	case "gpu":
		val := curGPU
		if s.WindowGPU != nil && *s.WindowGPU > val {
			val = *s.WindowGPU
		}
		if w.ConditionKind == "rapid_rise" {
			return s.GPUDelta >= w.Threshold, val, s.GPUDelta
		}
		return val >= w.Threshold, val, s.GPUDelta
	}
	return false, 0, 0
}

func copyFloatPtr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func upsertBountyEventLocked(
	groupKey string,
	category string,
	reasonKind string,
	watchID string,
	title string,
	icon string,
	rep ProcessItem,
	ports []int,
	peakVal float64,
	deltaVal float64,
	cumBytes uint64,
	memBytes uint64,
	winCPU *float64,
	winGPU *float64,
	gpuMem uint64,
	winNet *float64,
	netConns int,
	now time.Time,
	nowISO string,
) {
	for i := range bountyStateCache.Events {
		ev := &bountyStateCache.Events[i]
		if ev.GroupKey == groupKey && ev.Category == category {
			// Update existing event for this group + category
			if peakVal >= ev.PeakValue {
				ev.PeakValue = peakVal
				ev.PeakAt = nowISO
			}
			if deltaVal > ev.DeltaValue {
				ev.DeltaValue = deltaVal
			}
			if reasonKind == "watch_triggered" || (reasonKind == "rapid_rise" && ev.ReasonKind == "peak_high") {
				ev.ReasonKind = reasonKind
			}
			if watchID != "" {
				ev.WatchID = watchID
				ev.Acknowledged = false
			}
			if cumBytes > ev.CumulativeBytes {
				ev.CumulativeBytes = cumBytes
			}
			if memBytes > ev.MemoryBytes {
				ev.MemoryBytes = memBytes
			}
			if winCPU != nil {
				ev.CPUPercent = copyFloatPtr(winCPU)
			}
			if winGPU != nil {
				ev.GPUPercent = copyFloatPtr(winGPU)
			}
			if gpuMem > ev.GPUMemoryBytes {
				ev.GPUMemoryBytes = gpuMem
			}
			if winNet != nil {
				ev.NetworkBytesPerSec = copyFloatPtr(winNet)
			}
			if netConns > ev.NetworkConnections {
				ev.NetworkConnections = netConns
			}
			ev.Title = title
			ev.ProcessName = rep.Name
			if icon != "" {
				ev.Icon = icon
			}
			ev.PID = rep.PID
			ev.ProcessIdentity = rep.ProcessIdentity
			ev.ControlRunID = rep.ControlRunID
			if rep.Executable != "" {
				ev.Executable = rep.Executable
			}
			if rep.CommandLine != "" {
				ev.CommandLine = rep.CommandLine
				ev.CommandLineState = rep.CommandLineState
			}
			ev.Ownership = rep.Ownership
			ev.RunID = rep.RunID
			ev.RunStatus = rep.RunStatus
			ev.CommandID = rep.CommandID
			ev.CommandTitle = rep.CommandTitle
			ev.ProjectID = rep.ProjectID
			ev.ProjectName = rep.ProjectName
			ev.WorkspaceID = rep.WorkspaceID
			ev.WorkspaceName = rep.WorkspaceName
			ev.Directory = rep.Directory
			if len(ports) > 0 {
				ev.Ports = ports
			}
			ev.LastObservedAt = nowISO
			if firstT, err := time.Parse(time.RFC3339Nano, ev.FirstObservedAt); err == nil {
				sec := int(math.Max(2, now.Sub(firstT).Seconds()))
				ev.DurationSec = sec
			}
			ev.ProcessExited = false
			return
		}
	}

	newEv := BountyEvent{
		ID:                 engine.ID(),
		GroupKey:           groupKey,
		Category:           category,
		ReasonKind:         reasonKind,
		WatchID:            watchID,
		Title:              title,
		ProcessName:        rep.Name,
		Icon:               icon,
		PID:                rep.PID,
		ProcessIdentity:    rep.ProcessIdentity,
		ControlRunID:       rep.ControlRunID,
		Executable:         rep.Executable,
		CommandLine:        rep.CommandLine,
		CommandLineState:   rep.CommandLineState,
		Ownership:          rep.Ownership,
		RunID:              rep.RunID,
		RunStatus:          rep.RunStatus,
		CommandID:          rep.CommandID,
		CommandTitle:       rep.CommandTitle,
		ProjectID:          rep.ProjectID,
		ProjectName:        rep.ProjectName,
		WorkspaceID:        rep.WorkspaceID,
		WorkspaceName:      rep.WorkspaceName,
		Directory:          rep.Directory,
		Ports:              ports,
		PeakValue:          peakVal,
		DeltaValue:         deltaVal,
		CumulativeBytes:    cumBytes,
		MemoryBytes:        memBytes,
		CPUPercent:         copyFloatPtr(winCPU),
		GPUPercent:         copyFloatPtr(winGPU),
		GPUMemoryBytes:     gpuMem,
		NetworkBytesPerSec: copyFloatPtr(winNet),
		NetworkConnections: netConns,
		FirstObservedAt:    nowISO,
		PeakAt:             nowISO,
		LastObservedAt:     nowISO,
		DurationSec:        2,
		ProcessExited:      false,
	}
	bountyStateCache.Events = append([]BountyEvent{newEv}, bountyStateCache.Events...)
	trimBountyEventsLocked()
}

func trimBountyEventsLocked() {
	if len(bountyStateCache.Events) <= bountyMaxEvents {
		return
	}
	// Keep top events per category plus most recent
	byCat := map[string][]BountyEvent{}
	for _, ev := range bountyStateCache.Events {
		byCat[ev.Category] = append(byCat[ev.Category], ev)
	}
	kept := make([]BountyEvent, 0, bountyMaxEvents)
	for _, list := range byCat {
		sort.Slice(list, func(i, j int) bool {
			if list[i].ReasonKind != list[j].ReasonKind {
				if list[i].ReasonKind == "watch_triggered" {
					return true
				}
				if list[j].ReasonKind == "watch_triggered" {
					return false
				}
			}
			if list[i].PeakValue != list[j].PeakValue {
				return list[i].PeakValue > list[j].PeakValue
			}
			return list[i].LastObservedAt > list[j].LastObservedAt
		})
		limit := 12
		if len(list) < limit {
			limit = len(list)
		}
		kept = append(kept, list[:limit]...)
	}
	sort.Slice(kept, func(i, j int) bool {
		return kept[i].LastObservedAt > kept[j].LastObservedAt
	})
	if len(kept) > bountyMaxEvents {
		kept = kept[:bountyMaxEvents]
	}
	bountyStateCache.Events = kept
}

func buildBountySnapshotLocked(groups map[string]BountyGroupSummary) BountySnapshot {
	if groups == nil {
		groups = map[string]BountyGroupSummary{}
	}
	events := make([]BountyEvent, len(bountyStateCache.Events))
	copy(events, bountyStateCache.Events)
	watches := make([]BountyWatch, len(bountyStateCache.Watches))
	copy(watches, bountyStateCache.Watches)

	unread := 0
	for _, w := range watches {
		if w.UnreadHit {
			unread++
		}
	}
	return BountySnapshot{
		Groups:           groups,
		Events:           events,
		Watches:          watches,
		UnreadAlertCount: unread,
	}
}

func (h *Host) serveResourcesBounties(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	bountyMu.Lock()
	ensureBountyStateLoadedLocked(r.Context(), h.dataDir)

	if r.Method == http.MethodGet {
		snap := buildBountySnapshotLocked(nil)
		bountyMu.Unlock()
		json.NewEncoder(w).Encode(snap)
		return
	}
	if r.Method != http.MethodPost {
		bountyMu.Unlock()
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}

	var req struct {
		Action        string  `json:"action"`
		ID            string  `json:"id,omitempty"`
		Title         string  `json:"title,omitempty"`
		MatchKind     string  `json:"matchKind,omitempty"`
		TargetName    string  `json:"targetName,omitempty"`
		Executable    string  `json:"executable,omitempty"`
		CommandID     string  `json:"commandId,omitempty"`
		ProjectID     string  `json:"projectId,omitempty"`
		ProjectName   string  `json:"projectName,omitempty"`
		Category      string  `json:"category,omitempty"`
		ConditionKind string  `json:"conditionKind,omitempty"`
		Threshold     float64 `json:"threshold,omitempty"`
		NotifyToast   bool    `json:"notifyToast,omitempty"`
		Enabled       bool    `json:"enabled,omitempty"`
		Note          string  `json:"note,omitempty"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || d.Decode(&struct{}{}) != io.EOF {
		bountyMu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}

	nowISO := time.Now().UTC().Format(time.RFC3339Nano)

	switch req.Action {
	case "create_watch":
		targetName := strings.TrimSpace(req.TargetName)
		if targetName == "" {
			bountyMu.Unlock()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
			return
		}
		cat := req.Category
		if cat != "network" && cat != "cpu" && cat != "memory" && cat != "gpu" {
			cat = "network"
		}
		cond := req.ConditionKind
		if cond != "threshold" && cond != "rapid_rise" && cond != "cumulative" {
			cond = "threshold"
		}
		matchKind := req.MatchKind
		if matchKind != "command" && matchKind != "process" {
			matchKind = "process"
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = targetName
		}
		// Update if identical watch target + category + condition already exists
		updated := false
		for i := range bountyStateCache.Watches {
			existing := &bountyStateCache.Watches[i]
			if strings.EqualFold(existing.TargetName, targetName) &&
				existing.Category == cat &&
				existing.ConditionKind == cond &&
				existing.CommandID == req.CommandID {
				existing.Title = title
				existing.Threshold = req.Threshold
				existing.NotifyToast = req.NotifyToast
				existing.Enabled = true
				existing.Note = strings.TrimSpace(req.Note)
				if req.Executable != "" {
					existing.Executable = req.Executable
				}
				updated = true
				break
			}
		}
		if !updated {
			watch := BountyWatch{
				ID:            engine.ID(),
				Title:         title,
				MatchKind:     matchKind,
				TargetName:    targetName,
				Executable:    strings.TrimSpace(req.Executable),
				CommandID:     req.CommandID,
				ProjectID:     req.ProjectID,
				ProjectName:   req.ProjectName,
				Category:      cat,
				ConditionKind: cond,
				Threshold:     req.Threshold,
				NotifyToast:   req.NotifyToast,
				Enabled:       true,
				Note:          strings.TrimSpace(req.Note),
				CreatedAt:     nowISO,
			}
			bountyStateCache.Watches = append([]BountyWatch{watch}, bountyStateCache.Watches...)
			if len(bountyStateCache.Watches) > bountyMaxWatches {
				bountyStateCache.Watches = bountyStateCache.Watches[:bountyMaxWatches]
			}
		}
		saveBountyStateLocked(r.Context(), h.dataDir)

	case "toggle_watch":
		for i := range bountyStateCache.Watches {
			if bountyStateCache.Watches[i].ID == req.ID {
				bountyStateCache.Watches[i].Enabled = req.Enabled
				if !req.Enabled {
					bountyStateCache.Watches[i].UnreadHit = false
				}
				break
			}
		}
		saveBountyStateLocked(r.Context(), h.dataDir)

	case "delete_watch":
		filtered := bountyStateCache.Watches[:0]
		for _, wItem := range bountyStateCache.Watches {
			if wItem.ID != req.ID {
				filtered = append(filtered, wItem)
			}
		}
		bountyStateCache.Watches = filtered
		saveBountyStateLocked(r.Context(), h.dataDir)

	case "ack_alerts":
		for i := range bountyStateCache.Watches {
			if req.ID == "" || bountyStateCache.Watches[i].ID == req.ID {
				bountyStateCache.Watches[i].UnreadHit = false
			}
		}
		for i := range bountyStateCache.Events {
			if req.ID == "" || bountyStateCache.Events[i].ID == req.ID || bountyStateCache.Events[i].WatchID == req.ID {
				bountyStateCache.Events[i].Acknowledged = true
			}
		}
		saveBountyStateLocked(r.Context(), h.dataDir)

	case "clear_events":
		if req.ID != "" {
			filtered := bountyStateCache.Events[:0]
			for _, ev := range bountyStateCache.Events {
				if ev.ID != req.ID {
					filtered = append(filtered, ev)
				}
			}
			bountyStateCache.Events = filtered
		} else if req.Category != "" {
			filtered := bountyStateCache.Events[:0]
			for _, ev := range bountyStateCache.Events {
				if ev.Category != req.Category {
					filtered = append(filtered, ev)
				}
			}
			bountyStateCache.Events = filtered
		} else {
			bountyStateCache.Events = []BountyEvent{}
		}
		saveBountyStateLocked(r.Context(), h.dataDir)

	default:
		bountyMu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}

	snap := buildBountySnapshotLocked(nil)
	bountyMu.Unlock()
	json.NewEncoder(w).Encode(snap)
}
