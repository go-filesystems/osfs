// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import "syscall"

// statfsUsage reads Darwin statfs, whose blocks are f_bsize bytes.
func statfsUsage(st *syscall.Statfs_t) (total, free uint64, err error) {
	unit := uint64(st.Bsize)
	return st.Blocks * unit, st.Bavail * unit, nil
}
