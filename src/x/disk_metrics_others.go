//go:build !linux
// +build !linux

package x

import (
	"github.com/golang/glog"

	"github.com/dgraph-io/ristretto/z"
)

func MonitorDiskMetrics(_ string, _ string, lc *z.Closer) {
	defer lc.Done()
	glog.Infoln("File system metrics are not currently supported on non-Linux platforms")
}
