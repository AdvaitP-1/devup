package workspace

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DefaultExcludes are directory names skipped during tar streaming.
var DefaultExcludes = []string{
	".git",
	"node_modules",
	"__pycache__",
	".next",
	"target",
	"vendor",
	".devup",
	".venv",
	"dist",
}

// StreamTar writes a tar archive of dir to w, skipping any directories whose
// base name appears in excludes. Paths inside the archive are relative to dir.
func StreamTar(dir string, excludes []string, w io.Writer) error {
	skip := make(map[string]bool, len(excludes))
	for _, e := range excludes {
		skip[e] = true
	}

	tw := tar.NewWriter(w)
	defer tw.Close()

	dir = filepath.Clean(dir)
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		base := filepath.Base(path)
		if info.IsDir() && skip[base] && path != dir {
			return filepath.SkipDir
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)

		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			header.Linkname = link
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

// IsExcluded checks if a path component should be excluded.
func IsExcluded(name string, excludes []string) bool {
	for _, e := range excludes {
		if strings.EqualFold(name, e) {
			return true
		}
	}
	return false
}
