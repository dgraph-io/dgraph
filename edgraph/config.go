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
package edgraph

import (
	"expvar"
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

	AllottedMemory float64

	BaseWorkerPort      int
	ExportPath          string
	NumPendingProposals int
	Tracing             float64
	MyAddr              string
	ZeroAddr            string
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

	// User must specify this.
	AllottedMemory: -1.0,

	BaseWorkerPort:      12345,
	ExportPath:          "export",
	NumPendingProposals: 2000,
	Tracing:             0.0,
	MyAddr:              "",
	ZeroAddr:            "localhost:8888",
	MaxPendingCount:     1000,
	ExpandEdge:          true,
	InMemoryComm:        false,

	ConfigFile: "",
	DebugMode:  false,
}

// Sometimes users use config.yaml flag so /debug/vars doesn't have information about the
// value of the flags. Hence we dump conf options we care about to the conf map.
func setConfVar(conf Options) {
	newStr := func(s string) *expvar.String {
		v := new(expvar.String)
		v.Set(s)
		return v
	}

	newFloat := func(f float64) *expvar.Float {
		v := new(expvar.Float)
		v.Set(f)
		return v
	}

	newInt := func(i int) *expvar.Int {
		v := new(expvar.Int)
		v.Set(int64(i))
		return v
	}

	// Expvar doesn't have bool type so we use an int.
	newIntFromBool := func(b bool) *expvar.Int {
		v := new(expvar.Int)
		if b {
			v.Set(1)
		} else {
			v.Set(0)
		}
		return v
	}

	x.Conf.Set("posting_dir", newStr(conf.PostingDir))
	x.Conf.Set("posting_tables", newStr(conf.PostingTables))
	x.Conf.Set("wal_dir", newStr(conf.WALDir))
	x.Conf.Set("allotted_memory", newFloat(conf.AllottedMemory))
	x.Conf.Set("tracing", newFloat(conf.Tracing))
	x.Conf.Set("max_pending_count", newInt(int(conf.MaxPendingCount)))
	x.Conf.Set("num_pending_proposals", newInt(conf.NumPendingProposals))
	x.Conf.Set("expand_edge", newIntFromBool(conf.ExpandEdge))
}

func SetConfiguration(newConfig Options) {
	newConfig.validate()
	setConfVar(newConfig)
	Config = newConfig

	posting.Config.Mu.Lock()
	posting.Config.AllottedMemory = Config.AllottedMemory
	posting.Config.Mu.Unlock()

	worker.Config.BaseWorkerPort = Config.BaseWorkerPort
	worker.Config.ExportPath = Config.ExportPath
	worker.Config.NumPendingProposals = Config.NumPendingProposals
	worker.Config.Tracing = Config.Tracing
	worker.Config.MyAddr = Config.MyAddr
	worker.Config.ZeroAddr = Config.ZeroAddr
	worker.Config.RaftId = Config.RaftId
	worker.Config.MaxPendingCount = Config.MaxPendingCount
	worker.Config.ExpandEdge = Config.ExpandEdge
	worker.Config.InMemoryComm = Config.InMemoryComm

	x.Config.ConfigFile = Config.ConfigFile
	x.Config.DebugMode = Config.DebugMode
}

const MinAllottedMemory = 1024.0

func (o *Options) validate() {
	pd, err := filepath.Abs(o.PostingDir)
	x.Check(err)
	wd, err := filepath.Abs(o.WALDir)
	x.Check(err)
	x.AssertTruef(pd != wd, "Posting and WAL directory cannot be the same ('%s').", o.PostingDir)
	x.AssertTruefNoTrace(o.AllottedMemory != DefaultConfig.AllottedMemory,
		"Allotted memory (--memory_mb) must be specified, with value greater than 1024 MB")
	x.AssertTruefNoTrace(o.AllottedMemory >= MinAllottedMemory,
		"Allotted memory (--memory_mb) must be at least %.0f MB. Currently set to: %f", MinAllottedMemory, o.AllottedMemory)
}
