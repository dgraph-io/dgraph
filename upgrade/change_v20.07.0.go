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
	"strings"
	"time"

	"github.com/dgraph-io/dgo/v200"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

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
	scalarPredicateQuery = `
		{
			nodes(func: has(%s)) {
				uid
				%s
			}
		}
	`
	uidPredicateQuery = `
		{
			nodes(func: has(%s)) {
				uid
				%s {
					uid
				}
			}
		}
	`
	schemaQuery = `schema{}`
)

type uidNodeQueryResp struct {
	Nodes []*struct {
		Uid string `json:"uid"`
	} `json:"nodes"`
}

type predicateQueryResp struct {
	Nodes []map[string]interface{} `json:"nodes"`
}

type schemaTypeNode struct {
	Name   string `json:"name"`
	Fields []*struct {
		Name string `json:"name"`
	} `json:"fields"`
}

type schemaQueryResp struct {
	Schema []*pb.SchemaNode  `json:"schema"`
	Types  []*schemaTypeNode `json:"types"`
}

type upgradeTypeNameInfo struct {
	oldTypeName     string
	newTypeName     string
	oldUidNodeQuery string
}

type upgradePredNameInfo struct {
	oldPredName string
	newPredName string
	isUidPred   bool
}

func upgradeAclTypeNames() error {
	dg, conn, err := getDgoClient(true)
	if err != nil {
		return fmt.Errorf("error getting dgo client: %w", err)
	}
	defer conn.Close()

	aclTypeNameInfo := []*upgradeTypeNameInfo{
		{
			oldTypeName:     "User",
			newTypeName:     "dgraph.type.User",
			oldUidNodeQuery: queryACLUsersBefore_20_07_0,
		},
		{
			oldTypeName:     "Group",
			newTypeName:     "dgraph.type.Group",
			oldUidNodeQuery: queryACLGroupsBefore_20_07_0,
		},
		{
			oldTypeName:     "Rule",
			newTypeName:     "dgraph.type.Rule",
			oldUidNodeQuery: queryACLRulesBefore_20_07_0,
		},
	}

	for _, typeNameInfo := range aclTypeNameInfo {
		if err = upgradeTypeName(dg, typeNameInfo); err != nil {
			return fmt.Errorf("error upgrading ACL type name from `%s` to `%s`: %w",
				typeNameInfo.oldTypeName, typeNameInfo.newTypeName, err)
		}
	}

	return nil
}

func upgradeNonPredefinedNamesInReservedNamespace() error {
	dg, conn, err := getDgoClient(true)
	if err != nil {
		return fmt.Errorf("error getting dgo client: %w", err)
	}
	defer conn.Close()

	var schemaQueryResp schemaQueryResp
	if err = getQueryResult(dg, schemaQuery, &schemaQueryResp); err != nil {
		return fmt.Errorf("unable to query schema: %w", err)
	}

	// collect predicates to change
	reservedPredicatesInSchema := make(map[string]*pb.SchemaNode)
	nonReservedPredicatesInSchema := make(map[string]struct{})
	for _, schemaNode := range schemaQueryResp.Schema {
		if x.IsReservedPredicate(schemaNode.Predicate) && !x.IsPreDefinedPredicate(schemaNode.
			Predicate) {
			reservedPredicatesInSchema[schemaNode.Predicate] = schemaNode
		} else {
			nonReservedPredicatesInSchema[schemaNode.Predicate] = struct{}{}
		}
	}

	// collect types to change
	reservedTypesInSchema := make(map[string]*schemaTypeNode)
	nonReservedTypesInSchema := make(map[string]struct{})
	nonReservedTypesWithReservedPredicatesInSchema := make(map[string]*schemaTypeNode)
	for _, typeNode := range schemaQueryResp.Types {
		if x.IsReservedType(typeNode.Name) && !x.IsPreDefinedType(typeNode.Name) {
			reservedTypesInSchema[typeNode.Name] = typeNode
		} else {
			nonReservedTypesInSchema[typeNode.Name] = struct{}{}
			for _, field := range typeNode.Fields {
				if _, ok := reservedPredicatesInSchema[field.Name]; ok {
					nonReservedTypesWithReservedPredicatesInSchema[typeNode.Name] = typeNode
					break
				}
			}
		}
	}

	// return if no change is required
	if len(reservedPredicatesInSchema) == 0 && len(reservedTypesInSchema) == 0 && len(
		nonReservedTypesWithReservedPredicatesInSchema) == 0 {
		return nil
	}

	// ask user for new predicate names
	newPredicateNames := make(map[string]string)
	if len(reservedPredicatesInSchema) > 0 {
		fmt.Println("Please provide new names for predicates.")
		for oldPredName, _ := range reservedPredicatesInSchema {
			newPredicateNames[oldPredName] = askUserForNewName(oldPredName, x.IsReservedPredicate,
				nonReservedPredicatesInSchema)
		}
	}

	// ask user for new type names
	newTypeNames := make(map[string]string)
	if len(reservedTypesInSchema) > 0 {
		fmt.Println("Please provide new names for types.")
		for oldTypeName, _ := range reservedTypesInSchema {
			newTypeNames[oldTypeName] = askUserForNewName(oldTypeName, x.IsReservedType,
				nonReservedTypesInSchema)
		}
	}

	// build the new schema for alter
	var newSchemaBuilder strings.Builder
	for oldPredName, predDef := range reservedPredicatesInSchema {
		// add new predicate to schema
		newSchemaBuilder.WriteString(getPredSchemaString(newPredicateNames[oldPredName], predDef))
	}
	for oldTypeName, typeDef := range reservedTypesInSchema {
		// add new type to schema
		newSchemaBuilder.WriteString(getTypeSchemaString(newTypeNames[oldTypeName], typeDef,
			newPredicateNames))
	}
	for typeName, typeDef := range nonReservedTypesWithReservedPredicatesInSchema {
		// overwrite new predicate in existing type which uses old predicate
		newSchemaBuilder.WriteString(getTypeSchemaString(typeName, typeDef, newPredicateNames))
	}

	// execute the alter for new schema, don't drop any old types/predicates yet
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = dg.Alter(ctx, &api.Operation{Schema: newSchemaBuilder.String()}); err != nil {
		return fmt.Errorf("error updating new schema: %w", err)
	}

	// upgrade all the nodes with old predicates and drop the old predicates from schema
	for oldPredName, schemaNode := range reservedPredicatesInSchema {
		newPredName := newPredicateNames[oldPredName]
		if err = upgradePredicateName(dg, &upgradePredNameInfo{
			oldPredName: oldPredName,
			newPredName: newPredName,
			isUidPred:   schemaNode.Type == "uid",
		}); err != nil {
			return fmt.Errorf("error upgrading predicate name from `%s` to `%s`: %w", oldPredName,
				newPredName, err)
		}
	}

	// upgrade all the nodes with old types and drop the old types from schema
	for oldTypeName, newTypeName := range newTypeNames {
		if err = upgradeTypeName(dg, &upgradeTypeNameInfo{
			oldTypeName:     oldTypeName,
			newTypeName:     newTypeName,
			oldUidNodeQuery: fmt.Sprintf(typeQuery, oldTypeName),
		}); err != nil {
			return fmt.Errorf("error upgrading type name from `%s` to `%s`: %w", oldTypeName,
				newTypeName, err)
		}
	}

	return nil
}

// upgradeTypeName changes the name of the given type from old name to new name for all the nodes of
// that type, and then deletes the old type from schema via alter.
func upgradeTypeName(dg *dgo.Dgraph, typeNameInfo *upgradeTypeNameInfo) error {
	// query nodes for old type, using the provided query
	var oldQueryRes uidNodeQueryResp
	if err := getQueryResult(dg, typeNameInfo.oldUidNodeQuery, &oldQueryRes); err != nil {
		return fmt.Errorf("unable to query old type `%s`: %w", typeNameInfo.oldTypeName, err)
	}

	// find the number of nodes having old type
	var typeQueryRes uidNodeQueryResp
	if err := getQueryResult(dg, fmt.Sprintf(typeQuery, typeNameInfo.oldTypeName),
		&typeQueryRes); err != nil {
		return fmt.Errorf("unable to query all nodes for old type `%s`: %w",
			typeNameInfo.oldTypeName, err)
	}
	oldTypeCount := len(typeQueryRes.Nodes)
	typeQueryRes.Nodes = nil

	// build NQuads for changing old type name to new name
	var setNQuads, delNQuads []*api.NQuad
	for _, node := range oldQueryRes.Nodes {
		setNQuads = append(setNQuads, &api.NQuad{
			Subject:   node.Uid,
			Predicate: "dgraph.type",
			ObjectValue: &api.Value{
				Val: &api.Value_StrVal{StrVal: typeNameInfo.newTypeName},
			},
		})
		delNQuads = append(delNQuads, &api.NQuad{
			Subject:   node.Uid,
			Predicate: "dgraph.type",
			ObjectValue: &api.Value{
				Val: &api.Value_StrVal{StrVal: typeNameInfo.oldTypeName},
			},
		})
	}

	// send the mutation to change the old type name to new name
	if len(oldQueryRes.Nodes) > 0 {
		if err := mutateWithClient(dg, &api.Mutation{Set: setNQuads, Del: delNQuads}); err != nil {
			return fmt.Errorf("error upgrading data for old type `%s`: %w",
				typeNameInfo.oldTypeName, err)
		}
	}

	// drop the type from schema if it was not being used for other user-defined nodes
	if len(oldQueryRes.Nodes) == oldTypeCount {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := dg.Alter(ctx, &api.Operation{
			DropOp:    api.Operation_TYPE,
			DropValue: typeNameInfo.oldTypeName,
		}); err != nil {
			return fmt.Errorf("error deleting old type `%s` from schema: %w",
				typeNameInfo.oldTypeName, err)
		}
	}

	return nil
}

// upgradePredicateName changes the name of the given predicate from old name to new name for all
// the nodes that use it, and then deletes the old predicate name from schema via alter.
func upgradePredicateName(dg *dgo.Dgraph, predNameInfo *upgradePredNameInfo) error {
	var query string
	if predNameInfo.isUidPred {
		query = uidPredicateQuery
	} else {
		query = scalarPredicateQuery
	}

	// find out the nodes having old predicate
	var predicateQueryResp predicateQueryResp
	if err := getQueryResult(dg, fmt.Sprintf(query, predNameInfo.oldPredName,
		predNameInfo.oldPredName), &predicateQueryResp); err != nil {
		return fmt.Errorf("error querying old predicate `%s`: %w", predNameInfo.oldPredName, err)
	}

	// nothing to do
	if len(predicateQueryResp.Nodes) == 0 {
		return nil
	}

	// prepare setJson and deleteJson for the upgrade
	var setJson, deleteJson []map[string]interface{}
	for _, setJsonNode := range predicateQueryResp.Nodes {
		deleteJsonNode := copyMap(setJsonNode)

		setJsonNode[predNameInfo.newPredName] = setJsonNode[predNameInfo.oldPredName]
		delete(setJsonNode, predNameInfo.oldPredName)

		deleteJsonNode[predNameInfo.oldPredName] = nil

		setJson = append(setJson, setJsonNode)
		deleteJson = append(deleteJson, deleteJsonNode)
	}

	// marshal the JSONs
	setJsonBytes, err := json.Marshal(setJson)
	if err != nil {
		return fmt.Errorf("error marshalling setJson for old predicate `%s`: %w",
			predNameInfo.oldPredName, err)
	}
	deleteJsonBytes, err := json.Marshal(deleteJson)
	if err != nil {
		return fmt.Errorf("error marshalling deleteJson for old predicate `%s`: %w",
			predNameInfo.oldPredName, err)
	}

	// perform the mutation for upgrade
	err = mutateWithClient(dg, &api.Mutation{SetJson: setJsonBytes, DeleteJson: deleteJsonBytes})
	if err != nil {
		return fmt.Errorf("error upgrading predicate name from `%s` to `%s`: %w",
			predNameInfo.oldPredName, predNameInfo.newPredName, err)
	}

	// now drop the old predicate from schema
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := dg.Alter(ctx, &api.Operation{
		DropOp:    api.Operation_ATTR,
		DropValue: predNameInfo.oldPredName,
	}); err != nil {
		return fmt.Errorf("error deleting old predicate `%s` from schema: %w",
			predNameInfo.oldPredName, err)
	}

	return nil
}
