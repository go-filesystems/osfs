// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestUsageFailure(t *testing.T) {
	fsys, _ := tree(t)
	defer func(f func(*uint16, *uint64, *uint64, *uint64) error) { getDiskFreeSpaceEx = f }(getDiskFreeSpaceEx)
	boom := errors.New("boom")
	getDiskFreeSpaceEx = func(*uint16, *uint64, *uint64, *uint64) error { return boom }
	if _, _, err := fsys.Usage(); !errors.Is(err, boom) {
		t.Errorf("Usage = %v, want boom", err)
	}
}

// TestUsageFinalPathFailure: when the handle cannot be turned into a path,
// an FS opened by path falls back to it, and one made by OpenRoot, which has
// no path, reports the error.
func TestUsageFinalPathFailure(t *testing.T) {
	defer func(f func(windows.Handle, *uint16, uint32, uint32) (uint32, error)) {
		getFinalPathNameByHandle = f
	}(getFinalPathNameByHandle)
	boom := errors.New("boom")
	getFinalPathNameByHandle = func(windows.Handle, *uint16, uint32, uint32) (uint32, error) { return 0, boom }

	byPath, _ := tree(t)
	if total, _, err := byPath.Usage(); err != nil || total == 0 {
		t.Errorf("Usage by path = %d, %v; want the fallback to work", total, err)
	}

	r, err := os.OpenRoot(t.TempDir())
	must(t, err)
	byRoot, err := OpenRoot(r)
	must(t, err)
	defer byRoot.Close()
	if _, _, err := byRoot.Usage(); !errors.Is(err, boom) {
		t.Errorf("Usage by root = %v, want boom", err)
	}
}
