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

// needUpdateIndex determines whether we need to create any index jobs.
func (l *List) needUpdateIndex(mp *types.Posting) bool {
	if len(l.key) == 0 || l.key[0] == indexByte {
		// Posting is not the normal kind of mutation.
		return false
	}
	if mp.ValueBytes() == nil {
		// This is not setting any values. It is just changing UIDs which has nothing
		// to do with index.
		return false
	}
	return indexedAttr[l.keyAttr]
}

// indexAdd adds to jobs an indexJob that is an insert.
func (l *List) indexAdd(jobs []*indexJob, mp *types.Posting) []*indexJob {
	return append(jobs, &indexJob{l.keyAttr, l.keyUID, mp.ValueBytes(), false})
}

// indexDel adds to jobs an indexJob that is a delete.
func (l *List) indexDel(jobs []*indexJob, mp *types.Posting) []*indexJob {
	return append(jobs, &indexJob{l.keyAttr, l.keyUID, mp.ValueBytes(), true})
}
