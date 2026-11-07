// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build !linux && !darwin && !windows

package osfs

import (
	"io/fs"
	"time"

	filesystem "github.com/go-filesystems/interface"
)

// hostTimes reports Mtime only: io/fs.FileInfo carries no other time, and the
// field names of the others differ per host.
func hostTimes(_ *FS, _ string, fi fs.FileInfo) filesystem.FileTimes {
	return filesystem.FileTimes{Mtime: filesystem.NewTimestamp(fi.ModTime(), time.Nanosecond)}
}
