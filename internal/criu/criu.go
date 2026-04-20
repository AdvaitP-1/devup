package criu

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"devup/internal/logging"
)

const (
	DataDir   = "/var/lib/devup/criu"
	criuBin   = "criu"
	dumpDir   = "images" // subdirectory inside DataDir/<jobID> holding CRIU images
	logFile   = "criu.log"
	verbosity = "4" // -v4 for detailed diagnostics during early integration
)

// Available reports whether the criu binary is installed and the kernel
// supports checkpoint/restore. Set once by Init().
var Available bool

// Init checks for the criu binary and CONFIG_CHECKPOINT_RESTORE kernel support.
// Non-fatal: if CRIU is unavailable, Available stays false and migrate commands
// return a clear error.
func Init() error {
	if _, err := exec.LookPath(criuBin); err != nil {
		Available = false
		return fmt.Errorf("criu binary not found: %w", err)
	}

	out, err := exec.Command(criuBin, "check").CombinedOutput()
	if err != nil {
		Available = false
		return fmt.Errorf("criu check failed (kernel may lack CONFIG_CHECKPOINT_RESTORE): %w\n%s", err, out)
	}

	if err := os.MkdirAll(DataDir, 0755); err != nil {
		Available = false
		return fmt.Errorf("mkdir %s: %w", DataDir, err)
	}

	Available = true
	logging.Info("criu available", "data_dir", DataDir)
	return nil
}

// ImagesDir returns the path where CRIU images are stored for a given job.
func ImagesDir(jobID string) string {
	return filepath.Join(DataDir, jobID, dumpDir)
}

// Dump checkpoints the process tree rooted at pid. The process is frozen and
// killed (default CRIU behavior). On success, images are written to
// DataDir/<jobID>/images/.
//
// Flags:
//   - --shell-job: required because DevUp jobs inherit session/pgid from the agent
//   - --manage-cgroups: saves cgroup configuration so we can inspect it, though
//     we recreate cgroups on the target rather than restoring them
//   - --tcp-established: omitted intentionally — dev workloads should reconnect
//   - --ext-unix-sk: tolerate external unix sockets (common in dev tools)
//   - --file-locks: checkpoint file locks (node_modules, pip, etc.)
func Dump(jobID string, pid int) error {
	if !Available {
		return fmt.Errorf("criu not available (install with: apt install criu)")
	}

	imgDir := ImagesDir(jobID)
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		return fmt.Errorf("mkdir images dir: %w", err)
	}

	args := []string{
		"dump",
		"-t", fmt.Sprintf("%d", pid),
		"-D", imgDir,
		"-o", logFile,
		"-v" + verbosity,
		"--shell-job",
		"--ext-unix-sk",
		"--file-locks",
	}

	out, err := exec.Command(criuBin, args...).CombinedOutput()
	if err != nil {
		criuLog := readCriuLog(imgDir)
		return fmt.Errorf("criu dump failed for job %s (pid %d): %w\n%s\ncriu log:\n%s",
			jobID, pid, err, out, criuLog)
	}

	logging.Info("criu dump complete", "job_id", jobID, "pid", pid, "images", imgDir)
	return nil
}

// Restore recreates the process tree from a CRIU image directory. Returns the
// PID of the restored root process.
//
// The caller is responsible for:
//   - Ensuring the workspace files exist at the same paths as during dump
//   - Creating cgroups and adding the returned PID
//   - Creating network namespaces if needed (CRIU skips network via --external)
func Restore(jobID string) (int, error) {
	if !Available {
		return 0, fmt.Errorf("criu not available")
	}

	imgDir := ImagesDir(jobID)
	if _, err := os.Stat(imgDir); err != nil {
		return 0, fmt.Errorf("images dir missing: %w", err)
	}

	// --restore-detached: CRIU forks the restored tree and returns immediately
	// --pidfile: writes the root PID so we can track it
	pidFile := filepath.Join(DataDir, jobID, "restored.pid")

	args := []string{
		"restore",
		"-D", imgDir,
		"-o", logFile,
		"-v" + verbosity,
		"--shell-job",
		"--ext-unix-sk",
		"--file-locks",
		"--restore-detached",
		"--pidfile", pidFile,
	}

	out, err := exec.Command(criuBin, args...).CombinedOutput()
	if err != nil {
		criuLog := readCriuLog(imgDir)
		return 0, fmt.Errorf("criu restore failed for job %s: %w\n%s\ncriu log:\n%s",
			jobID, err, out, criuLog)
	}

	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, fmt.Errorf("read pidfile: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(pidData)), "%d", &pid); err != nil {
		return 0, fmt.Errorf("parse pid from %s: %w", pidFile, err)
	}

	logging.Info("criu restore complete", "job_id", jobID, "pid", pid)
	return pid, nil
}

// Cleanup removes the CRIU image directory for a job.
func Cleanup(jobID string) error {
	dir := filepath.Join(DataDir, jobID)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("cleanup criu images for %s: %w", jobID, err)
	}
	return nil
}

// Reconcile prunes CRIU image directories that don't correspond to active jobs.
func Reconcile(activeJobIDs map[string]bool) {
	if !Available {
		return
	}
	entries, err := os.ReadDir(DataDir)
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
		dir := filepath.Join(DataDir, e.Name())
		if err := os.RemoveAll(dir); err != nil {
			logging.Error("criu reconcile: cleanup failed", "job_id", e.Name(), "err", err)
		} else {
			logging.Info("criu reconcile: pruned stale images", "job_id", e.Name())
		}
	}
}

func readCriuLog(imgDir string) string {
	data, err := os.ReadFile(filepath.Join(imgDir, logFile))
	if err != nil {
		return "(no log available)"
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 50 {
		lines = lines[len(lines)-50:]
	}
	return strings.Join(lines, "\n")
}
