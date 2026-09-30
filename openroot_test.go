// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRoot(t *testing.T) {
	if _, err := OpenRoot(nil); err == nil {
		t.Error("OpenRoot(nil) succeeded")
	}

	closed, err := os.OpenRoot(t.TempDir())
	must(t, err)
	must(t, closed.Close())
	if _, err := OpenRoot(closed); err == nil {
		t.Error("OpenRoot of a closed Root succeeded")
	}

	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "f"), []byte("data"), 0o644))
	r, err := os.OpenRoot(dir)
	must(t, err)
	fsys, err := OpenRoot(r, ReadOnly(), MaxReadFile(3))
	must(t, err)
	if fsys.dir != "" {
		t.Errorf("dir = %q, want none", fsys.dir)
	}
	if err := fsys.WriteFile("g", nil, 0o644); !errors.Is(err, ErrReadOnly) {
		t.Errorf("WriteFile = %v, want ErrReadOnly", err)
	}
	if _, err := fsys.ReadFile("f"); !errors.Is(err, ErrTooLarge) {
		t.Errorf("ReadFile = %v, want ErrTooLarge", err)
	}
	total, _, err := fsys.Usage()
	switch {
	case errors.Is(err, errors.ErrUnsupported):
	case err != nil || total == 0:
		t.Errorf("Usage = %d, %v", total, err)
	}

	// The FS owns r: closing it closes r.
	must(t, fsys.Close())
	if _, err := r.Stat("."); err == nil {
		t.Error("the Root is still open after FS.Close")
	}
}

// TestOpenRootSurvivesASwap is the reason OpenRoot exists: a server checks a
// share's path, opens it as an os.Root, and only then hands it over. If the
// path is replaced by a symbolic link to somewhere else in between, the FS
// must go on serving the directory that was checked — the one the Root's
// descriptor holds — and never the link's target.
func TestOpenRootSurvivesASwap(t *testing.T) {
	base := t.TempDir()
	share := filepath.Join(base, "share")
	must(t, os.Mkdir(share, 0o755))
	must(t, os.WriteFile(filepath.Join(share, "f"), []byte("checked"), 0o644))
	outside := filepath.Join(base, "outside")
	must(t, os.Mkdir(outside, 0o755))
	must(t, os.WriteFile(filepath.Join(outside, "f"), []byte("escaped"), 0o644))

	r, err := os.OpenRoot(share)
	must(t, err)
	fsys, err := OpenRoot(r)
	must(t, err)
	defer fsys.Close()

	if err := os.Rename(share, filepath.Join(base, "moved")); err != nil {
		t.Skipf("the host will not rename an open directory: %v", err)
	}
	symlinkOrSkip(t, outside, share)

	if b, err := fsys.ReadFile("f"); err != nil || string(b) != "checked" {
		t.Errorf("ReadFile after the swap = %q, %v; want the checked tree", b, err)
	}
	if _, err := fsys.ReadFile("../outside/f"); err == nil {
		t.Error("climbed out of the Root")
	}
}
