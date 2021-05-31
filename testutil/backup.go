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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgraph/ee"
<<<<<<< HEAD
	"github.com/dgraph-io/dgraph/ee/enc"
=======
>>>>>>> master
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
	ee.RegisterEncFlag(flags)
	if err := config.BindPFlags(flags); err != nil {
		return nil, err
	}
<<<<<<< HEAD
	config.Set("encryption", enc.BuildEncFlag(KeyFile))
	_, encKey := ee.GetKeys(config)
=======
	config.Set("encryption", ee.BuildEncFlag(KeyFile))
	keys, err := ee.GetKeys(config)
	if err != nil {
		return nil, err
	}
>>>>>>> master

	opt := badger.DefaultOptions(pdir).
		WithBlockCacheSize(10 * (1 << 20)).
		WithIndexCacheSize(10 * (1 << 20)).
<<<<<<< HEAD
		WithEncryptionKey(encKey).
=======
		WithEncryptionKey(keys.EncKey).
>>>>>>> master
		WithNamespaceOffset(x.NamespaceOffset)
	return badger.OpenManaged(opt)
}

func StartBackupHttps(t *testing.T, backupDst string, forceFull bool) {
	const backupRequest = `mutation backup($dst: String!, $ff: Boolean!) {
		backup(input: {destination: $dst, forceFull: $ff}) {
			response {
				code
				message
			}
		}
	}`

	adminUrl := "https://" + SockAddrHttp + "/admin"
	params := GraphQLParams{
		Query: backupRequest,
		Variables: map[string]interface{}{
			"dst": backupDst,
			"ff":  forceFull,
		},
	}

	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(params)
	require.NoError(t, err)

	client := GetHttpsClient(t)
	response, err := client.Post(adminUrl, "application/json", &buffer)
	require.NoError(t, err)

	responseBody, err := ioutil.ReadAll(response.Body)
	require.NoError(t, err)
	require.Contains(t, string(responseBody), "Backup queued successfully")
}

func WaitForRestore(t *testing.T, dg *dgo.Dgraph) {
	for {
		resp, err := http.Get("http://" + SockAddrHttp + "/health")
		require.NoError(t, err)
		buf, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		if !strings.Contains(string(buf), "opRestore") {
			break
		}
		time.Sleep(4 * time.Second)
	}

	// Wait for the client to exit draining mode. This is needed because the client might
	// be connected to a follower and might be behind the leader in applying the restore.
	// Waiting for three consecutive successful queries is done to prevent a situation in
	// which the query succeeds at the first attempt because the follower is behind and
	// has not started to apply the restore proposal.
	numSuccess := 0
	for {
		// This is a dummy query that returns no results.
		_, err := dg.NewTxn().Query(context.Background(), `{
	   q(func: has(invalid_pred)) {
		   invalid_pred
	   }}`)

		if err == nil {
			numSuccess++
		} else {
			require.Contains(t, err.Error(), "the server is in draining mode")
			numSuccess = 0
		}

		// Apply restore works differently with race enabled.
		// We are seeing delays in apply proposals hence failure of queries.
		if numSuccess == 10 {
			// The server has been responsive three times in a row.
			break
		}
		time.Sleep(time.Second)
	}
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

	txn := db.NewTransactionAt(math.MaxUint64, false)
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

		values = append(values, x.ParseAttr(pk.Attr))
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
