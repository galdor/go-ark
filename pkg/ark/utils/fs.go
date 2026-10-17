package utils

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

func WalkFS(filePath string, fn func(string, string, fs.FileInfo) error) error {
	var ancestors []fs.FileInfo

	var walk func(string, string, fs.FileInfo) error

	walk = func(virtualPath, filePath string, info fs.FileInfo) error {
		if (info.Mode() & os.ModeSymlink) != 0 {
			targetPath, err := filepath.EvalSymlinks(filePath)
			if err != nil {
				return fmt.Errorf("cannot resolve symlink %q: %w",
					filePath, err)
			}

			filePath = targetPath

			info, err = os.Stat(filePath)
			if err != nil {
				return fmt.Errorf("cannot stat %q: %w", filePath, err)
			}
		}

		if !info.IsDir() {
			return fn(virtualPath, filePath, info)
		}

		for _, ancestor := range ancestors {
			if os.SameFile(ancestor, info) {
				return fmt.Errorf("symlink loop at %q (%q)", virtualPath,
					filePath)
			}
		}

		ancestors = append(ancestors, info)
		defer func() {
			ancestors = ancestors[:len(ancestors)-1]
		}()

		entries, err := os.ReadDir(filePath)
		if err != nil {
			return fmt.Errorf("cannot list directory %q: %w", filePath, err)
		}

		for _, entry := range entries {
			childInfo, err := entry.Info()
			if err != nil {
				return fmt.Errorf("cannot stat %q: %w",
					path.Join(filePath, entry.Name()), err)
			}

			childVirtualPath := path.Join(virtualPath, entry.Name())
			childFilePath := path.Join(filePath, entry.Name())

			err = walk(childVirtualPath, childFilePath, childInfo)
			if err != nil {
				return err
			}
		}

		return nil
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot stat %q: %w", filePath, err)
	}

	return walk(filePath, filePath, info)
}
