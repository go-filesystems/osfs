// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	filesystem "github.com/go-filesystems/interface"
)

// tree makes a directory "base" holding a file "secret" (content "outer") and
// the tree "base/tree" holding "inside" (content "inner"), and opens the tree.
// It returns base, so a test can check what happened outside.
func tree(t *testing.T, opts ...Option) (*FS, string) {
	t.Helper()
	base := t.TempDir()
	must(t, os.WriteFile(filepath.Join(base, "secret"), []byte("outer"), 0o644))
	must(t, os.Mkdir(filepath.Join(base, "tree"), 0o755))
	must(t, os.WriteFile(filepath.Join(base, "tree", "inside"), []byte("inner"), 0o644))
	fsys, err := Open(filepath.Join(base, "tree"), opts...)
	must(t, err)
	t.Cleanup(func() { fsys.Close() })
	return fsys, base
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// symlinkOrSkip creates a link on the host, skipping where the host does not
// let this process create one (Windows without the privilege).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create symlinks here: %v", err)
		}
		t.Fatal(err)
	}
}

func readHost(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	must(t, err)
	return string(b)
}

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "f"), nil, 0o644))

	if _, err := Open(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing dir: %v, want ErrNotExist", err)
	}
	if _, err := Open(filepath.Join(dir, "f")); err == nil {
		t.Error("a regular file opened as a tree")
	}
	fsys, err := Open(dir)
	must(t, err)
	if !filepath.IsAbs(fsys.dir) {
		t.Errorf("dir %q not absolute", fsys.dir)
	}
	must(t, fsys.Close())
	if _, err := fsys.ReadFile("f"); err == nil {
		t.Error("ReadFile after Close succeeded")
	}
	if _, _, err := fsys.Usage(); err == nil {
		t.Error("Usage after Close succeeded")
	}
}

// TestConformance drives every method of every capability once, checking the
// result on the host as well as through the FS.
func TestConformance(t *testing.T) {
	fsys, base := tree(t)
	host := filepath.Join(base, "tree")
	var (
		_ filesystem.Filesystem     = fsys
		_ filesystem.Opener         = fsys
		_ filesystem.Truncater      = fsys
		_ filesystem.MetadataSetter = fsys
		_ filesystem.Symlinker      = fsys
		_ filesystem.HardLinker     = fsys
	)

	// MkDir, WriteFile, ReadFile.
	must(t, fsys.MkDir("/d", 0o755))
	if err := fsys.MkDir("/d", 0o755); !errors.Is(err, fs.ErrExist) {
		t.Errorf("MkDir existing: %v", err)
	}
	if err := fsys.MkDir("/nope/d", 0o755); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("MkDir without parent: %v", err)
	}
	must(t, fsys.WriteFile("/d/a.txt", []byte("hello"), 0o640))
	if got := readHost(t, filepath.Join(host, "d", "a.txt")); got != "hello" {
		t.Errorf("host sees %q", got)
	}
	must(t, fsys.WriteFile("d/a.txt", []byte("hi"), 0o600)) // truncates
	if b, err := fsys.ReadFile("d/a.txt"); err != nil || string(b) != "hi" {
		t.Errorf("ReadFile = %q, %v", b, err)
	}
	if b, err := fsys.ReadFile("/d/empty"); err == nil {
		t.Errorf("ReadFile missing = %q", b)
	}
	must(t, fsys.WriteFile("/d/empty", nil, 0o644))
	if b, err := fsys.ReadFile("/d/empty"); err != nil || len(b) != 0 {
		t.Errorf("ReadFile empty = %q, %v", b, err)
	}
	if _, err := fsys.ReadFile("/d"); err == nil {
		t.Error("ReadFile of a directory succeeded")
	}
	if err := fsys.WriteFile("/d", []byte("x"), 0o644); err == nil {
		t.Error("WriteFile over a directory succeeded")
	}

	// Stat.
	st, err := fsys.Stat("/d/a.txt")
	must(t, err)
	if st.Mode()&0o170000 != 0o100000 || st.Size() != 2 {
		t.Errorf("Stat file: mode %o size %d", st.Mode(), st.Size())
	}
	if runtime.GOOS != "windows" && st.Mode()&0o777 != 0o640 {
		t.Errorf("Stat perm %o, want 640", st.Mode()&0o777)
	}
	for _, root := range []string{"", "/", ".", "//"} {
		st, err := fsys.Stat(root)
		must(t, err)
		if st.Mode()&0o170000 != 0o040000 {
			t.Errorf("Stat(%q) mode %o, want a directory", root, st.Mode())
		}
	}
	if _, err := fsys.Stat("/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat missing: %v", err)
	}

	// ListDir.
	must(t, fsys.WriteFile("/d/0first", []byte("z"), 0o644))
	must(t, fsys.MkDir("/d/sub", 0o755))
	ents, err := fsys.ListDir("/d")
	must(t, err)
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
		want := uint8(8)
		if e.Name() == "sub" {
			want = 4
		}
		if e.FileType() != want {
			t.Errorf("%s: FileType %d, want %d", e.Name(), e.FileType(), want)
		}
		if runtime.GOOS != "windows" && e.Inode() == 0 {
			t.Errorf("%s: inode 0", e.Name())
		}
	}
	if got, want := names, []string{"0first", "a.txt", "empty", "sub"}; !equal(got, want) {
		t.Errorf("ListDir = %v, want %v", got, want)
	}
	if _, err := fsys.ListDir("/d/a.txt"); err == nil {
		t.Error("ListDir of a file succeeded")
	}
	if _, err := fsys.ListDir("/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ListDir missing: %v", err)
	}

	// Rename.
	must(t, fsys.Rename("/d/0first", "/d/renamed"))
	if _, err := os.Stat(filepath.Join(host, "d", "renamed")); err != nil {
		t.Error(err)
	}
	must(t, fsys.Rename("/d/renamed", "/d/empty")) // replaces
	if b, _ := fsys.ReadFile("/d/empty"); string(b) != "z" {
		t.Errorf("after rename over: %q", b)
	}

	// Truncate.
	must(t, fsys.Truncate("/d/a.txt", 10))
	if b, _ := fsys.ReadFile("/d/a.txt"); !bytes.Equal(b, append([]byte("hi"), make([]byte, 8)...)) {
		t.Errorf("after grow: %q", b)
	}
	must(t, fsys.Truncate("/d/a.txt", 1))
	if b, _ := fsys.ReadFile("/d/a.txt"); string(b) != "h" {
		t.Errorf("after shrink: %q", b)
	}
	if err := fsys.Truncate("/d/a.txt", -1); err == nil {
		t.Error("negative Truncate succeeded")
	}
	if err := fsys.Truncate("/d", 0); err == nil {
		t.Error("Truncate of a directory succeeded")
	}

	// Chmod, Chown, Chtimes.
	must(t, fsys.Chmod("/d/a.txt", 0o600))
	if fi, _ := os.Stat(filepath.Join(host, "d", "a.txt")); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("host perm %v", fi.Mode().Perm())
	}
	if runtime.GOOS == "windows" {
		if err := fsys.Chown("/d/a.txt", 0, 0); err == nil {
			t.Error("Chown succeeded on Windows")
		}
	} else {
		must(t, fsys.Chown("/d/a.txt", uint32(os.Getuid()), uint32(os.Getgid())))
	}
	when := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	must(t, fsys.Chtimes("/d/a.txt", when, when))
	if fi, _ := os.Stat(filepath.Join(host, "d", "a.txt")); !fi.ModTime().Equal(when) {
		t.Errorf("host mtime %v", fi.ModTime())
	}

	// Link.
	must(t, fsys.Link("/d/a.txt", "/d/hard"))
	a, _ := os.Stat(filepath.Join(host, "d", "a.txt"))
	h, _ := os.Stat(filepath.Join(host, "d", "hard"))
	if !os.SameFile(a, h) {
		t.Error("hard link is not the same file")
	}
	if err := fsys.Link("/d/sub", "/d/subhard"); err == nil {
		t.Error("hard link to a directory succeeded")
	}
	if err := fsys.Link("/d/nope", "/d/x"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Link missing: %v", err)
	}
	if err := fsys.Link("/d/a.txt", "/d/hard"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Link over existing: %v", err)
	}

	// OpenFile.
	f, err := fsys.OpenFile("/d/a.txt")
	must(t, err)
	w, ok := f.(filesystem.WritableFile)
	if !ok {
		t.Fatal("OpenFile on a writable FS returned a read-only File")
	}
	if _, err := w.WriteAt([]byte("ello"), 1); err != nil {
		t.Fatal(err)
	}
	must(t, w.Sync())
	must(t, w.Close())
	if b, _ := fsys.ReadFile("/d/hard"); string(b) != "hello" {
		t.Errorf("through the hard link: %q", b)
	}
	if _, err := fsys.OpenFile("/d"); err == nil {
		t.Error("OpenFile of a directory succeeded")
	}
	if _, err := fsys.OpenFile("/d/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("OpenFile missing: %v", err)
	}

	// Symlink, ReadLink.
	if err := fsys.Symlink("a.txt", "/d/sym"); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create symlinks here: %v", err)
		}
		t.Fatal(err)
	}
	if tgt, err := fsys.ReadLink("/d/sym"); err != nil || tgt != "a.txt" {
		t.Errorf("ReadLink = %q, %v", tgt, err)
	}
	if err := fsys.Symlink("a.txt", "/d/sym"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Symlink over existing: %v", err)
	}
	must(t, fsys.Symlink("sub/deeper/x", "/d/rel")) // targets are not resolved
	if tgt, _ := fsys.ReadLink("/d/rel"); tgt != "sub/deeper/x" {
		t.Errorf("ReadLink slash target = %q", tgt)
	}
	if _, err := fsys.ReadLink("/d/a.txt"); err == nil {
		t.Error("ReadLink of a regular file succeeded")
	}
	st, err = fsys.Stat("/d/sym")
	must(t, err)
	if st.Mode()&0o170000 != 0o120000 {
		t.Errorf("Stat followed the link: mode %o", st.Mode())
	}
	if b, err := fsys.ReadFile("/d/sym"); err != nil || string(b) != "hello" {
		t.Errorf("ReadFile through an in-tree link = %q, %v", b, err)
	}
	ents, err = fsys.ListDir("/d")
	must(t, err)
	for _, e := range ents {
		if e.Name() == "sym" && e.FileType() != 10 {
			t.Errorf("symlink FileType %d", e.FileType())
		}
	}

	// DeleteFile, DeleteDir.
	must(t, fsys.DeleteFile("/d/sym"))
	if _, err := os.Stat(filepath.Join(host, "d", "a.txt")); err != nil {
		t.Error("deleting a link removed its target")
	}
	if err := fsys.DeleteFile("/d/sub"); err == nil {
		t.Error("DeleteFile of a directory succeeded")
	}
	if err := fsys.DeleteFile("/d/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("DeleteFile missing: %v", err)
	}
	if err := fsys.DeleteDir("/d"); err == nil {
		t.Error("DeleteDir of a non-empty directory succeeded")
	}
	if err := fsys.DeleteDir("/d/a.txt"); err == nil {
		t.Error("DeleteDir of a file succeeded")
	}
	if err := fsys.DeleteDir("/d/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("DeleteDir missing: %v", err)
	}
	must(t, fsys.DeleteDir("/d/sub"))
	for _, n := range []string{"a.txt", "hard", "empty", "rel"} {
		must(t, fsys.DeleteFile("/d/"+n))
	}
	must(t, fsys.DeleteDir("/d"))
	if _, err := os.Stat(filepath.Join(host, "d")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("host still has d: %v", err)
	}
	if err := fsys.DeleteDir("/"); err == nil {
		t.Error("DeleteDir of the root succeeded")
	}
	if _, err := os.Stat(host); err != nil {
		t.Errorf("root gone: %v", err)
	}

	// Usage.
	total, free, err := fsys.Usage()
	must(t, err)
	if total == 0 || free > total {
		t.Errorf("Usage = %d, %d", total, free)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestReadAtContract checks io.ReaderAt's rules as the interface states them.
func TestReadAtContract(t *testing.T) {
	fsys, _ := tree(t)
	must(t, fsys.WriteFile("f", []byte("0123456789"), 0o644))
	f, err := fsys.OpenFile("/f")
	must(t, err)
	defer f.Close()
	if f.Size() != 10 {
		t.Fatalf("Size %d", f.Size())
	}
	buf := make([]byte, 4)
	if n, err := f.ReadAt(buf, 2); n != 4 || err != nil || string(buf) != "2345" {
		t.Errorf("middle: %d %v %q", n, err, buf)
	}
	if n, err := f.ReadAt(buf, 6); n != 4 || (err != nil && err != io.EOF) || string(buf) != "6789" {
		t.Errorf("exact end: %d %v %q", n, err, buf)
	}
	if n, err := f.ReadAt(buf, 8); n != 2 || err != io.EOF || string(buf[:2]) != "89" {
		t.Errorf("short at end: %d %v", n, err)
	}
	for _, off := range []int64{10, 11, 1 << 40} {
		if n, err := f.ReadAt(buf, off); n != 0 || err != io.EOF {
			t.Errorf("at %d: %d %v", off, n, err)
		}
	}
	if _, err := f.ReadAt(buf, -1); err == nil || err == io.EOF {
		t.Errorf("negative offset: %v", err)
	}

	// A read-only File is a snapshot: growing the file elsewhere does not
	// move its end.
	must(t, fsys.WriteFile("g", []byte("abc"), 0o644))
	g, err := fsys.OpenFile("g")
	must(t, err)
	must(t, os.WriteFile(filepath.Join(fsys.dir, "g"), []byte("abcdef"), 0o644))
	if n, err := g.ReadAt(buf, 0); n != 3 || err != io.EOF {
		t.Errorf("snapshot end: %d %v", n, err)
	}
	// and a file shrunk elsewhere ends early, with io.EOF.
	must(t, os.Truncate(filepath.Join(fsys.dir, "g"), 1))
	if n, err := g.ReadAt(buf, 0); n != 1 || err != io.EOF {
		t.Errorf("shrunk elsewhere: %d %v", n, err)
	}
	must(t, g.Close())
}

func TestWritableFile(t *testing.T) {
	fsys, _ := tree(t)
	must(t, fsys.WriteFile("f", []byte("abc"), 0o644))
	f, err := fsys.OpenFile("f")
	must(t, err)
	w := f.(filesystem.WritableFile)
	defer w.Close()

	if n, err := w.WriteAt([]byte("XY"), 1); n != 2 || err != nil || w.Size() != 3 {
		t.Errorf("overwrite: %d %v size %d", n, err, w.Size())
	}
	if _, err := w.WriteAt([]byte("Z"), 9); err != nil || w.Size() != 10 {
		t.Errorf("extend: %v size %d", err, w.Size())
	}
	buf := make([]byte, 16)
	n, err := w.ReadAt(buf, 0)
	if n != 10 || err != io.EOF || !bytes.Equal(buf[:n], []byte("aXY\x00\x00\x00\x00\x00\x00Z")) {
		t.Errorf("read back: %d %v %q", n, err, buf[:n])
	}
	if _, err := w.WriteAt([]byte("x"), -1); err == nil {
		t.Error("negative WriteAt succeeded")
	}
	must(t, w.Truncate(2))
	if w.Size() != 2 {
		t.Errorf("Size after truncate %d", w.Size())
	}
	must(t, w.Truncate(5))
	if w.Size() != 5 {
		t.Errorf("Size after grow %d", w.Size())
	}
	if err := w.Truncate(-1); err == nil {
		t.Error("negative Truncate succeeded")
	}
	must(t, w.Sync())
	if b := readHost(t, filepath.Join(fsys.dir, "f")); b != "aX\x00\x00\x00" {
		t.Errorf("host sees %q", b)
	}
	must(t, w.Close())
	if err := w.Truncate(1); err == nil {
		t.Error("Truncate after Close succeeded")
	}
	if _, err := w.WriteAt([]byte("x"), 0); err == nil {
		t.Error("WriteAt after Close succeeded")
	}
}

// TestConcurrentIO runs parallel ReadAt and non-overlapping WriteAt on one
// handle; under -race it proves the handle is safe on io.ReaderAt/WriterAt's
// terms and that Size ends at the furthest write.
func TestConcurrentIO(t *testing.T) {
	fsys, _ := tree(t)
	must(t, fsys.WriteFile("f", nil, 0o644))
	f, err := fsys.OpenFile("f")
	must(t, err)
	w := f.(filesystem.WritableFile)
	defer w.Close()
	const blocks, bs = 32, 512
	var wg sync.WaitGroup
	for i := range blocks {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := w.WriteAt(bytes.Repeat([]byte{byte(i)}, bs), int64(i*bs)); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			buf := make([]byte, bs)
			if _, err := w.ReadAt(buf, int64(i*bs)); err != nil && err != io.EOF {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if w.Size() != blocks*bs {
		t.Fatalf("Size %d", w.Size())
	}
	buf := make([]byte, blocks*bs)
	if _, err := w.ReadAt(buf, 0); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	for i := range blocks {
		if !bytes.Equal(buf[i*bs:(i+1)*bs], bytes.Repeat([]byte{byte(i)}, bs)) {
			t.Fatalf("block %d wrong", i)
		}
	}
}

// TestCloseInvalidatesFiles proves the interface's rule that closing the FS
// invalidates every File obtained from it, and that a File closed on its own
// is forgotten.
func TestCloseInvalidatesFiles(t *testing.T) {
	fsys, _ := tree(t)
	a, err := fsys.OpenFile("inside")
	must(t, err)
	b, err := fsys.OpenFile("inside")
	must(t, err)
	must(t, b.Close())
	if len(fsys.files) != 1 {
		t.Fatalf("%d files tracked, want 1", len(fsys.files))
	}
	buf := make([]byte, 1)
	if _, err := a.ReadAt(buf, 0); err != nil {
		t.Fatal(err) // positive control: it worked before Close
	}
	must(t, fsys.Close())
	if _, err := a.ReadAt(buf, 0); err == nil {
		t.Error("ReadAt after FS.Close succeeded")
	}
	if _, err := fsys.OpenFile("inside"); err == nil {
		t.Error("OpenFile after Close succeeded")
	}

	// An open that races Close is closed and refused rather than tracked in a
	// map that no longer exists.
	f, err := os.Open(filepath.Join(fsys.dir, "inside"))
	must(t, err)
	if _, err := fsys.track(&file{fsys: fsys, f: f, name: "inside"}, true); !errors.Is(err, fs.ErrClosed) {
		t.Errorf("track after Close: %v", err)
	}
	if _, err := f.Stat(); err == nil {
		t.Error("the refused file was left open")
	}
}

func TestPaths(t *testing.T) {
	for in, want := range map[string]string{
		"":       ".",
		"/":      ".",
		"//":     ".",
		"a":      filepath.FromSlash("a"),
		"/a/b":   filepath.FromSlash("a/b"),
		"//a/b/": filepath.FromSlash("a/b/"),
		"a/../b": filepath.FromSlash("a/../b"), // left for the Root
	} {
		if got := rel(in); got != want {
			t.Errorf("rel(%q) = %q, want %q", in, got, want)
		}
	}
	fsys, _ := tree(t)
	for _, p := range []string{"inside", "/inside", "//inside", "./inside"} {
		if b, err := fsys.ReadFile(p); err != nil || string(b) != "inner" {
			t.Errorf("ReadFile(%q) = %q, %v", p, b, err)
		}
	}
}

func TestModeEncoding(t *testing.T) {
	for m, want := range map[fs.FileMode]uint16{
		0o644:                    0o100644,
		fs.ModeDir | 0o755:       0o040755,
		fs.ModeSymlink | 0o777:   0o120777,
		fs.ModeNamedPipe | 0o600: 0o010600,
		fs.ModeSocket | 0o700:    0o140700,
		fs.ModeDevice | fs.ModeCharDevice | 0o666:             0o020666,
		fs.ModeDevice | 0o660:                                 0o060660,
		fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky | 0o755: 0o107755,
		fs.ModeIrregular:                                      0o100000,
	} {
		if got := posixMode(m); got != want {
			t.Errorf("posixMode(%v) = %o, want %o", m, got, want)
		}
	}
	for m, want := range map[fs.FileMode]uint8{
		0:                                 8,
		fs.ModeDir:                        4,
		fs.ModeSymlink:                    10,
		fs.ModeNamedPipe:                  1,
		fs.ModeSocket:                     12,
		fs.ModeDevice | fs.ModeCharDevice: 2,
		fs.ModeDevice:                     6,
		fs.ModeIrregular:                  0,
	} {
		if got := direntType(m); got != want {
			t.Errorf("direntType(%v) = %d, want %d", m, got, want)
		}
	}
}
