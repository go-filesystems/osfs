// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package osfs

import (
	"io/fs"
	"syscall"
	"time"

	filesystem "github.com/go-filesystems/interface"
)

func hostTimes(_ *FS, _ string, fi fs.FileInfo) filesystem.FileTimes {
	d, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return filesystem.FileTimes{Mtime: filesystem.NewTimestamp(fi.ModTime(), 100*time.Nanosecond)}
	}
	return filesystem.FileTimes{
		Atime: filesystem.NewTimestamp(time.Unix(0, d.LastAccessTime.Nanoseconds()), 100*time.Nanosecond),
		Mtime: filesystem.NewTimestamp(time.Unix(0, d.LastWriteTime.Nanoseconds()), 100*time.Nanosecond),
		Btime: filesystem.NewTimestamp(time.Unix(0, d.CreationTime.Nanoseconds()), 100*time.Nanosecond),
	}
}
