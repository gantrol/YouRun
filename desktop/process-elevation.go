package desktop

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/processcontrol"
)

type elevatedControl struct {
	Targets []processcontrol.Target `json:"targets"`
	Mode    string                  `json:"mode"`
	Caller  processcontrol.Target   `json:"caller"`
}

// The elevated helper accepts one narrow operation from a live caller over an
// authenticated loopback connection. It reads/writes no user-supplied files and
// never launches a user-supplied command or modifies application privileges.
func processControlHelper(args []string) (bool, int) {
	if len(args) == 0 || args[0] != "_process-control" {
		return false, 0
	}
	if len(args) != 3 {
		return true, 2
	}
	u, err := url.Parse(args[1])
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "/control" || u.RawQuery != "" || u.Fragment != "" || len(args[2]) < 20 {
		return true, 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	request.Header.Set("Authorization", "Bearer "+args[2])
	response, err := client.Do(request)
	if err != nil {
		return true, 1
	}
	var operation elevatedControl
	d := json.NewDecoder(io.LimitReader(response.Body, 128<<10))
	d.DisallowUnknownFields()
	err = d.Decode(&operation)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || len(operation.Targets) == 0 || len(operation.Targets) > 512 || (operation.Mode != "force" && operation.Mode != "graceful") {
		return true, 2
	}
	if processcontrol.Verify(operation.Caller) != nil {
		return true, 1
	}
	for _, target := range operation.Targets {
		if target.PID == operation.Caller.PID || target.PID == os.Getpid() || target.PID == os.Getppid() {
			return true, 2
		}
	}
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if processcontrol.Verify(operation.Caller) != nil {
					cancel()
					return
				}
			}
		}
	}()
	results := processcontrol.Apply(ctx, operation.Targets, operation.Mode)
	data, _ := json.Marshal(results)
	request, _ = http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(data))
	request.Header.Set("Authorization", "Bearer "+args[2])
	response, err = client.Do(request)
	if err != nil {
		return true, 1
	}
	response.Body.Close()
	return true, 0
}

func elevateProcessControl(ctx context.Context, targets []processcontrol.Target, mode string) []processcontrol.Result {
	failure := func(code string) []processcontrol.Result {
		results := make([]processcontrol.Result, len(targets))
		for i, t := range targets {
			results[i] = processcontrol.Result{Target: t, Status: code}
		}
		return results
	}
	if !canElevateProcessControl() {
		return failure("processElevationUnsupported")
	}
	caller, err := processcontrol.Observe(os.Getpid())
	if err != nil {
		return failure("processControlFailed")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return failure("processControlFailed")
	}
	token := engine.ID() + engine.ID()
	done := make(chan []processcontrol.Result, 1)
	var claimed atomic.Bool
	server := &http.Server{ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil {
			http.Error(w, "expired", http.StatusGone)
			return
		}
		if r.Host != listener.Addr().String() || r.URL.Path != "/control" || r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "forbidden", 403)
			return
		}
		if r.Method == http.MethodGet && claimed.CompareAndSwap(false, true) {
			json.NewEncoder(w).Encode(elevatedControl{targets, mode, caller})
			return
		}
		if r.Method != http.MethodPost || !claimed.Load() {
			http.Error(w, "invalid", 400)
			return
		}
		var results []processcontrol.Result
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
		d.DisallowUnknownFields()
		if d.Decode(&results) != nil || len(results) != len(targets) {
			http.Error(w, "invalid", 400)
			return
		}
		for i, item := range results {
			if item.PID != targets[i].PID || item.StartedAt != targets[i].StartedAt || !strings.HasPrefix(item.Status, "process") {
				http.Error(w, "invalid", 400)
				return
			}
		}
		select {
		case done <- results:
		default:
		}
		w.WriteHeader(204)
	})}
	go server.Serve(listener)
	defer server.Close()
	closeHandle, err := launchElevatedControl("http://"+listener.Addr().String()+"/control", token)
	if err != nil {
		return failure(err.Error())
	}
	defer closeHandle()
	select {
	case results := <-done:
		return results
	case <-ctx.Done():
		return failure("processExitTimeout")
	}
}
