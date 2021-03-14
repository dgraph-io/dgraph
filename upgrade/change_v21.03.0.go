/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

package upgrade

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

const (
	queryCORS_v21_03_0 = `{
			cors(func: has(dgraph.cors)){
				uid
				dgraph.cors
			}
		}
	`
	querySchema_v21_03_0 = `{
			schema(func: has(dgraph.graphql.schema)){
				uid
				dgraph.graphql.schema
			}
		}
	`
)

type cors struct {
	UID  string   `json:"uid"`
	Cors []string `json:"dgraph.cors,omitempty"`
}

type schema struct {
	UID       string `json:"uid"`
	GQLSchema string `json:"dgraph.graphql.schema"`
}

func updateGQLSchema(jwt *api.Jwt, gqlSchema string, corsList []string) error {
	if len(gqlSchema) == 0 || len(corsList) == 0 {
		fmt.Println("Nothing to update in GraphQL shchema. Either schema or cors not found.")
		return nil
	}
	gqlSchema += "\n\n\n# Below schema elements will only work for dgraph" +
		" versions >= 21.03. In older versions it will be ignored."
	for _, c := range corsList {
		gqlSchema += fmt.Sprintf("\n# Dgraph.Allow-Origin \"%s\"", c)
	}

	// Update the schema.
	header := http.Header{}
	header.Set("X-Dgraph-AccessToken", jwt.AccessJwt)
	header.Set("X-Dgraph-AuthToken", Upgrade.Conf.GetString(authToken))
	updateSchemaParams := &GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					id
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": gqlSchema},
		Headers:   header,
	}

	resp, err := makeGqlRequest(updateSchemaParams, Upgrade.Conf.GetString(adminUrl))
	if err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return errors.Errorf("Error while updating the schema %s\n", resp.Errors.Error())
	}
	fmt.Println("Successfully updated the GraphQL schema.")
	return nil
}

var deprecatedPreds = map[string]struct{}{
	"dgraph.cors":                      {},
	"dgraph.graphql.schema_created_at": {},
	"dgraph.graphql.schema_history":    {},
	"dgraph.graphql.p_sha256hash":      {},
}

var deprecatedTypes = map[string]struct{}{
	"dgraph.type.cors":       {},
	"dgraph.graphql.history": {},
}

func dropDeprecated(dg *dgo.Dgraph) error {
	if !Upgrade.Conf.GetBool("deleteOld") {
		return nil
	}
	for pred := range deprecatedPreds {
		op := &api.Operation{
			DropOp:    api.Operation_ATTR,
			DropValue: pred,
		}
		if err := alterWithClient(dg, op); err != nil {
			return errors.Wrapf(err, "error deleting old predicate %s", pred)
		}
	}
	for typ := range deprecatedTypes {
		op := &api.Operation{
			DropOp:    api.Operation_TYPE,
			DropValue: typ,
		}
		if err := alterWithClient(dg, op); err != nil {
			return errors.Wrapf(err, "error deleting old type %s", typ)
		}
	}
	fmt.Println("Successfully dropped the deprecated predicates")
	return nil
}

func upgradeCORS() error {
	dg, cb := x.GetDgraphClient(Upgrade.Conf, true)
	defer cb()

	jwt, err := getAccessJwt()
	if err != nil {
		return errors.Wrap(err, "while getting jwt auth token")
	}

	// Get CORS.
	corsData := make(map[string][]cors)
	if err = getQueryResult(dg, queryCORS_v21_03_0, &corsData); err != nil {
		return errors.Wrap(err, "error querying cors")
	}

	var corsList []string
	var maxUid uint64
	for _, cors := range corsData["cors"] {
		uid, err := strconv.ParseUint(cors.UID, 0, 64)
		if err != nil {
			return err
		}
		if uid > maxUid {
			maxUid = uid
			if len(cors.Cors) == 1 && cors.Cors[0] == "*" {
				// No need to update the GraphQL schema if all origins are allowed.
				corsList = corsList[:0]
				continue
			}
			corsList = cors.Cors
		}
	}

	// Get GraphQL schema.
	schemaData := make(map[string][]schema)
	if err = getQueryResult(dg, querySchema_v21_03_0, &schemaData); err != nil {
		return errors.Wrap(err, "error querying graphql schema")
	}

	var gqlSchema string
	maxUid = 0
	for _, schema := range schemaData["schema"] {
		uid, err := strconv.ParseUint(schema.UID, 0, 64)
		if err != nil {
			return err
		}
		if uid > maxUid {
			maxUid = uid
			gqlSchema = schema.GQLSchema
		}
	}

	// Update the GraphQL schema.
	if err := updateGQLSchema(jwt, gqlSchema, corsList); err != nil {
		return err
	}

	// Drop all the deprecated predicates and types.
	return dropDeprecated(dg)
}

func getData(db *badger.DB, attr string, fn func(item *badger.Item) error) error {
	return db.View(func(txn *badger.Txn) error {
		attr = x.GalaxyAttr(attr)
		initKey := x.ParsedKey{
			Attr: attr,
		}
		prefix := initKey.DataPrefix()
		startKey := append(x.DataKey(attr, math.MaxUint64))

		itOpt := badger.DefaultIteratorOptions
		itOpt.AllVersions = true
		itOpt.Reverse = true
		itOpt.Prefix = prefix
		itr := txn.NewIterator(itOpt)
		defer itr.Close()
		for itr.Seek(startKey); itr.Valid(); itr.Next() {
			item := itr.Item()
			// We expect only complete posting list.
			x.AssertTrue(item.UserMeta() == posting.BitCompletePosting)
			if err := fn(item); err != nil {
				return err
			}
			break
		}
		return nil
	})
}

func updateGqlSchema(db *badger.DB, cors [][]byte) error {
	entry := &badger.Entry{}
	var version uint64
	err := getData(db, "dgraph.graphql.schema", func(item *badger.Item) error {
		var err error
		entry.Key = item.KeyCopy(nil)
		entry.Value, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}
		entry.UserMeta = item.UserMeta()
		entry.ExpiresAt = item.ExpiresAt()
		version = item.Version()
		return nil
	})
	if err != nil {
		return err
	}
	if entry.Key == nil {
		return nil
	}

	var corsBytes []byte
	corsBytes = append(corsBytes, []byte("\n\n\n# Below schema elements will only work for dgraph"+
		" versions >= 21.03. In older versions it will be ignored.")...)
	for _, val := range cors {
		corsBytes = append(corsBytes, []byte(fmt.Sprintf("\n# Dgraph.Allow-Origin \"%s\"",
			string(val)))...)
	}

	// Append the cors at the end of GraphQL schema.
	pl := pb.PostingList{}
	if err = pl.Unmarshal(entry.Value); err != nil {
		return err
	}
	pl.Postings[0].Value = append(pl.Postings[0].Value, corsBytes...)
	entry.Value, err = pl.Marshal()
	if err != nil {
		return err
	}

	txn := db.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()
	if err = txn.SetEntry(entry); err != nil {
		return err
	}
	return txn.CommitAt(version, nil)
}

func getCors(db *badger.DB) ([][]byte, error) {
	var corsVals [][]byte
	err := getData(db, "dgraph.cors", func(item *badger.Item) error {
		val, _ := item.ValueCopy(nil)
		pl := pb.PostingList{}
		if err := pl.Unmarshal(val); err != nil {
			return err
		}
		for _, p := range pl.Postings {
			corsVals = append(corsVals, p.Value)
		}
		return nil
	})
	return corsVals, err
}

func dropDepreciated(db *badger.DB) error {
	var prefixes [][]byte
	for pred := range deprecatedPreds {
		pred = x.GalaxyAttr(pred)
		prefixes = append(prefixes, x.SchemaKey(pred), x.PredicatePrefix(pred))
	}
	for typ := range deprecatedTypes {
		prefixes = append(prefixes, x.TypeKey(x.GalaxyAttr(typ)))
	}
	return db.DropPrefix(prefixes...)
}

// FixCors removes the internal predicates from the restored backup. This also appends CORS from
// dgraph.cors to GraphQL schema.
func FixCors(db *badger.DB) error {
	glog.Infof("Fixing cors information in the restored backup.")
	cors, err := getCors(db)
	if err != nil {
		return errors.Wrapf(err, "Error while getting cors from db.")
	}
	if err := updateGqlSchema(db, cors); err != nil {
		return errors.Wrapf(err, "Error while updating GraphQL schema.")
	}
	return errors.Wrapf(dropDepreciated(db), "Error while dropping depreciated preds/types.")
}
