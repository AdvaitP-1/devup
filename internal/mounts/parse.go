package mounts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devup/internal/api"
)

// ParseMount parses hostPath and guestPath into an api.Mount.
// hostPath is resolved relative to cwd; must be under home.
// guestPath must be under /workspace.
func ParseMount(hostPath, guestPath string, cwd, home string) (api.Mount, error) {
	home = filepath.Clean(home)
	hostPath = strings.TrimSpace(hostPath)
	guestPath = strings.TrimSpace(guestPath)
	if hostPath == "" || guestPath == "" {
		return api.Mount{}, fmt.Errorf("hostPath and guestPath required")
	}
	resolved := hostPath
	if !filepath.IsAbs(hostPath) {
		resolved = filepath.Join(cwd, hostPath)
	}
	resolved = filepath.Clean(resolved)
	rel, err := filepath.Rel(home, resolved)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return api.Mount{}, fmt.Errorf("mount path %s is not under %s", resolved, home)
	}
	vmHostPath := "/mnt/host/" + filepath.ToSlash(rel)
	if rel == "." {
		vmHostPath = "/mnt/host"
	}
	if guestPath != "/workspace" && !strings.HasPrefix(guestPath, "/workspace/") {
		return api.Mount{}, fmt.Errorf("guest_path %s must be under /workspace (e.g. /workspace or /workspace/foo)", guestPath)
	}
	return api.Mount{
		HostPath:  vmHostPath,
		GuestPath: guestPath,
		ReadOnly:  false,
	}, nil
}

// ParseMountFromString parses "hostPath:guestPath" into an api.Mount.
// If s is empty, returns default ".:/workspace".
func ParseMountFromString(s string, cwd, home string) (api.Mount, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		s = ".:/workspace"
	}
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return api.Mount{}, fmt.Errorf("mount format: hostPath:guestPath (e.g. .:/workspace)")
	}
	return ParseMount(parts[0], parts[1], cwd, home)
}

// DefaultMounts returns the default .:/workspace mount for the current directory.
func DefaultMounts() ([]api.Mount, error) {
	cwd, err := getCwd()
	if err != nil {
		return nil, err
	}
	home, err := getHomeDir()
	if err != nil {
		return nil, err
	}
	m, err := ParseMountFromString(".:/workspace", cwd, home)
	if err != nil {
		return nil, err
	}
	return []api.Mount{m}, nil
}

// ParseMountsFromFlags parses --mount flags from CLI args.
// If no --mount flags, returns nil (caller should use DefaultMounts).
func ParseMountsFromFlags(flags []string) ([]api.Mount, error) {
	home, err := getHomeDir()
	if err != nil {
		return nil, err
	}
	cwd, err := getCwd()
	if err != nil {
		return nil, err
	}
	var mounts []api.Mount
	for i := 0; i < len(flags); i++ {
		if flags[i] != "--mount" {
			continue
		}
		i++
		if i >= len(flags) {
			return nil, fmt.Errorf("--mount requires hostPath:guestPath")
		}
		m, err := ParseMountFromString(flags[i], cwd, home)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, m)
	}
	return mounts, nil
}

func getHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Clean(home), nil
}

func getCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cwd: %w", err)
	}
	return filepath.Clean(cwd), nil
}
