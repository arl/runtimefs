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

		mr.createLeafDir(ctx, p, base, idx, descs[idx])
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

func (mr *metricsRoot) createLeafDir(ctx context.Context, p *fs.Inode, base string, idx int, desc metrics.Description) {
	nodes := make(map[string]fs.InodeEmbedder)
	name, unit, _ := strings.Cut(base, ":")

	nodes["description"] = newStaticFile([]byte(desc.Description))
	nodes["cumulative"] = newStaticFile(b2str(desc.Cumulative))

	switch desc.Kind {
	case metrics.KindUint64:
		nodes[unit] = mr.createUint64Node(idx)

	case metrics.KindFloat64:
		nodes[unit] = mr.createFloat64Node(idx)

	case metrics.KindFloat64Histogram:
		nodes[unit] = mr.createHistNode(idx)

	case metrics.KindBad:
		panic("unexpected metrics.KindBad")
	default:
		break // unsupported metric kind (yet)
	}

	ch := p.NewPersistentInode(ctx, &fs.Inode{}, fs.StableAttr{Mode: fuse.S_IFDIR})
	p.AddChild(name, ch, true)
	p = ch

	for name, node := range nodes {
		ch := p.NewPersistentInode(ctx, node, fs.StableAttr{Mode: fuse.S_IFREG | 0444})
		p.AddChild(name, ch, true)
	}
}

func (mr *metricsRoot) createUint64Node(idx int) fs.InodeEmbedder {
	read := func(buf []byte) ([]byte, int64) {
		mr.mu.RLock()
		defer mr.mu.RUnlock()

		val := mr.samples[idx].Value.Uint64()
		return fmt.Appendf(buf, "%d\n", val), mr.lastUpdate
	}

	return newMetricsFile(read)
}

func (mr *metricsRoot) createFloat64Node(idx int) fs.InodeEmbedder {
	read := func(buf []byte) ([]byte, int64) {
		mr.mu.RLock()
		defer mr.mu.RUnlock()

		val := mr.samples[idx].Value.Float64()
		return fmt.Appendf(buf, "%v\n", val), mr.lastUpdate
	}

	return newMetricsFile(read)
}

func (mr *metricsRoot) createHistNode(idx int) fs.InodeEmbedder {
	read := func(buf []byte) ([]byte, int64) {
		mr.mu.RLock()
		defer mr.mu.RUnlock()

		// TODO: refine this.
		hist := mr.samples[idx].Value.Float64Histogram()
		return fmt.Appendf(buf, "%+v\n", hist.Counts), mr.lastUpdate
	}

	return newMetricsFile(read)
}

func b2str(v bool) []byte {
	if v {
		return []byte("1")
	}
	return []byte("0")
}
