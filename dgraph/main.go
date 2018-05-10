/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package main

import (
	"math/rand"
	"runtime"
	"time"

	"github.com/dgraph-io/dgraph/dgraph/cmd"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	// Setting a higher number here allows more disk I/O calls to be scheduled, hence considerably
	// improving throughput. The extra CPU overhead is almost negligible in comparison. The
	// benchmark notes are located in badger-bench/randread.
	runtime.GOMAXPROCS(128)
	cmd.Execute()
}
