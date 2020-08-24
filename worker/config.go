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

import (
	"path/filepath"
	"time"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// AllowMutations is the mode allowing all mutations.
	AllowMutations int = iota
	// DisallowMutations is the mode that disallows all mutations.
	DisallowMutations
	// StrictMutations is the mode that allows mutations if and only if they contain known preds.
	StrictMutations
)

// Options contains options for the Dgraph server.
type Options struct {
	// PostingDir is the path to the directory storing the postings..
	PostingDir string
	// BadgerTables is the name of the mode used to load the badger tables for the p directory.
	BadgerTables string
	// BadgerVlog is the name of the mode used to load the badger value log for the p directory.
	BadgerVlog string
	// BadgerWalTables is the name of the mode used to load the badger tables for the w directory.
	BadgerWalTables string
	// BadgerWalVlog is the name of the mode used to load the badger value log for the w directory.
	BadgerWalVlog string

	// BadgerCompressionLevel is the ZSTD compression level used by badger. A
	// higher value means more CPU intensive compression and better compression
	// ratio.
	BadgerCompressionLevel int
	// WALDir is the path to the directory storing the write-ahead log.
	WALDir string
	// MutationsMode is the mode used to handle mutation requests.
	MutationsMode int
	// AuthToken is the token to be passed for Alter HTTP requests.
	AuthToken string
	// AllottedMemory is the estimated size taken by the LRU cache.
	AllottedMemory float64

	// HmacSecret stores the secret used to sign JSON Web Tokens (JWT).
	HmacSecret x.SensitiveByteSlice
	// AccessJwtTtl is the TTL for the access JWT.
	AccessJwtTtl time.Duration
	// RefreshJwtTtl is the TTL of the refresh JWT.
	RefreshJwtTtl time.Duration
	// AclRefreshInterval is the interval used to refresh the ACL cache.
	AclRefreshInterval time.Duration
}

// Config holds an instance of the server options..
var Config Options

// SetConfiguration sets the server configuration to the given config.
func SetConfiguration(newConfig *Options) {
	if newConfig == nil {
		return
	}
	newConfig.validate()
	Config = *newConfig

	posting.Config.Lock()
	defer posting.Config.Unlock()
	posting.Config.AllottedMemory = Config.AllottedMemory
}

// MinAllottedMemory is the minimum amount of memory needed for the LRU cache.
const MinAllottedMemory = 1024.0

// AvailableMemory is the total size of the memory we were able to identify.
var AvailableMemory int64

func (opt *Options) validate() {
	pd, err := filepath.Abs(opt.PostingDir)
	x.Check(err)
	wd, err := filepath.Abs(opt.WALDir)
	x.Check(err)
	x.AssertTruef(pd != wd, "Posting and WAL directory cannot be the same ('%s').", opt.PostingDir)
	if opt.AllottedMemory < 0 {
		if allottedMemory := 0.25 * float64(AvailableMemory); allottedMemory > MinAllottedMemory {
			opt.AllottedMemory = allottedMemory
			glog.Infof(
				"LRU memory (--lru_mb) set to %vMB, 25%% of the total RAM found (%vMB)\n"+
					"For more information on --lru_mb please read "+
					"https://dgraph.io/docs/deploy/#config\n",
				opt.AllottedMemory, AvailableMemory)
		}
	}
	x.AssertTruefNoTrace(opt.AllottedMemory >= MinAllottedMemory,
		"LRU memory (--lru_mb) must be at least %.0f MB. Currently set to: %f\n"+
			"For more information on --lru_mb please read https://dgraph.io/docs/deploy/#config",
		MinAllottedMemory, opt.AllottedMemory)
}
