// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"
)

// TestReadFileHugeSparse is the regression test for a ReadFile that trusted
// the size the file reported: a sparse file of 2^50 bytes, made through the
// FS's own Truncate (so reachable by any client that may write, NFS SETATTR
// included), made ReadFile preallocate that much and panic with "makeslice:
// cap out of range"; 2^40 would have exhausted memory instead. It must now be
// refused with ErrTooLarge, without allocating.
func TestReadFileHugeSparse(t *testing.T) {
	fsys, _ := tree(t)
	must(t, fsys.WriteFile("/big", nil, 0o644))
	var made bool
	// The host filesystem decides how large a sparse file may be: 2^50 on
	// APFS, less on ext4 (16 TiB) and NTFS. Any of these is far above the
	// default limit and far above what the old code could allocate.
	for _, size := range []int64{1 << 50, 1 << 43, 1 << 40, 1 << 36} {
		if fsys.Truncate("/big", size) == nil {
			t.Logf("sparse file of %d bytes", size)
			made = true
			break
		}
	}
	if !made {
		t.Skip("the host refuses every large sparse size tried")
	}
	b, err := fsys.ReadFile("/big")
	if !errors.Is(err, ErrTooLarge) || b != nil {
		t.Errorf("ReadFile = %d bytes, %v; want nil, ErrTooLarge", len(b), err)
	}
}

func TestMaxReadFile(t *testing.T) {
	// "inside" holds the 5 bytes "inner".
	small, _ := tree(t, MaxReadFile(4))
	if b, err := small.ReadFile("inside"); !errors.Is(err, ErrTooLarge) || b != nil {
		t.Errorf("limit 4: %q, %v; want ErrTooLarge", b, err)
	}
	exact, _ := tree(t, MaxReadFile(5))
	if b, err := exact.ReadFile("inside"); err != nil || string(b) != "inner" {
		t.Errorf("limit 5: %q, %v", b, err)
	}
	// No limit, and a file larger than the preallocation hint: the buffer
	// has to grow past it and still return every byte.
	for _, n := range []int64{0, -1, math.MaxInt64} {
		open, _ := tree(t, MaxReadFile(n))
		if open.maxReadFile != unlimited {
			t.Errorf("MaxReadFile(%d) stored %d", n, open.maxReadFile)
		}
	}
	open, _ := tree(t, MaxReadFile(0))
	want := bytes.Repeat([]byte("0123456789abcdef"), 3*readFileHint/16+1)
	must(t, open.WriteFile("large", want, 0o644))
	if got, err := open.ReadFile("large"); err != nil || !bytes.Equal(got, want) {
		t.Errorf("large: %d bytes, %v; want %d", len(got), err, len(want))
	}
	def, _ := tree(t)
	if def.maxReadFile != DefaultMaxReadFile {
		t.Errorf("default limit %d", def.maxReadFile)
	}
}

// TestReadAllGrowing: a file that reports a size under the limit but holds
// more by the time it is read is refused too — the limit is on the bytes,
// not on the claim.
func TestReadAllGrowing(t *testing.T) {
	b, err := readAll(strings.NewReader("0123456789"), "f", 2, 5)
	if !errors.Is(err, ErrTooLarge) || b != nil {
		t.Errorf("readAll = %q, %v; want ErrTooLarge", b, err)
	}
	b, err = readAll(strings.NewReader("01234"), "f", 2, 5)
	if err != nil || string(b) != "01234" {
		t.Errorf("readAll at the limit = %q, %v", b, err)
	}
}
