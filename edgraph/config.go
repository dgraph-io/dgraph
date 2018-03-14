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
	"errors"
	"expvar"
	"fmt"
	"net"
	"path/filepath"
	"regexp"

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

	WhitelistedIPs      string
	ExportPath          string
	NumPendingProposals int
	Tracing             float64
	MyAddr              string
	ZeroAddr            string
	RaftId              uint64
	MaxPendingCount     uint64
	ExpandEdge          bool

	DebugMode bool
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

	WhitelistedIPs:      "",
	ExportPath:          "export",
	NumPendingProposals: 2000,
	Tracing:             0.0,
	MyAddr:              "",
	ZeroAddr:            fmt.Sprintf("localhost:%d", x.PortZeroGrpc),
	MaxPendingCount:     100,
	ExpandEdge:          true,

	DebugMode: false,
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

	worker.Config.ExportPath = Config.ExportPath
	worker.Config.NumPendingProposals = Config.NumPendingProposals
	worker.Config.Tracing = Config.Tracing
	worker.Config.MyAddr = Config.MyAddr
	worker.Config.ZeroAddr = Config.ZeroAddr
	worker.Config.RaftId = Config.RaftId
	worker.Config.ExpandEdge = Config.ExpandEdge

	ips, _ := parseIPsFromString(Config.WhitelistedIPs)
	worker.Config.WhiteListedIPs = ips

	x.Config.DebugMode = Config.DebugMode
}

const MinAllottedMemory = 1024.0

func (o *Options) validate() {
	pd, err := filepath.Abs(o.PostingDir)
	x.Check(err)
	wd, err := filepath.Abs(o.WALDir)
	x.Check(err)
	_, err = parseIPsFromString(o.WhitelistedIPs)
	x.Check(err)
	x.AssertTruef(pd != wd, "Posting and WAL directory cannot be the same ('%s').", o.PostingDir)
	x.AssertTruefNoTrace(o.AllottedMemory != DefaultConfig.AllottedMemory,
		"Allotted memory (--memory_mb) must be specified, with value greater than 1024 MB")
	x.AssertTruefNoTrace(o.AllottedMemory >= MinAllottedMemory,
		"Allotted memory (--memory_mb) must be at least %.0f MB. Currently set to: %f", MinAllottedMemory, o.AllottedMemory)
}

// Parses the comma-delimited whitelist ip string passed in as an argument from
// the command line and returns slice of []net.IP
func parseIPsFromString(str string) ([]net.IP, error) {
	if str == "" {
		return []net.IP{}, nil
	}

	var ips []net.IP
	ipStrings := regexp.MustCompile("\\s*,\\s*").Split(str, -1)

	for _, s := range ipStrings {
		ip := net.ParseIP(s)

		if ip == nil {
			return nil, errors.New(s + " is not a valid IP address")
		} else {
			ips = append(ips, ip)
		}
	}
	return ips, nil
}
