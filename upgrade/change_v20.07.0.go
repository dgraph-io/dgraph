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

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

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
	schemaQuery = `schema{}`
)

type node struct {
	Uid string `json:"uid"`
}

type nodeQueryResp struct {
	Nodes []node `json:"nodes"`
}

type typeNode struct {
	Name   string `json:"name"`
	Fields []*struct {
		Name string `json:"name"`
	} `json:"fields"`
}

type schemaQueryResp struct {
	Schema []*pb.SchemaNode `json:"schema"`
	Types  []*typeNode      `json:"types"`
}

type upgradeTypeNameInfo struct {
	oldTypeName string
	newTypeName string
	oldQuery    string
}

func getQueryResult(dg *dgo.Dgraph, query string) (*nodeQueryResp, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := dg.NewReadOnlyTxn().Query(ctx, query)
	if err != nil {
		return nil, err
	}

	var queryRes nodeQueryResp
	if err = json.Unmarshal(resp.GetJson(), &queryRes); err != nil {
		return nil, err
	}

	return &queryRes, nil
}

func getSchemaQueryResult(dg *dgo.Dgraph) (*schemaQueryResp, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := dg.NewReadOnlyTxn().Query(ctx, schemaQuery)
	if err != nil {
		return nil, err
	}

	var queryRes schemaQueryResp
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
		err = mutateWithClient(dg, &api.Mutation{Set: setNQuads, Del: delNQuads})
		if err != nil {
			return fmt.Errorf("error upgrading data for old ACL type %s: %w",
				typeNameInfo.oldTypeName, err)
		}
	}

	// remove the type from schema if it was not being used for other user-defined nodes
	if len(typeQueryRes.Nodes) == len(oldQueryRes.Nodes) {
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
			return fmt.Errorf("error upgrading ACL type name from %s to %s: %w",
				typeNameInfo.oldTypeName, typeNameInfo.newTypeName, err)
		}
	}

	return nil
}

func upgradeNonPredefinedNamesInReservedNamespace() error {
	dg, conn, err := getDgoClient(false)
	if err != nil {
		return fmt.Errorf("error getting dgo client: %w", err)
	}
	defer conn.Close()

	schemaQueryResp, err := getSchemaQueryResult(dg)
	if err != nil {
		return fmt.Errorf("unable to query schema: %w", err)
	}

	// collect predicates to change
	predicatesToChange := make(map[string]*pb.SchemaNode)
	for _, schemaNode := range schemaQueryResp.Schema {
		if x.IsReservedPredicate(schemaNode.Predicate) && !x.IsPreDefinedPredicate(schemaNode.
			Predicate) {
			predicatesToChange[schemaNode.Predicate] = schemaNode
		}
	}

	// collect types to change
	typesToChange := make(map[string]*typeNode)
	for _, typeNode := range schemaQueryResp.Types {
		if x.IsReservedType(typeNode.Name) && !x.IsPreDefinedType(typeNode.Name) {
			typesToChange[typeNode.Name] = typeNode
		}
	}

	// return if no change is required
	if len(predicatesToChange) == 0 && len(typesToChange) == 0 {
		return nil
	}

	// ask user for new predicate names
	newPredicateNames := make(map[string]string)
	if len(predicatesToChange) > 0 {
		fmt.Println("Please provide new names for predicates.")
		for oldPredName, _ := range predicatesToChange {
			newName, err := askUserForNewName(oldPredName, x.IsReservedPredicate)
			if err != nil {
				return err
			}
			newPredicateNames[oldPredName] = newName
		}
	}

	// ask user for new type names
	newTypeNames := make(map[string]string)
	if len(typesToChange) > 0 {
		fmt.Println("Please provide new names for types.")
		for oldTypeName, _ := range typesToChange {
			newName, err := askUserForNewName(oldTypeName, x.IsReservedType)
			if err != nil {
				return err
			}
			newTypeNames[oldTypeName] = newName
		}
	}

	// build the alter operations to execute
	var newSchemaBuilder strings.Builder
	dropOperations := make([]*api.Operation, 0, len(predicatesToChange)+len(typesToChange))
	for oldPredName, predDef := range predicatesToChange {
		newSchemaBuilder.WriteString(getPredSchemaString(newPredicateNames[oldPredName], predDef))
		dropOperations = append(dropOperations, &api.Operation{
			DropOp:    api.Operation_ATTR,
			DropValue: oldPredName,
		})
	}
	for oldTypeName, typeDef := range typesToChange {
		newSchemaBuilder.WriteString(getTypeSchemaString(newTypeNames[oldTypeName], typeDef,
			newPredicateNames))
		dropOperations = append(dropOperations, &api.Operation{
			DropOp:    api.Operation_TYPE,
			DropValue: oldTypeName,
		})
	}

	return nil
}

var reservedNameError = fmt.Errorf("new name can't start with `dgraph.`, please try again! ")

func askUserForNewName(oldName string, checkReservedFunc func(string) bool) (string, error) {
	var newName string
	var err error

	for i := 0; i < 3; i++ {
		fmt.Printf("Enter new name for `%s`: ", oldName)
		if _, err = fmt.Scan(&newName); err != nil {
			fmt.Println("Something went wrong while scanning input: ", err)
			fmt.Println("Try again!")
			continue
		}
		if checkReservedFunc(newName) {
			err = reservedNameError
			fmt.Println(err)
			continue
		}
		break
	}

	return newName, err
}

func getPredSchemaString(newPredName string, schemaNode *pb.SchemaNode) string {
	var builder strings.Builder
	builder.WriteString(newPredName)
	builder.WriteString(": ")

	if schemaNode.List {
		builder.WriteString("[")
	}
	builder.WriteString(schemaNode.Type)
	if schemaNode.List {
		builder.WriteString("]")
	}
	builder.WriteString(" ")

	if schemaNode.Count {
		builder.WriteString("@count ")
	}
	if schemaNode.Index {
		builder.WriteString("@index(")
		comma := ""
		for _, tokenizer := range schemaNode.Tokenizer {
			builder.WriteString(comma)
			builder.WriteString(tokenizer)
			comma = ", "
		}
		builder.WriteString(") ")
	}
	if schemaNode.Lang {
		builder.WriteString("@lang ")
	}
	if schemaNode.NoConflict {
		builder.WriteString("@noconflict ")
	}
	if schemaNode.Reverse {
		builder.WriteString("@reverse ")
	}
	if schemaNode.Upsert {
		builder.WriteString("@upsert ")
	}

	builder.WriteString(".\n")

	return builder.String()
}

func getTypeSchemaString(newTypeName string, typeNode *typeNode,
	newPredNames map[string]string) string {
	var builder strings.Builder
	builder.WriteString("type ")
	builder.WriteString(newTypeName)
	builder.WriteString(" {\n")

	for _, oldPred := range typeNode.Fields {
		builder.WriteString("  ")
		newPredName, ok := newPredNames[oldPred.Name]
		if ok {
			builder.WriteString(newPredName)
		} else {
			builder.WriteString(oldPred.Name)
		}
		builder.WriteString("\n")
	}

	builder.WriteString("}")

	return builder.String()
}
