//go:build !linux
// +build !linux

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"github.com/golang/glog"

	"github.com/dgraph-io/ristretto/v2/z"
)

func MonitorDiskMetrics(_ string, _ string, lc *z.Closer) {
	defer lc.Done()
	glog.Infoln("File system metrics are not currently supported on non-Linux platforms")
}
