// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build !unix

package osfs

// oNonblock is 0 where there are no FIFOs to block on.
const oNonblock = 0

// isROFS reports false: a read-only volume refuses writes as a permission
// error here, which OpenFile already handles.
func isROFS(error) bool { return false }
