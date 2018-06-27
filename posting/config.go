/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package posting

import "sync"

type Options struct {
	Mu             sync.Mutex
	AllottedMemory float64

	CommitFraction float64
}

var Config Options
