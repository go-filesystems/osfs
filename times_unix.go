// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux || darwin

package osfs

import (
	"time"

	filesystem "github.com/go-filesystems/interface"
)

func unixTS(sec, nsec int64, res time.Duration) filesystem.Timestamp {
	return filesystem.NewTimestamp(time.Unix(sec, nsec), res)
}
