package workspace

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestStreamTarSkipsExcludedDirectoriesAndKeepsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write app.txt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "ignored.txt"), []byte("skip"), 0644); err != nil {
		t.Fatalf("write ignored.txt: %v", err)
	}
	if err := os.Symlink("app.txt", filepath.Join(root, "app-link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	var buf bytes.Buffer
	if err := StreamTar(root, DefaultExcludes, &buf); err != nil {
		t.Fatalf("StreamTar returned error: %v", err)
	}

	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))
	entries := make(map[string]*tar.Header)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		copyHdr := *hdr
		entries[hdr.Name] = &copyHdr
	}

	if _, ok := entries["app.txt"]; !ok {
		t.Fatal("expected regular file to be present in tar")
	}
	if _, ok := entries["node_modules/ignored.txt"]; ok {
		t.Fatal("expected excluded node_modules contents to be skipped")
	}
	link, ok := entries["app-link"]
	if !ok {
		t.Fatal("expected symlink to be present in tar")
	}
	if link.Typeflag != tar.TypeSymlink {
		t.Fatalf("expected symlink header, got type %d", link.Typeflag)
	}
	if link.Linkname != "app.txt" {
		t.Fatalf("expected symlink target app.txt, got %q", link.Linkname)
	}
}

func TestIsExcludedIsCaseInsensitive(t *testing.T) {
	if !IsExcluded("Node_Modules", DefaultExcludes) {
		t.Fatal("expected case-insensitive match")
	}
	if IsExcluded("src", DefaultExcludes) {
		t.Fatal("did not expect non-excluded directory to match")
	}
}
