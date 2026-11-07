// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"io/fs"
	"testing"
	"time"
)

// bareInfo is a FileInfo whose Sys is not the host's stat structure: hostTimes
// must fall back to ModTime alone.
type bareInfo struct{ fs.FileInfo }

func (bareInfo) Sys() any           { return nil }
func (bareInfo) ModTime() time.Time { return time.Unix(1700000000, 0) }

func TestHostTimesFallsBackToModTime(t *testing.T) {
	ft := hostTimes(nil, "/x", bareInfo{})
	if m, ok := ft.Mtime.Get(); !ok || !m.Equal(time.Unix(1700000000, 0)) {
		t.Fatalf("mtime = %v %v", m, ok)
	}
	if ft.Atime.Recorded() || ft.Ctime.Recorded() || ft.Btime.Recorded() {
		t.Fatalf("only mtime is known: %+v", ft)
	}
}
