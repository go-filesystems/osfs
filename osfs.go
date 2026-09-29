// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	filesystem "github.com/go-filesystems/interface"
)

// ErrReadOnly is wrapped by the error every mutating method returns when the
// FS was opened with [ReadOnly].
var ErrReadOnly = errors.New("osfs: read-only filesystem")

// errNotRegular is returned for a path that names something other than a
// regular file where one is required (ReadFile, OpenFile, WriteFile,
// Truncate): a directory, a FIFO, a device, a socket.
var errNotRegular = errors.New("not a regular file")

// errNegative is returned for a negative offset or size.
var errNegative = errors.New("negative offset or size")

// Option configures [Open].
type Option func(*options)

type options struct {
	readOnly bool
}

// ReadOnly makes every mutating method return an error wrapping
// [ErrReadOnly], and opens every file O_RDONLY.
func ReadOnly() Option { return func(o *options) { o.readOnly = true } }

// FS is a host directory tree, confined with [os.Root]. It is safe for
// concurrent use.
type FS struct {
	root     *os.Root
	dir      string
	readOnly bool

	mu    sync.Mutex
	files map[*file]struct{} // nil once closed
}

var (
	_ filesystem.Filesystem     = (*FS)(nil)
	_ filesystem.Opener         = (*FS)(nil)
	_ filesystem.Truncater      = (*FS)(nil)
	_ filesystem.MetadataSetter = (*FS)(nil)
	_ filesystem.Symlinker      = (*FS)(nil)
	_ filesystem.HardLinker     = (*FS)(nil)
)

// Open opens the directory dir, which must exist and be a directory, as a
// filesystem. Nothing outside dir is reachable through the result.
func Open(dir string, opts ...Option) (*FS, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	var root *os.Root
	fi, err := os.Stat(dir)
	if err == nil && !fi.IsDir() {
		err = &fs.PathError{Op: "open", Path: dir, Err: syscall.ENOTDIR}
	}
	if err == nil {
		root, err = os.OpenRoot(dir)
	}
	if err != nil {
		return nil, fmt.Errorf("osfs: %w", err)
	}
	// The absolute form is only used to answer Usage on systems that take a
	// path rather than a descriptor; if it cannot be computed the relative
	// one is still right as long as the working directory does not change.
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return &FS{
		root:     root,
		dir:      dir,
		readOnly: o.readOnly,
		files:    map[*file]struct{}{},
	}, nil
}

// Close closes the tree and every File obtained from it with OpenFile.
func (fsys *FS) Close() error {
	fsys.mu.Lock()
	files := fsys.files
	fsys.files = nil
	fsys.mu.Unlock()
	for h := range files {
		_ = h.f.Close()
	}
	return fsys.root.Close()
}

// rel turns an interface path into a name for the Root. It does NOT clean the
// path: ".." is left for the Root to resolve, and refuse if it climbs out.
func rel(p string) string {
	p = strings.TrimLeft(p, "/")
	if p == "" {
		return "."
	}
	return filepath.FromSlash(p)
}

// readOnlyErr is the error a mutating method returns on a read-only FS.
func readOnlyErr(op, p string) error {
	return &fs.PathError{Op: op, Path: p, Err: ErrReadOnly}
}

// open opens a regular file through the Root and returns it with its size.
// Anything else it opened is closed again: that check is made on the opened
// descriptor, so it cannot be raced by swapping the path afterwards.
func (fsys *FS) open(p string, flag int, perm os.FileMode) (*os.File, int64, error) {
	f, err := fsys.root.OpenFile(rel(p), flag|oNonblock, perm)
	if err != nil {
		return nil, 0, err
	}
	fi, err := f.Stat()
	if err == nil && !fi.Mode().IsRegular() {
		err = &fs.PathError{Op: "open", Path: p, Err: errNotRegular}
	}
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, fi.Size(), nil
}

// ReadFile returns the whole content of the regular file at p.
func (fsys *FS) ReadFile(p string) ([]byte, error) {
	f, size, err := fsys.open(p, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// size is only a hint: the file may grow or shrink while it is read.
	buf := bytes.NewBuffer(make([]byte, 0, size+bytes.MinRead))
	_, err = buf.ReadFrom(f)
	return buf.Bytes(), err
}

// ListDir returns the entries of the directory at p, sorted by name, without
// "." and "..". See the package documentation for the FileType values.
func (fsys *FS) ListDir(p string) ([]filesystem.DirEntry, error) {
	d, err := fsys.root.OpenFile(rel(p), os.O_RDONLY|oNonblock, 0)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	entries, err := d.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	out := make([]filesystem.DirEntry, 0, len(entries))
	for _, e := range entries {
		// An entry removed between ReadDir and Info is still listed, with an
		// unknown inode: a listing is a snapshot, and it was there.
		var ino uint64
		if info, err := e.Info(); err == nil {
			ino = inodeOf(info)
		}
		out = append(out, filesystem.NewDirEntry(ino, e.Name(), direntType(e.Type())))
	}
	return out, nil
}

// Stat describes the file at p. A final symbolic link is not followed.
func (fsys *FS) Stat(p string) (filesystem.Stat, error) {
	fi, err := fsys.root.Lstat(rel(p))
	if err != nil {
		return nil, err
	}
	return filesystem.NewStat(posixMode(fi.Mode()), uint64(fi.Size()), inodeOf(fi)), nil
}

// WriteFile creates or truncates the regular file at p and writes data to it.
// perm is used only when the file is created. The parent must exist.
func (fsys *FS) WriteFile(p string, data []byte, perm os.FileMode) error {
	if fsys.readOnly {
		return readOnlyErr("write", p)
	}
	f, _, err := fsys.open(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return errors.Join(err, f.Close())
}

// ReadLink returns the target of the symbolic link at p, slash-separated.
func (fsys *FS) ReadLink(p string) (string, error) {
	t, err := fsys.root.Readlink(rel(p))
	return filepath.ToSlash(t), err
}

// MkDir creates the directory p. Its parent must exist and p must not.
func (fsys *FS) MkDir(p string, perm os.FileMode) error {
	if fsys.readOnly {
		return readOnlyErr("mkdir", p)
	}
	return fsys.root.Mkdir(rel(p), perm)
}

// DeleteFile removes the non-directory at p. A symbolic link is removed
// itself; its target is not touched.
func (fsys *FS) DeleteFile(p string) error {
	if fsys.readOnly {
		return readOnlyErr("remove", p)
	}
	fi, err := fsys.root.Lstat(rel(p))
	if err == nil && fi.IsDir() {
		err = &fs.PathError{Op: "remove", Path: p, Err: syscall.EISDIR}
	}
	if err != nil {
		return err
	}
	return fsys.root.Remove(rel(p))
}

// DeleteDir removes the EMPTY directory at p, as rmdir(2) does. It is not
// recursive: a non-empty directory is an error.
func (fsys *FS) DeleteDir(p string) error {
	if fsys.readOnly {
		return readOnlyErr("rmdir", p)
	}
	fi, err := fsys.root.Lstat(rel(p))
	if err == nil && !fi.IsDir() {
		err = &fs.PathError{Op: "rmdir", Path: p, Err: syscall.ENOTDIR}
	}
	if err != nil {
		return err
	}
	return fsys.root.Remove(rel(p))
}

// Rename moves oldPath to newPath, replacing newPath if the host allows it.
func (fsys *FS) Rename(oldPath, newPath string) error {
	if fsys.readOnly {
		return readOnlyErr("rename", oldPath)
	}
	return fsys.root.Rename(rel(oldPath), rel(newPath))
}

// Truncate resizes the regular file at p to size bytes.
func (fsys *FS) Truncate(p string, size int64) error {
	if fsys.readOnly {
		return readOnlyErr("truncate", p)
	}
	if size < 0 {
		return &fs.PathError{Op: "truncate", Path: p, Err: errNegative}
	}
	f, _, err := fsys.open(p, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return errors.Join(f.Truncate(size), f.Close())
}

// Chmod sets the permission bits of p (following a final in-tree link).
func (fsys *FS) Chmod(p string, perm os.FileMode) error {
	if fsys.readOnly {
		return readOnlyErr("chmod", p)
	}
	return fsys.root.Chmod(rel(p), perm)
}

// Chown sets the owner and group of p (following a final in-tree link). The
// host decides whether the caller may; on Windows it always fails.
func (fsys *FS) Chown(p string, uid, gid uint32) error {
	if fsys.readOnly {
		return readOnlyErr("chown", p)
	}
	return fsys.root.Chown(rel(p), int(uid), int(gid))
}

// Chtimes sets the access and modification times of p.
func (fsys *FS) Chtimes(p string, atime, mtime time.Time) error {
	if fsys.readOnly {
		return readOnlyErr("chtimes", p)
	}
	return fsys.root.Chtimes(rel(p), atime, mtime)
}

// Symlink creates the symbolic link linkPath with the literal target. The
// target is not checked — it may point anywhere — but it is never followed
// out of the tree: see the package documentation.
func (fsys *FS) Symlink(target, linkPath string) error {
	if fsys.readOnly {
		return readOnlyErr("symlink", linkPath)
	}
	return fsys.root.Symlink(filepath.FromSlash(target), rel(linkPath))
}

// Link creates newPath as a hard link to oldPath, which must not be a
// directory. If oldPath is a symbolic link, the link itself is linked.
func (fsys *FS) Link(oldPath, newPath string) error {
	if fsys.readOnly {
		return readOnlyErr("link", oldPath)
	}
	fi, err := fsys.root.Lstat(rel(oldPath))
	if err == nil && fi.IsDir() {
		err = &fs.PathError{Op: "link", Path: oldPath, Err: syscall.EISDIR}
	}
	if err != nil {
		return err
	}
	return fsys.root.Link(rel(oldPath), rel(newPath))
}

// OpenFile opens the regular file at p for positional access. The result is
// a [filesystem.WritableFile] unless the FS is read-only or the host refuses
// to open the file for writing (its permissions, a read-only mount), in which
// case it is a plain, read-only [filesystem.File].
func (fsys *FS) OpenFile(p string) (filesystem.File, error) {
	writable := !fsys.readOnly
	flag := os.O_RDONLY
	if writable {
		flag = os.O_RDWR
	}
	f, size, err := fsys.open(p, flag, 0)
	if writable && (errors.Is(err, fs.ErrPermission) || isROFS(err)) {
		writable = false
		f, size, err = fsys.open(p, os.O_RDONLY, 0)
	}
	if err != nil {
		return nil, err
	}
	h := &file{fsys: fsys, f: f, name: p}
	h.size.Store(size)
	return fsys.track(h, writable)
}

// track registers h so that Close can close it. It fails, closing h, once the
// FS itself is closed.
func (fsys *FS) track(h *file, writable bool) (filesystem.File, error) {
	fsys.mu.Lock()
	defer fsys.mu.Unlock()
	if fsys.files == nil {
		_ = h.f.Close()
		return nil, &fs.PathError{Op: "open", Path: h.name, Err: fs.ErrClosed}
	}
	fsys.files[h] = struct{}{}
	if writable {
		return &writableFile{h}, nil
	}
	return h, nil
}

func (fsys *FS) forget(h *file) {
	fsys.mu.Lock()
	delete(fsys.files, h)
	fsys.mu.Unlock()
}

// posixMode encodes m as POSIX st_mode bits: the file type, the permission
// bits, and setuid / setgid / sticky.
func posixMode(m fs.FileMode) uint16 {
	var t uint16
	switch {
	case m&fs.ModeDir != 0:
		t = 0o040000
	case m&fs.ModeSymlink != 0:
		t = 0o120000
	case m&fs.ModeNamedPipe != 0:
		t = 0o010000
	case m&fs.ModeSocket != 0:
		t = 0o140000
	case m&fs.ModeCharDevice != 0:
		t = 0o020000
	case m&fs.ModeDevice != 0:
		t = 0o060000
	default:
		t = 0o100000
	}
	t |= uint16(m.Perm())
	if m&fs.ModeSetuid != 0 {
		t |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		t |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		t |= 0o1000
	}
	return t
}

// direntType encodes a type as a dirent d_type value.
func direntType(m fs.FileMode) uint8 {
	switch {
	case m&fs.ModeDir != 0:
		return 4 // DT_DIR
	case m&fs.ModeSymlink != 0:
		return 10 // DT_LNK
	case m&fs.ModeNamedPipe != 0:
		return 1 // DT_FIFO
	case m&fs.ModeSocket != 0:
		return 12 // DT_SOCK
	case m&fs.ModeCharDevice != 0:
		return 2 // DT_CHR
	case m&fs.ModeDevice != 0:
		return 6 // DT_BLK
	case m&fs.ModeType == 0:
		return 8 // DT_REG
	}
	return 0 // DT_UNKNOWN
}
