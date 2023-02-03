//go:build linux
// +build linux

package x

// Only setting linux because some of the darwin/BSDs have a different struct for syscall.statfs_t

import (
	"context"
	"syscall"
	"time"

	"github.com/golang/glog"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/dgraph-io/ristretto/z"
)

func MonitorDiskMetrics(dirTag string, dir string, lc *z.Closer) {
	defer lc.Done()
	ctx, err := tag.New(context.Background(), tag.Upsert(KeyDirType, dirTag))

	fastTicker := time.NewTicker(10 * time.Second)
	defer fastTicker.Stop()

	if err != nil {
		glog.Errorln("Invalid Tag", err)
		return
	}

	for {
		select {
		case <-lc.HasBeenClosed():
			return
		case <-fastTicker.C:
			s := syscall.Statfs_t{}
			err = syscall.Statfs(dir, &s)
			if err != nil {
				continue
			}
			reservedBlocks := s.Bfree - s.Bavail
			total := int64(s.Frsize) * int64(s.Blocks-reservedBlocks)
			free := int64(s.Frsize) * int64(s.Bavail)
			stats.Record(ctx, DiskFree.M(free), DiskUsed.M(total-free), DiskTotal.M(total))
		}
	}

}
