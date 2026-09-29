// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build unix

package osfs

import (
	"errors"
	"syscall"
)

// oNonblock is added to every open, so that opening a FIFO found in the tree
// returns at once instead of waiting for a peer; the type of what was opened
// is then checked and anything but a regular file (or a directory, for
// ListDir) is refused. It has no effect on regular files.
const oNonblock = syscall.O_NONBLOCK

// isROFS reports whether err says the host filesystem is mounted read-only.
func isROFS(err error) bool { return errors.Is(err, syscall.EROFS) }
