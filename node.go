package runtimefs

import (
	"context"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// readFunc reads the latest metric value, convert it and appends the result to
// the byte slice, and returns the updated slice, as well as the timestamp of
// latest update (time of the sample read from /runtime/metrics)
type readFunc func(buf []byte) ([]byte, int64)

type metricsFile struct {
	fs.Inode

	mu    sync.RWMutex
	data  []byte
	mtime int64
	ctime int64

	read readFunc
}

var _ = (fs.NodeOpener)((*metricsFile)(nil))

func newMetricsFile(readval readFunc) *metricsFile {
	return &metricsFile{
		read:  readval,
		ctime: time.Now().Unix(),
	}
}

// Getattr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
var _ = (fs.NodeGetattrer)((*metricsFile)(nil))

func (mf *metricsFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	out.Mode = fuse.S_IFREG | 0444
	out.Size = uint64(len(mf.data))
	out.Mtime = uint64(max(mf.mtime, mf.ctime))
	out.Atime = uint64(max(mf.mtime, mf.ctime))
	out.Ctime = uint64(mf.ctime)
	return fs.OK
}

// Open reads the latest metrics value.
func (mf *metricsFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	// Disallow writes.
	if flags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		return nil, 0, syscall.EROFS
	}

	mf.mu.Lock()
	defer mf.mu.Unlock()

	mf.data, mf.mtime = mf.read(mf.data[:0])

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

type staticFile struct {
	fs.Inode

	data  []byte
	ctime int64
}

func newStaticFile(data string) *staticFile {
	return &staticFile{
		data:  []byte(data + "\n"),
		ctime: time.Now().Unix(),
	}
}

var _ = (fs.NodeOpener)((*staticFile)(nil))

// Getattr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
var _ = (fs.NodeGetattrer)((*staticFile)(nil))

func (sf *staticFile) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Atime = uint64(sf.ctime)
	out.Ctime = uint64(sf.ctime)
	out.Mtime = uint64(sf.ctime)
	out.Mode = fuse.S_IFREG | 0444
	out.Size = uint64(len(sf.data))
	return fs.OK
}

// Open lazily unpacks zip data
func (sf *staticFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	// Disallow writes.
	if flags&(syscall.O_RDWR|syscall.O_WRONLY) != 0 {
		return nil, 0, syscall.EROFS
	}

	// We don't return a filehandle since we don't really need one. The file
	// content is immutable, so hint the kernel to cache the data.
	return nil, fuse.FOPEN_KEEP_CACHE, fs.OK
}

// Read simply returns the data that was already unpacked in the Open call
func (sf *staticFile) Read(ctx context.Context, f fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := min(int(off)+len(dest), len(sf.data))
	return fuse.ReadResultData(sf.data[off:end]), fs.OK
}
