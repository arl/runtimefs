package runtimefs

import (
	"context"
	"fmt"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type metricsFile struct {
	fs.Inode
	name string

	readval func(buf []byte) []byte

	mu   sync.RWMutex
	data []byte
}

var _ = (fs.NodeOpener)((*metricsFile)(nil))

// Getattr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
var _ = (fs.NodeGetattrer)((*metricsFile)(nil))

func (mf *metricsFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	fmt.Println("getattr on", mf.name)

	mf.mu.Lock()
	defer mf.mu.Unlock()
	out.Size = uint64(len(mf.data))
	return 0
}

// Open reads the latest metrics value.
func (mf *metricsFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	mf.data = mf.data[:0]
	mf.data = mf.readval(mf.data)

	fh := bytesFileHandle{
		content: mf.data,
	}


	// Return FOPEN_DIRECT_IO so content is not cached.
	return fh, fuse.FOPEN_DIRECT_IO, fs.OK
}

// Read returns a view on the data buffer we've already filled in the Open call.
func (mf *metricsFile) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := min(int(off)+len(dest), len(mf.data))
	return fuse.ReadResultData(mf.data[off:end]), fs.OK
}

// bytesFileHandle is a file handle that carries separate content for each Open
// call.
type bytesFileHandle struct {
	content []byte
}

// bytesFileHandle allows reads
var _ = (fs.FileReader)((*bytesFileHandle)(nil))

func (fh *bytesFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := off + int64(len(dest))
	if end > int64(len(fh.content)) {
		end = int64(len(fh.content))
	}

	// We could copy to the `dest` buffer, but since we have a
	// []byte already, return that.
	return fuse.ReadResultData(fh.content[off:end]), 0
}
