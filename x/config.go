/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package x

type Options struct {
	DebugMode      bool
	PortOffset     int
	QueryEdgeLimit uint64
}

var Config Options
