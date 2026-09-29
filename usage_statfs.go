// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux || darwin || freebsd

package osfs

import (
	"os"
	"syscall"
)

// fstatfs is a variable so that a test can make it fail.
var fstatfs = syscall.Fstatfs

// Usage returns the size of the host filesystem holding the tree and the
// bytes available on it to an unprivileged user, from fstatfs(2) on the
// tree's own directory descriptor.
func (fsys *FS) Usage() (total, free uint64, err error) {
	d, err := fsys.root.Open(".")
	if err != nil {
		return 0, 0, err
	}
	defer d.Close()
	var st syscall.Statfs_t
	if err := fstatfs(int(d.Fd()), &st); err != nil {
		return 0, 0, &os.SyscallError{Syscall: "fstatfs", Err: err}
	}
	return statfsUsage(&st)
}
