// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin

package osfs

import (
	"io/fs"
	"syscall"
	"time"

	filesystem "github.com/go-filesystems/interface"
)

func hostTimes(_ *FS, _ string, fi fs.FileInfo) filesystem.FileTimes {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return filesystem.FileTimes{Mtime: filesystem.NewTimestamp(fi.ModTime(), time.Nanosecond)}
	}
	return filesystem.FileTimes{
		Atime: unixTS(st.Atimespec.Sec, st.Atimespec.Nsec, time.Nanosecond),
		Mtime: unixTS(st.Mtimespec.Sec, st.Mtimespec.Nsec, time.Nanosecond),
		Ctime: unixTS(st.Ctimespec.Sec, st.Ctimespec.Nsec, time.Nanosecond),
		Btime: unixTS(st.Birthtimespec.Sec, st.Birthtimespec.Nsec, time.Nanosecond),
	}
}
