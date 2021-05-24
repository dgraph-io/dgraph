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
	"fmt"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

const (
	queryACLUsersBefore_v20_07_0 = `
		{
			nodes(func: type(User)) @filter(has(dgraph.xid)) {
				uid
			}
		}
	`
	queryACLGroupsBefore_v20_07_0 = `
		{
			nodes(func: type(Group)) @filter(has(dgraph.xid)) {
				uid
			}
		}
	`
	queryACLRulesBefore_v20_07_0 = `
		{
			nodes(func: type(Rule)) @filter(has(dgraph.rule.predicate)) {
				uid
			}
		}
	`
	typeCountQuery = `
		{
			nodes(func: type(%s)) {
				count(uid)
			}
		}
	`
	typeSchemaQuery = `schema (type: %s) {}`
)

type uidNodeQueryResp struct {
	Nodes []*struct {
		Uid string `json:"uid"`
	} `json:"nodes"`
}

type countNodeQueryResp struct {
	Nodes []*struct {
		Count int `json:"count"`
	} `json:"nodes"`
}

type schemaTypeField struct {
	Name string `json:"name"`
}

type schemaTypeNode struct {
	Name   string             `json:"name"`
	Fields []*schemaTypeField `json:"fields"`
}

type schemaQueryResp struct {
	Schema []*pb.SchemaNode  `json:"schema"`
	Types  []*schemaTypeNode `json:"types"`
}

type updateTypeNameInfo struct {
	oldTypeName              string
	newTypeName              string
	oldUidNodeQuery          string
	dropOldTypeIfUnused      bool
	predsToRemoveFromOldType map[string]struct{}
}

func upgradeAclTypeNames() error {
	deleteOld := Upgrade.Conf.GetBool("deleteOld")
	// prepare upgrade info for ACL type names
	aclTypeNameInfo := []*updateTypeNameInfo{
		{
			oldTypeName:         "User",
			newTypeName:         "dgraph.type.User",
			oldUidNodeQuery:     queryACLUsersBefore_v20_07_0,
			dropOldTypeIfUnused: deleteOld,
			predsToRemoveFromOldType: map[string]struct{}{
				"dgraph.xid":        {},
				"dgraph.password":   {},
				"dgraph.user.group": {},
			},
		},
		{
			oldTypeName:         "Group",
			newTypeName:         "dgraph.type.Group",
			oldUidNodeQuery:     queryACLGroupsBefore_v20_07_0,
			dropOldTypeIfUnused: deleteOld,
			predsToRemoveFromOldType: map[string]struct{}{
				"dgraph.xid":      {},
				"dgraph.acl.rule": {},
			},
		},
		{
			oldTypeName:         "Rule",
			newTypeName:         "dgraph.type.Rule",
			oldUidNodeQuery:     queryACLRulesBefore_v20_07_0,
			dropOldTypeIfUnused: deleteOld,
			predsToRemoveFromOldType: map[string]struct{}{
				"dgraph.rule.predicate":  {},
				"dgraph.rule.permission": {},
			},
		},
	}

	// get dgo client
	dg, cb := x.GetDgraphClient(Upgrade.Conf, true)
	defer cb()

	// apply upgrades for old ACL type names, one by one.
	for _, typeNameInfo := range aclTypeNameInfo {
		if err := typeNameInfo.updateTypeName(dg); err != nil {
			return fmt.Errorf("error upgrading ACL type name from `%s` to `%s`: %w",
				typeNameInfo.oldTypeName, typeNameInfo.newTypeName, err)
		}
		if err := typeNameInfo.updateTypeSchema(dg); err != nil {
			return fmt.Errorf("error upgrading schema for old ACL type `%s`: %w",
				typeNameInfo.oldTypeName, err)
		}
	}

	return nil
}

// updateTypeName changes the name of the given type from old name to new name for all the nodes of
// that type.
func (t *updateTypeNameInfo) updateTypeName(dg *dgo.Dgraph) error {
	if t == nil {
		return nil
	}

	// query nodes for old type, using the provided query
	var oldQueryRes uidNodeQueryResp
	if err := getQueryResult(dg, t.oldUidNodeQuery, &oldQueryRes); err != nil {
		return fmt.Errorf("unable to query old type `%s`: %w", t.oldTypeName, err)
	}

	// build NQuads for changing old type name to new name
	var setNQuads, delNQuads []*api.NQuad
	for _, node := range oldQueryRes.Nodes {
		setNQuads = append(setNQuads, getTypeNquad(node.Uid, t.newTypeName))
		delNQuads = append(delNQuads, getTypeNquad(node.Uid, t.oldTypeName))
	}

	// send the mutation to change the old type name to new name
	if len(oldQueryRes.Nodes) > 0 {
		if err := mutateWithClient(dg, &api.Mutation{Set: setNQuads, Del: delNQuads}); err != nil {
			return fmt.Errorf("error upgrading data for old type `%s`: %w",
				t.oldTypeName, err)
		}
	}

	return nil
}

// updateTypeSchema drops the old type from schema if it is not being used by any node and it is
// allowed to drop the type. Otherwise, it just alters the schema of old type to remove the old
// predicates.
func (t *updateTypeNameInfo) updateTypeSchema(dg *dgo.Dgraph) error {
	if t == nil {
		return nil
	}

	// find the number of nodes using old type name as their type
	var countQueryRes countNodeQueryResp
	if err := getQueryResult(dg, fmt.Sprintf(typeCountQuery, t.oldTypeName),
		&countQueryRes); err != nil {
		return fmt.Errorf("unable to query node count for type `%s`: %w", t.oldTypeName, err)
	}
	if len(countQueryRes.Nodes) != 1 {
		return fmt.Errorf("unable to find number of nodes for type `%s`", t.oldTypeName)
	}

	// if the old type was not being used by any nodes, and if it is allowed to drop the old type,
	// then drop it from schema
	if countQueryRes.Nodes[0].Count == 0 && t.dropOldTypeIfUnused {
		if err := alterWithClient(dg, &api.Operation{
			DropOp:    api.Operation_TYPE,
			DropValue: t.oldTypeName,
		}); err != nil {
			return fmt.Errorf("unable to drop old type `%s` from schema: %w", t.oldTypeName, err)
		}
	} else if (len(t.predsToRemoveFromOldType)) > 0 {
		// otherwise just alter the schema of type to remove the old predicates

		// query the schema for type
		var schemaTypeQueryResp schemaQueryResp
		if err := getQueryResult(dg, fmt.Sprintf(typeSchemaQuery, t.oldTypeName),
			&schemaTypeQueryResp); err != nil || len(schemaTypeQueryResp.Types) != 1 {
			return fmt.Errorf("unable to query schema for type `%s`: %w", t.oldTypeName, err)
		}
		// rewrite the schema to remove old predicates
		schema := getTypeSchemaString(t.oldTypeName, schemaTypeQueryResp.Types[0], nil,
			t.predsToRemoveFromOldType)
		// overwrite the schema for type with alter
		if err := alterWithClient(dg, &api.Operation{Schema: schema}); err != nil {
			return fmt.Errorf("unable to update schema for type `%s`: %w", t.oldTypeName, err)
		}
	}

	return nil
}
