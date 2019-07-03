# go-sysconf

[![Build Status][1]][2]
[![Go Report Card][3]][4]
[![GoDoc][5]][6]

`sysconf` for Go, without using cgo or external binaries (e.g. getconf).

Supported operating systems: Linux, Darwin, DragonflyBSD, FreeBSD, NetBSD, OpenBSD.
Support for Solaris is planned but not yet implemented.

All POSIX.1 and POSIX.2 variables are supported, see [References](#references) for a complete list.

Additionally, the following non-standard variables are supported on some operating systems:

| Variable | Supported on |
|---|---|
| `SC_PHYS_PAGES`       | Linux, Darwin, FreeBSD, NetBSD, OpenBSD |
| `SC_AVPHYS_PAGES`     | Linux, OpenBSD |
| `SC_NPROCESSORS_CONF` | Linux, Darwin, FreeBSD, NetBSD, OpenBSD |
| `SC_NPROCESSORS_ONLN` | Linux, Darwin, FreeBSD, NetBSD, OpenBSD |
| `SC_UIO_MAXIOV`       | Linux |

## Usage

```Go
package main

import (
	"fmt"

	"github.com/tklauser/go-sysconf"
)

func main() {
	// get clock ticks, this will return the same as C.sysconf(C._SC_CLK_TCK)
	clktck, err := sysconf.Sysconf(sysconf.SC_CLK_TCK)
	if err != nil {
		fmt.Printf("SC_CLK_TCK: %v\n", clktck)
	}
}
```

## References

* [POSIX documenation for `sysconf`](http://pubs.opengroup.org/onlinepubs/9699919799/functions/sysconf.html)
* [Linux manpage for `sysconf(3)`](http://man7.org/linux/man-pages/man3/sysconf.3.html)
* [glibc constants for `sysconf` parameters](https://www.gnu.org/software/libc/manual/html_node/Constants-for-Sysconf.html)

[1]: https://travis-ci.org/tklauser/go-sysconf.svg?branch=master
[2]: https://travis-ci.org/tklauser/go-sysconf
[3]: https://goreportcard.com/badge/github.com/tklauser/go-sysconf
[4]: https://goreportcard.com/report/github.com/tklauser/go-sysconf
[5]: https://godoc.org/github.com/tklauser/go-sysconf?status.svg
[6]: https://godoc.org/github.com/tklauser/go-sysconf
