// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build unix

package osfs

import (
	"io/fs"
	"syscall"
)

// inodeOf returns the host inode number of fi.
func inodeOf(fi fs.FileInfo) (ino uint64) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		ino = uint64(st.Ino)
	}
	return ino
}
