// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import "syscall"

// statfsUsage reads FreeBSD statfs, whose blocks are f_bsize bytes and whose
// f_bavail goes negative once the reserve is eaten into.
func statfsUsage(st *syscall.Statfs_t) (total, free uint64, err error) {
	unit := st.Bsize
	return st.Blocks * unit, uint64(max(st.Bavail, 0)) * unit, nil
}
