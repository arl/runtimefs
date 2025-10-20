package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/arl/runtimefs"
)

const mountDir = "./mnt"

func main() {
	unmount, err := runtimefs.Mount(mountDir)
	if err != nil {
		fmt.Printf("Failed to mount: %s", err)
		return
	}

	fmt.Println("Press Ctrl+C to unmount and exit")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	<-ctx.Done()

	fmt.Println("Unmounting...")
	if err := unmount(); err != nil {
		fmt.Printf("Failed to unmount: %s", err)
	}
	fmt.Println("Unmounted")
}
