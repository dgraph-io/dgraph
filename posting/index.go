/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package posting

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	stype "github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// Posting list keys are prefixed with this rune if it is a mutation meant for
	// the index.
	indexRune   = ':'
	dateFormat1 = "2006-01-02"
	dateFormat2 = "2006-01-02T15:04:05"
)

var (
	indexLog   trace.EventLog
	indexStore *store.Store
)

func init() {
	indexLog = trace.NewEventLog("index", "Logger")
}

// InitIndex initializes the index with the given data store.
func InitIndex(ds *store.Store) {
	if ds == nil {
		return
	}
	indexStore = ds
}

// IndexKey creates a key for indexing the term for given attribute.
func IndexKey(attr string, term []byte) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, len(attr)+len(term)+2))
	_, err := buf.WriteRune(indexRune)
	x.Check(err)
	_, err = buf.WriteString(attr)
	x.Check(err)
	_, err = buf.WriteRune('|')
	x.Check(err)
	_, err = buf.Write(term)
	x.Check(err)
	return buf.Bytes()
}

func exactMatchIndexKeys(attr string, data []byte) [][]byte {
	return [][]byte{IndexKey(attr, data)}
}

func tokenizedIndexKeys(attr string, data []byte) ([][]byte, error) {
	t := schema.TypeOf(attr)
	if !t.IsScalar() {
		return nil, x.Errorf("Cannot index attribute %s of type object.", attr)
	}
	s := t.(stype.Scalar)
	switch s.ID() {
	case stype.GeoID:
		return geo.IndexKeys(data)
	case stype.Int32ID:
		return intIndex(attr, data)
	case stype.FloatID:
		return floatIndex(attr, data)
	case stype.DateID:
		return dateIndex1(attr, data)
	case stype.DateTimeID:
		return dateIndex2(attr, data)
	case stype.BoolID:
	default:
		return exactMatchIndexKeys(attr, data), nil
	}
	return nil, nil
}

func intIndex(attr string, data []byte) ([][]byte, error) {
	fmt.Println(strconv.Atoi(string(data)))
	return [][]byte{IndexKey(attr, data)}, nil
}

func floatIndex(attr string, data []byte) ([][]byte, error) {
	f, _ := strconv.ParseFloat(string(data), 64)
	in := int(f)
	fmt.Println(in)
	return [][]byte{IndexKey(attr, []byte(strconv.Itoa(in)))}, nil
}

func dateIndex1(attr string, data []byte) ([][]byte, error) {
	t, _ := time.Parse(dateFormat1, string(data))
	fmt.Println(t.Year())
	return [][]byte{IndexKey(attr, []byte(strconv.Itoa(t.Year())))}, nil
}

func dateIndex2(attr string, data []byte) ([][]byte, error) {
	t, _ := time.Parse(dateFormat2, string(data))
	fmt.Println(t.Year())
	return [][]byte{IndexKey(attr, []byte(strconv.Itoa(t.Year())))}, nil
}

// processIndexTerm adds mutation(s) for a single term, to maintain index.
func processIndexTerm(ctx context.Context, attr string, uid uint64, term []byte, del bool) {
	x.Assert(uid != 0)
	keys, err := tokenizedIndexKeys(attr, term)
	if err != nil {
		// This data is not indexable
		return
	}
	edge := x.DirectedEdge{
		Timestamp: time.Now(),
		ValueId:   uid,
		Attribute: attr,
		Source:    "idx",
	}

	for _, key := range keys {
		plist, decr := GetOrCreate(key, indexStore)
		defer decr()
		x.Assertf(plist != nil, "plist is nil [%s] %d %s", key, edge.ValueId, edge.Attribute)

		if del {
			_, err := plist.AddMutation(ctx, edge, Del)
			if err != nil {
				x.TraceError(ctx, x.Wrapf(err, "Error deleting %s for attr %s entity %d: %v",
					string(key), edge.Attribute, edge.Entity))
			}
			indexLog.Printf("DEL [%s] [%d] OldTerm [%s]", edge.Attribute, edge.Entity, string(key))
		} else {
			_, err := plist.AddMutation(ctx, edge, Set)
			if err != nil {
				x.TraceError(ctx, x.Wrapf(err, "Error adding %s for attr %s entity %d: %v",
					string(key), edge.Attribute, edge.Entity))
			}
			indexLog.Printf("SET [%s] [%d] NewTerm [%s]", edge.Attribute, edge.Entity, string(key))
		}
	}
}

// AddMutationWithIndex is AddMutation with support for indexing.
func (l *List) AddMutationWithIndex(ctx context.Context, t x.DirectedEdge, op byte) error {
	x.Assertf(len(t.Attribute) > 0 && t.Attribute[0] != indexRune,
		"[%s] [%d] [%s] %d %d\n", t.Attribute, t.Entity, string(t.Value), t.ValueId, op)

	var lastPost types.Posting
	var hasLastPost bool
	var indexTerm []byte

	if t.ValueType != 0 {
		p := stype.ValueForType(stype.TypeID(t.ValueType))
		err := p.UnmarshalBinary(t.Value)
		if err != nil {
			return err
		}
		indexTerm, err = p.MarshalText()
		if err != nil {
			return err
		}
	} else {
		indexTerm = t.Value
	}

	doUpdateIndex := indexStore != nil && (t.Value != nil) &&
		schema.IsIndexed(t.Attribute)
	if doUpdateIndex {
		// Check last posting for original value BEFORE any mutation actually happens.
		if l.Length() >= 1 {
			x.Assert(l.Get(&lastPost, l.Length()-1))
			hasLastPost = true
		}
	}
	hasMutated, err := l.AddMutation(ctx, t, op)
	if err != nil {
		return err
	}
	if !hasMutated || !doUpdateIndex {
		return nil
	}

	// Exact matches.
	if hasLastPost && lastPost.ValueBytes() != nil {
		delTerm := lastPost.ValueBytes()
		delType := lastPost.ValType()
		if delType != 0 {
			p := stype.ValueForType(stype.TypeID(delType))
			err = p.UnmarshalBinary(delTerm)
			if err != nil {
				return err
			}
			delTerm, err = p.MarshalText()
			if err != nil {
				return err
			}

		}
		processIndexTerm(ctx, t.Attribute, t.Entity, delTerm, true)
	}
	if op == Set {
		processIndexTerm(ctx, t.Attribute, t.Entity, indexTerm, false)
	}
	return nil
}
