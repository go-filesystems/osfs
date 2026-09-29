// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"os"

	"golang.org/x/sys/windows"
)

// getDiskFreeSpaceEx is a variable so that a test can make it fail.
var getDiskFreeSpaceEx = windows.GetDiskFreeSpaceEx

// Usage returns the size of the volume holding the tree and the bytes
// available on it to the caller (quotas included), from GetDiskFreeSpaceExW
// on the directory the FS was opened with.
func (fsys *FS) Usage() (total, free uint64, err error) {
	if _, err := fsys.root.Stat("."); err != nil {
		return 0, 0, err
	}
	var avail, all, allFree uint64
	if err := getDiskFreeSpaceEx(windows.StringToUTF16Ptr(fsys.dir), &avail, &all, &allFree); err != nil {
		return 0, 0, &os.SyscallError{Syscall: "GetDiskFreeSpaceExW", Err: err}
	}
	return all, avail, nil
}
