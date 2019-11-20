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

package bulk

import (
	"fmt"
	"log"
	"math"
	"sync"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	wk "github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type schemaStore struct {
	sync.RWMutex
	schemaMap map[string]*pb.SchemaUpdate
	types     []*pb.TypeUpdate
	*state
}

func newSchemaStore(initial *schema.ParsedSchema, opt options, state *state) *schemaStore {
	s := &schemaStore{
		schemaMap: map[string]*pb.SchemaUpdate{},
		state:     state,
	}

	// Load all initial predicates. Some predicates that might not be used when
	// the alpha is started (e.g ACL predicates) might be included but it's
	// better to include them in case the input data contains triples with these
	// predicates.
	for _, update := range schema.CompleteInitialSchema() {
		s.schemaMap[update.Predicate] = update
	}

	if opt.StoreXids {
		s.schemaMap["xid"] = &pb.SchemaUpdate{
			ValueType: pb.Posting_STRING,
			Tokenizer: []string{"hash"},
		}
	}

	for _, sch := range initial.Preds {
		p := sch.Predicate
		sch.Predicate = "" // Predicate is stored in the (badger) key, so not needed in the value.
		if _, ok := s.schemaMap[p]; ok {
			fmt.Printf("Predicate %q already exists in schema\n", p)
			continue
		}
		s.schemaMap[p] = sch
	}

	s.types = initial.Types

	return s
}

func (s *schemaStore) getSchema(pred string) *pb.SchemaUpdate {
	s.RLock()
	defer s.RUnlock()
	return s.schemaMap[pred]
}

func (s *schemaStore) setSchemaAsList(pred string) {
	s.Lock()
	defer s.Unlock()
	schema, ok := s.schemaMap[pred]
	if !ok {
		return
	}
	schema.List = true
}

func (s *schemaStore) validateType(de *pb.DirectedEdge, objectIsUID bool) {
	if objectIsUID {
		de.ValueType = pb.Posting_UID
	}

	s.RLock()
	sch, ok := s.schemaMap[de.Attr]
	s.RUnlock()
	if !ok {
		s.Lock()
		sch, ok = s.schemaMap[de.Attr]
		if !ok {
			sch = &pb.SchemaUpdate{ValueType: de.ValueType}
			if objectIsUID {
				sch.List = true
			}
			s.schemaMap[de.Attr] = sch
		}
		s.Unlock()
	}

	err := wk.ValidateAndConvert(de, sch)
	if err != nil {
		log.Fatalf("RDF doesn't match schema: %v", err)
	}
}

func (s *schemaStore) getPredicates(db *badger.DB) []string {
	txn := db.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()

	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	itr := txn.NewIterator(opts)
	defer itr.Close()

	m := make(map[string]struct{})
	for itr.Rewind(); itr.Valid(); {
		item := itr.Item()
		pk, err := x.Parse(item.Key())
		x.Check(err)
		m[pk.Attr] = struct{}{}
		itr.Seek(pk.SkipPredicate())
		continue
	}

	var preds []string
	for pred := range m {
		preds = append(preds, pred)
	}
	return preds
}

func (s *schemaStore) write(db *badger.DB, preds []string) {
	txn := db.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()
	for _, pred := range preds {
		sch, ok := s.schemaMap[pred]
		if !ok {
			continue
		}
		k := x.SchemaKey(pred)
		v, err := sch.Marshal()
		x.Check(err)
		x.Check(txn.SetEntry(&badger.Entry{
			Key:      k,
			Value:    v,
			UserMeta: posting.BitSchemaPosting}))
	}

	// Write all the types as all groups should have access to all the types.
	for _, typ := range s.types {
		k := x.TypeKey(typ.TypeName)
		v, err := typ.Marshal()
		x.Check(err)
		x.Check(txn.SetEntry(&badger.Entry{
			Key:      k,
			Value:    v,
			UserMeta: posting.BitSchemaPosting,
		}))
	}

	// Write schema always at timestamp 1, s.state.writeTs may not be equal to 1
	// if bulk loader was restarted or other similar scenarios.
	x.Check(txn.CommitAt(1, nil))
}
