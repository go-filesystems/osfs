// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"syscall"
	"testing"
)

func TestStatfsUsageLinux(t *testing.T) {
	st := syscall.Statfs_t{Blocks: 100, Bavail: 40, Bsize: 4096, Frsize: 1024}
	if total, free, _ := statfsUsage(&st); total != 102400 || free != 40960 {
		t.Errorf("frsize: %d %d", total, free)
	}
	st.Frsize = 0
	if total, free, _ := statfsUsage(&st); total != 409600 || free != 163840 {
		t.Errorf("bsize fallback: %d %d", total, free)
	}
}
