package runtimefs

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/metrics"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type metricsFile struct {
	fs.Inode
	name string

	readval func() []byte

	mu   sync.Mutex
	data []byte
}

var _ = (fs.NodeOpener)((*metricsFile)(nil))

// Getattr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
var _ = (fs.NodeGetattrer)((*metricsFile)(nil))

func (mf *metricsFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	// out.Size = zf.file.UncompressedSize64
	out.Size = 13
	return 0
}

// Open lazily unpacks zip data
func (mf *metricsFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	mf.data = mf.readval()

	// We don't return a filehandle since we don't really need
	// one.  The file content is immutable, so hint the kernel to
	// cache the data.
	return nil, fuse.FOPEN_KEEP_CACHE, fs.OK
}

// Read simply returns the data that was already unpacked in the Open call
func (mf *metricsFile) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fmt.Println("reading file:", mf.name, "off:", off, "len:", len(dest))
	end := min(int(off)+len(dest), len(mf.data))
	return fuse.ReadResultData(mf.data[off:end]), fs.OK
}

// metricsRoot is the root of the metrics filesystem. It populates the
// filesystem with data from runtime/metrics.
type metricsRoot struct {
	fs.Inode

	mu      sync.RWMutex
	samples []metrics.Sample
}

func newMetricsRoot(every time.Duration) *metricsRoot {
	descs := metrics.All()
	samples := make([]metrics.Sample, len(descs))
	for i := range samples {
		samples[i].Name = descs[i].Name
	}
	metrics.Read(samples)

	return &metricsRoot{
		samples: samples,
	}
}

// The root populates the tree in its OnAdd method
var _ = (fs.NodeOnAdder)((*metricsRoot)(nil))

func (mr *metricsRoot) OnAdd(ctx context.Context) {
	// OnAdd is called once we are attached to an Inode. We can
	// then construct a tree.  We construct the entire tree, and
	// we don't want parts of the tree to disappear when the
	// kernel is short on memory, so we use persistent inodes.

	for idx, f := range mr.samples {
		dir, base := filepath.Split(f.Name)

		p := &mr.Inode
		for _, component := range strings.Split(dir, "/") {
			if len(component) == 0 {
				continue
			}
			ch := p.GetChild(component)
			if ch == nil {
				ch = p.NewPersistentInode(ctx, &fs.Inode{},
					fs.StableAttr{Mode: fuse.S_IFDIR})
				p.AddChild(component, ch, true)
			}

			p = ch
		}

		var node fs.InodeEmbedder
		switch f.Value.Kind() {
		case metrics.KindUint64:
			node = &metricsFile{
				name: f.Name,
				readval: func() []byte {
					mr.mu.RLock()
					defer mr.mu.RUnlock()

					val := mr.samples[idx].Value.Uint64()
					return fmt.Appendf(nil, "%d", val)
				},
			}
		case metrics.KindFloat64:
			node = &metricsFile{
				name: f.Name,
				readval: func() []byte {
					mr.mu.RLock()
					defer mr.mu.RUnlock()

					val := mr.samples[idx].Value.Float64()
					return fmt.Appendf(nil, "%v", val)
				},
			}
		case metrics.KindFloat64Histogram:
			node = &metricsFile{
				name: f.Name,
				readval: func() []byte {
					mr.mu.RLock()
					defer mr.mu.RUnlock()

					// TODO: do not have buckets for now
					hist := mr.samples[idx].Value.Float64Histogram()
					return fmt.Appendf(nil, "%+v", hist.Counts)
				},
			}
		case metrics.KindBad:
			panic("unexpected metrics.KindBad")
		default:
			// unsupported (yet?) kind
			continue
		}

		ch := p.NewPersistentInode(ctx, node, fs.StableAttr{})
		p.AddChild(base, ch, true)
	}

	// Start reading metrics periodically
	go func() {
		ctx := context.TODO()
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mr.mu.Lock()
				metrics.Read(mr.samples)
				mr.mu.Unlock()
			}
		}
	}()
}

// func (mr *metricsRoot) createMetricNode(name string, kind metrics.Kind) fs.InodeEmbedder {
