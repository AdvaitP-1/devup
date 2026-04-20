package cgroup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"devup/internal/logging"
)

const CgroupRoot = "/sys/fs/cgroup/devup"

// Limits describes resource constraints for a single job.
type Limits struct {
	MemoryMaxBytes int64 // memory.max; 0 = unlimited
	CPUQuotaUs     int   // cpu.max quota in microseconds; 0 = unlimited
	CPUPeriodUs    int   // cpu.max period in microseconds; default 100000
	PidsMax        int   // pids.max; 0 = unlimited
}

// Available reports whether cgroups v2 unified hierarchy is usable.
var Available bool

// Init creates the devup cgroup subtree and enables controllers.
// Called once at agent startup. If cgroups v2 is unavailable, sets
// Available=false and returns the error (non-fatal to the caller).
func Init() error {
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
		Available = false
		return fmt.Errorf("cgroups v2 not available: %w", err)
	}

	if err := os.MkdirAll(CgroupRoot, 0755); err != nil {
		Available = false
		return fmt.Errorf("mkdir %s: %w", CgroupRoot, err)
	}

	// Enable memory, cpu, pids controllers on the parent so children inherit them
	controllers := "+memory +cpu +pids"
	if err := os.WriteFile("/sys/fs/cgroup/cgroup.subtree_control", []byte(controllers), 0644); err != nil {
		logging.Error("enable root controllers (may already be set)", "err", err)
	}
	if err := os.WriteFile(filepath.Join(CgroupRoot, "cgroup.subtree_control"), []byte(controllers), 0644); err != nil {
		logging.Error("enable devup controllers", "err", err)
	}

	Available = true
	return nil
}

// Create makes a cgroup directory for the job and writes limit files.
func Create(jobID string, l Limits) error {
	if !Available {
		return fmt.Errorf("cgroups not available")
	}
	dir := filepath.Join(CgroupRoot, jobID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir cgroup %s: %w", dir, err)
	}

	if l.MemoryMaxBytes > 0 {
		if err := os.WriteFile(filepath.Join(dir, "memory.max"), []byte(strconv.FormatInt(l.MemoryMaxBytes, 10)), 0644); err != nil {
			return fmt.Errorf("write memory.max: %w", err)
		}
	}

	if l.CPUQuotaUs > 0 {
		period := l.CPUPeriodUs
		if period <= 0 {
			period = 100000
		}
		val := fmt.Sprintf("%d %d", l.CPUQuotaUs, period)
		if err := os.WriteFile(filepath.Join(dir, "cpu.max"), []byte(val), 0644); err != nil {
			return fmt.Errorf("write cpu.max: %w", err)
		}
	}

	if l.PidsMax > 0 {
		if err := os.WriteFile(filepath.Join(dir, "pids.max"), []byte(strconv.Itoa(l.PidsMax)), 0644); err != nil {
			return fmt.Errorf("write pids.max: %w", err)
		}
	}

	return nil
}

// AddProcess writes a PID into the cgroup's cgroup.procs file.
func AddProcess(jobID string, pid int) error {
	if !Available {
		return fmt.Errorf("cgroups not available")
	}
	path := filepath.Join(CgroupRoot, jobID, "cgroup.procs")
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
}

// Destroy removes a job's cgroup directory. The kernel requires all
// processes to have exited first. If processes remain, they are killed.
func Destroy(jobID string) error {
	if !Available {
		return nil
	}
	dir := filepath.Join(CgroupRoot, jobID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	// First attempt
	if err := os.Remove(dir); err == nil {
		return nil
	}

	// Kill remaining processes and retry
	killProcsInCgroup(dir)
	return os.Remove(dir)
}

// Reconcile prunes stale cgroup directories that no longer correspond
// to running jobs. Called at startup and periodically.
func Reconcile(activeJobIDs map[string]bool) {
	if !Available {
		return
	}
	entries, err := os.ReadDir(CgroupRoot)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if activeJobIDs[e.Name()] {
			continue
		}
		if err := Destroy(e.Name()); err != nil {
			logging.Error("cgroup reconcile: destroy failed", "job_id", e.Name(), "err", err)
		} else {
			logging.Info("cgroup reconcile: pruned stale cgroup", "job_id", e.Name())
		}
	}
}

func killProcsInCgroup(dir string) {
	data, err := os.ReadFile(filepath.Join(dir, "cgroup.procs"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if pid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil && pid > 0 {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}
