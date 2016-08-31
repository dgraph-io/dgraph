/*
 * Copyright 2016 DGraph Labs, Inc.
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

package index

import (
	"bufio"
	"log"
	"os"
	"path"
	"strconv"
	"sync"

	"github.com/blevesearch/bleve"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
)

// jobOp is used in both backfill and frontfill.
type jobOp int

const (
	jobOpAdd     = iota
	jobOpDelete  = iota
	jobOpReplace = iota

	// We try to batch our updates so that they are more efficient.
	batchSize = 10000

	// For backfilling, we can store up to this many jobs / additions in channel.
	backfillBufSize = 20000
)

type indexJob struct {
	op    jobOp
	uid   uint64
	value string
	pred  string // Which predicate. Can be left empty if its value can be assumed.
}

// Indices is the core object for working with Bleve indices.
type Indices struct {
	basedir string
	idx     map[string]*index // Maps predicate / attribute to index.
	cfg     *Configs

	// For backfill.
	errC chan error
}

// predIndex is index for one predicate.
type index struct {
	cfg   *Config
	child []*childIndex

	// For backfill.
	errC chan error
}

type childIndex struct {
	childID    int         // Tell us which child this is, inside predIndex.
	bleveIndex bleve.Index // Guarded by bleveLock.
	cfg        *Config
	bleveLock  sync.RWMutex

	// For backfill.
	batch  *bleve.Batch
	jobC   chan indexJob
	parser valueParser

	// For frontfill.
	mutationC chan *mutation
}

// predPrefix takes a predicate name, and generate a random-looking filename.
func predPrefix(basedir, attr string) string {
	lo, hi := farm.Fingerprint128([]byte(attr))
	filename := strconv.FormatUint(lo, 36) + "_" + strconv.FormatUint(hi, 36)
	return path.Join(basedir, filename)
}

// CreateIndices creates new empty dirs given config file and basedir.
func CreateIndices(config *Configs, basedir string) error {
	x.Check(os.MkdirAll(basedir, 0700))
	config.write(basedir) // Copy config to basedir.
	for _, c := range config.Cfg {
		if err := createPredIndex(c, basedir); err != nil {
			return err
		}
	}
	return nil
}

func createPredIndex(c *Config, basedir string) error {
	prefix := predPrefix(basedir, c.Attribute)
	for i := 0; i < c.NumChild; i++ {
		indexMapping := bleve.NewIndexMapping()
		filename := prefix + "_" + strconv.Itoa(i)
		bleveIndex, err := bleve.New(filename, indexMapping)
		if err != nil {
			return x.Wrap(err)
		}
		bleveIndex.Close()
	}
	return nil
}

// NewIndices constructs Indices from basedir which contains Bleve indices. We
// expect a config file in basedir
func NewIndices(basedir string) (*Indices, error) {
	// Read default config at basedir.
	configFilename := getDefaultConfig(basedir)
	fin, err := os.Open(configFilename)
	x.Check(err)
	defer fin.Close()
	cfg, err := NewConfigs(bufio.NewReader(fin))
	if err != nil {
		return nil, err
	}
	indices := &Indices{
		basedir: basedir,
		idx:     make(map[string]*index),
		cfg:     cfg,
		errC:    make(chan error),
	}
	for _, c := range cfg.Cfg {
		index, err := newPredIndex(c, basedir)
		if err != nil {
			return nil, err
		}
		indices.idx[c.Attribute] = index
	}
	log.Printf("Successfully loaded indices at [%s]\n", basedir)
	return indices, nil
}

func newPredIndex(c *Config, basedir string) (*index, error) {
	prefix := predPrefix(basedir, c.Attribute)
	index := &index{
		cfg:  c,
		errC: make(chan error),
	}
	for i := 0; i < c.NumChild; i++ {
		filename := prefix + "_" + strconv.Itoa(i)
		bi, err := bleve.Open(filename)
		if err != nil {
			return nil, x.Wrap(err)
		}
		child := &childIndex{
			childID:    i,
			bleveIndex: bi,
			batch:      bi.NewBatch(),
			jobC:       make(chan indexJob, backfillBufSize),
			parser:     getParser(c.Type),
			cfg:        c,
		}
		index.child = append(index.child, child)
	}
	return index, nil
}
