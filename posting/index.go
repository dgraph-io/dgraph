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
	"time"

	"golang.org/x/net/trace"

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
	indexLog        trace.EventLog
	indexStore      *store.Store
	indexCfgs       indexConfigs
	indexConfigFile = flag.String("indexconfig", "",
		"File containing index config. If empty, we assume no index.")
	indexedAttr = make(map[string]bool)
)

func init() {
	indexLog = trace.NewEventLog("index", "Logger")
	x.AddInit(func() {
		if indexConfigFile == nil || len(*indexConfigFile) == 0 {
			indexLog.Printf("No valid config file: %v", *indexConfigFile)
			return
		}
		f, err := ioutil.ReadFile(*indexConfigFile)
		x.Check(err)
		indexLog.Printf("Reading index configs from [%s]", *indexConfigFile)
		ReadIndexConfigs(f)
	})
}

// ReadIndexConfigs parses configs from given byte array.
func ReadIndexConfigs(f []byte) {
	x.Check(json.Unmarshal(f, &indexCfgs))
	for _, c := range indexCfgs.Cfg {
		indexedAttr[c.Attr] = true
	}
	if len(indexedAttr) == 0 {
		indexLog.Printf("No indexed attributes!")
	} else {
		for k := range indexedAttr {
			indexLog.Printf("Indexed attribute [%s]", k)
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

// IndexKey creates a key for indexing the term for given attribute.
func IndexKey(attr string, term []byte) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, len(attr)+len(term)+2))
	_, err := buf.WriteRune(indexRune)
	x.Check(err)
	_, err = buf.WriteString(attr)
	x.Check(err)
	_, err = buf.WriteRune('|')
	x.Check(err)
	_, err = buf.Write(term)
	x.Check(err)
	return buf.Bytes()
}

// processIndexTerm adds mutation(s) for a single term, to maintain index.
func processIndexTerm(ctx context.Context, attr string, uid uint64, term []byte, del bool) {
	x.Assert(uid != 0)
	edge := x.DirectedEdge{
		Timestamp: time.Now(),
		ValueId:   uid,
		Attribute: attr,
		Source:    "idx",
	}
	key := IndexKey(edge.Attribute, term)
	plist, decr := GetOrCreate(key, indexStore)
	defer decr()
	x.Assertf(plist != nil, "plist is nil [%s] %d %s", key, edge.ValueId, edge.Attribute)

	if del {
		_, err := plist.AddMutation(ctx, edge, Del)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error deleting %s for attr %s entity %d: %v",
				string(term), edge.Attribute, edge.Entity))
		}
		indexLog.Printf("DEL [%s] [%d] OldTerm [%s]", edge.Attribute, edge.Entity, string(term))
	} else {
		_, err := plist.AddMutation(ctx, edge, Set)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error adding %s for attr %s entity %d: %v",
				string(term), edge.Attribute, edge.Entity))
		}
		indexLog.Printf("SET [%s] [%d] NewTerm [%s]", edge.Attribute, edge.Entity, string(term))
	}
}

// AddMutationWithIndex is AddMutation with support for indexing.
func (l *List) AddMutationWithIndex(ctx context.Context, t x.DirectedEdge, op byte) error {
	x.Assertf(len(t.Attribute) > 0 && t.Attribute[0] != indexRune,
		"[%s] [%d] [%s] %d %d\n", t.Attribute, t.Entity, string(t.Value), t.ValueId, op)

	var lastPost types.Posting
	var hasLastPost bool
	doUpdateIndex := indexStore != nil && (t.Value != nil) && indexedAttr[t.Attribute]
	if doUpdateIndex {
		// Check last posting for original value BEFORE any mutation actually happens.
		if l.Length() >= 1 {
			x.Assert(l.Get(&lastPost, l.Length()-1))
			hasLastPost = true
		}
	}
	hasMutated, err := l.AddMutation(ctx, t, op)
	if err != nil {
		return err
	}
	if !hasMutated || !doUpdateIndex {
		return nil
	}
	if hasLastPost && lastPost.ValueBytes() != nil {
		processIndexTerm(ctx, t.Attribute, t.Entity, lastPost.ValueBytes(), true)
	}
	if op == Set {
		processIndexTerm(ctx, t.Attribute, t.Entity, t.Value, false)
	}
	return nil
}
