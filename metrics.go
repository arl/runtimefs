package runtimefs

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/metrics"
	"strings"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// metricsRoot is the root of the metrics filesystem. It populates the
// filesystem with data from runtime/metrics.
type metricsRoot struct {
	fs.Inode

	rootctx context.Context
	every   time.Duration

	mu      sync.RWMutex
	samples []metrics.Sample
}

func newMetricsRoot(ctx context.Context, every time.Duration) *metricsRoot {
	descs := metrics.All()
	samples := make([]metrics.Sample, len(descs))
	for i := range samples {
		samples[i].Name = descs[i].Name
	}
	metrics.Read(samples)

	return &metricsRoot{
		rootctx: ctx,
		every:   every,
		samples: samples,
	}
}

// The root populates the tree in its OnAdd method
var _ = (fs.NodeOnAdder)((*metricsRoot)(nil))

func (mr *metricsRoot) OnAdd(ctx context.Context) {
	// OnAdd is called once we are attached to an Inode. We can then construct a
	// tree. We construct the entire tree, and since we don't want parts of it
	// to disappear when the kernel is short on memory, we use persistent inodes.

	// Start reading metrics periodically
	go func() {
		ticker := time.NewTicker(mr.every)
		for {
			select {
			case <-mr.rootctx.Done():
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				mr.mu.Lock()
				metrics.Read(mr.samples)
				mr.mu.Unlock()
			}
		}
	}()

	for idx, sample := range mr.samples {
		dir, base := filepath.Split(sample.Name)

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

		if node := mr.createMetricNode(idx); node != nil {
			ch := p.NewPersistentInode(ctx, node, fs.StableAttr{})
			p.AddChild(base, ch, true)
		}
	}
}

func (mr *metricsRoot) createMetricNode(idx int) fs.InodeEmbedder {
	sample := mr.samples[idx]

	var node fs.InodeEmbedder

	switch sample.Value.Kind() {
	case metrics.KindUint64:
		node = &metricsFile{
			name: sample.Name,
			readval: func(buf []byte) []byte {
				mr.mu.RLock()
				defer mr.mu.RUnlock()

				val := mr.samples[idx].Value.Uint64()
				return fmt.Appendf(buf, "%d", val)
			},
		}
	case metrics.KindFloat64:
		node = &metricsFile{
			name: sample.Name,
			readval: func(buf []byte) []byte {
				mr.mu.RLock()
				defer mr.mu.RUnlock()

				val := mr.samples[idx].Value.Float64()
				return fmt.Appendf(buf, "%v", val)
			},
		}
	case metrics.KindFloat64Histogram:
		node = &metricsFile{
			name: sample.Name,
			readval: func(buf []byte) []byte {
				mr.mu.RLock()
				defer mr.mu.RUnlock()

				// TODO: refine this.
				hist := mr.samples[idx].Value.Float64Histogram()
				return fmt.Appendf(buf, "%+v", hist.Counts)
			},
		}
	case metrics.KindBad:
		panic("unexpected metrics.KindBad")
	default:
		break // unsupported metric kind (yet)
	}

	return node
}
