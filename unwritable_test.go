// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"os"
	"testing"

	filesystem "github.com/go-filesystems/interface"
)

// TestUnwritableFileOpensReadOnly: a file the host will not open for writing
// comes back as a plain File, not as an error and not as a WritableFile. It
// runs everywhere: on Windows mode 0444 is the read-only attribute, and an
// O_RDWR open of it is refused as a permission error, as on Unix.
func TestUnwritableFileOpensReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens a 0444 file for writing")
	}
	fsys, _ := tree(t)
	must(t, fsys.WriteFile("locked", []byte("l"), 0o444))
	// Windows will not remove a read-only file with the tree.
	t.Cleanup(func() { fsys.Chmod("locked", 0o644) })
	f, err := fsys.OpenFile("locked")
	must(t, err)
	defer f.Close()
	if _, ok := f.(filesystem.WritableFile); ok {
		t.Error("a 0444 file came back writable")
	}
	// Control: a 0644 file comes back writable.
	g, err := fsys.OpenFile("inside")
	must(t, err)
	defer g.Close()
	if _, ok := g.(filesystem.WritableFile); !ok {
		t.Error("control: a 0644 file came back read-only")
	}
}
