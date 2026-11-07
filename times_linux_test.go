// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux

package osfs

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestBirthOf(t *testing.T) {
	if birthOf(&unix.Statx_t{}, nil).Recorded() {
		t.Error("mask without STATX_BTIME must be not recorded")
	}
	if birthOf(&unix.Statx_t{Mask: unix.STATX_BTIME}, errors.New("x")).Recorded() {
		t.Error("a failed statx must be not recorded")
	}
	stx := &unix.Statx_t{Mask: unix.STATX_BTIME, Btime: unix.StatxTimestamp{Sec: 1700000000, Nsec: 5}}
	got, ok := birthOf(stx, nil).Get()
	if !ok || !got.Equal(time.Unix(1700000000, 5)) {
		t.Errorf("got %v %v", got, ok)
	}
}

func TestBirthOfMissingPath(t *testing.T) {
	fsys, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	if birth(fsys, "/nope").Recorded() {
		t.Error("a path that does not open has no birth time")
	}
}
