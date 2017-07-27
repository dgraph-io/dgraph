/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package dgraph

import (
	"path/filepath"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type Options struct {
	PostingDir    string
	PostingTables string
	WALDir        string
	Nomutations   bool
	NumPending    int

	AllottedMemory float64
	CommitFraction float64

	BaseWorkerPort      int
	ExportPath          string
	NumPendingProposals int
	Tracing             float64
	GroupIds            string
	MyAddr              string
	PeerAddr            string
	RaftId              uint64
	MaxPendingCount     uint64
	ExpandEdge          bool
	InMemoryComm        bool

	ConfigFile string
	DebugMode  bool
}

// TODO(tzdybal) - remove global
var Config Options

var DefaultConfig = Options{
	PostingDir:    "p",
	PostingTables: "loadtoram",
	WALDir:        "w",
	Nomutations:   false,
	NumPending:    1000,

	// User must specify this.
	AllottedMemory: -1.0,
	CommitFraction: 0.10,

	BaseWorkerPort:      12345,
	ExportPath:          "export",
	NumPendingProposals: 2000,
	Tracing:             0.0,
	GroupIds:            "0,1",
	MyAddr:              "",
	PeerAddr:            "",
	RaftId:              1,
	MaxPendingCount:     1000,
	ExpandEdge:          true,
	InMemoryComm:        false,

	ConfigFile: "",
	DebugMode:  false,
}

func SetConfiguration(newConfig Options) {
	newConfig.validate()

	Config = newConfig

	posting.Config.AllottedMemory = Config.AllottedMemory
	posting.Config.CommitFraction = Config.CommitFraction

	worker.Config.BaseWorkerPort = Config.BaseWorkerPort
	worker.Config.ExportPath = Config.ExportPath
	worker.Config.NumPendingProposals = Config.NumPendingProposals
	worker.Config.Tracing = Config.Tracing
	worker.Config.GroupIds = Config.GroupIds
	worker.Config.MyAddr = Config.MyAddr
	worker.Config.PeerAddr = Config.PeerAddr
	worker.Config.RaftId = Config.RaftId
	worker.Config.MaxPendingCount = Config.MaxPendingCount
	worker.Config.ExpandEdge = Config.ExpandEdge
	worker.Config.InMemoryComm = Config.InMemoryComm

	x.Config.ConfigFile = Config.ConfigFile
	x.Config.DebugMode = Config.DebugMode
}

func (o *Options) validate() {
	pd, err := filepath.Abs(o.PostingDir)
	x.Check(err)
	wd, err := filepath.Abs(o.WALDir)
	x.Check(err)
	x.AssertTruef(pd != wd, "Posting and WAL directory cannot be the same ('%s').", o.PostingDir)
	x.AssertTruef(o.AllottedMemory > 1024.0, "Allotted memory for Dgraph should be at least 1GB. Current set to: %f", o.AllottedMemory)
}
