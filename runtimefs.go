package runtimefs

import (
	"context"
	"fmt"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type helloRoot struct {
	fs.Inode
}

func (r *helloRoot) OnAdd(ctx context.Context) {
	fmt.Println("helloroot on add")
	ch := r.NewPersistentInode(
		ctx, &fs.MemRegularFile{
			Data: []byte("file.txt"),
			Attr: fuse.Attr{
				Mode: 0644,
			},
		}, fs.StableAttr{Ino: 2})
	r.AddChild("file.txt", ch, false)
}

func (r *helloRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0755
	return 0
}

var _ = (fs.NodeGetattrer)((*helloRoot)(nil))
var _ = (fs.NodeOnAdder)((*helloRoot)(nil))

type UnmountFunc func() error

func Mount(dirpath string) (UnmountFunc, error) {
	tree()
	opts := &fs.Options{
		// OnAdd: func(ctx context.Context) {
		// 	slog.Info("mounted")
		// },
	}

	server, err := fs.Mount(dirpath, &helloRoot{}, opts)
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
