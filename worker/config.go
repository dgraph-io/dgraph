/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import "net"

type IPRange struct {
	Lower, Upper net.IP
}

type Options struct {
	ExportPath          string
	NumPendingProposals int
	// TODO: Get rid of this here.
	Tracing             float64
	MyAddr              string
	ZeroAddr            string
	RaftId              uint64
	ExpandEdge          bool
	WhiteListedIPRanges []IPRange
	MaxRetries          int
	StrictMutations     bool
	AclEnabled          bool
}

var Config Options
