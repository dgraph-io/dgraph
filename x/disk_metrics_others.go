// +build !linux

package x

import (
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
)

func MonitorDiskMetrics(_ string, _ string, lc *z.Closer) {
	defer lc.Done()
	glog.Infoln("File system metrics are not currently supported on non-Linux platforms")
}
