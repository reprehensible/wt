package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyItemsAndCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	if err := os.MkdirAll(filepath.Join(src, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "node_modules", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("env"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := copyItems(src, dst, []string{"node_modules", ".env", "missing"}); err != nil {
		t.Fatalf("copy items: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules", "a.txt")); err != nil {
		t.Fatalf("expected copied dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".env")); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}
}

func TestCopyItemsStatError(t *testing.T) {
	oldStat := osStat
	defer func() { osStat = oldStat }()
	osStat = func(name string) (fs.FileInfo, error) {
		return nil, errors.New("stat fail")
	}

	if err := copyItems("/src", "/dst", []string{"file"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCopyItemsCopyDirError(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	oldWalk := filepathWalkDir
	defer func() { filepathWalkDir = oldWalk }()
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return errors.New("walk fail")
	}

	if err := copyItems(src, t.TempDir(), []string{"node_modules"}); err == nil {
		t.Fatalf("expected copy dir error")
	}
}

func TestCopyItemsCopyFileError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	filePath := filepath.Join(src, ".env")

	if err := os.WriteFile(filePath, []byte("env"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldOpen := osOpen
	defer func() { osOpen = oldOpen }()
	osOpen = func(name string) (*os.File, error) {
		return nil, errors.New("open fail")
	}

	if err := copyItems(src, dst, []string{".env"}); err == nil {
		t.Fatalf("expected copy file error")
	}
}

func TestCopyDirErrors(t *testing.T) {
	oldWalk := filepathWalkDir
	oldMkdir := osMkdirAll
	oldStderr := stderr
	defer func() {
		filepathWalkDir = oldWalk
		osMkdirAll = oldMkdir
		stderr = oldStderr
	}()

	// Walk error should warn and continue
	var buf bytes.Buffer
	stderr = &buf
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("walk fail"))
	}
	if err := copyDir("/src", "/dst"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "warning:") {
		t.Fatalf("expected warning, got %q", buf.String())
	}

	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, "file"), fakeDirEntry{name: "file", isDir: false, infoErr: errors.New("info fail")}, nil)
	}
	if err := copyDir("root", "/dst"); err == nil {
		t.Fatalf("expected info error")
	}

	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn("dir", fakeDirEntry{name: "dir", isDir: true}, nil)
	}
	osMkdirAll = func(path string, perm fs.FileMode) error {
		return errors.New("mkdir fail")
	}
	if err := copyDir("/src", "/dst"); err == nil {
		t.Fatalf("expected mkdir error")
	}

	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn("file", fakeDirEntry{name: "file", isDir: false}, nil)
	}
	if err := copyDir("", "/dst"); err == nil {
		t.Fatalf("expected rel error")
	}
}

func TestCopyMatchingFilesSuccess(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create .env in root and subdirectory
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("ROOT"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", ".env"), []byte("SUB"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Create a file that shouldn't be copied
	if err := os.WriteFile(filepath.Join(src, "other.txt"), []byte("OTHER"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := copyMatchingFiles(src, dst, []string{".env"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check root .env
	content, err := os.ReadFile(filepath.Join(dst, ".env"))
	if err != nil {
		t.Fatalf("read root .env: %v", err)
	}
	if string(content) != "ROOT" {
		t.Fatalf("expected ROOT, got %q", content)
	}

	// Check sub/.env
	content, err = os.ReadFile(filepath.Join(dst, "sub", ".env"))
	if err != nil {
		t.Fatalf("read sub/.env: %v", err)
	}
	if string(content) != "SUB" {
		t.Fatalf("expected SUB, got %q", content)
	}

	// Check other.txt was not copied
	if _, err := os.Stat(filepath.Join(dst, "other.txt")); !os.IsNotExist(err) {
		t.Fatalf("other.txt should not be copied")
	}
}

func TestCopyMatchingFilesErrors(t *testing.T) {
	oldWalk := filepathWalkDir
	oldStderr := stderr
	defer func() {
		filepathWalkDir = oldWalk
		stderr = oldStderr
	}()

	// Walk error should warn and continue
	var buf bytes.Buffer
	stderr = &buf
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("walk fail"))
	}
	if err := copyMatchingFiles("/src", "/dst", []string{".env"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "warning:") {
		t.Fatalf("expected warning, got %q", buf.String())
	}

	// Info error
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, ".env"), fakeDirEntry{name: ".env", isDir: false, infoErr: errors.New("info fail")}, nil)
	}
	if err := copyMatchingFiles("/src", "/dst", []string{".env"}); err == nil {
		t.Fatalf("expected info error")
	}

	// Rel error (relative root with absolute path)
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn("/absolute/path/.env", fakeDirEntry{name: ".env", isDir: false}, nil)
	}
	if err := copyMatchingFiles("relative", "/dst", []string{".env"}); err == nil {
		t.Fatalf("expected rel error")
	}
}

func TestCopyMatchingFilesCopyError(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldOpen := osOpen
	defer func() { osOpen = oldOpen }()
	osOpen = func(name string) (*os.File, error) {
		return nil, errors.New("open fail")
	}

	if err := copyMatchingFiles(src, t.TempDir(), []string{".env"}); err == nil {
		t.Fatalf("expected copy error")
	}
}

func TestCopyFileErrors(t *testing.T) {
	oldMkdir := osMkdirAll
	oldOpen := osOpen
	oldOpenFile := osOpenFile
	oldCopy := ioCopy
	defer func() {
		osMkdirAll = oldMkdir
		osOpen = oldOpen
		osOpenFile = oldOpenFile
		ioCopy = oldCopy
	}()

	osMkdirAll = func(path string, perm fs.FileMode) error {
		return errors.New("mkdir fail")
	}
	if err := copyFile("src", "dst", 0o644); err == nil {
		t.Fatalf("expected mkdir error")
	}

	osMkdirAll = oldMkdir
	osOpen = func(name string) (*os.File, error) {
		return nil, errors.New("open fail")
	}
	if err := copyFile("src", "dst", 0o644); err == nil {
		t.Fatalf("expected open error")
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	osOpen = func(name string) (*os.File, error) {
		return os.Open(src)
	}
	osOpenFile = func(name string, flag int, perm fs.FileMode) (*os.File, error) {
		return nil, errors.New("openfile fail")
	}
	if err := copyFile(src, filepath.Join(tmp, "dst.txt"), 0o644); err == nil {
		t.Fatalf("expected openfile error")
	}

	osOpenFile = oldOpenFile
	ioCopy = func(dst io.Writer, src io.Reader) (int64, error) {
		return 0, errors.New("copy fail")
	}
	if err := copyFile(src, filepath.Join(tmp, "dst2.txt"), 0o644); err == nil {
		t.Fatalf("expected copy error")
	}
}

func TestCopyFileSuccess(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "dst.txt")

	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := copyFile(src, dst, 0o644); err != nil {
		t.Fatalf("copy: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "data" {
		t.Fatalf("unexpected data %q", string(data))
	}
}
