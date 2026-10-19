//go:build windows && (amd64 || arm64)

package desktop

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gofrs/flock"
	"golang.org/x/sys/windows"
)

const (
	networkTraceIdle  = 15 * time.Second
	traceControlQuery = 0
	traceControlStop  = 1
)

var (
	networkTraceDLL     = windows.NewLazySystemDLL("advapi32.dll")
	startNetworkTrace   = networkTraceDLL.NewProc("StartTraceW")
	controlNetworkTrace = networkTraceDLL.NewProc("ControlTraceW")
	openNetworkTrace    = networkTraceDLL.NewProc("OpenTraceW")
	processNetworkTrace = networkTraceDLL.NewProc("ProcessTrace")
	closeNetworkTrace   = networkTraceDLL.NewProc("CloseTrace")
	networkCallback     = windows.NewCallback(consumeNetworkEvent)
	tcpNetworkGUID      = windows.GUID{Data1: 0x9a280ac0, Data2: 0xc8e0, Data3: 0x11d1, Data4: [8]byte{0x84, 0xe2, 0x00, 0xc0, 0x4f, 0xb9, 0x98, 0xa2}}
	udpNetworkGUID      = windows.GUID{Data1: 0xbf3a50c5, Data2: 0xa9c9, Data3: 0x4988, Data4: [8]byte{0xa0, 0x05, 0x2d, 0xf0, 0xb7, 0xc8, 0x0f, 0x80}}
	networkTraceMu      sync.Mutex
	activeNetworkTrace  atomic.Pointer[processNetworkSampler]
	networkRetryAt      time.Time
)

// These layouts follow evntrace.h/evntcons.h on Windows amd64 and arm64.
// Reserved fields retain the SDK's size and 8-byte alignment.
type networkTraceProperties struct {
	Size, ProviderID                                            uint32
	HistoricalContext, TimeStamp                                uint64
	GUID                                                        windows.GUID
	ClientContext, Flags                                        uint32
	BufferSize, MinimumBuffers, MaximumBuffers, MaximumFileSize uint32
	LogFileMode, FlushTimer, EnableFlags, AgeLimit              uint32
	NumberOfBuffers, FreeBuffers, EventsLost, BuffersWritten    uint32
	LogBuffersLost, RealTimeBuffersLost                         uint32
	LoggerThreadID                                              uintptr
	LogFileNameOffset, LoggerNameOffset                         uint32
	Name                                                        [1024]uint16
}

type networkTraceLogfile struct {
	LogFileName, LoggerName        *uint16
	CurrentTime                    int64
	BuffersRead, ProcessTraceMode  uint32
	CurrentEvent                   [11]uint64
	LogfileHeader                  [35]uint64
	BufferCallback                 uintptr
	BufferSize, Filled, EventsLost uint32
	EventRecordCallback            uintptr
	IsKernelTrace                  uint32
	Context                        uintptr
}

type networkEventRecord struct {
	Size, HeaderType, Flags, EventProperty uint16
	ThreadID, ProcessID                    uint32
	TimeStamp                              int64
	ProviderID                             windows.GUID
	EventID                                uint16
	Version, Channel, Level, Opcode        uint8
	Task                                   uint16
	Keyword, ProcessorTime                 uint64
	ActivityID                             windows.GUID
	BufferContext                          uint32
	ExtendedDataCount, UserDataLength      uint16
	ExtendedData, UserData                 unsafe.Pointer
	UserContext                            uintptr
}

type processNetworkBytes struct {
	startedNs           int64
	total, sampledBytes uint64
	sampledAt           time.Time
}

type processNetworkSampler struct {
	mu                  sync.Mutex
	name                *uint16
	lease               *flock.Flock
	consumer            uintptr
	logfile             *networkTraceLogfile
	done                chan struct{}
	running             atomic.Bool
	startedAt, lastUsed time.Time
	bytes               map[int]processNetworkBytes
	lost, dropped       uint64
}

func newNetworkTraceProperties() *networkTraceProperties {
	p := &networkTraceProperties{}
	p.Size = uint32(unsafe.Sizeof(*p))
	p.LoggerNameOffset = uint32(unsafe.Offsetof(p.Name))
	p.Flags = 0x00020000 // WNODE_FLAG_TRACED_GUID
	p.ClientContext = 1  // Query performance counter; ProcessTrace converts timestamps.
	p.BufferSize, p.MinimumBuffers, p.MaximumBuffers = 64, 4, 32
	p.FlushTimer = 1
	p.LogFileMode = 0x00000100 | 0x02000000 | 0x10000000 // REAL_TIME | SYSTEM_LOGGER | NO_PER_PROCESSOR_BUFFERING
	p.EnableFlags = 0x00010000 | 0x10000000              // NETWORK_TCPIP | NO_SYSCONFIG
	return p
}

func startProcessNetworkSampler() *processNetworkSampler {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil
	}
	lockPath := filepath.Join(os.TempDir(), "YouRun-network-"+user.User.Sid.String()+".lock")
	// A stable, user-specific session and lease allow CLI and GUI consumers to
	// share one system logger. Acquiring an abandoned lease permits cleaning up
	// only our own orphaned session after an abnormal process exit.
	digest := sha256.Sum256([]byte(lockPath))
	name, err := windows.UTF16PtrFromString(fmt.Sprintf("YouRun Network %x", digest[:12]))
	if err != nil {
		return nil
	}
	lease := flock.New(lockPath)
	owned, err := lease.TryLock()
	if err != nil {
		lease.Close()
		return nil
	}
	if owned {
		controlNetworkTrace.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(newNetworkTraceProperties())), traceControlStop)
		var session uint64
		status, _, _ := startNetworkTrace.Call(uintptr(unsafe.Pointer(&session)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(newNetworkTraceProperties())))
		if status != 0 {
			lease.Close()
			return nil
		}
	} else {
		lease.Close()
		lease = nil
	}
	logfile := &networkTraceLogfile{LoggerName: name, ProcessTraceMode: 0x00000100 | 0x10000000, EventRecordCallback: networkCallback}
	consumer, _, _ := openNetworkTrace.Call(uintptr(unsafe.Pointer(logfile)))
	if consumer == ^uintptr(0) {
		if lease != nil {
			controlNetworkTrace.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(newNetworkTraceProperties())), traceControlStop)
			lease.Close()
		}
		return nil
	}
	now := time.Now()
	s := &processNetworkSampler{name: name, lease: lease, consumer: consumer, logfile: logfile, done: make(chan struct{}), startedAt: now, lastUsed: now, bytes: map[int]processNetworkBytes{}}
	s.running.Store(true)
	activeNetworkTrace.Store(s)
	go func() {
		processNetworkTrace.Call(uintptr(unsafe.Pointer(&s.consumer)), 1, 0, 0)
		s.running.Store(false)
		close(s.done)
	}()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				networkTraceMu.Lock()
				if activeNetworkTrace.Load() == s {
					stopProcessNetworkSampler(s)
				}
				networkTraceMu.Unlock()
				return
			case <-ticker.C:
				networkTraceMu.Lock()
				idle := time.Since(s.lastUsed) >= networkTraceIdle
				if activeNetworkTrace.Load() == s && idle {
					stopProcessNetworkSampler(s)
				}
				networkTraceMu.Unlock()
				if idle {
					return
				}
			}
		}
	}()
	return s
}

func primeOSNetworkMetrics() {
	networkTraceMu.Lock()
	defer networkTraceMu.Unlock()
	s := activeNetworkTrace.Load()
	if s != nil && !s.running.Load() {
		stopProcessNetworkSampler(s)
		s = nil
	}
	if s == nil && !time.Now().Before(networkRetryAt) {
		s = startProcessNetworkSampler()
		if s == nil {
			// Access denied is expected for an unelevated process. Never substitute
			// disk/pipe/device I/O, and never prompt for elevation just to sample.
			networkRetryAt = time.Now().Add(30 * time.Second)
		}
	}
	if s != nil {
		s.lastUsed = time.Now()
	}
}

func stopProcessNetworkSampler(s *processNetworkSampler) {
	activeNetworkTrace.Store(nil)
	if s.lease != nil {
		controlNetworkTrace.Call(0, uintptr(unsafe.Pointer(s.name)), uintptr(unsafe.Pointer(newNetworkTraceProperties())), traceControlStop)
	}
	closeNetworkTrace.Call(s.consumer)
	<-s.done
	if s.lease != nil {
		s.lease.Close()
	}
}

func closeOSNetworkMetrics() {
	networkTraceMu.Lock()
	defer networkTraceMu.Unlock()
	if s := activeNetworkTrace.Load(); s != nil {
		stopProcessNetworkSampler(s)
	}
}

func consumeNetworkEvent(event *networkEventRecord) uintptr {
	s := activeNetworkTrace.Load()
	if s == nil || event == nil || event.UserData == nil || event.UserDataLength < 8 || event.Version != 2 {
		return 0
	}
	if event.ProviderID != tcpNetworkGUID && event.ProviderID != udpNetworkGUID {
		return 0
	}
	switch event.Opcode {
	case 10, 11, 26, 27: // IPv4/IPv6 send and receive; no retransmit/copy events.
	default:
		return 0
	}
	// The documented v2 TCP/UDP payload begins with PID and packet size. The
	// event header PID can belong to a system thread rather than the sender.
	data := unsafe.Slice((*byte)(event.UserData), 8)
	pid := int(binary.LittleEndian.Uint32(data[:4]))
	if pid <= 0 {
		return 0
	}
	eventNs := (event.TimeStamp - 116444736000000000) * 100
	if eventNs < s.startedAt.UnixNano() {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	counter, exists := s.bytes[pid]
	if !exists && len(s.bytes) >= 8192 {
		s.dropped++
		return 0
	}
	if counter.startedNs != 0 && eventNs < counter.startedNs {
		return 0
	}
	counter.total += uint64(binary.LittleEndian.Uint32(data[4:8]))
	s.bytes[pid] = counter
	return 0
}

func collectOSNetworkMetrics(processes map[int]rawProcess) {
	networkTraceMu.Lock()
	defer networkTraceMu.Unlock()
	s := activeNetworkTrace.Load()
	if s == nil || !s.running.Load() {
		return
	}
	properties := newNetworkTraceProperties()
	status, _, _ := controlNetworkTrace.Call(0, uintptr(unsafe.Pointer(s.name)), uintptr(unsafe.Pointer(properties)), traceControlQuery)
	if status != 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lost := uint64(properties.EventsLost) + uint64(properties.RealTimeBuffersLost) + s.dropped
	now := time.Now()
	for pid, process := range processes {
		if process.StartedAt.IsZero() {
			continue
		}
		startedNs := process.StartedAt.UnixNano()
		counter := s.bytes[pid]
		if (counter.startedNs != 0 && counter.startedNs != startedNs) || process.StartedAt.After(s.startedAt) && counter.startedNs == 0 {
			// PID reuse/new processes need their own baseline; bytes from a prior
			// process identity must never become this process's network traffic.
			counter = processNetworkBytes{startedNs: startedNs, sampledAt: now}
		}
		counter.startedNs = startedNs
		if counter.sampledAt.IsZero() {
			counter.sampledAt = s.startedAt
		}
		elapsed := now.Sub(counter.sampledAt).Seconds()
		ready := now.Sub(s.startedAt) >= time.Second
		if lost == s.lost && ready && elapsed >= 0.12 {
			rate := float64(counter.total-counter.sampledBytes) / elapsed
			process.NetworkBytesPerSec = &rate
		} else if lost == s.lost {
			process.NetworkSampling = true
		}
		if ready || lost != s.lost {
			counter.sampledBytes, counter.sampledAt = counter.total, now
		}
		s.bytes[pid] = counter
		processes[pid] = process
	}
	for pid := range s.bytes {
		if _, exists := processes[pid]; !exists {
			delete(s.bytes, pid)
		}
	}
	s.lost = lost
}
