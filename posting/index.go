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
	// Posting list keys are prefixed with this rune if it is a mutation meant for
	// the index.
	indexRune = ':'
)

type indexConfigs struct {
	Cfg []*indexConfig `json:"config"`
}

type indexConfig struct {
	Attr string `json:"attribute"`
	// TODO(jchiu): Add other tokenizer here in future.
}

var (
	indexStore      *store.Store
	indexCfgs       indexConfigs
	indexConfigFile = flag.String("indexconfig", "index.json", "File containing index config. If empty, we create an empty config.")
	indexedAttr     = make(map[string]bool)
)

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

// InitIndex initializes the index with the given data store.
func InitIndex(ds *store.Store) {
	if ds == nil {
		return
	}
	indexStore = ds
}

func IndexKey(attr string, value []byte) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, len(attr)+len(value)+2))
	_, err := buf.WriteRune(indexRune)
	x.Check(err)
	_, err = buf.WriteString(attr)
	x.Check(err)
	_, err = buf.WriteString("|")
	x.Check(err)
	_, err = buf.Write(value)
	x.Check(err)
	return buf.Bytes()
}

// processIndexJob adds mutations to maintain our index.
func processIndexJob(attr string, uid uint64, term []byte, del bool) {
	edge := x.DirectedEdge{
		Timestamp: time.Now(),
		ValueId:   uid,
		Attribute: attr,
	}
	key := IndexKey(edge.Attribute, term)
	plist := GetOrCreate(key, indexStore)
	x.Assertf(plist != nil, "plist is nil [%s] %d %s", key, edge.ValueId, edge.Attribute)
	if del {
		plist.AddMutation(context.Background(), edge, Del)
	} else {
		plist.AddMutation(context.Background(), edge, Set)
	}
}

// AddMutationWithIndex is AddMutation with support for indexing.
func (l *List) AddMutationWithIndex(ctx context.Context, t x.DirectedEdge, op byte) error {
	keyUID, keyAttr := DecodeKey(l.Key())
	x.Assert(len(keyAttr) > 0 && keyAttr[0] != indexRune)
	doUpdateIndex := t.Value != nil && indexedAttr[keyAttr]
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
	if !hasMutated || !doUpdateIndex || indexStore == nil {
		return nil
	}
	if lastPost != nil && lastPost.ValueBytes() != nil {
		processIndexJob(keyAttr, keyUID, lastPost.ValueBytes(), true)
	}
	if op == Set {
		processIndexJob(keyAttr, keyUID, t.Value, false)
	}
	return nil
}
