// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	filesystem "github.com/go-filesystems/interface"
)

var _ filesystem.TimesReader = (*FS)(nil)

// Times returns the four times the host records for p, as lstat(2) (and, for
// the birth time, statx(2) on Linux) reports them. A final symbolic link is not
// followed.
//
// What is recorded depends on the host, and what the host does not keep is
// reported as not recorded rather than as a zero time:
//
//   - Linux: atime, mtime, ctime from stat; birth time from statx when the host
//     filesystem keeps one (STATX_BTIME set in stx_mask) and p is a regular
//     file or a directory; a symbolic link has none reported.
//   - macOS: all four, from st_birthtimespec.
//   - Windows: creation (Btime), last access and last write; there is no change
//     time in the data os.FileInfo carries, so Ctime is not recorded.
//   - Any other host: Mtime only, from ModTime.
//
// The resolution is the unit of the host's interface (1 ns; 100 ns on
// Windows). The filesystem underneath may store less (ext3 1 s, HFS+ 1 s, FAT
// 2 s), and the host does not say so.
func (fsys *FS) Times(p string) (filesystem.FileTimes, error) {
	fi, err := fsys.root.Lstat(rel(p))
	if err != nil {
		return filesystem.FileTimes{}, err
	}
	return hostTimes(fsys, p, fi), nil
}
