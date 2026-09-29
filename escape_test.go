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
)

// Every escape test below pairs the refused operation with a POSITIVE
// CONTROL: the same operation, on a path that stays in the tree, succeeding.
// Without it a test of "is refused" would pass just as well if everything
// failed. And each checks on the host that the outside was not touched.

// outsideIntact checks that base/secret still holds "outer" and nothing new
// appeared next to it.
func outsideIntact(t *testing.T, base string) {
	t.Helper()
	if got := readHost(t, filepath.Join(base, "secret")); got != "outer" {
		t.Errorf("outside file now holds %q", got)
	}
	ents, err := os.ReadDir(base)
	must(t, err)
	if len(ents) != 2 {
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Errorf("outside directory now holds %v", names)
	}
}

func TestDotDotIsRefused(t *testing.T) {
	fsys, base := tree(t)
	must(t, fsys.MkDir("a", 0o755))

	// Control: ".." that stays in the tree resolves.
	for _, p := range []string{"a/../inside", "/a/../inside", "./a/./../inside"} {
		if b, err := fsys.ReadFile(p); err != nil || string(b) != "inner" {
			t.Fatalf("control ReadFile(%q) = %q, %v", p, b, err)
		}
	}
	// And the file being aimed at is really there on the host.
	if readHost(t, filepath.Join(base, "secret")) != "outer" {
		t.Fatal("fixture broken")
	}

	for _, p := range []string{"../secret", "/../secret", "a/../../secret", "a/../../tree/../secret"} {
		if b, err := fsys.ReadFile(p); err == nil {
			t.Errorf("ReadFile(%q) escaped: %q", p, b)
		}
		if _, err := fsys.OpenFile(p); err == nil {
			t.Errorf("OpenFile(%q) escaped", p)
		}
		if _, err := fsys.Stat(p); err == nil {
			t.Errorf("Stat(%q) escaped", p)
		}
		if err := fsys.WriteFile(p, []byte("pwned"), 0o644); err == nil {
			t.Errorf("WriteFile(%q) escaped", p)
		}
		if err := fsys.Truncate(p, 0); err == nil {
			t.Errorf("Truncate(%q) escaped", p)
		}
		if err := fsys.Chmod(p, 0o777); err == nil {
			t.Errorf("Chmod(%q) escaped", p)
		}
		if err := fsys.Chtimes(p, time.Unix(0, 0), time.Unix(0, 0)); err == nil {
			t.Errorf("Chtimes(%q) escaped", p)
		}
		if err := fsys.DeleteFile(p); err == nil {
			t.Errorf("DeleteFile(%q) escaped", p)
		}
		if err := fsys.Link(p, "stolen"); err == nil {
			t.Errorf("Link(%q, stolen) escaped", p)
		}
		if err := fsys.Rename(p, "stolen"); err == nil {
			t.Errorf("Rename(%q, stolen) escaped", p)
		}
		if err := fsys.Rename("inside", p); err == nil {
			t.Errorf("Rename(inside, %q) escaped", p)
		}
	}
	for _, p := range []string{"..", "../", "a/../.."} {
		if _, err := fsys.ListDir(p); err == nil {
			t.Errorf("ListDir(%q) escaped", p)
		}
		if err := fsys.MkDir(p+"/newdir", 0o755); err == nil {
			t.Errorf("MkDir(%q/newdir) escaped", p)
		}
		if err := fsys.Symlink("x", p+"/newlink"); err == nil {
			t.Errorf("Symlink(%q/newlink) escaped", p)
		}
		if err := fsys.DeleteDir(p); err == nil {
			t.Errorf("DeleteDir(%q) escaped", p)
		}
	}
	if b, err := fsys.ReadFile("inside"); err != nil || string(b) != "inner" {
		t.Errorf("tree damaged: %q, %v", b, err)
	}
	outsideIntact(t, base)
}

// TestLeadingSlashStaysInside: "/secret" is the tree's own secret, never the
// host's. The tree gets its own file of that name so the two can be told
// apart.
func TestLeadingSlashStaysInside(t *testing.T) {
	fsys, base := tree(t)
	must(t, os.WriteFile(filepath.Join(base, "tree", "secret"), []byte("tree's own"), 0o644))
	for _, p := range []string{"/secret", "//secret", "///secret"} {
		if b, err := fsys.ReadFile(p); err != nil || string(b) != "tree's own" {
			t.Errorf("ReadFile(%q) = %q, %v", p, b, err)
		}
	}
	// The host's absolute path, handed over as an interface path, names a
	// file inside the tree — here, one that does not exist.
	abs := filepath.ToSlash(filepath.Join(base, "secret"))
	if b, err := fsys.ReadFile(abs); err == nil {
		t.Errorf("ReadFile(%q) read the host file: %q", abs, b)
	}
	outsideIntact(t, base)
}

// TestHostAbsolutePathIsRefused covers systems where a path can still be
// absolute after the leading slashes are gone: a drive letter or UNC name on
// Windows. The Root refuses them.
func TestHostAbsolutePathIsRefused(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("no absolute form survives the leading-slash trim here")
	}
	fsys, base := tree(t)
	abs := filepath.Join(base, "secret") // C:\...\secret
	if readHost(t, abs) != "outer" {
		t.Fatal("fixture broken")
	}
	if b, err := fsys.ReadFile(abs); err == nil {
		t.Errorf("ReadFile(%q) escaped: %q", abs, b)
	}
	if err := fsys.WriteFile(abs, []byte("pwned"), 0o644); err == nil {
		t.Errorf("WriteFile(%q) escaped", abs)
	}
	if b, err := fsys.ReadFile("inside"); err != nil || string(b) != "inner" {
		t.Errorf("control: %q, %v", b, err)
	}
	outsideIntact(t, base)
}

// TestPlantedSymlinkIsNotFollowed: a client may create a link to anywhere —
// Symlink stores the target verbatim — but the server never follows it out.
func TestPlantedSymlinkIsNotFollowed(t *testing.T) {
	fsys, base := tree(t)
	secret := filepath.Join(base, "secret")

	// Control: an in-tree link, created the same way, is followed.
	if err := fsys.Symlink("inside", "good"); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create symlinks here: %v", err)
		}
		t.Fatal(err)
	}
	if b, err := fsys.ReadFile("good"); err != nil || string(b) != "inner" {
		t.Fatalf("control: ReadFile through an in-tree link = %q, %v", b, err)
	}

	for name, target := range map[string]string{
		"abs":      filepath.ToSlash(secret),
		"relative": "../secret",
		"chain":    "good/../../secret",
	} {
		t.Run(name, func(t *testing.T) {
			must(t, fsys.Symlink(target, "evil"))
			defer func() { must(t, fsys.DeleteFile("evil")) }()

			// The link itself is visible, as a link, with its target: a
			// network client needs exactly that to resolve it on its side.
			if got, err := fsys.ReadLink("evil"); err != nil || got != target {
				t.Errorf("ReadLink = %q, %v", got, err)
			}
			if st, err := fsys.Stat("evil"); err != nil || st.Mode()&0o170000 != 0o120000 {
				t.Errorf("Stat = %v, %v; want the link itself", st, err)
			}

			if b, err := fsys.ReadFile("evil"); err == nil {
				t.Errorf("ReadFile followed the link out: %q", b)
			}
			if _, err := fsys.OpenFile("evil"); err == nil {
				t.Error("OpenFile followed the link out")
			}
			if err := fsys.WriteFile("evil", []byte("pwned"), 0o644); err == nil {
				t.Error("WriteFile followed the link out")
			}
			if err := fsys.Truncate("evil", 0); err == nil {
				t.Error("Truncate followed the link out")
			}
			if err := fsys.Chmod("evil", 0o777); err == nil {
				t.Error("Chmod followed the link out")
			}
			if err := fsys.Chtimes("evil", time.Unix(0, 0), time.Unix(0, 0)); err == nil {
				t.Error("Chtimes followed the link out")
			}
			if runtime.GOOS != "windows" {
				if err := fsys.Chown("evil", uint32(os.Getuid()), uint32(os.Getgid())); err == nil {
					t.Error("Chown followed the link out")
				}
			}
			outsideIntact(t, base)
			if fi, _ := os.Stat(secret); fi.Mode().Perm() == 0o777 || fi.ModTime().Unix() == 0 {
				t.Error("the outside file's metadata changed")
			}
		})
	}

	// DeleteFile on the link removes the link, never what it points at.
	must(t, fsys.Symlink(filepath.ToSlash(secret), "evil"))
	must(t, fsys.DeleteFile("evil"))
	outsideIntact(t, base)
}

// TestDirectorySymlinkIsNotTraversed: a link to a directory outside, used as
// an intermediate component. Every operation that would reach through it is
// refused, including the ones that would CREATE something outside.
func TestDirectorySymlinkIsNotTraversed(t *testing.T) {
	fsys, base := tree(t)
	must(t, fsys.MkDir("real", 0o755))

	// Control: the same component, as an in-tree link to an in-tree dir.
	if err := fsys.Symlink("real", "okdir"); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create symlinks here: %v", err)
		}
		t.Fatal(err)
	}
	must(t, fsys.WriteFile("okdir/made", []byte("x"), 0o644))
	must(t, fsys.MkDir("okdir/sub", 0o755))
	if ents, err := fsys.ListDir("okdir"); err != nil || len(ents) != 2 {
		t.Fatalf("control: ListDir through an in-tree dir link = %v, %v", ents, err)
	}

	must(t, fsys.Symlink(filepath.ToSlash(base), "out"))
	if _, err := fsys.ListDir("out"); err == nil {
		t.Error("ListDir followed the link out")
	}
	if b, err := fsys.ReadFile("out/secret"); err == nil {
		t.Errorf("ReadFile through the link: %q", b)
	}
	if _, err := fsys.Stat("out/secret"); err == nil {
		t.Error("Stat through the link")
	}
	if _, err := fsys.ReadLink("out/secret"); err == nil {
		t.Error("ReadLink through the link")
	}
	if err := fsys.WriteFile("out/new", []byte("pwned"), 0o644); err == nil {
		t.Error("WriteFile created a file outside")
	}
	if err := fsys.MkDir("out/newdir", 0o755); err == nil {
		t.Error("MkDir created a directory outside")
	}
	if err := fsys.Symlink("x", "out/newlink"); err == nil {
		t.Error("Symlink created a link outside")
	}
	if err := fsys.Link("inside", "out/newhard"); err == nil {
		t.Error("Link created a hard link outside")
	}
	if err := fsys.Link("out/secret", "stolen"); err == nil {
		t.Error("Link pulled an outside file in")
	}
	if err := fsys.Rename("inside", "out/moved"); err == nil {
		t.Error("Rename moved a file outside")
	}
	if err := fsys.Rename("out/secret", "stolen"); err == nil {
		t.Error("Rename pulled an outside file in")
	}
	if err := fsys.DeleteFile("out/secret"); err == nil {
		t.Error("DeleteFile removed an outside file")
	}
	if err := fsys.DeleteDir("out/tree"); err == nil {
		t.Error("DeleteDir reached outside")
	}
	if err := fsys.Truncate("out/secret", 0); err == nil {
		t.Error("Truncate reached outside")
	}
	if err := fsys.Chmod("out/secret", 0o777); err == nil {
		t.Error("Chmod reached outside")
	}
	outsideIntact(t, base)
	if _, err := os.Stat(filepath.Join(base, "tree", "inside")); err != nil {
		t.Errorf("inside was moved: %v", err)
	}
}

// TestHostSymlinkIsNotFollowed: the same, for a link the host put there
// before the FS was opened — a container volume may well hold one.
func TestHostSymlinkIsNotFollowed(t *testing.T) {
	fsys, base := tree(t)
	symlinkOrSkip(t, filepath.Join(base, "secret"), filepath.Join(base, "tree", "hostlink"))
	symlinkOrSkip(t, "inside", filepath.Join(base, "tree", "hostgood"))
	if b, err := fsys.ReadFile("hostgood"); err != nil || string(b) != "inner" {
		t.Fatalf("control: %q, %v", b, err)
	}
	if b, err := fsys.ReadFile("hostlink"); err == nil {
		t.Errorf("followed a host link out: %q", b)
	}
	if err := fsys.WriteFile("hostlink", []byte("pwned"), 0o644); err == nil {
		t.Error("wrote through a host link")
	}
	outsideIntact(t, base)
}

// TestHardLinkEscape: neither end of Link can be outside.
func TestHardLinkEscape(t *testing.T) {
	fsys, base := tree(t)
	must(t, fsys.Link("inside", "inside2")) // control
	if err := fsys.Link("../secret", "stolen"); err == nil {
		t.Error("hard-linked an outside file into the tree")
	}
	if err := fsys.Link("inside", "../planted"); err == nil {
		t.Error("hard-linked a tree file outside")
	}
	if _, err := fsys.Stat("stolen"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stolen: %v", err)
	}
	outsideIntact(t, base)
}
