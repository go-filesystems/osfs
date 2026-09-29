// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

// Package osfs serves a directory of the host as a
// [github.com/go-filesystems/interface] Filesystem.
//
// Every other driver in the go-filesystems org decodes an on-disk format out
// of an image. This one has no format: it hands each call to the host kernel,
// relative to a directory opened once with [os.OpenRoot]. It exists so that a
// server written against the interface (go-fileshare/fileshare serves SMB,
// NFS, WebDAV, SFTP and S3 that way) can export a directory tree — a container
// volume, a home directory — exactly as it exports an image.
//
// # Confinement
//
// The point of the package is that a client cannot leave the tree. Every path
// operation goes through the [os.Root], never through a path joined by hand,
// so the confinement is the kernel-backed one os.Root implements (openat and
// friends, resolved one component at a time), not a string check that a
// clever path could get past:
//
//   - ".." that climbs above the tree is refused;
//   - a symbolic link whose target lies outside the tree is refused when it is
//     followed — whether a client created it through [FS.Symlink] or it was
//     already on the host;
//   - a symbolic link inside the tree that stays inside the tree works.
//
// Symlink stores its target as given (the interface requires it, and so does
// os.Root), so a client CAN create a link that points at /etc. What it cannot
// do is make the server follow it: ReadFile, OpenFile, WriteFile, ListDir,
// Truncate, Chmod, Chown, Chtimes, Link and the rest all refuse a path that
// leaves the tree through such a link. Stat and ReadLink describe the link
// itself, which is what a network client needs to resolve it on its own side.
//
// # Paths
//
// Paths are slash-separated and relative to the root of the tree; a leading
// "/" is accepted and means the same thing, so "/a/b", "a/b" and "//a/b" name
// one file and "", "/" and "." name the root. That matches the other drivers
// of the org. It follows that there is no way to name a host absolute path on
// Unix: "/etc/passwd" is the file etc/passwd INSIDE the tree.
//
// # What each call means here
//
//   - Stat does not follow a final symbolic link (it is lstat), so servers
//     see S_IFLNK and can answer READLINK. Mode carries the POSIX type and
//     permission bits. Inode is the host's inode number on Unix; on other
//     systems it is 0, which consumers already treat as "unknown".
//   - ListDir returns the entries sorted by name, without "." and "..".
//     FileType is the dirent d_type value (DT_REG 8, DT_DIR 4, DT_LNK 10,
//     DT_FIFO 1, DT_CHR 2, DT_BLK 6, DT_SOCK 12, DT_UNKNOWN 0).
//   - DeleteFile removes any non-directory, including a symbolic link (never
//     its target). DeleteDir removes an EMPTY directory, as rmdir(2) does: it
//     is not recursive, because every protocol server that calls it expects
//     rmdir semantics and a recursive delete of a host tree is not a thing to
//     get by accident. Neither is idempotent: a missing path is an error
//     wrapping [io/fs.ErrNotExist].
//   - ReadFile, OpenFile, WriteFile and Truncate only ever touch regular
//     files. They open with O_NONBLOCK on Unix and check the type of what they
//     opened, so a FIFO in the tree is refused instead of hanging the server.
//   - OpenFile returns a [filesystem.WritableFile] when the FS is writable and
//     the host lets the file be opened read-write; otherwise a plain
//     [filesystem.File]. Reads and writes are positional (pread/pwrite on the
//     *os.File); nothing is buffered. Sync is fsync(2) (F_FULLFSYNC on
//     Darwin, FlushFileBuffers on Windows), so "the backing store" is the host
//     filesystem the directory lives on.
//
// # Read-only
//
// With [ReadOnly], every mutating method returns an error wrapping
// [ErrReadOnly] before it touches anything, and files are opened O_RDONLY.
package osfs
