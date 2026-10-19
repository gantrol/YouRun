//go:build !windows || (!amd64 && !arm64)

package desktop

func primeOSNetworkMetrics()                     {}
func collectOSNetworkMetrics(map[int]rawProcess) {}
func closeOSNetworkMetrics()                     {}
