/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */
package worker

import "net"

type IPRange struct {
	Lower, Upper net.IP
}

type Options struct {
	BaseWorkerPort      int
	ExportPath          string
	NumPendingProposals int
	Tracing             float64
	GroupIds            string
	MyAddr              string
	ZeroAddr            string
	RaftId              uint64
	ExpandEdge          bool
	WhiteListedIPRanges []IPRange
}

var Config Options
