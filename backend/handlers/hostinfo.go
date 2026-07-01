package handlers

import (
	"log"
	"os"
	"strconv"
	"strings"
)

// HostInfo captures host-level memory and load figures for the agent snapshot.
//
// /proc/loadavg and /proc/meminfo are NOT namespaced: inside a standard
// container they report the host kernel's values, so reading them here yields
// the real host memory/load rather than the container's cgroup limits.
type HostInfo struct {
	Load1             float64 `json:"load1"`
	Load5             float64 `json:"load5"`
	Load15            float64 `json:"load15"`
	MemTotalBytes     uint64  `json:"mem_total_bytes"`
	MemAvailableBytes uint64  `json:"mem_available_bytes"`
	MemUsedPercent    float64 `json:"mem_used_percent"`
}

// ReadHostInfo parses /proc/loadavg and /proc/meminfo. It returns a partial
// struct (with a logged warning) when a file is unreadable — e.g. on a non-Linux
// dev host — rather than failing, so the caller can still serve the snapshot and
// set host to null on error.
func ReadHostInfo() (*HostInfo, error) {
	info := &HostInfo{}
	var firstErr error

	loadRaw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		log.Printf("hostinfo: unreadable /proc/loadavg: %v", err)
		firstErr = err
	} else {
		info.Load1, info.Load5, info.Load15 = parseLoadAvg(string(loadRaw))
	}

	memRaw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		log.Printf("hostinfo: unreadable /proc/meminfo: %v", err)
		if firstErr == nil {
			firstErr = err
		}
	} else {
		info.MemTotalBytes, info.MemAvailableBytes, info.MemUsedPercent = parseMemInfo(string(memRaw))
	}

	return info, firstErr
}

// parseLoadAvg extracts the 1/5/15-minute load averages from the first three
// whitespace-separated fields of /proc/loadavg. Unparseable fields yield 0.
func parseLoadAvg(raw string) (load1, load5, load15 float64) {
	fields := strings.Fields(raw)
	if len(fields) >= 1 {
		load1, _ = strconv.ParseFloat(fields[0], 64)
	}
	if len(fields) >= 2 {
		load5, _ = strconv.ParseFloat(fields[1], 64)
	}
	if len(fields) >= 3 {
		load15, _ = strconv.ParseFloat(fields[2], 64)
	}
	return load1, load5, load15
}

// parseMemInfo reads MemTotal and MemAvailable (reported in kB) from
// /proc/meminfo, converting to bytes and computing the used percentage.
// mem_used_percent is 0 when total is unknown to avoid a divide-by-zero.
func parseMemInfo(raw string) (totalBytes, availableBytes uint64, usedPercent float64) {
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				totalBytes = kb * 1024
			}
		case "MemAvailable:":
			if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				availableBytes = kb * 1024
			}
		}
	}
	if totalBytes > 0 {
		usedPercent = (1 - float64(availableBytes)/float64(totalBytes)) * 100
	}
	return totalBytes, availableBytes, usedPercent
}
