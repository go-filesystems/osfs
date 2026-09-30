// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"os"

	"golang.org/x/sys/windows"
)

// getDiskFreeSpaceEx and getFinalPathNameByHandle are variables so that a
// test can make them fail.
var (
	getDiskFreeSpaceEx       = windows.GetDiskFreeSpaceEx
	getFinalPathNameByHandle = windows.GetFinalPathNameByHandle
)

// Usage returns the size of the volume holding the tree and the bytes
// available on it to the caller (quotas included), from GetDiskFreeSpaceExW.
//
// That call takes a path, not a handle, so Usage asks the kernel where the
// tree's own directory handle points (GetFinalPathNameByHandleW) rather than
// trusting a name. Only if that fails does it fall back to the path the FS
// was opened with — and an FS made by [OpenRoot] has none, so there it is an
// error.
func (fsys *FS) Usage() (total, free uint64, err error) {
	d, err := fsys.root.Open(".")
	if err != nil {
		return 0, 0, err
	}
	defer d.Close()
	// 32768 UTF-16 units is the longest path Windows has, NUL included, so
	// one call is enough.
	buf := make([]uint16, 32768)
	path := &buf[0]
	if _, err := getFinalPathNameByHandle(windows.Handle(d.Fd()), path, uint32(len(buf)), 0); err != nil {
		if fsys.dir == "" {
			return 0, 0, &os.SyscallError{Syscall: "GetFinalPathNameByHandleW", Err: err}
		}
		path = windows.StringToUTF16Ptr(fsys.dir)
	}
	var avail, all, allFree uint64
	if err := getDiskFreeSpaceEx(path, &avail, &all, &allFree); err != nil {
		return 0, 0, &os.SyscallError{Syscall: "GetDiskFreeSpaceExW", Err: err}
	}
	return all, avail, nil
}
