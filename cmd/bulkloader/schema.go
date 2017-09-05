package main

import (
	"log"
	"sync"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	wk "github.com/dgraph-io/dgraph/worker"
)

type schemaState struct {
	strict bool
	*protos.SchemaUpdate
}

type schemaStore struct {
	sync.RWMutex
	m map[string]schemaState
}

func newSchemaStore(initial []*protos.SchemaUpdate) *schemaStore {
	s := &schemaStore{
		m: map[string]schemaState{},
	}
	for _, sch := range initial {
		p := sch.Predicate
		sch.Predicate = ""
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

	write := false
	s.RLock()
	sch, ok := s.m[de.Attr]
	if !ok {
		s.RUnlock()
		s.Lock()
		write = true
		sch, ok = s.m[de.Attr]
		if !ok {
			sch = schemaState{false, &protos.SchemaUpdate{ValueType: de.ValueType}}
			s.m[de.Attr] = sch
		}
	}
	if write {
		s.Unlock()
	} else {
		s.RUnlock()
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
