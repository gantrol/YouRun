package desktop

import (
	"strconv"
	"strings"
)

// RuntimeHost describes attribution only. It never replaces a control target,
// its process identity, or the independently verified ControlRunID.
type RuntimeHost struct {
	PID             int    `json:"pid"`
	ProcessIdentity string `json:"processIdentity"`
	Name            string `json:"name"`
	DisplayName     string `json:"displayName,omitempty"`
	Executable      string `json:"executable,omitempty"`
}

func isWebViewProcess(p rawProcess) bool {
	return strings.EqualFold(p.Name, "msedgewebview2.exe")
}

// Resolve from the existing OS snapshot; no additional process/file probes.
// Missing parents, unknown start times, PID reuse and cycles stop attribution.
// An unresolved tree stays separate instead of joining all WebView processes.
func attributeRuntimeGroup(item *ProcessItem, procs map[int]rawProcess, observedAt string) {
	current := procs[item.PID]
	if !isWebViewProcess(current) {
		return
	}
	root := current
	seen := map[int]bool{current.PID: true}
	for depth := 0; depth < 64; depth++ {
		parent, ok := procs[current.PPID]
		if !ok || parent.PID <= 0 || seen[parent.PID] || current.StartedAt.IsZero() || parent.StartedAt.IsZero() || parent.StartedAt.After(current.StartedAt) {
			break
		}
		seen[parent.PID] = true
		if !isWebViewProcess(parent) {
			item.RuntimeHost = &RuntimeHost{
				PID: parent.PID, ProcessIdentity: processIdentity(parent),
				Name: parent.Name, DisplayName: parent.DisplayName, Executable: parent.Executable,
			}
			break
		}
		root, current = parent, parent
	}
	item.RuntimeRootPID = root.PID
	scope := item.Ownership
	if item.RunID != "" {
		scope += ":" + item.RunID
	}
	identity := processIdentity(root)
	key := "tree:" + strconv.Itoa(root.PID) + ":" + identity
	if host := item.RuntimeHost; host != nil {
		key = "host:" + strconv.Itoa(host.PID) + ":" + host.ProcessIdentity
	} else if identity == "" {
		// Without a start identity, do not join history across PID generations.
		key += ":" + observedAt
	}
	item.GroupKey = "runtime:webview2:" + scope + ":" + key
}

func processGroupTitle(item ProcessItem) string {
	if host := item.RuntimeHost; host != nil {
		return "WebView2 · " + friendlyProcessLabelGo(host.Name, host.DisplayName)
	}
	if item.RuntimeRootPID > 0 {
		return "WebView2 · PID " + strconv.Itoa(item.RuntimeRootPID)
	}
	return friendlyProcessLabelGo(item.Name, item.DisplayName)
}
