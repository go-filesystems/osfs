# osfs

A host directory as a [go-filesystems/interface](https://github.com/go-filesystems/interface)
`Filesystem`, confined with [`os.Root`](https://pkg.go.dev/os#Root).

Every other driver of the org decodes a format out of an image. This one hands
each call to the host kernel, relative to one directory, so that a server written
against the interface — [go-fileshare/fileshare](https://github.com/go-fileshare/fileshare)
serves SMB, NFS, WebDAV, SFTP and S3 that way — can export a directory tree
(a container volume, a home directory) exactly as it exports an image.

```go
fsys, err := osfs.Open("/data/photos")                // read-write
fsys, err := osfs.Open("/data/photos", osfs.ReadOnly()) // every write refused
total, free, err := fsys.Usage()                       // statfs of the tree
```

## Confinement

A client cannot leave the tree. `..` that climbs out, and a symbolic link whose
target is outside, are refused by the kernel-backed `os.Root` — not by a string
check. A client may *create* a link to `/etc` with `Symlink` (the interface stores
targets as given), but no call will follow it. A FIFO planted in the tree is
refused rather than left to hang an open.

## Capabilities

`Filesystem`, `Opener` (positional `ReadAt`/`WriteAt` straight on the `*os.File`,
`WritableFile` when writable), `Truncater`, `MetadataSetter`, `Symlinker`,
`HardLinker`. Pure Go, `CGO_ENABLED=0`. Tested on Linux, macOS and Windows, and
under QEMU on riscv64, loong64, ppc64le and s390x; built for the BSDs, plan9, js
and wasip1.

## License

BSD-3-Clause.
