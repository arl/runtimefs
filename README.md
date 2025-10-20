# runtimefs


Package runtimefs provides a FUSE filesystem that exposes runtime/metrics data as files and directories.

Metrics are organized in a directory hierarchy that mirrors their names.


## Quick start


Install the library (only depends on github.com/hanwen/go-fuse/v2):

```
go get github.com/arl/runtimefs@latest
```

Example usage:

```go
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
```

## Metric representation


### Single value metrics (`KindFloat64`, `KindUint64`)

Single value metrics are represented as a directory containing:
 - a file named after the unit (e.g. `bytes`, `seconds`) that contains the current value
 - a file named `description` that contains the metric description
 - a file named `cumulative` (1 or 0) that indicates whether the metric is cumulative or not

For example, `/memory/classes/heap/objects` is shown as:

    <mount_dir>/memory/classes/heap/objects/
    ├── bytes
    ├── cumulative
    └── description


### Histogram metrics (kind `KindFloat64Histogram`)

Histogram metrics are represented as a directory containing:
 - a file named after the unit (e.g. `bytes`, `seconds`) that contains the current value (one line per bucket)
 - a file named `buckets` that contains the bucket boundaries (one line per boundary)
 - a file named `description` that contains the metric description
 - a file named `cumulative` (1 or 0) that indicates whether the metric is cumulative or not

For example, `/sched/pauses/total/gc` is shown as:

    <mount_dir>/sched/pauses/total/gc/
    ├── buckets
    ├── bytes
    ├── cumulative
    └── description



## What you can do with it?

You can use standard command line tools to explore and monitor runtime metrics. For example:

 - Use `cat` to read the current value of a metric:

```
cat <mount_dir>/memory/classes/heap/objects/bytes
```

 - Use `watch` to monitor a metric over time:

```
watch cat <mount_dir>/gc/cycles/total/count
```

 - Use `ls` to list available metrics:

```
ls -R <mount_dir>
```

 - Show histogram buckets and their values:
  
```
paste mnt/sched/pauses/total/gc/buckets mnt/sched/pauses/total/gc/seconds
```

 - Let your creativity run wild! and please let me know :-)