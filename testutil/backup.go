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
	"fmt"
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// KeyFile is set to the path of the file containing the key. Used for testing purposes only.
var KeyFile string

func openDgraph(pdir string) (*badger.DB, error) {
	// Get key.
	config := viper.New()
	flags := &pflag.FlagSet{}
	enc.RegisterFlags(flags)
	if err := config.BindPFlags(flags); err != nil {
		return nil, err
	}
	config.Set("encryption_key_file", KeyFile)
	k, err := enc.ReadKey(config)
	if err != nil {
		return nil, err
	}

	opt := badger.DefaultOptions(pdir).
		WithBlockCacheSize(10 * (1 << 20)).
		WithIndexCacheSize(10 * (1 << 20)).
		WithEncryptionKey(k)
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

	txn := db.NewTransactionAt(readTs, false)
	defer txn.Discard()
	itr := txn.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()

	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		pk, err := x.Parse(item.Key())
		x.Check(err)
		switch {
		case pk.Attr != attr:
			continue
		case !pk.IsData():
			continue
		}

		pl, err := posting.ReadPostingList(item.Key(), itr)
		if err != nil {
			return nil, err
		}

		err = pl.Iterate(readTs, 0, func(p *pb.Posting) error {
			vID := types.TypeID(p.ValType)
			src := types.ValueForType(vID)
			src.Value = p.Value
			str, err := types.Convert(src, types.StringID)
			if err != nil {
				return err
			}
			value := str.Value.(string)
			values[fmt.Sprintf("%#x", pk.Uid)] = value

			return nil
		})

		if err != nil {
			return nil, err
		}
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

	// Predicates and types in the schema are written with timestamp 1.
	txn := db.NewTransactionAt(1, false)
	defer txn.Discard()
	itr := txn.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()

	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		pk, err := x.Parse(item.Key())
		x.Check(err)

		switch {
		case item.UserMeta() != posting.BitSchemaPosting:
			continue
		case pk.IsSchema() && dType != schemaPredicate:
			continue
		case pk.IsType() && dType != schemaType:
			continue
		}

		values = append(values, pk.Attr)
	}
	return values, nil
}

// GetPredicateNames returns the list of all the predicates stored in the restored pdir.
func GetPredicateNames(pdir string) ([]string, error) {
	return readSchema(pdir, schemaPredicate)
}

// GetTypeNames returns the list of all the types stored in the restored pdir.
func GetTypeNames(pdir string) ([]string, error) {
	return readSchema(pdir, schemaType)
}

// CheckSchema checks the names of the predicates and types in the schema against the given names.
func CheckSchema(t *testing.T, preds, types []string) {
	pdirs := []string{
		"./data/restore/p1",
		"./data/restore/p2",
		"./data/restore/p3",
	}

	restoredPreds := make([]string, 0)
	for _, pdir := range pdirs {
		groupPreds, err := GetPredicateNames(pdir)
		require.NoError(t, err)
		restoredPreds = append(restoredPreds, groupPreds...)

		restoredTypes, err := GetTypeNames(pdir)
		require.NoError(t, err)
		require.ElementsMatch(t, types, restoredTypes)
	}
	require.ElementsMatch(t, preds, restoredPreds)
}
