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

package schema

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	pstate *state
	pstore *badger.ManagedDB
)

func (s *state) init() {
	s.predicate = make(map[string]*intern.SchemaUpdate)
	s.elog = trace.NewEventLog("Dgraph", "Schema")
}

type state struct {
	sync.RWMutex
	// Map containing predicate to type information.
	predicate map[string]*intern.SchemaUpdate
	elog      trace.EventLog
}

// SateFor returns the schema for given group
func State() *state {
	return pstate
}

func (s *state) DeleteAll() {
	s.Lock()
	defer s.Unlock()

	for pred := range s.predicate {
		// We set schema for _predicate_, hence it shouldn't be deleted.
		if pred != x.PredicateListAttr {
			delete(s.predicate, pred)
		}
	}
}

// Delete updates the schema in memory and disk
func (s *state) Delete(attr string) error {
	s.Lock()
	defer s.Unlock()

	delete(s.predicate, attr)
	txn := pstore.NewTransactionAt(1, true)
	if err := txn.Delete(x.SchemaKey(attr)); err != nil {
		return err
	}
	// Delete is called rarely so sync write should be fine.
	return txn.CommitAt(1, nil)
}

func logUpdate(schema intern.SchemaUpdate, pred string) string {
	typ := types.TypeID(schema.ValueType).Name()
	if schema.List {
		typ = fmt.Sprintf("[%s]", typ)
	}
	return fmt.Sprintf("Setting schema for attr %s: %v, tokenizer: %v, directive: %v, count: %v\n",
		pred, typ, schema.Tokenizer, schema.Directive, schema.Count)
}

// Set sets the schema for given predicate in memory
// schema mutations must flow through update function, which are
// synced to db
func (s *state) Set(pred string, schema intern.SchemaUpdate) {
	s.Lock()
	defer s.Unlock()
	s.predicate[pred] = &schema
	s.elog.Printf(logUpdate(schema, pred))
}

// Get gets the schema for given predicate
func (s *state) Get(pred string) (intern.SchemaUpdate, bool) {
	s.Lock()
	defer s.Unlock()
	schema, has := s.predicate[pred]
	if !has {
		return intern.SchemaUpdate{}, false
	}
	return *schema, true
}

// TypeOf returns the schema type of predicate
func (s *state) TypeOf(pred string) (types.TypeID, error) {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return types.TypeID(schema.ValueType), nil
	}
	return types.TypeID(100), x.Errorf("Schema not defined for predicate: %v.", pred)
}

// IsIndexed returns whether the predicate is indexed or not
func (s *state) IsIndexed(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return len(schema.Tokenizer) > 0
	}
	return false
}

// IndexedFields returns the list of indexed fields
func (s *state) IndexedFields() []string {
	s.RLock()
	defer s.RUnlock()
	var out []string
	for k, v := range s.predicate {
		if len(v.Tokenizer) > 0 {
			out = append(out, k)
		}
	}
	return out
}

// Predicates returns the list of predicates for given group
func (s *state) Predicates() []string {
	s.RLock()
	defer s.RUnlock()
	out := make([]string, 0, len(s.predicate))
	for k := range s.predicate {
		out = append(out, k)
	}
	return out
}

// Tokenizer returns the tokenizer for given predicate
func (s *state) Tokenizer(pred string) []tok.Tokenizer {
	s.RLock()
	defer s.RUnlock()
	schema, ok := s.predicate[pred]
	x.AssertTruef(ok, "schema state not found for %s", pred)
	var tokenizers []tok.Tokenizer
	for _, it := range schema.Tokenizer {
		t, has := tok.GetTokenizer(it)
		x.AssertTruef(has, "Invalid tokenizer %s", it)
		tokenizers = append(tokenizers, t)
	}
	return tokenizers
}

// TokenizerNames returns the tokenizer names for given predicate
func (s *state) TokenizerNames(pred string) []string {
	s.RLock()
	defer s.RUnlock()
	schema, ok := s.predicate[pred]
	x.AssertTruef(ok, "schema state not found for %s", pred)
	var tokenizers []string
	for _, it := range schema.Tokenizer {
		t, found := tok.GetTokenizer(it)
		x.AssertTruef(found, "Tokenizer not found for %s", it)
		tokenizers = append(tokenizers, t.Name())
	}
	return tokenizers
}

// IsReversed returns whether the predicate has reverse edge or not
func (s *state) IsReversed(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return schema.Directive == intern.SchemaUpdate_REVERSE
	}
	return false
}

// HasCount returns whether we want to mantain a count index for the given predicate or not.
func (s *state) HasCount(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return schema.Count
	}
	return false
}

// IsList returns whether the predicate is of list type.
func (s *state) IsList(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return schema.List
	}
	return false
}

func Init(ps *badger.ManagedDB) {
	pstore = ps
	reset()
}

func Load(predicate string) error {
	if len(predicate) == 0 {
		return x.Errorf("Empty predicate")
	}
	key := x.SchemaKey(predicate)
	txn := pstore.NewTransactionAt(1, false)
	defer txn.Discard()
	item, err := txn.Get(key)
	if err == badger.ErrKeyNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	val, err := item.Value()
	if err != nil {
		return err
	}
	var s intern.SchemaUpdate
	x.Check(s.Unmarshal(val))
	State().Set(predicate, s)
	State().elog.Printf(logUpdate(s, predicate))
	x.Printf(logUpdate(s, predicate))
	return nil
}

// LoadFromDb reads schema information from db and stores it in memory
func LoadFromDb() error {
	prefix := x.SchemaPrefix()
	txn := pstore.NewTransactionAt(1, false)
	defer txn.Discard()
	itr := txn.NewIterator(badger.DefaultIteratorOptions) // Need values, reversed=false.
	defer itr.Close()

	for itr.Seek(prefix); itr.Valid(); itr.Next() {
		item := itr.Item()
		key := item.Key()
		if !bytes.HasPrefix(key, prefix) {
			break
		}
		pk := x.Parse(key)
		if pk == nil {
			continue
		}
		attr := pk.Attr
		var s intern.SchemaUpdate
		val, err := item.Value()
		if err != nil {
			return err
		}
		if len(val) == 0 {
			continue
		}
		x.Checkf(s.Unmarshal(val), "Error while loading schema from db")
		State().Set(attr, s)
	}
	return nil
}

func reset() {
	pstate = new(state)
	pstate.init()
}
