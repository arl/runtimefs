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

	mu         sync.RWMutex
	samples    []metrics.Sample
	lastUpdate int64
}

func newMetricsRoot(ctx context.Context, every time.Duration) *metricsRoot {
	return &metricsRoot{
		rootctx: ctx,
		every:   every,
	}
}

// The root populates the tree in its OnAdd method
var _ = (fs.NodeOnAdder)((*metricsRoot)(nil))

// OnAdd is called once we are attached to an Inode. We can then construct a
// tree. We construct the entire tree, and since we don't want parts of it to
// disappear when the kernel is short on memory, we use persistent inodes.
func (mr *metricsRoot) OnAdd(ctx context.Context) {
	descs := metrics.All()
	mr.samples = make([]metrics.Sample, len(descs))
	for i := range mr.samples {
		mr.samples[i].Name = descs[i].Name
	}
	metrics.Read(mr.samples)

	// Build the tree.
	for idx, sample := range mr.samples {
		dir, base := filepath.Split(sample.Name)

		p := &mr.Inode
		for _, component := range strings.Split(dir, "/") {
			if len(component) == 0 {
				continue
			}
			ch := p.GetChild(component)
			if ch == nil {
				ch = p.NewPersistentInode(ctx, &fs.Inode{}, fs.StableAttr{Mode: fuse.S_IFDIR})
				p.AddChild(component, ch, true)
			}

			p = ch
		}

		name, unit, _ := strings.Cut(base, ":")

		desc := descs[idx]
		switch desc.Kind {
		case metrics.KindUint64:
			read := func(buf []byte) ([]byte, int64) {
				mr.mu.RLock()
				defer mr.mu.RUnlock()

				val := mr.samples[idx].Value.Uint64()
				return fmt.Appendf(buf, "%d\n", val), mr.lastUpdate
			}

			createSingleValueMetric(ctx, p, desc, name, unit, read)
		case metrics.KindFloat64:
			read := func(buf []byte) ([]byte, int64) {
				mr.mu.RLock()
				defer mr.mu.RUnlock()

				val := mr.samples[idx].Value.Float64()
				return fmt.Appendf(buf, "%v\n", val), mr.lastUpdate
			}

			createSingleValueMetric(ctx, p, desc, name, unit, read)
		case metrics.KindFloat64Histogram:
			read := func(buf []byte) ([]byte, int64) {
				mr.mu.RLock()
				defer mr.mu.RUnlock()

				// TODO: refine this.
				hist := mr.samples[idx].Value.Float64Histogram()
				return fmt.Appendf(buf, "%+v\n", hist.Counts), mr.lastUpdate
			}

			createHistogramMetric(ctx, p, desc, name, unit, read)

		case metrics.KindBad:
			panic("unexpected metrics.KindBad")
		default:
			continue // unsupported metric kind.
		}

	}

	// Update metrics.
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
				mr.lastUpdate = time.Now().Unix()
				mr.mu.Unlock()
			}
		}
	}()
}

func createSingleValueMetric(ctx context.Context, p *fs.Inode, desc metrics.Description, name, unit string, read readFunc) {
	ch := p.NewPersistentInode(ctx, &fs.Inode{}, fs.StableAttr{Mode: fuse.S_IFDIR})
	p.AddChild(name, ch, true)
	p = ch

	attr := fs.StableAttr{Mode: fuse.S_IFREG | 0444}

	node := p.NewPersistentInode(ctx, newMetricsFile(read), attr)
	p.AddChild(unit, node, true)

	node = p.NewPersistentInode(ctx, newStaticFile(desc.Description), attr)
	p.AddChild("description", node, true)

	node = p.NewPersistentInode(ctx, newStaticFile(b2str(desc.Cumulative)), attr)
	p.AddChild("cumulative", node, true)
}

func createHistogramMetric(ctx context.Context, p *fs.Inode, desc metrics.Description, name, unit string, read readFunc) {
	ch := p.NewPersistentInode(ctx, &fs.Inode{}, fs.StableAttr{Mode: fuse.S_IFDIR})
	p.AddChild(name, ch, true)
	p = ch

	attr := fs.StableAttr{Mode: fuse.S_IFREG | 0444}

	node := p.NewPersistentInode(ctx, newMetricsFile(read), attr)
	p.AddChild(unit, node, true)

	node = p.NewPersistentInode(ctx, newStaticFile(desc.Description), attr)
	p.AddChild("description", node, true)

	node = p.NewPersistentInode(ctx, newStaticFile(b2str(desc.Cumulative)), attr)
	p.AddChild("cumulative", node, true)
}

func b2str(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
