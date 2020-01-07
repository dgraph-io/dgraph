/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package testutil

import (
	"context"
	"fmt"
	"math"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	bpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func openDgraph(pdir string) (*badger.DB, error) {
	opt := badger.DefaultOptions(pdir).WithTableLoadingMode(options.MemoryMap).
		WithReadOnly(true)
	return badger.OpenManaged(opt)
}

// GetPredicateValues reads the specified p directory and returns the values for the given
// attribute in a map.
func GetPredicateValues(pdir, attr string, readTs uint64) (map[string]string, error) {
	db, err := openDgraph(pdir)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	values := make(map[string]string)

	stream := db.NewStreamAt(math.MaxUint64)
	stream.ChooseKey = func(item *badger.Item) bool {
		pk, err := x.Parse(item.Key())
		x.Check(err)
		switch {
		case pk.Attr != attr:
			return false
		case pk.IsSchema():
			return false
		}
		return pk.IsData()
	}
	stream.KeyToList = func(key []byte, it *badger.Iterator) (*bpb.KVList, error) {
		pk, err := x.Parse(key)
		x.Check(err)
		pl, err := posting.ReadPostingList(key, it)
		if err != nil {
			return nil, err
		}
		var list bpb.KVList
		err = pl.Iterate(readTs, 0, func(p *pb.Posting) error {
			vID := types.TypeID(p.ValType)
			src := types.ValueForType(vID)
			src.Value = p.Value
			str, err := types.Convert(src, types.StringID)
			if err != nil {
				fmt.Println(err)
				return err
			}
			value := str.Value.(string)
			list.Kv = append(list.Kv, &bpb.KV{
				Key:   []byte(fmt.Sprintf("%#x", pk.Uid)),
				Value: []byte(value),
			})
			return nil
		})
		return &list, err
	}
	stream.Send = func(list *bpb.KVList) error {
		for _, kv := range list.Kv {
			values[string(kv.Key)] = string(kv.Value)
		}
		return nil
	}
	if err := stream.Orchestrate(context.Background()); err != nil {
		return nil, err
	}
	return values, err
}

type dataType int

const (
	schemaPredicate dataType = iota
	schemaType
)

func readSchema(pdir string, dType dataType) ([]string, error) {
	db, err := openDgraph(pdir)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	values := make([]string, 0)

	stream := db.NewStreamAt(math.MaxUint64)
	stream.ChooseKey = func(item *badger.Item) bool {
		pk, err := x.Parse(item.Key())
		x.Check(err)

		if item.UserMeta() != posting.BitSchemaPosting {
			return false
		}
		switch {
		case pk.IsSchema() && dType == schemaPredicate:
			return true
		case pk.IsType() && dType == schemaType:
			return true
		default:
			return false
		}
	}
	stream.KeyToList = func(key []byte, it *badger.Iterator) (*bpb.KVList, error) {
		pk, err := x.Parse(key)
		x.Check(err)

		var list bpb.KVList
		list.Kv = append(list.Kv, &bpb.KV{
			Key: []byte(pk.Attr),
		})
		return &list, nil
	}
	stream.Send = func(list *bpb.KVList) error {
		for _, kv := range list.Kv {
			values = append(values, string(kv.Key))
		}
		return nil
	}
	if err := stream.Orchestrate(context.Background()); err != nil {
		return nil, err
	}

	return values, err
}

// GetPredicateNames returns the list of all the predicates stored in the restored pdir.
func GetPredicateNames(pdir string, readTs uint64) ([]string, error) {
	return readSchema(pdir, schemaPredicate)
}

// GetTypeNames returns the list of all the types stored in the restored pdir.
func GetTypeNames(pdir string, readTs uint64) ([]string, error) {
	return readSchema(pdir, schemaType)
}
