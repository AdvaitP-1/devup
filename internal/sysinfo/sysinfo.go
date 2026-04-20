package sysinfo

import (
	"os"
	"strconv"
	"strings"
)

// Stats holds live system resource telemetry read from procfs.
type Stats struct {
	MemTotalMB int
	MemFreeMB  int     // MemAvailable from /proc/meminfo (includes cached/reclaimable)
	LoadAvg1   float64 // 1-minute load average
}

// Read returns current system stats. Returns zero values on any error
// (safe for non-Linux or degraded environments).
func Read() Stats {
	var s Stats
	s.MemTotalMB, s.MemFreeMB = readMeminfo()
	s.LoadAvg1 = readLoadAvg()
	return s
}

func readMeminfo() (totalMB, freeMB int) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		kB, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalMB = kB / 1024
		case "MemAvailable:":
			freeMB = kB / 1024
		}
	}
	return totalMB, freeMB
}

func readLoadAvg() float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return v
}
