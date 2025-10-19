package runtimefs

import (
	"context"
	"fmt"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
)

type UnmountFunc func() error

func Mount(dirpath string) (UnmountFunc, error) {
	opts := &fs.Options{}

	ctx, cancel := context.WithCancel(context.Background())
	root := newMetricsRoot(ctx, 1*time.Second)

	server, err := fs.Mount(dirpath, root, opts)
	if err != nil {
		return nil, fmt.Errorf("runtimefs failed to mount: %s", err)
	}

	return func() error {
		cancel()
		server.Unmount()
		server.Wait()
		return nil
	}, nil
}
