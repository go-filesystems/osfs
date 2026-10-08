// Copyright (c) 2026, go-filesystems
// SPDX-License-Identifier: BSD-3-Clause

package osfs

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	filesystem "github.com/go-filesystems/interface"
)

func sequentialFixture(t *testing.T) (*FS, []byte) {
	t.Helper()
	dir := t.TempDir()
	want := make([]byte, 3<<20+17)
	for i := range want {
		want[i] = byte(i * 7)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), want, 0o644); err != nil {
		t.Fatal(err)
	}
	fsys, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fsys.Close() })
	return fsys, want
}

// Read and Seek move the descriptor's position; ReadAt neither reads it nor
// moves it.
func TestReadSeekAreTheDescriptors(t *testing.T) {
	fsys, want := sequentialFixture(t)
	f, err := fsys.OpenFile("/f")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	hf, ok := f.(filesystem.HostFile)
	if !ok {
		t.Fatalf("%T is not a filesystem.HostFile", f)
	}
	hf.HostFile()
	var rs io.ReadSeeker = hf
	if _, err := rs.Seek(100, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	p := make([]byte, 10)
	if _, err := f.ReadAt(p, 5000); err != nil || !bytes.Equal(p, want[5000:5010]) {
		t.Fatalf("ReadAt after Seek: %v", err)
	}
	if _, err := io.ReadFull(rs, p); err != nil || !bytes.Equal(p, want[100:110]) {
		t.Fatalf("Read after ReadAt did not continue from the Seek: %v", err)
	}
	if pos, _ := rs.Seek(0, io.SeekCurrent); pos != 110 {
		t.Fatalf("position %d, want 110", pos)
	}
}

// What a server does with it: seek to a range, then hand the descriptor to a
// TCP connection's ReadFrom (sendfile(2) on Linux and Darwin), bounded by
// a LimitedReader. The peer gets exactly the range.
func TestARangeGoesOutThroughTheConnection(t *testing.T) {
	fsys, want := sequentialFixture(t)
	for _, writable := range []bool{false, true} {
		fsys.readOnly = !writable
		f, err := fsys.OpenFile("/f")
		if err != nil {
			t.Fatal(err)
		}
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		got := make(chan []byte, 1)
		go func() {
			c, err := ln.Accept()
			if err != nil {
				got <- nil
				return
			}
			b, _ := io.ReadAll(c)
			c.Close()
			got <- b
		}()
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		const off, n = 4097, 2<<20 + 3
		if _, err := f.(io.Seeker).Seek(off, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		sent, err := c.(*net.TCPConn).ReadFrom(&io.LimitedReader{R: f.(io.Reader), N: n})
		c.Close()
		if err != nil || sent != n {
			t.Fatalf("writable=%v: ReadFrom sent %d, %v; want %d", writable, sent, err, n)
		}
		if b := <-got; !bytes.Equal(b, want[off:off+n]) {
			t.Fatalf("writable=%v: the peer got %d bytes, not the range", writable, len(b))
		}
		f.Close()
		ln.Close()
	}
}
