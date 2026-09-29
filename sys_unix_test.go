// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build unix

package osfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestEtcSymlink is the case the package exists for: a client plants a link
// to /etc and asks the server to read through it.
func TestEtcSymlink(t *testing.T) {
	if _, err := os.ReadFile("/etc/passwd"); err != nil {
		t.Skipf("no /etc/passwd to aim at: %v", err)
	}
	fsys, _ := tree(t)
	must(t, fsys.Symlink("/etc", "etc"))
	must(t, fsys.Symlink("/etc/passwd", "passwd"))
	for _, p := range []string{"etc/passwd", "passwd", "/etc/passwd"} {
		if b, err := fsys.ReadFile(p); err == nil {
			t.Errorf("ReadFile(%q) read the host's /etc/passwd (%d bytes)", p, len(b))
		}
		if _, err := fsys.OpenFile(p); err == nil {
			t.Errorf("OpenFile(%q) opened the host's /etc/passwd", p)
		}
	}
	if _, err := fsys.ListDir("etc"); err == nil {
		t.Error("ListDir listed the host's /etc")
	}
	// Control: the same names, as real entries of the tree, are served.
	must(t, fsys.DeleteFile("etc"))
	must(t, fsys.MkDir("etc", 0o755))
	must(t, fsys.WriteFile("etc/passwd", []byte("tree"), 0o644))
	if b, err := fsys.ReadFile("/etc/passwd"); err != nil || string(b) != "tree" {
		t.Errorf("control: %q, %v", b, err)
	}
}

// TestFIFOIsRefusedNotWaitedOn: a FIFO in the tree must not hang the server.
// Every call returns; the test's own deadline is the proof.
func TestFIFOIsRefusedNotWaitedOn(t *testing.T) {
	fsys, base := tree(t)
	fifo := filepath.Join(base, "tree", "fifo")
	must(t, syscall.Mkfifo(fifo, 0o644))

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := fsys.ReadFile("fifo"); !errors.Is(err, errNotRegular) {
			t.Errorf("ReadFile(fifo) = %v", err)
		}
		if _, err := fsys.OpenFile("fifo"); !errors.Is(err, errNotRegular) {
			t.Errorf("OpenFile(fifo) = %v", err)
		}
		if _, err := fsys.ListDir("fifo"); err == nil {
			t.Error("ListDir(fifo) succeeded")
		}
		// With no reader, a write-open fails at once (ENXIO) ...
		if err := fsys.WriteFile("fifo", []byte("x"), 0o644); err == nil {
			t.Error("WriteFile(fifo) with no reader succeeded")
		}
		// ... and with one it opens, and the type check refuses it.
		r, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Error(err)
			return
		}
		defer r.Close()
		if err := fsys.WriteFile("fifo", []byte("x"), 0o644); !errors.Is(err, errNotRegular) {
			t.Errorf("WriteFile(fifo) with a reader = %v", err)
		}
		if err := fsys.Truncate("fifo", 0); !errors.Is(err, errNotRegular) {
			t.Errorf("Truncate(fifo) = %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("a call on a FIFO blocked")
	}
	st, err := fsys.Stat("fifo")
	must(t, err)
	if st.Mode()&0o170000 != 0o010000 {
		t.Errorf("Stat(fifo) mode %o", st.Mode())
	}
	ents, err := fsys.ListDir("/")
	must(t, err)
	for _, e := range ents {
		if e.Name() == "fifo" && e.FileType() != 1 {
			t.Errorf("fifo FileType %d", e.FileType())
		}
	}
}

// TestUnreadableFileIsAnError: a file that cannot be opened even for reading
// is an error, not a read-only File. (Windows has no mode that denies reading.)
func TestUnreadableFileIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens a 0000 file")
	}
	fsys, _ := tree(t)
	// A file that cannot be opened even for reading is an error.
	must(t, fsys.WriteFile("sealed", []byte("s"), 0o000))
	if _, err := fsys.OpenFile("sealed"); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("OpenFile(0000) = %v", err)
	}
}

func TestInode(t *testing.T) {
	fsys, base := tree(t)
	st, err := fsys.Stat("inside")
	must(t, err)
	fi, err := os.Stat(filepath.Join(base, "tree", "inside"))
	must(t, err)
	if st.Inode() != fi.Sys().(*syscall.Stat_t).Ino {
		t.Errorf("Inode %d, host %d", st.Inode(), fi.Sys().(*syscall.Stat_t).Ino)
	}
	if inodeOf(fakeInfo{}) != 0 {
		t.Error("an inode from a FileInfo without Stat_t")
	}
}

type fakeInfo struct{ os.FileInfo }

func (fakeInfo) Sys() any { return nil }

func TestIsROFS(t *testing.T) {
	if !isROFS(&fs.PathError{Err: syscall.EROFS}) || isROFS(syscall.EACCES) {
		t.Error("isROFS")
	}
}
