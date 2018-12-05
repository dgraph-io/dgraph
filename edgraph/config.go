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

package edgraph

import (
	"expvar"
	"path/filepath"
	"time"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type Options struct {
	PostingDir   string
	BadgerTables string
	BadgerVlog   string
	WALDir       string
	Nomutations  bool
	AuthToken    string

	AllottedMemory float64

	HmacSecret    []byte
	AccessJwtTtl  time.Duration
	RefreshJwtTtl time.Duration
}

var Config Options

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
	x.Conf.Set("posting_dir", newStr(conf.PostingDir))
	x.Conf.Set("wal_dir", newStr(conf.WALDir))
	x.Conf.Set("allotted_memory", newFloat(conf.AllottedMemory))

	// Set some vars from worker.Config.
	x.Conf.Set("tracing", newFloat(worker.Config.Tracing))
	x.Conf.Set("num_pending_proposals", newInt(worker.Config.NumPendingProposals))
	x.Conf.Set("expand_edge", newIntFromBool(worker.Config.ExpandEdge))
}

func SetConfiguration(newConfig Options) {
	newConfig.validate()
	setConfVar(newConfig)
	Config = newConfig

	posting.Config.Mu.Lock()
	posting.Config.AllottedMemory = Config.AllottedMemory
	posting.Config.Mu.Unlock()
}

const MinAllottedMemory = 1024.0

func (o *Options) validate() {
	pd, err := filepath.Abs(o.PostingDir)
	x.Check(err)
	wd, err := filepath.Abs(o.WALDir)
	x.Check(err)
	x.AssertTruef(pd != wd, "Posting and WAL directory cannot be the same ('%s').", o.PostingDir)
	x.AssertTruefNoTrace(o.AllottedMemory != -1,
		"LRU memory (--lru_mb) must be specified. (At least 1024 MB)")
	x.AssertTruefNoTrace(o.AllottedMemory >= MinAllottedMemory,
		"LRU memory (--lru_mb) must be at least %.0f MB. Currently set to: %f",
		MinAllottedMemory, o.AllottedMemory)
}
