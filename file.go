// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"io"
	"io/fs"
	"os"
	"sync/atomic"

	filesystem "github.com/go-filesystems/interface"
)

// file is the read-only File OpenFile returns: an *os.File and the size it
// had when it was opened.
//
// ReadAt answers from that size, not from whatever the host file has become
// since, because the interface's contract is stated against Size: an offset
// at or past Size returns 0, io.EOF, and a read that stops at Size says
// io.EOF. A file shrunk by someone else meanwhile still ends early with
// io.EOF, from the host.
type file struct {
	fsys *FS
	f    *os.File
	name string
	size atomic.Int64
}

// writableFile is the WritableFile OpenFile returns on a writable FS. Its
// Size follows its own writes and truncations.
type writableFile struct{ *file }

var (
	_ filesystem.File         = (*file)(nil)
	_ filesystem.WritableFile = (*writableFile)(nil)
)

// ReadAt reads len(p) bytes at off, following io.ReaderAt to the letter.
func (h *file) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, &fs.PathError{Op: "readat", Path: h.name, Err: errNegative}
	}
	size := h.size.Load()
	if off >= size {
		return 0, io.EOF
	}
	want := p
	if int64(len(p)) > size-off {
		want = p[:size-off]
	}
	n, err := h.f.ReadAt(want, off)
	if err == nil && n < len(p) {
		err = io.EOF
	}
	return n, err
}

// Size is the length of the file: at open time for a read-only File, and
// after this handle's own writes for a WritableFile.
func (h *file) Size() int64 { return h.size.Load() }

// Close closes the file.
func (h *file) Close() error {
	h.fsys.forget(h)
	return h.f.Close()
}

// WriteAt writes p at off with pwrite(2). Concurrent calls are safe when
// their ranges do not overlap, as io.WriterAt requires.
func (h *writableFile) WriteAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, &fs.PathError{Op: "writeat", Path: h.name, Err: errNegative}
	}
	n, err := h.f.WriteAt(p, off)
	end := off + int64(n)
	for {
		cur := h.size.Load()
		if end <= cur || h.size.CompareAndSwap(cur, end) {
			return n, err
		}
	}
}

// Truncate resizes the file to size bytes; growing is sparse where the host
// filesystem allows it.
func (h *writableFile) Truncate(size int64) error {
	if size < 0 {
		return &fs.PathError{Op: "truncate", Path: h.name, Err: errNegative}
	}
	if err := h.f.Truncate(size); err != nil {
		return err
	}
	h.size.Store(size)
	return nil
}

// Sync flushes the file to the host filesystem: fsync(2), F_FULLFSYNC on
// Darwin, FlushFileBuffers on Windows.
func (h *writableFile) Sync() error { return h.f.Sync() }
