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
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	pstate *state
	pstore *badger.DB
)

// We maintain two schemas for a predicate if a background task is building indexes
// for that predicate. Now, we need to use the new schema for mutations whereas
// a query schema for queries. While calling functions in this package, we need
// to set the context correctly as to which schema should be returned.
// Query schema is defined as (old schema - tokenizers to drop based on new schema).
type contextKey int

const (
	isWrite contextKey = iota
)

// GetWriteContext returns a context that sets the schema context for writing.
func GetWriteContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, isWrite, true)
}

func (s *state) init() {
	s.predicate = make(map[string]*pb.SchemaUpdate)
	s.types = make(map[string]*pb.TypeUpdate)
	s.elog = trace.NewEventLog("Dgraph", "Schema")
	s.mutSchema = make(map[string]*pb.SchemaUpdate)
}

type state struct {
	sync.RWMutex
	// Map containing predicate to type information.
	predicate map[string]*pb.SchemaUpdate
	types     map[string]*pb.TypeUpdate
	elog      trace.EventLog
	// mutSchema holds the schema update that is being applied in the background.
	mutSchema map[string]*pb.SchemaUpdate
}

// State returns the struct holding the current schema.
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

	for pred := range s.mutSchema {
		delete(s.mutSchema, pred)
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
	delete(s.mutSchema, attr)
	return nil
}

// DeleteType updates the schema in memory and disk
func (s *state) DeleteType(typeName string) error {
	if s == nil {
		return nil
	}

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

func logUpdate(schema *pb.SchemaUpdate, pred string) string {
	if schema == nil {
		return ""
	}

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
func (s *state) Set(pred string, schema *pb.SchemaUpdate) {
	if schema == nil {
		return
	}

	s.Lock()
	defer s.Unlock()
	s.predicate[pred] = schema
	s.elog.Printf(logUpdate(schema, pred))
}

// SetMutSchema sets the mutation schema for the given predicate.
func (s *state) SetMutSchema(pred string, schema *pb.SchemaUpdate) {
	s.Lock()
	defer s.Unlock()
	s.mutSchema[pred] = schema
}

// DeleteMutSchema deletes the schema for given predicate from mutSchema.
func (s *state) DeleteMutSchema(pred string) {
	s.Lock()
	defer s.Unlock()
	delete(s.mutSchema, pred)
}

// GetIndexingPredicates returns the list of predicates for which we are building indexes.
func GetIndexingPredicates() []string {
	s := State()
	s.Lock()
	defer s.Unlock()
	if len(s.mutSchema) == 0 {
		return nil
	}

	ps := make([]string, 0, len(s.mutSchema))
	for p := range s.mutSchema {
		ps = append(ps, p)
	}
	return ps
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
func (s *state) Get(ctx context.Context, pred string) (pb.SchemaUpdate, bool) {
	isWrite, _ := ctx.Value(isWrite).(bool)
	s.RLock()
	defer s.RUnlock()
	// If this is write context, mutSchema will have the updated schema.
	// If mutSchema doesn't have the predicate key, we use the schema from s.predicate.
	if isWrite {
		if schema, ok := s.mutSchema[pred]; ok {
			return *schema, true
		}
	}

	schema, ok := s.predicate[pred]
	if !ok {
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
func (s *state) IsIndexed(ctx context.Context, pred string) bool {
	isWrite, _ := ctx.Value(isWrite).(bool)
	s.RLock()
	defer s.RUnlock()
	if isWrite {
		// TODO(Aman): we could return the query schema if it is a delete.
		if schema, ok := s.mutSchema[pred]; ok && len(schema.Tokenizer) > 0 {
			return true
		}
	}

	if schema, ok := s.predicate[pred]; ok {
		return len(schema.Tokenizer) > 0
	}

	return false
}

// Predicates returns the list of predicates for given group
func (s *state) Predicates() []string {
	if s == nil {
		return nil
	}

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
	if s == nil {
		return nil
	}

	s.RLock()
	defer s.RUnlock()
	var out []string
	for k := range s.types {
		out = append(out, k)
	}
	return out
}

// Tokenizer returns the tokenizer for given predicate
func (s *state) Tokenizer(ctx context.Context, pred string) []tok.Tokenizer {
	isWrite, _ := ctx.Value(isWrite).(bool)
	s.RLock()
	defer s.RUnlock()
	var su *pb.SchemaUpdate
	if isWrite {
		if schema, ok := s.mutSchema[pred]; ok {
			su = schema
		}
	}
	if su == nil {
		if schema, ok := s.predicate[pred]; ok {
			su = schema
		}
	}
	x.AssertTruef(su != nil, "schema state not found for %s", pred)
	tokenizers := make([]tok.Tokenizer, 0, len(su.Tokenizer))
	for _, it := range su.Tokenizer {
		t, found := tok.GetTokenizer(it)
		x.AssertTruef(found, "Invalid tokenizer %s", it)
		tokenizers = append(tokenizers, t)
	}
	return tokenizers
}

// TokenizerNames returns the tokenizer names for given predicate
func (s *state) TokenizerNames(ctx context.Context, pred string) []string {
	var names []string
	tokenizers := s.Tokenizer(ctx, pred)
	for _, t := range tokenizers {
		names = append(names, t.Name())
	}
	return names
}

// HasTokenizer is a convenience func that checks if a given tokenizer is found in pred.
// Returns true if found, else false.
func (s *state) HasTokenizer(ctx context.Context, id byte, pred string) bool {
	for _, t := range s.Tokenizer(ctx, pred) {
		if t.Identifier() == id {
			return true
		}
	}
	return false
}

// IsReversed returns whether the predicate has reverse edge or not
func (s *state) IsReversed(ctx context.Context, pred string) bool {
	isWrite, _ := ctx.Value(isWrite).(bool)
	s.RLock()
	defer s.RUnlock()
	if isWrite {
		if schema, ok := s.mutSchema[pred]; ok && schema.Directive == pb.SchemaUpdate_REVERSE {
			return true
		}
	}
	if schema, ok := s.predicate[pred]; ok {
		return schema.Directive == pb.SchemaUpdate_REVERSE
	}
	return false
}

// HasCount returns whether we want to mantain a count index for the given predicate or not.
func (s *state) HasCount(ctx context.Context, pred string) bool {
	isWrite, _ := ctx.Value(isWrite).(bool)
	s.RLock()
	defer s.RUnlock()
	if isWrite {
		if schema, ok := s.mutSchema[pred]; ok && schema.Count {
			return true
		}
	}
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

func (s *state) HasNoConflict(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	return s.predicate[pred].GetNoConflict()
}

// IndexingInProgress checks whether indexing is going on for a given predicate.
func (s *state) IndexingInProgress() bool {
	s.RLock()
	defer s.RUnlock()
	return len(s.mutSchema) > 0
}

// Init resets the schema state, setting the underlying DB to the given pointer.
func Init(ps *badger.DB) {
	pstore = ps
	reset()
}

// Load reads the schema for the given predicate from the DB.
func Load(predicate string) error {
	if len(predicate) == 0 {
		return errors.Errorf("Empty predicate")
	}
	delete(State().mutSchema, predicate)
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
	State().Set(predicate, &s)
	State().elog.Printf(logUpdate(&s, predicate))
	glog.Infoln(logUpdate(&s, predicate))
	return nil
}

// LoadFromDb reads schema information from db and stores it in memory
func LoadFromDb() error {
	if err := LoadSchemaFromDb(); err != nil {
		return err
	}
	return LoadTypesFromDb()
}

// LoadSchemaFromDb iterates through the DB and loads all the stored schema updates.
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
		pk, err := x.Parse(key)
		if err != nil {
			glog.Errorf("Error while parsing key %s: %v", hex.Dump(key), err)
			continue
		}
		attr := pk.Attr
		var s pb.SchemaUpdate
		err = item.Value(func(val []byte) error {
			if len(val) == 0 {
				s = pb.SchemaUpdate{Predicate: attr, ValueType: pb.Posting_DEFAULT}
			}
			x.Checkf(s.Unmarshal(val), "Error while loading schema from db")
			State().Set(attr, &s)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// LoadTypesFromDb iterates through the DB and loads all the stored type updates.
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
		pk, err := x.Parse(key)
		if err != nil {
			glog.Errorf("Error while parsing key %s: %v", hex.Dump(key), err)
			continue
		}
		attr := pk.Attr
		var t pb.TypeUpdate
		err = item.Value(func(val []byte) error {
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

// InitialTypes returns the type updates to insert at the beginning of
// Dgraph's execution. It looks at the worker options to determine which
// types to insert.
func InitialTypes() []*pb.TypeUpdate {
	return initialTypesInternal(false)
}

// CompleteInitialTypes returns all the type updates regardless of the worker
// options. This is useful in situations where the worker options are not known
// in advance or it is required to consider all initial pre-defined types. An
// example of such situation is while allowing type updates to go through during
// alter if they are same as existing pre-defined types. This is useful for
// live loading a previously exported schema.
func CompleteInitialTypes() []*pb.TypeUpdate {
	return initialTypesInternal(true)
}

// NOTE: whenever defining a new type here, please also add it in x/keys.go: preDefinedTypeMap
func initialTypesInternal(all bool) []*pb.TypeUpdate {
	var initialTypes []*pb.TypeUpdate
	initialTypes = append(initialTypes,
		&pb.TypeUpdate{
			TypeName: "dgraph.graphql",
			Fields: []*pb.SchemaUpdate{
				{
					Predicate: "dgraph.graphql.schema",
					ValueType: pb.Posting_STRING,
				},
				{
					Predicate: "dgraph.graphql.xid",
					ValueType: pb.Posting_STRING,
				},
			},
		}, &pb.TypeUpdate{
			TypeName: "dgraph.graphql.history",
			Fields: []*pb.SchemaUpdate{
				{
					Predicate: "dgraph.graphql.schema_history",
					ValueType: pb.Posting_STRING,
				}, {
					Predicate: "dgraph.graphql.schema_created_at",
					ValueType: pb.Posting_DATETIME,
				},
			},
		}, &pb.TypeUpdate{
			TypeName: "dgraph.graphql.persisted_query",
			Fields: []*pb.SchemaUpdate{
				{
					Predicate: "dgraph.graphql.p_query",
					ValueType: pb.Posting_STRING,
				}, {
					Predicate: "dgraph.graphql.p_sha256hash",
					ValueType: pb.Posting_STRING,
				},
			},
		})

	if all || x.WorkerConfig.AclEnabled {
		// These type definitions are required for deleteUser and deleteGroup GraphQL API to work
		// properly.
		initialTypes = append(initialTypes, &pb.TypeUpdate{
			TypeName: "dgraph.type.User",
			Fields: []*pb.SchemaUpdate{
				{
					Predicate: "dgraph.xid",
					ValueType: pb.Posting_STRING,
				},
				{
					Predicate: "dgraph.password",
					ValueType: pb.Posting_PASSWORD,
				},
				{
					Predicate: "dgraph.user.group",
					ValueType: pb.Posting_UID,
				},
			},
		},
			&pb.TypeUpdate{
				TypeName: "dgraph.type.Group",
				Fields: []*pb.SchemaUpdate{
					{
						Predicate: "dgraph.xid",
						ValueType: pb.Posting_STRING,
					},
					{
						Predicate: "dgraph.acl.rule",
						ValueType: pb.Posting_UID,
					},
				},
			},
			&pb.TypeUpdate{
				TypeName: "dgraph.type.Rule",
				Fields: []*pb.SchemaUpdate{
					{
						Predicate: "dgraph.rule.predicate",
						ValueType: pb.Posting_STRING,
					},
					{
						Predicate: "dgraph.rule.permission",
						ValueType: pb.Posting_INT,
					},
				},
			})
	}

	return initialTypes
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

	initialSchema = append(initialSchema,
		&pb.SchemaUpdate{
			Predicate: "dgraph.cors",
			ValueType: pb.Posting_STRING,
			List:      true,
			Directive: pb.SchemaUpdate_INDEX,
			Tokenizer: []string{"exact"},
			Upsert:    true,
		}, &pb.SchemaUpdate{
			Predicate: "dgraph.type",
			ValueType: pb.Posting_STRING,
			Directive: pb.SchemaUpdate_INDEX,
			Tokenizer: []string{"exact"},
			List:      true,
		}, &pb.SchemaUpdate{
			Predicate: "dgraph.drop.op",
			ValueType: pb.Posting_STRING,
		}, &pb.SchemaUpdate{
			Predicate: "dgraph.graphql.schema",
			ValueType: pb.Posting_STRING,
		}, &pb.SchemaUpdate{
			Predicate: "dgraph.graphql.xid",
			ValueType: pb.Posting_STRING,
			Directive: pb.SchemaUpdate_INDEX,
			Tokenizer: []string{"exact"},
			Upsert:    true,
		}, &pb.SchemaUpdate{
			Predicate: "dgraph.graphql.schema_history",
			ValueType: pb.Posting_STRING,
		}, &pb.SchemaUpdate{
			Predicate: "dgraph.graphql.schema_created_at",
			ValueType: pb.Posting_DATETIME,
		}, &pb.SchemaUpdate{
			Predicate: "dgraph.graphql.p_query",
			ValueType: pb.Posting_STRING,
		}, &pb.SchemaUpdate{
			Predicate: "dgraph.graphql.p_sha256hash",
			ValueType: pb.Posting_STRING,
			Directive: pb.SchemaUpdate_INDEX,
			Tokenizer: []string{"exact"},
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
				Predicate: "dgraph.acl.rule",
				ValueType: pb.Posting_UID,
				List:      true,
			},
			{
				Predicate: "dgraph.rule.predicate",
				ValueType: pb.Posting_STRING,
				Directive: pb.SchemaUpdate_INDEX,
				Tokenizer: []string{"exact"},
				Upsert:    true, // Not really sure if this will work.
			},
			{
				Predicate: "dgraph.rule.permission",
				ValueType: pb.Posting_INT,
			},
		}...)
	}

	return initialSchema
}

// IsPreDefPredChanged returns true if the initial update for the pre-defined
// predicate is different than the passed update.
// If the passed update is not a pre-defined predicate then it just returns false.
func IsPreDefPredChanged(update *pb.SchemaUpdate) bool {
	// Return false for non-pre-defined predicates.
	if !x.IsPreDefinedPredicate(update.Predicate) {
		return false
	}

	initialSchema := CompleteInitialSchema()
	for _, original := range initialSchema {
		if original.Predicate != update.Predicate {
			continue
		}
		return !proto.Equal(original, update)
	}
	return true
}

// IsPreDefTypeChanged returns true if the initial update for the pre-defined
// type is different than the passed update.
// If the passed update is not a pre-defined type than it just returns false.
func IsPreDefTypeChanged(update *pb.TypeUpdate) bool {
	// Return false for non-pre-defined types.
	if !x.IsPreDefinedType(update.TypeName) {
		return false
	}

	initialTypes := CompleteInitialTypes()
	for _, original := range initialTypes {
		if original.TypeName != update.TypeName {
			continue
		}
		if len(original.Fields) != len(update.Fields) {
			return true
		}
		for i, field := range original.Fields {
			if field.Predicate != update.Fields[i].Predicate {
				return true
			}
		}
	}

	return false
}

func reset() {
	pstate = new(state)
	pstate.init()
}
