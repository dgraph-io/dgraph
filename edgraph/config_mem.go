// +build linux darwin
// +build cgo

// This file is compiled on linux and darwin when cgo is enabled.

package edgraph

// #include <unistd.h>
import "C"

func init() {
	bytes := int64(C.sysconf(C._SC_PHYS_PAGES) * C.sysconf(C._SC_PAGE_SIZE))
	availableMemory = bytes / 1024 / 1024
}
