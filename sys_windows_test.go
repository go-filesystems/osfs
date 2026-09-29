// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"errors"
	"testing"
)

func TestUsageFailure(t *testing.T) {
	fsys, _ := tree(t)
	defer func(f func(*uint16, *uint64, *uint64, *uint64) error) { getDiskFreeSpaceEx = f }(getDiskFreeSpaceEx)
	boom := errors.New("boom")
	getDiskFreeSpaceEx = func(*uint16, *uint64, *uint64, *uint64) error { return boom }
	if _, _, err := fsys.Usage(); !errors.Is(err, boom) {
		t.Errorf("Usage = %v, want boom", err)
	}
}
