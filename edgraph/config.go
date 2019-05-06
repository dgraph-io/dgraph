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
	"path/filepath"
	"time"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

const (
	AllowMutations int = iota
	DisallowMutations
	StrictMutations
)

type Options struct {
	PostingDir     string
	BadgerTables   string
	BadgerVlog     string
	WALDir         string
	MutationsMode  int
	AuthToken      string
	AllottedMemory float64

	HmacSecret         []byte
	AccessJwtTtl       time.Duration
	RefreshJwtTtl      time.Duration
	AclRefreshInterval time.Duration
}

var Config Options

func SetConfiguration(newConfig Options) {
	newConfig.validate()
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
