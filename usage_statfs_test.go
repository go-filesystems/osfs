// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux || darwin || freebsd

package osfs

import (
	"errors"
	"syscall"
	"testing"
)

func TestUsageFailure(t *testing.T) {
	fsys, _ := tree(t)
	defer func(f func(int, *syscall.Statfs_t) error) { fstatfs = f }(fstatfs)
	fstatfs = func(int, *syscall.Statfs_t) error { return syscall.EIO }
	if _, _, err := fsys.Usage(); !errors.Is(err, syscall.EIO) {
		t.Errorf("Usage = %v, want EIO", err)
	}
}
