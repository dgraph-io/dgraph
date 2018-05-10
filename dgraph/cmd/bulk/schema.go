/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package bulk

import (
	"fmt"
	"log"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	wk "github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type schemaStore struct {
	sync.RWMutex
	m map[string]*intern.SchemaUpdate
	*state
}

func newSchemaStore(initial []*intern.SchemaUpdate, opt options, state *state) *schemaStore {
	s := &schemaStore{
		m: map[string]*intern.SchemaUpdate{
			"_predicate_": &intern.SchemaUpdate{
				ValueType: intern.Posting_STRING,
				List:      true,
			},
		},
		state: state,
	}
	if opt.StoreXids {
		s.m["xid"] = &intern.SchemaUpdate{
			ValueType: intern.Posting_STRING,
			Tokenizer: []string{"hash"},
		}
	}
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

func (s *schemaStore) getSchema(pred string) *intern.SchemaUpdate {
	s.RLock()
	defer s.RUnlock()
	return s.m[pred]
}

func (s *schemaStore) validateType(de *intern.DirectedEdge, objectIsUID bool) {
	if objectIsUID {
		de.ValueType = intern.Posting_UID
	}

	s.RLock()
	sch, ok := s.m[de.Attr]
	s.RUnlock()
	if !ok {
		s.Lock()
		sch, ok = s.m[de.Attr]
		if !ok {
			sch = &intern.SchemaUpdate{ValueType: de.ValueType}
			s.m[de.Attr] = sch
		}
		s.Unlock()
	}

	err := wk.ValidateAndConvert(de, sch)
	if err != nil {
		log.Fatalf("RDF doesn't match schema: %v", err)
	}
}

func (s *schemaStore) write(db *badger.ManagedDB) {
	// Write schema always at timestamp 1, s.state.writeTs may not be equal to 1
	// if bulk loader was restarted or other similar scenarios.
	txn := db.NewTransactionAt(1, true)
	for pred, sch := range s.m {
		k := x.SchemaKey(pred)
		v, err := sch.Marshal()
		x.Check(err)
		x.Check(txn.SetWithMeta(k, v, posting.BitCompletePosting))
	}
	x.Check(txn.CommitAt(1, nil))
}
