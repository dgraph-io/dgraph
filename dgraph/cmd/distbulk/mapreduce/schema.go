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

package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
    wk "github.com/dgraph-io/dgraph/worker"
    "math"
)

type schemaStore struct {
	m map[string]*pb.SchemaUpdate
}

func readSchema(filename string) []*pb.SchemaUpdate {
	f, err := os.Open(filename)
	x.Check(err)
	defer f.Close()

	var r io.Reader = f
	if filepath.Ext(filename) == ".gz" {
		r, err = gzip.NewReader(f)
		x.Check(err)
	}

	buf, err := ioutil.ReadAll(r)
	x.Check(err)

	initialSchema, err := schema.Parse(string(buf))
	x.Check(err)
	return initialSchema
}

func newSchemaStore(initial []*pb.SchemaUpdate, storeXids bool) *schemaStore {
	s := &schemaStore{m: map[string]*pb.SchemaUpdate{}}
	for _, sch := range initial {
		p := sch.Predicate
		sch.Predicate = "" // Predicate is stored in the (badger) key, so not needed in the value.
		if _, ok := s.m[p]; ok {
			x.Check(fmt.Errorf("predicate %q already exists in schema", p))
		}
		s.m[p] = sch
	}
	return s
}

func (s *schemaStore) getSchema(pred string) *pb.SchemaUpdate {
	return s.m[pred]
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
		pk := x.Parse(item.Key())
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

func (s *schemaStore) convertToSchemaType(de *pb.DirectedEdge, objectIsUID bool) {
    if objectIsUID {
        de.ValueType = pb.Posting_UID
    }

    sch, ok := s.m[de.Attr]
    if !ok {
        sch = &pb.SchemaUpdate{ValueType: de.ValueType}
        s.m[de.Attr] = sch
    }

    err := wk.ValidateAndConvert(de, sch)
    x.Check(err)
}

func (s *schemaStore) write(db *badger.DB) {
	// Write schema always at timestamp 1, s.state.writeTs may not be equal to 1
	// if bulk loader was restarted or other similar scenarios.

	// Get predicates from the schema store so that the db includes all
	// predicates from the schema file.
	preds := make([]string, 0, len(s.m))
	for pred := range s.m {
		preds = append(preds, pred)
	}

	// Add predicates from the db so that final schema includes predicates
	// used in the rdf file but not included in the schema file.
	for _, pred := range s.getPredicates(db) {
		if _, ok := s.m[pred]; ! ok {
			preds = append(preds, pred)
		}
	}

	txn := db.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()
	for _, pred := range preds {
		sch, ok := s.m[pred]
		if !ok {
			continue
		}
		k := x.SchemaKey(pred)
		v, err := sch.Marshal()
		x.Check(err)
		x.Check(txn.SetWithMeta(k, v, posting.BitCompletePosting))
	}
	x.Check(txn.CommitAt(1, nil))
}
