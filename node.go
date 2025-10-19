package runtimefs

import (
	"context"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type metricsFile struct {
	fs.Inode
	name string

	mu    sync.RWMutex
	data  []byte
	mtime int64

	readval func(buf []byte) ([]byte, int64)
}

var _ = (fs.NodeOpener)((*metricsFile)(nil))

// Getattr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
var _ = (fs.NodeGetattrer)((*metricsFile)(nil))

func (mf *metricsFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	out.Size = uint64(len(mf.data))
	out.Mtime = uint64(mf.mtime)
	return 0
}

// Open reads the latest metrics value.
func (mf *metricsFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	// Disallow writes.
	if flags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		return nil, 0, syscall.EROFS
	}

	mf.mu.Lock()
	defer mf.mu.Unlock()

	mf.data = mf.data[:0]
	mf.data, mf.mtime = mf.readval(mf.data)

	// Return FOPEN_DIRECT_IO so content is not cached.
	return nil, fuse.FOPEN_DIRECT_IO, fs.OK
}

// Read returns a view on the data buffer we've already filled in the Open call.
func (mf *metricsFile) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	end := min(int(off)+len(dest), len(mf.data))
	buf := make([]byte, end-int(off))
	copy(buf, mf.data[off:end])

	return fuse.ReadResultData(buf), fs.OK
}
