// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build !linux && !darwin && !freebsd && !windows

package osfs

import (
	"errors"
	"fmt"
)

// Usage is not implemented on this system; the error wraps
// errors.ErrUnsupported.
func (fsys *FS) Usage() (total, free uint64, err error) {
	return 0, 0, fmt.Errorf("osfs: Usage: %w", errors.ErrUnsupported)
}
