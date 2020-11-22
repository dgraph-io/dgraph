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
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

// getDgoClient creates a gRPC connection and uses that to create a new dgo client.
// The gRPC.ClientConn returned by this must be closed after use.
func getDgoClient(withLogin bool) (*dgo.Dgraph, *grpc.ClientConn, error) {
	alpha := Upgrade.Conf.GetString(alpha)

	// TODO(Aman): add TLS configuration.
	conn, err := grpc.Dial(alpha, grpc.WithInsecure())
	if err != nil {
		return nil, nil, fmt.Errorf("unable to connect to Dgraph cluster: %w", err)
	}

	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	if withLogin {
		userName := Upgrade.Conf.GetString(user)
		password := Upgrade.Conf.GetString(password)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		// login to cluster
		if err = dg.Login(ctx, userName, password); err != nil {
			x.Check(conn.Close())
			return nil, nil, fmt.Errorf("unable to login to Dgraph cluster: %w", err)
		}
	}

	return dg, conn, nil
}

// getQueryResult executes the given query and unmarshals the result in given pointer queryResPtr.
// If any error is encountered, it returns the error.
func getQueryResult(dg *dgo.Dgraph, query string, queryResPtr interface{}) error {
	var resp *api.Response
	var err error

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err = dg.NewReadOnlyTxn().Query(ctx, query)
		if err != nil {
			fmt.Println("error in query, retrying:", err)
			continue
		}
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(resp.GetJson(), queryResPtr)
}

// mutateWithClient uses the given dgraph client to execute the given mutation.
// It retries max 3 times before returning failure error, if any.
func mutateWithClient(dg *dgo.Dgraph, mutation *api.Mutation) error {
	if mutation == nil {
		return nil
	}

	mutation.CommitNow = true

	var err error
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err = dg.NewTxn().Mutate(ctx, mutation)
		if err != nil {
			fmt.Println("error in mutation, retrying:", err)
			continue
		}

		return nil
	}

	return err
}

// alterWithClient uses the given dgraph client to execute the given alter operation.
// It retries max 3 times before returning failure error, if any.
func alterWithClient(dg *dgo.Dgraph, operation *api.Operation) error {
	if operation == nil {
		return nil
	}

	var err error
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = dg.Alter(ctx, operation)
		if err != nil {
			fmt.Println("error in alter, retrying:", err)
			continue
		}

		return nil
	}

	return err
}

// getTypeSchemaString generates a string which can be used to alter a type in schema.
// It generates the type string using new type and predicate names. So, if this type
// previously had a predicate for which we got a new name, then the generated string
// will contain the new name for that predicate. Also, if some predicates need to be
// removed from the type, then they can be supplied in predsToRemove. For example:
// initialType:
// 	type Person {
// 		name
// 		age
// 		unnecessaryEdge
// 	}
// also,
// 	newTypeName = "Human"
// 	newPredNames = {
// 		"age": "ageOnEarth'
// 	}
// 	predsToRemove = {
// 		"unnecessaryEdge": {}
// 	}
// then returned type string will be:
// 	type Human {
// 		name
// 		ageOnEarth
// 	}
func getTypeSchemaString(newTypeName string, typeNode *schemaTypeNode,
	newPredNames map[string]string, predsToRemove map[string]struct{}) string {
	var builder strings.Builder
	builder.WriteString("type ")
	builder.WriteString(newTypeName)
	builder.WriteString(" {\n")

	for _, oldPred := range typeNode.Fields {
		if _, ok := predsToRemove[oldPred.Name]; ok {
			continue
		}

		builder.WriteString("  ")
		newPredName, ok := newPredNames[oldPred.Name]
		if ok {
			builder.WriteString(newPredName)
		} else {
			builder.WriteString(oldPred.Name)
		}
		builder.WriteString("\n")
	}

	builder.WriteString("}\n")

	return builder.String()
}

func getTypeNquad(uid, typeName string) *api.NQuad {
	return &api.NQuad{
		Subject:   uid,
		Predicate: "dgraph.type",
		ObjectValue: &api.Value{
			Val: &api.Value_StrVal{StrVal: typeName},
		},
	}
}
