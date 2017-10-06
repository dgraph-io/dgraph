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

type schemaState struct {
	strict bool
	*protos.SchemaUpdate
}

type schemaStore struct {
	sync.RWMutex
	m map[string]schemaState
}

func newSchemaStore(initial []*protos.SchemaUpdate, opt options) *schemaStore {
	s := &schemaStore{
		m: map[string]schemaState{
			"_predicate_": {
				strict:       true,
				SchemaUpdate: nil,
			},
			"_lease_": {
				strict:       true,
				SchemaUpdate: &protos.SchemaUpdate{ValueType: uint32(protos.Posting_INT)},
			},
		},
	}
	if opt.StoreXids {
		s.m["xid"] = schemaState{
			strict:       true,
			SchemaUpdate: &protos.SchemaUpdate{ValueType: uint32(protos.Posting_STRING)},
		}
	}
	for _, sch := range initial {
		p := sch.Predicate
		sch.Predicate = ""
		if _, ok := s.m[p]; ok {
			x.Check(fmt.Errorf("predicate %q already exists in schema", p))
		}
		s.m[p] = schemaState{true, sch}
	}
	return s
}

func (s *schemaStore) getSchema(pred string) *protos.SchemaUpdate {
	s.RLock()
	defer s.RUnlock()
	return s.m[pred].SchemaUpdate
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
			sch = schemaState{false, &protos.SchemaUpdate{ValueType: de.ValueType}}
			s.m[de.Attr] = sch
		}
		s.Unlock()
	}

	schTyp := types.TypeID(sch.ValueType)
	err := wk.ValidateAndConvert(de, schTyp)
	if sch.strict && err != nil {
		// TODO: It's unclear to me as to why it's only an error to have a bad
		// conversion if the schema was established explicitly rather than
		// automatically.
		log.Fatalf("RDF doesn't match schema: %v", err)
	}
}

func (s *schemaStore) write(kv *badger.KV) {
	for pred, sch := range s.m {
		k := x.SchemaKey(pred)
		var v []byte
		var err error
		if sch.SchemaUpdate != nil {
			v, err = sch.SchemaUpdate.Marshal()
			x.Check(err)
		}
		x.Check(kv.Set(k, v, 0x00))
	}
}
