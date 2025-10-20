package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/arl/runtimefs"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: example <mountpoint>")
		return
	}
	unmount, err := runtimefs.Mount(os.Args[1])
	if err != nil {
		fmt.Printf("Exiting: %s", err)
		return
	}

	fmt.Println("Press Ctrl+C to unmount and exit")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	<-ctx.Done()

	fmt.Println("Unmounting...")
	if err := unmount(); err != nil {
		fmt.Printf("Error during unmount: %s", err)
	}
	fmt.Println("Unmounted")
}
