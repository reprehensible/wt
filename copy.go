package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

var defaultCopyConfigItems = []string{"AGENTS.md", "CLAUDE.md"}
var defaultCopyConfigRecursive = []string{".env"}
var defaultCopyLibItems = []string{"node_modules"}

var (
	osMkdirAll      = os.MkdirAll
	osStat          = os.Stat
	osOpen          = os.Open
	osOpenFile      = os.OpenFile
	filepathWalkDir = filepath.WalkDir
	ioCopy          = io.Copy
)

func copyItems(srcRoot, dstRoot string, items []string) error {
	for _, item := range items {
		src := filepath.Join(srcRoot, item)
		info, err := osStat(src)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if info.IsDir() {
			if err := copyDir(src, filepath.Join(dstRoot, item)); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(src, filepath.Join(dstRoot, item), info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func copyMatchingFiles(srcRoot, dstRoot string, names []string) error {
	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}
	return filepathWalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(stderr, "warning: cannot access %s: %v\n", path, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !nameSet[d.Name()] {
			return nil
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, filepath.Join(dstRoot, rel), info.Mode())
	})
}

func copyDir(src, dst string) error {
	return filepathWalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(stderr, "warning: cannot access %s: %v\n", path, err)
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return osMkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	if err := osMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := osOpen(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := osOpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := ioCopy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
