//go:build (linux || darwin) && cgo
// +build linux darwin
// +build cgo

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

// This file is compiled on linux and darwin when cgo is enabled.

package edgraph

import (
	"github.com/hypermodeinc/dgraph/v25/worker"
)

// #include <unistd.h>
import "C"

func init() {
	bytes := int64(C.sysconf(C._SC_PHYS_PAGES) * C.sysconf(C._SC_PAGE_SIZE))
	worker.AvailableMemory = bytes / 1024 / 1024
}
