package mounts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMountResolvesRelativePathsWithinHome(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(home, "project")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}

	mount, err := ParseMount("./app", "/workspace/app", cwd, home)
	if err != nil {
		t.Fatalf("ParseMount returned error: %v", err)
	}

	if mount.HostPath != "/mnt/host/project/app" {
		t.Fatalf("expected translated host path, got %q", mount.HostPath)
	}
	if mount.GuestPath != "/workspace/app" {
		t.Fatalf("expected guest path to be preserved, got %q", mount.GuestPath)
	}
}

func TestParseMountRejectsPathsOutsideHome(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(home, "project")
	outsideHome := filepath.Join(filepath.Dir(home), "outside")

	_, err := ParseMount(outsideHome, "/workspace", cwd, home)
	if err == nil {
		t.Fatal("expected outside-home mount to fail")
	}
}

func TestParseMountRejectsGuestPathsOutsideWorkspace(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(home, "project")

	_, err := ParseMount(".", "/tmp", cwd, home)
	if err == nil {
		t.Fatal("expected guest path validation to fail")
	}
}

func TestParseMountFromStringDefaultsToWorkspaceRoot(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(home, "repo")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}

	mount, err := ParseMountFromString("", cwd, home)
	if err != nil {
		t.Fatalf("ParseMountFromString returned error: %v", err)
	}

	if mount.HostPath != "/mnt/host/repo" {
		t.Fatalf("expected default host path, got %q", mount.HostPath)
	}
	if mount.GuestPath != "/workspace" {
		t.Fatalf("expected default guest path, got %q", mount.GuestPath)
	}
}

func TestParseMountsFromFlagsCollectsMultipleMounts(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve home symlinks: %v", err)
	}
	cwd := filepath.Join(home, "project")
	if err := os.MkdirAll(filepath.Join(cwd, "api"), 0755); err != nil {
		t.Fatalf("mkdir api dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cwd, "web"), 0755); err != nil {
		t.Fatalf("mkdir web dir: %v", err)
	}

	t.Setenv("HOME", home)
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}()
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	mounts, err := ParseMountsFromFlags([]string{
		"--mount", "./api:/workspace/api",
		"--ignore", "value",
		"--mount", "./web:/workspace/web",
	})
	if err != nil {
		t.Fatalf("ParseMountsFromFlags returned error: %v", err)
	}
	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(mounts))
	}
	if mounts[0].HostPath != "/mnt/host/project/api" {
		t.Fatalf("unexpected first mount host path: %q", mounts[0].HostPath)
	}
	if mounts[1].HostPath != "/mnt/host/project/web" {
		t.Fatalf("unexpected second mount host path: %q", mounts[1].HostPath)
	}
}
