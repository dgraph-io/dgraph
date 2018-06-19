/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package edgraph

import (
	"bytes"
	"errors"
	"expvar"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type Options struct {
	PostingDir    string
	BadgerTables  string
	BadgerVlog    string
	BadgerOptions string
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
	BadgerTables:  "mmap",
	BadgerVlog:    "mmap",
	BadgerOptions: "default",
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

	// This is so we can find these options in /debug/vars.
	x.Conf.Set("badger.tables", newStr(conf.BadgerTables))
	x.Conf.Set("badger.vlog", newStr(conf.BadgerVlog))
	x.Conf.Set("badger.options", newStr(conf.BadgerOptions))

	x.Conf.Set("posting_dir", newStr(conf.PostingDir))
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

	ips, err := parseIPsFromString(Config.WhitelistedIPs)

	if err != nil {
		fmt.Println("IP ranges could not be parsed from --whitelist " + Config.WhitelistedIPs)
		worker.Config.WhiteListedIPRanges = []worker.IPRange{}
	} else {
		worker.Config.WhiteListedIPRanges = ips
	}

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
		"LRU memory (--lru_mb) must be specified, with value greater than 1024 MB")
	x.AssertTruefNoTrace(o.AllottedMemory >= MinAllottedMemory,
		"LRU memory (--lru_mb) must be at least %.0f MB. Currently set to: %f",
		MinAllottedMemory, o.AllottedMemory)
}

// Parses the comma-delimited whitelist ip-range string passed in as an argument
// from the command line and returns slice of []IPRange
//
// ex. "144.142.126.222:144.124.126.400,190.59.35.57:190.59.35.99"
func parseIPsFromString(str string) ([]worker.IPRange, error) {
	if str == "" {
		return []worker.IPRange{}, nil
	}

	var ipRanges []worker.IPRange
	ipRangeStrings := strings.Split(str, ",")

	// Check that the each of the ranges are valid
	for _, s := range ipRangeStrings {
		ipsTuple := strings.Split(s, ":")

		// Assert that the range consists of an upper and lower bound
		if len(ipsTuple) != 2 {
			return nil, errors.New("IP range must have a lower and upper bound")
		}

		lowerBoundIP := net.ParseIP(ipsTuple[0])
		upperBoundIP := net.ParseIP(ipsTuple[1])

		if lowerBoundIP == nil || upperBoundIP == nil {
			// Assert that both upper and lower bound are valid IPs
			return nil, errors.New(
				ipsTuple[0] + " or " + ipsTuple[1] + " is not a valid IP address",
			)
		} else if bytes.Compare(lowerBoundIP, upperBoundIP) > 0 {
			// Assert that the lower bound is less than the upper bound
			return nil, errors.New(
				ipsTuple[0] + " cannot be greater than " + ipsTuple[1],
			)
		} else {
			ipRanges = append(ipRanges, worker.IPRange{Lower: lowerBoundIP, Upper: upperBoundIP})
		}
	}
	return ipRanges, nil
}
