package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	wk "github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type schemaStore struct {
	sync.RWMutex
	m map[string]*protos.SchemaUpdate
}

func newSchemaStore(initial []*protos.SchemaUpdate, opt options) *schemaStore {
	s := &schemaStore{
		m: map[string]*protos.SchemaUpdate{
			"_predicate_": &protos.SchemaUpdate{
				ValueType: uint32(types.StringID),
				List:      true,
				Explicit:  true,
			},
		},
	}
	if opt.StoreXids {
		s.m["xid"] = &protos.SchemaUpdate{
			ValueType: uint32(protos.Posting_STRING),
			Tokenizer: []string{"hash"},
			Explicit:  true,
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

func (s *schemaStore) getSchema(pred string) *protos.SchemaUpdate {
	s.RLock()
	defer s.RUnlock()
	return s.m[pred]
}

func (s *schemaStore) validateType(de *protos.DirectedEdge, objectIsUID bool) {
	if objectIsUID {
		de.ValueType = uint32(protos.Posting_UID)
	}

	s.RLock()
	sch, ok := s.m[de.Attr]
	s.RUnlock()
	if !ok {
		s.Lock()
		sch, ok = s.m[de.Attr]
		if !ok {
			sch = &protos.SchemaUpdate{ValueType: de.ValueType}
			s.m[de.Attr] = sch
		}
		s.Unlock()
	}

	schTyp := types.TypeID(sch.ValueType)
	err := wk.ValidateAndConvert(de, schTyp)
	if sch.GetExplicit() && err != nil {
		log.Fatalf("RDF doesn't match schema: %v", err)
	}
}

func (s *schemaStore) write(db *badger.DB) {
	txn := db.NewTransaction(true)
	for pred, sch := range s.m {
		k := x.SchemaKey(pred)
		v, err := sch.Marshal()
		x.Check(err)
		x.Check(txn.Set(k, v, 0x00))
	}
	x.Check(txn.Commit(nil))
}
