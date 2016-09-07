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

package posting

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"time"

	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// Posting list keys are prefixed with this byte if it is a mutation meant for
	// the index.
	indexByte = ':'
)

type indexConfigs struct {
	Cfg []*indexConfig `json:"config"`
}

type indexConfig struct {
	Attr string `json:"attribute"`
	// TODO(jchiu): Add other tokenizer here in future.
}

func init() {
	x.AddInit(func() {
		x.Assert(indexConfigFile != nil && len(*indexConfigFile) > 0)
		f, err := ioutil.ReadFile(*indexConfigFile)
		x.Check(err)
		log.Printf("Reading index configs from [%s]\n", *indexConfigFile)
		ReadIndexConfigs(f)
	})
}

func ReadIndexConfigs(f []byte) {
	x.Check(json.Unmarshal(f, &indexCfgs))
	for _, c := range indexCfgs.Cfg {
		indexedAttr[c.Attr] = true
	}
	if len(indexedAttr) == 0 {
		log.Println("No indexed attributes!")
	} else {
		for k := range indexedAttr {
			log.Printf("Indexed attribute [%s]\n", k)
		}
	}
}

// indexJob describes a mutation for a normal posting list.
type indexJob struct {
	attr string
	uid  uint64 // Source entity / node.
	term []byte
	del  bool // Is this a delete or insert?
}

var (
	indexJobC       chan indexJob
	indexStore      *store.Store
	indexDone       chan struct{}
	indexCfgs       indexConfigs
	indexConfigFile = flag.String("indexconfig", "index.json", "File containing index config. If empty, we create an empty config.")
	indexedAttr     = make(map[string]bool)
)

// InitIndex initializes the index with the given data store.
func InitIndex(ds *store.Store) {
	if ds == nil {
		return
	}
	indexStore = ds
	indexJobC = make(chan indexJob, 100)
	indexDone = make(chan struct{})
	go processIndexJob()
}

func CloseIndex() {
	close(indexJobC)
	<-indexDone
}

func IndexKey(attr string, value []byte) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, len(attr)+len(value)+2))
	x.Check(buf.WriteByte(indexByte))
	_, err := buf.WriteString(attr)
	x.Check(err)
	_, err = buf.WriteString("|")
	x.Check(err)
	_, err = buf.Write(value)
	x.Check(err)
	return buf.Bytes()
}

// processIndexJob consumes and processes jobs from indexJobC. It can create new
// posting lists and will add new mutations.
func processIndexJob() {
	for job := range indexJobC {
		edge := x.DirectedEdge{
			Timestamp: time.Now(),
			ValueId:   job.uid,
			Attribute: job.attr,
		}
		key := IndexKey(edge.Attribute, job.term)
		plist := GetOrCreate(key, indexStore)
		x.Assertf(plist != nil, "plist is nil [%s] %d %s", key, edge.ValueId, edge.Attribute)
		if job.del {
			plist.AddMutation(context.Background(), edge, Del)
		} else {
			plist.AddMutation(context.Background(), edge, Set)
		}
	}
	indexDone <- struct{}{}
}

// AddMutationWithIndex is AddMutation with support for indexing.
func (l *List) AddMutationWithIndex(ctx context.Context, t x.DirectedEdge, op byte) error {
	doUpdateIndex := t.Value != nil &&
		(len(l.key) > 0 && l.key[0] != indexByte) && indexedAttr[l.keyAttr]
	var lastPost *types.Posting
	if doUpdateIndex {
		// Check last posting for original value BEFORE any mutation actually happens.
		if l.Length() >= 1 {
			lastPost = new(types.Posting)
			x.Assert(l.Get(lastPost, l.Length()-1))
		}
	}

	hasMutated, err := l.AddMutation(ctx, t, op)
	if err != nil {
		return err
	}

	if !hasMutated {
		// No mutation happened. No need to do anything for index.
		// Another way to detect no mutation is to do a []bytes comparison after the
		// mutation finishes, but this will cause additional copying.
		return nil
	}

	if lastPost != nil && lastPost.ValueBytes() != nil {
		l.indexDel(lastPost.ValueBytes())
	}
	if op == Set {
		l.indexAdd(t.Value)
	}
	return nil
}

// indexAdd adds to jobs an indexJob that is an insert.
func (l *List) indexAdd(newValue []byte) {
	if indexJobC != nil {
		indexJobC <- indexJob{l.keyAttr, l.keyUID, newValue, false}
	}
}

// indexDel adds to jobs an indexJob that is a delete.
func (l *List) indexDel(oldValue []byte) {
	if indexJobC != nil {
		indexJobC <- indexJob{l.keyAttr, l.keyUID, oldValue, true}
	}
}
