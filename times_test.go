// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	filesystem "github.com/go-filesystems/interface"
)

func TestTimesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fsys, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	if err := fsys.WriteFile("/f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	mt := time.Date(2010, 11, 12, 13, 14, 15, 0, time.UTC)
	if err := fsys.Chtimes("/f", at, mt); err != nil {
		t.Fatal(err)
	}
	var tr filesystem.TimesReader = fsys
	ft, err := tr.Times("/f")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := ft.Mtime.Get(); !ok || !got.Equal(mt) {
		t.Fatalf("mtime = %v %v, want %v", got, ok, mt)
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return // mtime only
	}
	if got, ok := ft.Atime.Get(); !ok || !got.Equal(at) {
		t.Fatalf("atime = %v %v, want %v", got, ok, at)
	}
	if runtime.GOOS != "windows" {
		// Chtimes refreshes ctime to now (POSIX): after the mtime we set.
		c, ok := ft.Ctime.Get()
		if !ok || !c.After(mt) {
			t.Fatalf("ctime = %v %v", c, ok)
		}
	}
	if runtime.GOOS == "windows" && ft.Ctime.Recorded() {
		t.Fatal("windows has no ctime in FileInfo")
	}
	if b, ok := ft.Btime.Get(); ok && b.IsZero() {
		t.Fatal("recorded zero btime")
	}
}

func TestTimesDirectoryAndSymlinkNotFollowed(t *testing.T) {
	dir := t.TempDir()
	fsys, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	if err := fsys.MkDir("/d", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.Times("/d"); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile("/f", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := fsys.Chtimes("/f", old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("f", filepath.Join(dir, "l")); err != nil {
		t.Skip("no symlinks:", err)
	}
	ft, err := fsys.Times("/l")
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := ft.Mtime.Get(); m.Equal(old) {
		t.Fatal("symlink was followed")
	}
	if runtime.GOOS == "linux" && ft.Btime.Recorded() {
		t.Fatal("a symlink reports no btime on linux")
	}
}

func TestTimesNotExist(t *testing.T) {
	fsys, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	if _, err := fsys.Times("/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}
