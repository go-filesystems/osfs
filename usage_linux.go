// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import "syscall"

// statfsUsage reads Linux statfs: blocks are counted in f_frsize units, and
// f_bsize stands in for a kernel that leaves f_frsize at zero.
func statfsUsage(st *syscall.Statfs_t) (total, free uint64, err error) {
	unit := uint64(st.Frsize)
	if unit == 0 {
		unit = uint64(st.Bsize)
	}
	return st.Blocks * unit, st.Bavail * unit, nil
}
