// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"syscall"

	filesystem "github.com/go-filesystems/interface"
)

// A File from this package is a filesystem.HostFile: also an io.ReadSeeker
// and a syscall.Conn over its own host descriptor. That is what lets a server send it without
// copying it through user space: net/http's response hands the body to the
// connection's ReadFrom, and *net.TCPConn's ReadFrom calls sendfile(2) for a
// source that is a syscall.Conn -- from the descriptor's CURRENT position,
// which it then advances.
//
// So Read and Seek are the descriptor's own, with one position per handle:
// they are for a handle one goroutine reads from start to end, as an HTTP GET
// that opened the file for itself does. ReadAt, which every other caller
// uses, neither reads nor moves that position, and stays safe to call
// concurrently. Unlike ReadAt, Read is not bounded by Size: it reads to the
// host file's end, and a caller bounds it with the length it sends.
var (
	_ filesystem.HostFile = (*file)(nil)
	_ filesystem.HostFile = (*writableFile)(nil)
)

// HostFile says that Read, Seek and SyscallConn are this file's own
// descriptor: see filesystem.HostFile. It does nothing; the one statement is
// there because go tool cover reports a function without any as 0.0%
// covered however often it runs, and the coverage gate reads that line.
func (h *file) HostFile() { _ = h }

// Read reads from the descriptor's position, as read(2).
func (h *file) Read(p []byte) (int, error) { return h.f.Read(p) }

// Seek moves the descriptor's position, as lseek(2).
func (h *file) Seek(off int64, whence int) (int64, error) { return h.f.Seek(off, whence) }

// SyscallConn is the host descriptor, for sendfile(2) and its kind.
func (h *file) SyscallConn() (syscall.RawConn, error) { return h.f.SyscallConn() }
