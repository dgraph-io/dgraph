/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
)

const (
	queryACLUsersBefore_20_07_0 = `
		{
			nodes(func: has(dgraph.xid)) @filter(type(User)) {
				uid
			}
		}
	`
	queryACLGroupsBefore_20_07_0 = `
		{
			nodes(func: has(dgraph.xid)) @filter(type(Group)) {
				uid
			}
		}
	`
	queryACLRulesBefore_20_07_0 = `
		{
			nodes(func: has(dgraph.rule.predicate)) @filter(type(Rule)) {
				uid
			}
		}
	`
	typeQuery = `
		{
			nodes(func: type(%s)) {
				uid
			}
		}
	`
)

type node struct {
	Uid string `json:"uid"`
}

type queryResult struct {
	Nodes []node `json:"nodes"`
}

type upgradeTypeNameInfo struct {
	oldTypeName string
	newTypeName string
	oldQuery    string
}

func getQueryResult(dg *dgo.Dgraph, query string) (*queryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := dg.NewReadOnlyTxn().Query(ctx, query)
	if err != nil {
		return nil, err
	}

	var queryRes queryResult
	if err = json.Unmarshal(resp.GetJson(), &queryRes); err != nil {
		return nil, err
	}

	return &queryRes, nil
}

func upgradeACLTypeName(dg *dgo.Dgraph, typeNameInfo *upgradeTypeNameInfo) error {
	// query ACL nodes for old ACL type name
	oldQueryRes, err := getQueryResult(dg, typeNameInfo.oldQuery)
	if err != nil {
		return fmt.Errorf("unable to query old ACL type %s: %w", typeNameInfo.oldTypeName, err)
	}

	// query all nodes having old ACL type name
	typeQueryRes, err := getQueryResult(dg, fmt.Sprintf(typeQuery, typeNameInfo.oldTypeName))
	if err != nil {
		return fmt.Errorf("unable to query all nodes for old ACL type %s: %w",
			typeNameInfo.oldTypeName, err)
	}

	// build NQuads for changing old type name to new name
	var setNQuads, delNQuads []*api.NQuad
	for _, node := range oldQueryRes.Nodes {
		setNQuads = append(setNQuads, &api.NQuad{
			Subject:   node.Uid,
			Predicate: "dgraph.type",
			ObjectId:  typeNameInfo.newTypeName,
		})
		delNQuads = append(delNQuads, &api.NQuad{
			Subject:   node.Uid,
			Predicate: "dgraph.type",
			ObjectId:  typeNameInfo.oldTypeName,
		})
	}

	// send the mutation to change the old type name to new name
	if len(oldQueryRes.Nodes) > 0 {
		err = mutateWithClient(dg, &api.Mutation{Set: setNQuads, Del: delNQuads})
		if err != nil {
			return fmt.Errorf("error upgrading data for old ACL type %s: %w",
				typeNameInfo.oldTypeName, err)
		}
	}

	// remove the type from schema if it was not being used for other user-defined nodes
	if len(typeQueryRes.Nodes) > len(oldQueryRes.Nodes) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err = dg.Alter(ctx, &api.Operation{
			DropOp:    api.Operation_TYPE,
			DropValue: typeNameInfo.oldTypeName,
		}); err != nil {
			return fmt.Errorf("error deleting old ACL type %s from schema: %w",
				typeNameInfo.oldTypeName, err)
		}
	}

	return nil
}

func upgradeAclTypeNames() error {
	dg, conn, err := getDgoClient(true)
	if err != nil {
		return fmt.Errorf("error getting dgo client: %w", err)
	}
	defer conn.Close()

	aclTypeNameInfo := []*upgradeTypeNameInfo{
		{
			oldTypeName: "User",
			newTypeName: "dgraph.type.User",
			oldQuery:    queryACLUsersBefore_20_07_0,
		},
		{
			oldTypeName: "Group",
			newTypeName: "dgraph.type.Group",
			oldQuery:    queryACLGroupsBefore_20_07_0,
		},
		{
			oldTypeName: "Rule",
			newTypeName: "dgraph.type.Rule",
			oldQuery:    queryACLRulesBefore_20_07_0,
		},
	}

	for _, typeNameInfo := range aclTypeNameInfo {
		if err = upgradeACLTypeName(dg, typeNameInfo); err != nil {
			return fmt.Errorf("error upgrading ACL type name from %s to %s: %sw",
				typeNameInfo.oldTypeName, typeNameInfo.newTypeName, err)
		}
	}

	return nil
}

func upgradeNonPredefinedNamesInReservedNamespace() error {
	//dg, conn, err := getDgoClient(false)
	//if err != nil {
	//	return fmt.Errorf("error getting dgo client: %w", err)
	//}
	//defer conn.Close()
	return nil
}
