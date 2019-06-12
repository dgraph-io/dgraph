/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

package schema

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

var (
	pstate *state
	pstore *badger.DB
)

func (s *state) init() {
	s.predicate = make(map[string]*pb.SchemaUpdate)
	s.types = make(map[string]*pb.TypeUpdate)
	s.elog = trace.NewEventLog("Dgraph", "Schema")
}

type state struct {
	sync.RWMutex
	// Map containing predicate to type information.
	predicate map[string]*pb.SchemaUpdate
	types     map[string]*pb.TypeUpdate
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
		delete(s.predicate, pred)
	}

	for typ := range s.types {
		delete(s.types, typ)
	}
}

// Delete updates the schema in memory and disk
func (s *state) Delete(attr string) error {
	s.Lock()
	defer s.Unlock()

	glog.Infof("Deleting schema for predicate: [%s]", attr)
	txn := pstore.NewTransactionAt(1, true)
	if err := txn.Delete(x.SchemaKey(attr)); err != nil {
		return err
	}
	// Delete is called rarely so sync write should be fine.
	if err := txn.CommitAt(1, nil); err != nil {
		return err
	}

	delete(s.predicate, attr)
	return nil
}

// DeleteType updates the schema in memory and disk
func (s *state) DeleteType(typeName string) error {
	s.Lock()
	defer s.Unlock()

	glog.Infof("Deleting type definition for type: [%s]", typeName)
	txn := pstore.NewTransactionAt(1, true)
	if err := txn.Delete(x.TypeKey(typeName)); err != nil {
		return err
	}
	// Delete is called rarely so sync write should be fine.
	if err := txn.CommitAt(1, nil); err != nil {
		return err
	}

	delete(s.types, typeName)
	return nil
}

func logUpdate(schema pb.SchemaUpdate, pred string) string {
	typ := types.TypeID(schema.ValueType).Name()
	if schema.List {
		typ = fmt.Sprintf("[%s]", typ)
	}
	return fmt.Sprintf("Setting schema for attr %s: %v, tokenizer: %v, directive: %v, count: %v\n",
		pred, typ, schema.Tokenizer, schema.Directive, schema.Count)
}

func logTypeUpdate(typ pb.TypeUpdate, typeName string) string {
	return fmt.Sprintf("Setting type definition for type %s: %v\n", typeName, typ)
}

// Set sets the schema for the given predicate in memory.
// Schema mutations must flow through the update function, which are synced to the db.
func (s *state) Set(pred string, schema pb.SchemaUpdate) {
	s.Lock()
	defer s.Unlock()
	s.predicate[pred] = &schema
	s.elog.Printf(logUpdate(schema, pred))
}

// SetType sets the type for the given predicate in memory.
// schema mutations must flow through the update function, which are synced to the db.
func (s *state) SetType(typeName string, typ pb.TypeUpdate) {
	s.Lock()
	defer s.Unlock()
	s.types[typeName] = &typ
	s.elog.Printf(logTypeUpdate(typ, typeName))
}

// Get gets the schema for the given predicate.
func (s *state) Get(pred string) (pb.SchemaUpdate, bool) {
	s.RLock()
	defer s.RUnlock()
	schema, has := s.predicate[pred]
	if !has {
		return pb.SchemaUpdate{}, false
	}
	return *schema, true
}

// GetType gets the type definition for the given type name.
func (s *state) GetType(typeName string) (pb.TypeUpdate, bool) {
	s.RLock()
	defer s.RUnlock()
	typ, has := s.types[typeName]
	if !has {
		return pb.TypeUpdate{}, false
	}
	return *typ, true
}

// TypeOf returns the schema type of predicate
func (s *state) TypeOf(pred string) (types.TypeID, error) {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return types.TypeID(schema.ValueType), nil
	}
	return types.UndefinedID, errors.Errorf("Schema not defined for predicate: %v.", pred)
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
	var out []string
	for k := range s.predicate {
		out = append(out, k)
	}
	return out
}

// Types returns the list of types.
func (s *state) Types() []string {
	s.RLock()
	defer s.RUnlock()
	var out []string
	for k := range s.types {
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
		t, found := tok.GetTokenizer(it)
		x.AssertTruef(found, "Invalid tokenizer %s", it)
		tokenizers = append(tokenizers, t)
	}
	return tokenizers
}

// TokenizerNames returns the tokenizer names for given predicate
func (s *state) TokenizerNames(pred string) []string {
	var names []string
	tokenizers := s.Tokenizer(pred)
	for _, t := range tokenizers {
		names = append(names, t.Name())
	}
	return names
}

// HasTokenizer is a convenience func that checks if a given tokenizer is found in pred.
// Returns true if found, else false.
func (s *state) HasTokenizer(id byte, pred string) bool {
	for _, t := range s.Tokenizer(pred) {
		if t.Identifier() == id {
			return true
		}
	}
	return false
}

// IsReversed returns whether the predicate has reverse edge or not
func (s *state) IsReversed(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return schema.Directive == pb.SchemaUpdate_REVERSE
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

func (s *state) HasUpsert(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return schema.Upsert
	}
	return false
}

func (s *state) HasLang(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return schema.Lang
	}
	return false
}

func Init(ps *badger.DB) {
	pstore = ps
	reset()
}

func Load(predicate string) error {
	if len(predicate) == 0 {
		return errors.Errorf("Empty predicate")
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
	var s pb.SchemaUpdate
	err = item.Value(func(val []byte) error {
		x.Check(s.Unmarshal(val))
		return nil
	})
	if err != nil {
		return err
	}
	State().Set(predicate, s)
	State().elog.Printf(logUpdate(s, predicate))
	glog.Infoln(logUpdate(s, predicate))
	return nil
}

// LoadFromDb reads schema information from db and stores it in memory
func LoadFromDb() error {
	if err := LoadSchemaFromDb(); err != nil {
		return err
	}
	return LoadTypesFromDb()
}

func LoadSchemaFromDb() error {
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
		var s pb.SchemaUpdate
		err := item.Value(func(val []byte) error {
			if len(val) == 0 {
				s = pb.SchemaUpdate{Predicate: attr, ValueType: pb.Posting_DEFAULT}
			}
			x.Checkf(s.Unmarshal(val), "Error while loading schema from db")
			State().Set(attr, s)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func LoadTypesFromDb() error {
	prefix := x.TypePrefix()
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
		var t pb.TypeUpdate
		err := item.Value(func(val []byte) error {
			if len(val) == 0 {
				t = pb.TypeUpdate{TypeName: attr}
			}
			x.Checkf(t.Unmarshal(val), "Error while loading types from db")
			State().SetType(attr, t)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// InitialSchema returns the schema updates to insert at the beginning of
// Dgraph's execution. It looks at the worker options to determine which
// attributes to insert.
func InitialSchema() []*pb.SchemaUpdate {
	return initialSchemaInternal(false)
}

// CompleteInitialSchema returns all the schema updates regardless of the worker
// options. This is useful in situations where the worker options are not known
// in advance and it's better to create all the reserved predicates and remove
// them later than miss some of them. An example of such situation is during bulk
// loading.
func CompleteInitialSchema() []*pb.SchemaUpdate {
	return initialSchemaInternal(true)
}

func initialSchemaInternal(all bool) []*pb.SchemaUpdate {
	var initialSchema []*pb.SchemaUpdate

	initialSchema = append(initialSchema, &pb.SchemaUpdate{
		Predicate: "dgraph.type",
		ValueType: pb.Posting_STRING,
		Directive: pb.SchemaUpdate_INDEX,
		Tokenizer: []string{"exact"},
		List:      true,
	})

	if all || x.WorkerConfig.AclEnabled {
		// propose the schema update for acl predicates
		initialSchema = append(initialSchema, []*pb.SchemaUpdate{
			{
				Predicate: "dgraph.xid",
				ValueType: pb.Posting_STRING,
				Directive: pb.SchemaUpdate_INDEX,
				Upsert:    true,
				Tokenizer: []string{"exact"},
			},
			{
				Predicate: "dgraph.password",
				ValueType: pb.Posting_PASSWORD,
			},
			{
				Predicate: "dgraph.user.group",
				Directive: pb.SchemaUpdate_REVERSE,
				ValueType: pb.Posting_UID,
				List:      true,
			},
			{
				Predicate: "dgraph.group.acl",
				ValueType: pb.Posting_STRING,
			}}...)
	}

	return initialSchema
}

// IsReservedPredicateChanged returns true if the initial update for the reserved
// predicate pred is different than the passed update.
func IsReservedPredicateChanged(pred string, update *pb.SchemaUpdate) bool {
	// Return false for non-reserved predicates.
	if !x.IsReservedPredicate(pred) {
		return false
	}

	initialSchema := InitialSchema()
	for _, original := range initialSchema {
		if original.Predicate != pred {
			continue
		}
		return !proto.Equal(original, update)
	}
	return true
}

func reset() {
	pstate = new(state)
	pstate.init()
}
