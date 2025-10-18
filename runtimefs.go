package runtimefs

import (
	"fmt"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
)

type UnmountFunc func() error

func Mount(dirpath string) (UnmountFunc, error) {
	opts := &fs.Options{}

	root := newMetricsRoot(1 * time.Second)

	server, err := fs.Mount(dirpath, root, opts)
	if err != nil {
		return nil, fmt.Errorf("runtimefs failed to mount: %s", err)
	}

	// TODO: check if wait mount is necessary
	// server.WaitMount()

	return func() error {
		server.Unmount()
		server.Wait()
		return nil
	}, nil
}
