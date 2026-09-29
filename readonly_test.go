// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	filesystem "github.com/go-filesystems/interface"
)

// mutations are every mutating method, each aimed at a path where it would
// succeed on a writable FS.
var mutations = map[string]func(*FS) error{
	"WriteFile":  func(f *FS) error { return f.WriteFile("new", []byte("x"), 0o644) },
	"MkDir":      func(f *FS) error { return f.MkDir("newdir", 0o755) },
	"DeleteFile": func(f *FS) error { return f.DeleteFile("victim") },
	"DeleteDir":  func(f *FS) error { return f.DeleteDir("emptydir") },
	"Rename":     func(f *FS) error { return f.Rename("victim", "moved") },
	"Truncate":   func(f *FS) error { return f.Truncate("inside", 0) },
	"Chmod":      func(f *FS) error { return f.Chmod("inside", 0o600) },
	"Chown": func(f *FS) error {
		return f.Chown("inside", uint32(os.Getuid()), uint32(os.Getgid()))
	},
	"Chtimes": func(f *FS) error { return f.Chtimes("inside", time.Unix(1, 0), time.Unix(1, 0)) },
	"Symlink": func(f *FS) error { return f.Symlink("inside", "newlink") },
	"Link":    func(f *FS) error { return f.Link("inside", "newhard") },
}

func roFixture(t *testing.T, opts ...Option) (*FS, string) {
	fsys, base := tree(t, opts...)
	host := filepath.Join(base, "tree")
	must(t, os.WriteFile(filepath.Join(host, "victim"), []byte("v"), 0o644))
	must(t, os.Mkdir(filepath.Join(host, "emptydir"), 0o755))
	return fsys, host
}

func TestReadOnlyRefusesEveryMutation(t *testing.T) {
	for name, op := range mutations {
		t.Run(name, func(t *testing.T) {
			ro, host := roFixture(t, ReadOnly())
			before := snapshot(t, host)
			err := op(ro)
			if !errors.Is(err, ErrReadOnly) {
				t.Fatalf("%s on a read-only FS: %v, want ErrReadOnly", name, err)
			}
			if after := snapshot(t, host); after != before {
				t.Errorf("%s changed the tree:\n%s\n->\n%s", name, before, after)
			}

			// Control: the same call on a writable FS succeeds, so the
			// refusal above is the read-only mode and nothing else. (Chown
			// is the one the host may refuse on its own: Windows has none.)
			rw, _ := roFixture(t)
			if err := op(rw); err != nil && !(name == "Chown" && isWindows) &&
				!(name == "Symlink" && isWindows) {
				t.Errorf("control: %s on a writable FS: %v", name, err)
			}
		})
	}
}

func TestReadOnlyStillReads(t *testing.T) {
	ro, _ := roFixture(t, ReadOnly())
	if b, err := ro.ReadFile("inside"); err != nil || string(b) != "inner" {
		t.Errorf("ReadFile = %q, %v", b, err)
	}
	if ents, err := ro.ListDir("/"); err != nil || len(ents) != 3 {
		t.Errorf("ListDir = %v, %v", ents, err)
	}
	if _, err := ro.Stat("inside"); err != nil {
		t.Error(err)
	}
	if _, _, err := ro.Usage(); err != nil {
		t.Error(err)
	}
	f, err := ro.OpenFile("inside")
	must(t, err)
	defer f.Close()
	if _, ok := f.(filesystem.WritableFile); ok {
		t.Fatal("OpenFile on a read-only FS returned a WritableFile")
	}
	// The descriptor itself is O_RDONLY: writing to it fails at the host.
	if _, err := f.(*file).f.WriteAt([]byte("x"), 0); err == nil {
		t.Error("the read-only File's descriptor accepts writes")
	}
	// Control: on a writable FS the descriptor does accept them.
	rw, _ := roFixture(t)
	g, err := rw.OpenFile("inside")
	must(t, err)
	defer g.Close()
	if _, err := g.(*writableFile).f.WriteAt([]byte("i"), 0); err != nil {
		t.Errorf("control: %v", err)
	}
}

// snapshot describes the tree: names, sizes, modes and times.
func snapshot(t *testing.T, root string) string {
	t.Helper()
	var s string
	must(t, filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		s += p + " " + fi.Mode().String() + " " + fi.ModTime().String() + " " + string(rune(fi.Size())) + "\n"
		return nil
	}))
	return s
}
