//go:build windows

package desktop

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type rawFileHandle struct {
	ID            string
	PID           int
	ProcessName   string
	ReferenceType string
	TargetFile    string
}

type exeMetadata struct {
	displayName string
	icon        string
}

var (
	modPsapi    = windows.NewLazySystemDLL("psapi.dll")
	modNtdll    = windows.NewLazySystemDLL("ntdll.dll")
	modIphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
	modRstrtmgr = windows.NewLazySystemDLL("rstrtmgr.dll")
	modVersion  = windows.NewLazySystemDLL("version.dll")
	modShell32  = windows.NewLazySystemDLL("shell32.dll")
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modGdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procGetProcessMemoryInfo      = modPsapi.NewProc("GetProcessMemoryInfo")
	procNtQueryInformationProcess = modNtdll.NewProc("NtQueryInformationProcess")
	procGetExtendedTcpTable       = modIphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable       = modIphlpapi.NewProc("GetExtendedUdpTable")
	procRmStartSession            = modRstrtmgr.NewProc("RmStartSession")
	procRmRegisterResources       = modRstrtmgr.NewProc("RmRegisterResources")
	procRmGetList                 = modRstrtmgr.NewProc("RmGetList")
	procRmEndSession              = modRstrtmgr.NewProc("RmEndSession")
	procGetFileVersionInfoSizeW   = modVersion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW       = modVersion.NewProc("GetFileVersionInfoW")
	procVerQueryValueW            = modVersion.NewProc("VerQueryValueW")
	procSHGetFileInfoW            = modShell32.NewProc("SHGetFileInfoW")
	procDrawIconEx                = modUser32.NewProc("DrawIconEx")
	procDestroyIcon               = modUser32.NewProc("DestroyIcon")
	procCreateCompatibleDC        = modGdi32.NewProc("CreateCompatibleDC")
	procDeleteDC                  = modGdi32.NewProc("DeleteDC")
	procCreateDIBSection          = modGdi32.NewProc("CreateDIBSection")
	procSelectObject              = modGdi32.NewProc("SelectObject")
	procDeleteObject              = modGdi32.NewProc("DeleteObject")

	sidCacheMu   sync.Mutex
	sidCache     = map[string]string{}
	exeMetaMu    sync.Mutex
	exeMetaCache = map[string]exeMetadata{}
)

type shFileInfoW struct {
	HIcon         windows.Handle
	IIcon         int32
	DwAttributes  uint32
	SzDisplayName [windows.MAX_PATH]uint16
	SzTypeName    [80]uint16
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

func queryExecutableDisplayName(exePath string) string {
	pathPtr, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return ""
	}
	var handle uint32
	size, _, _ := procGetFileVersionInfoSizeW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&handle)),
	)
	if size == 0 || size > 4*1024*1024 {
		return ""
	}
	data := make([]byte, size)
	r1, _, _ := procGetFileVersionInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		size,
		uintptr(unsafe.Pointer(&data[0])),
	)
	if r1 == 0 {
		return ""
	}

	candidates := make([]string, 0, 8)
	seen := map[string]bool{}
	addCandidate := func(lc string) {
		lc = strings.ToLower(lc)
		if !seen[lc] {
			seen[lc] = true
			candidates = append(candidates, lc)
		}
	}

	if subPtr, subErr := windows.UTF16PtrFromString(`\VarFileInfo\Translation`); subErr == nil {
		var transPtr *uint16
		var transLen uint32
		if ok, _, _ := procVerQueryValueW.Call(
			uintptr(unsafe.Pointer(&data[0])),
			uintptr(unsafe.Pointer(subPtr)),
			uintptr(unsafe.Pointer(&transPtr)),
			uintptr(unsafe.Pointer(&transLen)),
		); ok != 0 && transPtr != nil && transLen >= 4 {
			count := int(transLen) / 4
			if count > 8 {
				count = 8
			}
			words := unsafe.Slice(transPtr, count*2)
			for i := 0; i < count; i++ {
				lang := words[i*2]
				cp := words[i*2+1]
				addCandidate(fmt.Sprintf("%04x%04x", lang, cp))
			}
		}
	}
	addCandidate("080404b0")
	addCandidate("040904b0")
	addCandidate("040904e4")
	addCandidate("04090000")

	for _, keyName := range []string{"FileDescription", "ProductName"} {
		for _, lc := range candidates {
			query := fmt.Sprintf(`\StringFileInfo\%s\%s`, lc, keyName)
			qPtr, qErr := windows.UTF16PtrFromString(query)
			if qErr != nil {
				continue
			}
			var valPtr *uint16
			var valLen uint32
			ok, _, _ := procVerQueryValueW.Call(
				uintptr(unsafe.Pointer(&data[0])),
				uintptr(unsafe.Pointer(qPtr)),
				uintptr(unsafe.Pointer(&valPtr)),
				uintptr(unsafe.Pointer(&valLen)),
			)
			if ok != 0 && valPtr != nil && valLen > 0 && valLen < 512 {
				chars := unsafe.Slice(valPtr, int(valLen))
				str := strings.TrimSpace(windows.UTF16ToString(chars))
				if str != "" {
					return str
				}
			}
		}
	}
	return ""
}

func extractExecutableIconDataURL(exePath string) string {
	pathPtr, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return ""
	}
	const shgfiIcon = 0x000000100
	const shgfiLargeIcon = 0x000000000
	var shfi shFileInfoW
	r1, _, _ := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&shfi)),
		unsafe.Sizeof(shfi),
		shgfiIcon|shgfiLargeIcon,
	)
	if r1 == 0 || shfi.HIcon == 0 {
		return ""
	}
	defer procDestroyIcon.Call(uintptr(shfi.HIcon))

	const iconSize = 32
	hdc, _, _ := procCreateCompatibleDC.Call(0)
	if hdc == 0 {
		return ""
	}
	defer procDeleteDC.Call(hdc)

	bmi := bitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:    iconSize,
		Height:   -iconSize,
		Planes:   1,
		BitCount: 32,
	}
	var bits unsafe.Pointer
	hbmp, _, _ := procCreateDIBSection.Call(
		hdc,
		uintptr(unsafe.Pointer(&bmi)),
		0,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hbmp == 0 || bits == nil {
		return ""
	}
	defer procDeleteObject.Call(hbmp)

	oldObj, _, _ := procSelectObject.Call(hdc, hbmp)
	defer procSelectObject.Call(hdc, oldObj)

	const diNormal = 0x0003
	drawn, _, _ := procDrawIconEx.Call(
		hdc,
		0,
		0,
		uintptr(shfi.HIcon),
		iconSize,
		iconSize,
		0,
		0,
		diNormal,
	)
	if drawn == 0 {
		return ""
	}

	raw := unsafe.Slice((*byte)(bits), iconSize*iconSize*4)
	hasAlpha := false
	hasColor := false
	for i := 0; i < len(raw); i += 4 {
		if raw[i+3] > 0 {
			hasAlpha = true
			break
		}
		if raw[i] > 0 || raw[i+1] > 0 || raw[i+2] > 0 {
			hasColor = true
		}
	}
	if !hasAlpha && !hasColor {
		return ""
	}

	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	for i := 0; i < len(raw); i += 4 {
		b := raw[i]
		g := raw[i+1]
		r := raw[i+2]
		a := raw[i+3]
		if !hasAlpha {
			if r > 0 || g > 0 || b > 0 {
				a = 255
			}
		} else if a > 0 && a < 255 && r <= a && g <= a && b <= a {
			r = uint8((uint32(r) * 255) / uint32(a))
			g = uint8((uint32(g) * 255) / uint32(a))
			b = uint8((uint32(b) * 255) / uint32(a))
		}
		img.Pix[i] = r
		img.Pix[i+1] = g
		img.Pix[i+2] = b
		img.Pix[i+3] = a
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func resolveExecutableMetadata(exePath string) (string, string) {
	if exePath == "" || strings.Contains(strings.ToLower(exePath), "trash") {
		return "", ""
	}
	key := strings.ToLower(filepath.Clean(exePath))
	exeMetaMu.Lock()
	if cached, ok := exeMetaCache[key]; ok {
		exeMetaMu.Unlock()
		return cached.displayName, cached.icon
	}
	exeMetaMu.Unlock()

	meta := exeMetadata{
		displayName: queryExecutableDisplayName(exePath),
		icon:        extractExecutableIconDataURL(exePath),
	}
	exeMetaMu.Lock()
	exeMetaCache[key] = meta
	exeMetaMu.Unlock()
	return meta.displayName, meta.icon
}

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

func queryProcessCommandLine(handle windows.Handle) (string, string) {
	// ProcessCommandLineInformation = 60
	const processCommandLineInformation = 60
	var needed uint32
	status, _, _ := procNtQueryInformationProcess.Call(
		uintptr(handle),
		processCommandLineInformation,
		0,
		0,
		uintptr(unsafe.Pointer(&needed)),
	)
	if status == 0xC0000022 { // STATUS_ACCESS_DENIED
		return "", "denied"
	}
	if needed == 0 || needed > 65536 {
		return "", "unavailable"
	}
	buf := make([]byte, needed)
	status, _, _ = procNtQueryInformationProcess.Call(
		uintptr(handle),
		processCommandLineInformation,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(needed),
		uintptr(unsafe.Pointer(&needed)),
	)
	if status == 0xC0000022 {
		return "", "denied"
	}
	if status != 0 {
		return "", "unavailable"
	}
	us := (*unicodeString)(unsafe.Pointer(&buf[0]))
	if us.Length == 0 || us.Buffer == nil {
		return "", "unavailable"
	}
	chars := unsafe.Slice(us.Buffer, int(us.Length)/2)
	return windows.UTF16ToString(chars), "ok"
}

func queryProcessUser(handle windows.Handle) string {
	var token windows.Token
	if err := windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token); err != nil {
		return ""
	}
	defer token.Close()
	tu, err := token.GetTokenUser()
	if err != nil || tu == nil || tu.User.Sid == nil {
		return ""
	}
	sidStr := tu.User.Sid.String()
	sidCacheMu.Lock()
	if cached, ok := sidCache[sidStr]; ok {
		sidCacheMu.Unlock()
		return cached
	}
	sidCacheMu.Unlock()
	account, _, _, err := tu.User.Sid.LookupAccount("")
	if err != nil {
		account = sidStr
	}
	sidCacheMu.Lock()
	sidCache[sidStr] = account
	sidCacheMu.Unlock()
	return account
}

func collectOSProcesses(_ context.Context) (map[int]rawProcess, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	now := time.Now()
	numCPU := float64(runtime.NumCPU())
	if numCPU < 1 {
		numCPU = 1
	}

	result := map[int]rawProcess{}
	activeKeys := map[cpuSampleKey]bool{}

	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		pid := int(entry.ProcessID)
		if pid == 0 {
			continue
		}
		name := windows.UTF16ToString(entry.ExeFile[:])
		rp := rawProcess{
			PID:              pid,
			PPID:             int(entry.ParentProcessID),
			Name:             name,
			CommandLineState: "unavailable",
		}
		h, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if openErr != nil {
			if errors.Is(openErr, windows.ERROR_ACCESS_DENIED) {
				rp.CommandLineState = "denied"
				rp.AccessDenied = true
			}
			result[pid] = rp
			continue
		}

		// Executable path
		var exeBuf [windows.MAX_PATH * 2]uint16
		exeLen := uint32(len(exeBuf))
		if qErr := windows.QueryFullProcessImageName(h, 0, &exeBuf[0], &exeLen); qErr == nil && exeLen > 0 {
			rp.Executable = windows.UTF16ToString(exeBuf[:exeLen])
			rp.DisplayName, rp.Icon = resolveExecutableMetadata(rp.Executable)
		}

		// Memory working set
		var mem processMemoryCounters
		mem.CB = uint32(unsafe.Sizeof(mem))
		if r1, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mem)), uintptr(mem.CB)); r1 != 0 {
			rp.WorkingSet(&mem)
		}

		// Process times & CPU %
		var created, exited, kernel, user windows.Filetime
		if tErr := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); tErr == nil {
			startedNs := created.Nanoseconds()
			rp.StartedAt = time.Unix(0, startedNs)
			total100ns := (uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) +
				(uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime))
			key := cpuSampleKey{pid: pid, startedNs: startedNs}
			activeKeys[key] = true

			cpuSamplesMu.Lock()
			prev, hasPrev := cpuSamples[key]
			if !hasPrev {
				cpuSamples[key] = cpuSampleVal{total100ns: total100ns, sampledAt: now}
				rp.CPUSampling = true
			} else {
				elapsed := now.Sub(prev.sampledAt)
				if elapsed >= 120*time.Millisecond {
					elapsed100ns := float64(elapsed.Nanoseconds()) / 100.0
					var delta uint64
					if total100ns >= prev.total100ns {
						delta = total100ns - prev.total100ns
					}
					pct := (float64(delta) / elapsed100ns / numCPU) * 100.0
					if pct < 0 {
						pct = 0
					}
					if pct > 100 {
						pct = 100
					}
					rp.CPUPercent = &pct
					cpuSamples[key] = cpuSampleVal{total100ns: total100ns, sampledAt: now}
				} else {
					pct := 0.0
					rp.CPUPercent = &pct
				}
			}
			cpuSamplesMu.Unlock()
		}

		// User & CommandLine
		rp.User = queryProcessUser(h)
		cmdLine, cmdState := queryProcessCommandLine(h)
		rp.CommandLine = cmdLine
		rp.CommandLineState = cmdState
		if cmdState == "denied" {
			rp.AccessDenied = true
		}

		// Check if full handle inspection would be denied (for file-handle coverage accounting)
		if !rp.AccessDenied {
			if hFull, fullErr := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_DUP_HANDLE, false, entry.ProcessID); fullErr != nil {
				if errors.Is(fullErr, windows.ERROR_ACCESS_DENIED) {
					rp.AccessDenied = true
				}
			} else {
				windows.CloseHandle(hFull)
			}
		}

		windows.CloseHandle(h)
		result[pid] = rp
	}

	cpuSamplesMu.Lock()
	for k := range cpuSamples {
		if !activeKeys[k] {
			delete(cpuSamples, k)
		}
	}
	cpuSamplesMu.Unlock()

	return result, nil
}

func (rp *rawProcess) WorkingSet(mem *processMemoryCounters) {
	rp.MemoryBytes = uint64(mem.WorkingSetSize)
}

func ntohs(n uint32) int {
	return int(uint16(n)>>8 | (uint16(n)&0xff)<<8)
}

func tcpStateLabel(state uint32) string {
	switch state {
	case 2:
		return "listening"
	case 5:
		return "established"
	case 8:
		return "close_wait"
	case 11:
		return "time_wait"
	default:
		return "other"
	}
}

func collectOSPorts(_ context.Context) ([]rawPort, error) {
	var result []rawPort

	// 1. TCP IPv4 (AF_INET = 2, TCP_TABLE_OWNER_PID_ALL = 5)
	if buf, err := getExtendedTable(procGetExtendedTcpTable, 2, 5); err == nil && len(buf) >= 4 {
		count := binary.LittleEndian.Uint32(buf[:4])
		offset := 4
		const rowSize = 24
		for i := uint32(0); i < count && offset+rowSize <= len(buf); i++ {
			row := buf[offset : offset+rowSize]
			offset += rowSize
			state := binary.LittleEndian.Uint32(row[0:4])
			localIP := net.IP(row[4:8]).String()
			localPort := ntohs(binary.LittleEndian.Uint32(row[8:12]))
			remoteIP := net.IP(row[12:16]).String()
			remotePort := ntohs(binary.LittleEndian.Uint32(row[16:20]))
			pid := int(binary.LittleEndian.Uint32(row[20:24]))
			remoteAddr := ""
			if state != 2 && remotePort > 0 {
				remoteAddr = fmt.Sprintf("%s:%d", remoteIP, remotePort)
			}
			result = append(result, rawPort{
				Protocol:      "TCP",
				IPVersion:     "IPv4",
				ProtocolLabel: "TCP / IPv4",
				LocalAddress:  fmt.Sprintf("%s:%d", localIP, localPort),
				IP:            localIP,
				Port:          localPort,
				RemoteAddress: remoteAddr,
				State:         tcpStateLabel(state),
				PID:           pid,
			})
		}
	}

	// 2. TCP IPv6 (AF_INET6 = 23, TCP_TABLE_OWNER_PID_ALL = 5)
	if buf, err := getExtendedTable(procGetExtendedTcpTable, 23, 5); err == nil && len(buf) >= 4 {
		count := binary.LittleEndian.Uint32(buf[:4])
		offset := 4
		const rowSize = 56
		for i := uint32(0); i < count && offset+rowSize <= len(buf); i++ {
			row := buf[offset : offset+rowSize]
			offset += rowSize
			localIP := net.IP(row[0:16]).String()
			localPort := ntohs(binary.LittleEndian.Uint32(row[20:24]))
			remoteIP := net.IP(row[24:40]).String()
			remotePort := ntohs(binary.LittleEndian.Uint32(row[44:48]))
			state := binary.LittleEndian.Uint32(row[48:52])
			pid := int(binary.LittleEndian.Uint32(row[52:56]))
			remoteAddr := ""
			if state != 2 && remotePort > 0 {
				remoteAddr = fmt.Sprintf("[%s]:%d", remoteIP, remotePort)
			}
			result = append(result, rawPort{
				Protocol:      "TCP",
				IPVersion:     "IPv6",
				ProtocolLabel: "TCP / IPv6",
				LocalAddress:  fmt.Sprintf("[%s]:%d", localIP, localPort),
				IP:            localIP,
				Port:          localPort,
				RemoteAddress: remoteAddr,
				State:         tcpStateLabel(state),
				PID:           pid,
			})
		}
	}

	// 3. UDP IPv4 (AF_INET = 2, UDP_TABLE_OWNER_PID = 1)
	if buf, err := getExtendedTable(procGetExtendedUdpTable, 2, 1); err == nil && len(buf) >= 4 {
		count := binary.LittleEndian.Uint32(buf[:4])
		offset := 4
		const rowSize = 12
		for i := uint32(0); i < count && offset+rowSize <= len(buf); i++ {
			row := buf[offset : offset+rowSize]
			offset += rowSize
			localIP := net.IP(row[0:4]).String()
			localPort := ntohs(binary.LittleEndian.Uint32(row[4:8]))
			pid := int(binary.LittleEndian.Uint32(row[8:12]))
			result = append(result, rawPort{
				Protocol:      "UDP",
				IPVersion:     "IPv4",
				ProtocolLabel: "UDP",
				LocalAddress:  fmt.Sprintf("%s:%d", localIP, localPort),
				IP:            localIP,
				Port:          localPort,
				State:         "bound",
				PID:           pid,
			})
		}
	}

	// 4. UDP IPv6 (AF_INET6 = 23, UDP_TABLE_OWNER_PID = 1)
	if buf, err := getExtendedTable(procGetExtendedUdpTable, 23, 1); err == nil && len(buf) >= 4 {
		count := binary.LittleEndian.Uint32(buf[:4])
		offset := 4
		const rowSize = 28
		for i := uint32(0); i < count && offset+rowSize <= len(buf); i++ {
			row := buf[offset : offset+rowSize]
			offset += rowSize
			localIP := net.IP(row[0:16]).String()
			localPort := ntohs(binary.LittleEndian.Uint32(row[20:24]))
			pid := int(binary.LittleEndian.Uint32(row[24:28]))
			result = append(result, rawPort{
				Protocol:      "UDP",
				IPVersion:     "IPv6",
				ProtocolLabel: "UDP",
				LocalAddress:  fmt.Sprintf("[%s]:%d", localIP, localPort),
				IP:            localIP,
				Port:          localPort,
				State:         "bound",
				PID:           pid,
			})
		}
	}

	return result, nil
}

func getExtendedTable(proc *windows.LazyProc, af uintptr, tableClass uintptr) ([]byte, error) {
	var size uint32
	r1, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&size)), 0, af, tableClass, 0)
	if r1 != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) || size == 0 {
		return nil, fmt.Errorf("table query size failed: %d", r1)
	}
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, size)
		r1, _, _ = proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, af, tableClass, 0)
		if r1 == 0 {
			return buf[:size], nil
		}
		if r1 != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			return nil, fmt.Errorf("table query failed: %d", r1)
		}
	}
	return nil, errors.New("table buffer insufficient")
}

type rmUniqueProcess struct {
	PID              uint32
	ProcessStartTime windows.Filetime
}

type rmProcessInfo struct {
	Process          rmUniqueProcess
	StrAppName       [256]uint16
	StrServiceShort  [64]uint16
	ApplicationType  uint32
	AppStatus        uint32
	TSSessionId      uint32
	BRestartable     int32
}

func queryRestartManager(files []string) []rawFileHandle {
	if len(files) == 0 {
		return nil
	}
	var session uint32
	var sessionKey [64]uint16
	r1, _, _ := procRmStartSession.Call(
		uintptr(unsafe.Pointer(&session)),
		0,
		uintptr(unsafe.Pointer(&sessionKey[0])),
	)
	if r1 != 0 {
		return nil
	}
	defer procRmEndSession.Call(uintptr(session))

	ptrs := make([]*uint16, 0, len(files))
	for _, f := range files {
		if p, err := windows.UTF16PtrFromString(f); err == nil {
			ptrs = append(ptrs, p)
		}
	}
	if len(ptrs) == 0 {
		return nil
	}
	r1, _, _ = procRmRegisterResources.Call(
		uintptr(session),
		uintptr(len(ptrs)),
		uintptr(unsafe.Pointer(&ptrs[0])),
		0,
		0,
		0,
		0,
	)
	if r1 != 0 {
		return nil
	}

	var needed, count, rebootReasons uint32
	r1, _, _ = procRmGetList.Call(
		uintptr(session),
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&count)),
		0,
		uintptr(unsafe.Pointer(&rebootReasons)),
	)
	if r1 != uintptr(windows.ERROR_MORE_DATA) || needed == 0 {
		return nil
	}
	count = needed
	infos := make([]rmProcessInfo, count)
	r1, _, _ = procRmGetList.Call(
		uintptr(session),
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&count)),
		uintptr(unsafe.Pointer(&infos[0])),
		uintptr(unsafe.Pointer(&rebootReasons)),
	)
	if r1 != 0 {
		return nil
	}
	targetSample := files[0]
	out := make([]rawFileHandle, 0, count)
	for i := uint32(0); i < count; i++ {
		pid := int(infos[i].Process.PID)
		if pid <= 0 {
			continue
		}
		appName := windows.UTF16ToString(infos[i].StrAppName[:])
		out = append(out, rawFileHandle{
			ID:            "rm-" + strconv.Itoa(pid) + "-" + targetSample,
			PID:           pid,
			ProcessName:   appName,
			ReferenceType: "open_file",
			TargetFile:    targetSample,
		})
	}
	return out
}

func pathMatchesTarget(candidate, target string, isDir, recursive bool) bool {
	if candidate == "" || target == "" {
		return false
	}
	cleanCand := filepath.Clean(candidate)
	cleanTarg := filepath.Clean(target)
	if strings.EqualFold(cleanCand, cleanTarg) {
		return true
	}
	if !isDir {
		return false
	}
	rel, err := filepath.Rel(cleanTarg, cleanCand)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	if !recursive && strings.ContainsRune(rel, filepath.Separator) {
		return false
	}
	return true
}

func collectOSFileHandles(ctx context.Context, targetPath string, isDir, recursive bool, procs map[int]rawProcess, attributions map[int]runAttribution) ([]rawFileHandle, int, error) {
	unreadable := 0
	for _, p := range procs {
		if p.AccessDenied {
			unreadable++
		}
	}

	seenPID := map[int]bool{}
	var results []rawFileHandle

	// 1. Check Restart Manager on target file or files in target directory
	var candidateFiles []string
	if !isDir {
		candidateFiles = []string{targetPath}
	} else if !recursive {
		if entries, err := os.ReadDir(targetPath); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					candidateFiles = append(candidateFiles, filepath.Join(targetPath, entry.Name()))
					if len(candidateFiles) >= 96 {
						break
					}
				}
			}
		}
	} else {
		_ = filepath.WalkDir(targetPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil || ctx.Err() != nil {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				candidateFiles = append(candidateFiles, path)
				if len(candidateFiles) >= 128 {
					return fs.SkipAll
				}
			}
			return nil
		})
	}

	for start := 0; start < len(candidateFiles); start += 32 {
		if ctx.Err() != nil {
			break
		}
		end := start + 32
		if end > len(candidateFiles) {
			end = len(candidateFiles)
		}
		for _, h := range queryRestartManager(candidateFiles[start:end]) {
			if !seenPID[h.PID] {
				seenPID[h.PID] = true
				results = append(results, h)
			}
		}
	}

	// 2. Check process executable image paths and active run working directories (when target is a directory)
	selfPID := os.Getpid()
	for pid, proc := range procs {
		if pid == selfPID || seenPID[pid] {
			continue
		}
		if proc.Executable != "" && pathMatchesTarget(proc.Executable, targetPath, isDir, recursive) {
			seenPID[pid] = true
			results = append(results, rawFileHandle{
				ID:            "exe-" + strconv.Itoa(pid),
				PID:           pid,
				ProcessName:   proc.Name,
				ReferenceType: "open_file",
				TargetFile:    proc.Executable,
			})
			continue
		}
		if isDir {
			if attr, ok := attributions[pid]; ok && attr.Directory != "" && pathMatchesTarget(attr.Directory, targetPath, true, recursive) {
				seenPID[pid] = true
				results = append(results, rawFileHandle{
					ID:            "cwd-" + strconv.Itoa(pid),
					PID:           pid,
					ProcessName:   proc.Name,
					ReferenceType: "open_file",
					TargetFile:    attr.Directory,
				})
			}
		}
	}

	return results, unreadable, nil
}
