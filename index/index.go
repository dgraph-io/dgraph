/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package index indexes values in database. This can be used for filtering.
package index

import (
	"bufio"
	"os"

	"github.com/dgraph-io/dgraph/index/indexer"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// We try to batch our updates so that they are more efficient.
	batchSize = 10000

	backfillBufSize  = 20000
	frontfillBufSize = 200
)

type mutation struct {
	remove bool   // If false, this is a "insert with overwrite" operation.
	attr   string // The predicate. Can be left empty if obvious in code.
	uid    uint64 // The subject.
	value  string // If del, this field is ignored.
}

// Indices holds predicate indices and is the core object of the package.
type Indices struct {
	dir     string
	cfg     *Configs
	indexer indexer.Indexer
	idx     map[string]*predIndex // Maps predicate / attribute to index.

	// For backfill.
	errC chan error
}

// index is index for one predicate.
type predIndex struct {
	indexer indexer.Indexer
	cfg     *Config
	child   []*childIndex

	// For backfill.
	errC chan error
}

type childIndex struct {
	parent     *predIndex
	childID    int
	batch      indexer.Batch
	backfillC  chan *mutation
	frontfillC chan *mutation
}

// CreateIndices creates new empty dirs given config file and basedir.
func CreateIndices(cfg *Configs, dir string) (*Indices, error) {
	x.Check(os.MkdirAll(dir, 0700))
	cfg.write(dir)
	indexer := indexer.New(cfg.Indexer)
	if err := indexer.Create(dir); err != nil {
		return nil, err
	}
	return initIndices(cfg, dir, indexer)
}

// NewIndices constructs Indices from basedir which contains a config file.
func NewIndices(dir string) (*Indices, error) {
	configFilename := getDefaultConfig(dir)
	fin, err := os.Open(configFilename)
	if err != nil {
		return nil, err
	}
	defer fin.Close()
	cfg, err := NewConfigs(bufio.NewReader(fin))
	if err != nil {
		return nil, err
	}

	indexer := indexer.New(cfg.Indexer)
	err = indexer.Open(dir)
	if err != nil {
		return nil, err
	}
	return initIndices(cfg, dir, indexer)
}

func initIndices(cfg *Configs, dir string, indexer indexer.Indexer) (*Indices, error) {
	idx := make(map[string]*predIndex)
	indices := &Indices{
		dir:     dir,
		cfg:     cfg,
		indexer: indexer,
		idx:     idx,
		errC:    make(chan error),
	}

	for _, c := range cfg.Cfg { // For each predicate.
		idx := &predIndex{
			indexer: indexer,
			cfg:     c,
			errC:    make(chan error),
		}
		indices.idx[c.Attr] = idx

		batch, err := indexer.NewBatch()
		if err != nil {
			return nil, err
		}

		for i := 0; i < c.NumChild; i++ {
			child := &childIndex{
				parent:     idx,
				childID:    i,
				batch:      batch,
				backfillC:  make(chan *mutation, backfillBufSize),
				frontfillC: make(chan *mutation, frontfillBufSize),
			}
			go child.handleFrontfill()
			idx.child = append(idx.child, child)
		}
	}
	return indices, nil
}
