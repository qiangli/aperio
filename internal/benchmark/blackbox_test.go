package benchmark

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0644)

	files, err := snapshotFiles(dir)
	if err != nil {
		t.Fatalf("snapshotFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
	if files["a.txt"].size != 5 {
		t.Errorf("a.txt size: expected 5, got %d", files["a.txt"].size)
	}
}

func TestSnapshotFilesSkipsGit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	files, _ := snapshotFiles(dir)
	if len(files) != 1 {
		t.Errorf("expected 1 file (skipping .git), got %d", len(files))
	}
}

func TestDiffFilesCreated(t *testing.T) {
	before := map[string]fileInfo{
		"a.txt": {path: "a.txt", size: 5, modTime: time.Now()},
	}
	after := map[string]fileInfo{
		"a.txt": {path: "a.txt", size: 5, modTime: before["a.txt"].modTime},
		"b.txt": {path: "b.txt", size: 10, modTime: time.Now()},
	}

	changes := diffFiles(before, after)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].changeType != "created" {
		t.Errorf("expected 'created', got %s", changes[0].changeType)
	}
	if changes[0].path != "b.txt" {
		t.Errorf("expected b.txt, got %s", changes[0].path)
	}
}

func TestDiffFilesModified(t *testing.T) {
	now := time.Now()
	before := map[string]fileInfo{
		"a.txt": {path: "a.txt", size: 5, modTime: now},
	}
	after := map[string]fileInfo{
		"a.txt": {path: "a.txt", size: 10, modTime: now.Add(time.Second)},
	}

	changes := diffFiles(before, after)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].changeType != "modified" {
		t.Errorf("expected 'modified', got %s", changes[0].changeType)
	}
}

func TestDiffFilesNoChanges(t *testing.T) {
	now := time.Now()
	files := map[string]fileInfo{
		"a.txt": {path: "a.txt", size: 5, modTime: now},
	}

	changes := diffFiles(files, files)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}
}

func TestDiffFilesEmpty(t *testing.T) {
	changes := diffFiles(nil, nil)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}
}
