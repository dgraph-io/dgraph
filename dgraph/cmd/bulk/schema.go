/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package bulk

import (
	"fmt"
	"log"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/types"
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

	schTyp := types.TypeID(sch.ValueType)
	err := wk.ValidateAndConvert(de, schTyp)
	if err != nil {
		log.Fatalf("RDF doesn't match schema: %v", err)
	}
}

func (s *schemaStore) write(db *badger.ManagedDB) {
	txn := db.NewTransactionAt(s.state.writeTs, true)
	for pred, sch := range s.m {
		k := x.SchemaKey(pred)
		v, err := sch.Marshal()
		x.Check(err)
		x.Check(txn.SetWithMeta(k, v, posting.BitCompletePosting))
	}
	x.Check(txn.CommitAt(s.state.writeTs, nil))
}
