package transfer

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type transferFile struct {
	Local, Remote string
	Size          int64
}
type transferPlan struct {
	Files []transferFile
	Dirs  []string
}

func targetPath(destination, source string, multiple bool) string {
	if strings.HasSuffix(destination, "/") || multiple {
		return strings.TrimRight(destination, "/") + "/" + filepath.Base(filepath.Clean(source))
	}
	return destination
}

// OSS directories contribute their contents at the configured prefix; SSH keeps
// the source directory name when the destination ends in / or sources are multiple.
func planTransfers(sources []string, destination string, isOSS bool) (transferPlan, error) {
	var plan transferPlan
	for _, source := range sources {
		info, err := os.Stat(source)
		if err != nil {
			return plan, err
		}
		root := targetPath(destination, source, len(sources) > 1)
		if info.Mode().IsRegular() {
			plan.Files = append(plan.Files, transferFile{source, root, info.Size()})
			continue
		}
		if !info.IsDir() {
			return plan, fmt.Errorf("source %q is not a regular file or directory", source)
		}
		if isOSS {
			root = destination
		}
		err = filepath.WalkDir(source, func(local string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(source, local)
			if err != nil {
				return err
			}
			remote := path.Join(root, filepath.ToSlash(rel))
			if entry.IsDir() {
				plan.Dirs = append(plan.Dirs, remote)
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			// Match Rust directory traversal: regular files only; do not follow symlinks.
			if info.Mode().IsRegular() {
				plan.Files = append(plan.Files, transferFile{local, remote, info.Size()})
			}
			return nil
		})
		if err != nil {
			return plan, err
		}
	}
	if isOSS {
		for i := range plan.Files {
			// The Rust SDK treats the initial slash as a URL path separator.
			plan.Files[i].Remote = strings.TrimLeft(strings.ReplaceAll(plan.Files[i].Remote, "\\", "/"), "/")
			if plan.Files[i].Remote == "" {
				return plan, fmt.Errorf("OSS object key must not be empty")
			}
		}
	}
	return plan, nil
}
