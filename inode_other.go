// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build !unix

package osfs

import "io/fs"

// inodeOf returns 0, "unknown", where FileInfo carries no inode number.
// Consumers of the interface already replace a zero with a value of their own.
func inodeOf(fs.FileInfo) uint64 { return 0 }
