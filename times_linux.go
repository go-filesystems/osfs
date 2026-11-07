// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux

package osfs

import (
	"io/fs"
	"os"
	"syscall"
	"time"

	filesystem "github.com/go-filesystems/interface"
	"golang.org/x/sys/unix"
)

func hostTimes(fsys *FS, p string, fi fs.FileInfo) filesystem.FileTimes {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return filesystem.FileTimes{Mtime: filesystem.NewTimestamp(fi.ModTime(), time.Nanosecond)}
	}
	ft := filesystem.FileTimes{
		Atime: unixTS(int64(st.Atim.Sec), int64(st.Atim.Nsec), time.Nanosecond),
		Mtime: unixTS(int64(st.Mtim.Sec), int64(st.Mtim.Nsec), time.Nanosecond),
		Ctime: unixTS(int64(st.Ctim.Sec), int64(st.Ctim.Nsec), time.Nanosecond),
	}
	if fi.Mode().IsRegular() || fi.IsDir() {
		ft.Btime = birth(fsys, p)
	}
	return ft
}

// birth asks statx(2) for STATX_BTIME on an open descriptor, and reports not
// recorded when the host filesystem clears the bit in stx_mask.
func birth(fsys *FS, p string) filesystem.Timestamp {
	f, err := fsys.root.OpenFile(rel(p), os.O_RDONLY|oNonblock, 0)
	if err != nil {
		return filesystem.Timestamp{}
	}
	defer f.Close()
	var stx unix.Statx_t
	if err := unix.Statx(int(f.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_BTIME, &stx); err != nil {
		return filesystem.Timestamp{}
	}
	if stx.Mask&unix.STATX_BTIME == 0 {
		return filesystem.Timestamp{}
	}
	return unixTS(stx.Btime.Sec, int64(stx.Btime.Nsec), time.Nanosecond)
}
