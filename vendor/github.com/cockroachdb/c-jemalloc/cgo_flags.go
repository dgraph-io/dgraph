// Package jemalloc uses the cgo compilation facilities to build the
// jemalloc library.
package jemalloc

// #cgo CFLAGS: -Iinternal/include -std=gnu99
// #cgo CPPFLAGS: -D_REENTRANT
// #cgo linux CPPFLAGS: -D_GNU_SOURCE
// #cgo darwin CFLAGS: -Idarwin_includes/internal/include -Idarwin_includes/internal/include/jemalloc/internal
// #cgo linux CFLAGS: -Ilinux_includes/internal/include -Ilinux_includes/internal/include/jemalloc/internal
// #cgo freebsd CFLAGS: -Ifreebsd_includes/internal/include -Ifreebsd_includes/internal/include/jemalloc/internal
// #cgo LDFLAGS: -lm -lpthread
import "C"
