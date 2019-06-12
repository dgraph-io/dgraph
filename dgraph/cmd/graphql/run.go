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

package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
	_ "github.com/vektah/gqlparser/validator/rules" // make gql validator init() all rules
)

type options struct {
	schemaFile     string
	alpha          string
	useCompression bool
}

type graphqlSchema struct {
	Type   string    `json:"dgraph.type,omitempty"`
	Schema string    `json:"dgraph.graphql.schema.schema,omitempty"`
	Date   time.Time `json:"dgraph.graphql.schema.date,omitempty"`
}

type graphqlSchemas struct {
	Schemas []graphqlSchema `json:"graphqlSchemas,omitempty"`
}

var opt options

var GraphQL x.SubCommand

func init() {
	GraphQL.Cmd = &cobra.Command{
		Use:   "graphql",
		Short: "Run the Dgraph GraphQL tool",
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(GraphQL.Conf).Stop()
			if err := run(); err != nil {
				if glog.V(2) {
					fmt.Printf("Error : %+v\n", err)
				} else {
					fmt.Printf("Error : %s\n", err)
				}
				os.Exit(1)
			}
		},
	}

	GraphQL.EnvPrefix = "DGRAPH_GRAPHQL"

	flags := GraphQL.Cmd.Flags()
	flags.StringP("alpha", "a", "127.0.0.1:9080",
		"Comma-separated list of Dgraph alpha gRPC server addresses")
	flags.StringP("schema", "s", "schema.graphql",
		"Location of GraphQL schema file")

	cmdInit := &cobra.Command{
		Use:   "init",
		Short: "Initializes Dgraph for a GraphQL schema",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(GraphQL.Conf).Stop()
			if err := initDgraph(); err != nil {
				if glog.V(2) {
					fmt.Printf("Error : %+v\n", err)
				} else {
					fmt.Printf("Error : %s\n", err)
				}
				os.Exit(1)
			}
		},
	}
	cmdInit.Flags().AddFlag(GraphQL.Cmd.Flag("alpha"))
	cmdInit.Flags().AddFlag(GraphQL.Cmd.Flag("schema"))
	GraphQL.Cmd.AddCommand(cmdInit)

	// TLS configuration
	x.RegisterClientTLSFlags(flags)
}

func run() error {
	x.PrintVersion()
	opt = options{
		schemaFile: GraphQL.Conf.GetString("schema"),
		alpha:      GraphQL.Conf.GetString("alpha"),
	}

	glog.Infof("Bringing up GraphQL API for Dgraph at %s\n", opt.alpha)

	dgraphClient, disconnect, err := connect()
	if err != nil {
		return err
	}
	defer disconnect()

	q := `query {
		graphqlSchemas(func: type(dgraph.graphql.schema)) {
			dgraph.graphql.schema.schema
			dgraph.graphql.schema.date
		}
	}`

	ctx := context.Background()
	resp, err := dgraphClient.NewTxn().Query(ctx, q)
	if err != nil {
		return errors.Wrap(err, "while querying GraphQL schema from Dgraph")
	}

	var schemas graphqlSchemas
	err = json.Unmarshal(resp.Json, &schemas)
	if err != nil {
		return errors.Wrap(err, "while reading GraphQL schema")
	}

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: string(schemas.Schemas[0].Schema)})
	if gqlErr != nil {
		return errors.Wrap(gqlErr, "while parsing GraphQL schema")
	}

	schema, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return errors.Wrap(gqlErr, "while validating GraphQL schema")
	}

	handler := &graphqlHTTPHandler{
		dgraphClient: dgraphClient,
		schema:       schema,
	}

	http.Handle("/graphql", handler)

	// the ports and urls etc that the endpoint serves should be input options
	return errors.Wrap(http.ListenAndServe(":8765", nil), "GraphQL server failed")
}

func initDgraph() error {
	x.PrintVersion()
	opt = options{
		schemaFile: GraphQL.Conf.GetString("schema"),
		alpha:      GraphQL.Conf.GetString("alpha"),
	}

	fmt.Printf("Processing schema file %q\n", opt.schemaFile)

	f, err := os.Open(opt.schemaFile)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	gqlSchema := string(b)

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: gqlSchema})
	if gqlErr != nil {
		return fmt.Errorf("cannot parse schema %s", gqlErr)
	}

	schema, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return fmt.Errorf("GraphQL schema is invalid %s", gqlErr)
	}

	// TODO: extract out as todo's below are done
	var schemaB strings.Builder
	for _, def := range schema.Types {
		switch def.Kind {
		case ast.Object:
			if def.Name == "Query" ||
				def.Name == "Mutation" ||
				((strings.HasPrefix(def.Name, "Add") ||
					strings.HasPrefix(def.Name, "Delete") ||
					strings.HasPrefix(def.Name, "Mutate")) &&
					strings.HasSuffix(def.Name, "Payload")) {
				continue
			}

			var ty, preds strings.Builder
			fmt.Fprintf(&ty, "type %s {\n", def.Name)
			for _, f := range def.Fields {
				if f.Type.Name() == "ID" {
					continue
				}

				switch schema.Types[f.Type.Name()].Kind {
				case ast.Object:
					// TODO: still need to write [] ! and reverse in here
					fmt.Fprintf(&ty, "  %s.%s: uid\n", def.Name, f.Name)
					fmt.Fprintf(&preds, "%s.%s: uid .\n", def.Name, f.Name)
				case ast.Scalar:
					// TODO: indexes needed here
					fmt.Fprintf(&ty, "  %s.%s: %s\n", def.Name, f.Name, strings.ToLower(f.Type.Name()))
					fmt.Fprintf(&preds, "%s.%s: %s .\n", def.Name, f.Name, strings.ToLower(f.Type.Name()))
				case ast.Enum:
					fmt.Fprintf(&ty, "  %s.%s: string\n", def.Name, f.Name)
					fmt.Fprintf(&preds, "%s.%s: string @index(exact) .\n", def.Name, f.Name)
				}
			}
			fmt.Fprintf(&ty, "}\n")

			fmt.Fprintf(&schemaB, "%s%s\n", ty.String(), preds.String())

		case ast.Scalar:
			// nothing to do here?  There should only be known scalars, and that
			// should have been checked by the validation.
			// fmt.Printf("Got a scalar: %v %v\n", name, def)
		case ast.Enum:
			// ignore this? it's handled by the edges
			// fmt.Printf("Got an enum: %v %v\n", name, def)
		default:
			// ignore anything else?
			// fmt.Printf("Got something else: %v %v\n", name, def)
		}
	}

	dgSchema := schemaB.String()
	glog.V(2).Infof("Built Dgraph schema:\n\n%s\n", dgSchema)

	fmt.Printf("Loading schema into Dgraph at %q\n", opt.alpha)
	dgraphClient, disconnect, err := connect()
	if err != nil {
		return err
	}
	defer disconnect()

	// TODO:
	// check the current Dgraph schema, is it compatible with these additions.
	// also need to check against any stored GraphQL schemas?
	// - e.g. moving no ! to !, or redefining a type, or changing a relation's type
	// ... need to allow for schema migrations, but probably should have some checks

	op := &api.Operation{}
	op.Schema = dgSchema

	ctx := context.Background()
	// plus auth token like live?
	err = dgraphClient.Alter(ctx, op)
	if err != nil {
		return errors.Wrap(err, "failed to write Dgraph schema")
	}

	s := graphqlSchema{
		Type:   "dgraph.graphql.schema",
		Schema: gqlSchema,
		Date:   time.Now(),
	}

	mu := &api.Mutation{
		CommitNow: true,
	}
	pb, err := json.Marshal(s)
	if err != nil {
		return errors.Wrap(err, "couldn't generate mutation to save schema")
	}

	mu.SetJson = pb
	_, err = dgraphClient.NewTxn().Mutate(ctx, mu)
	if err != nil {
		return errors.Wrap(err, "failed to save GraphQL schema to Dgraph")
		// But this would mean we are in an inconsistent state,
		// so would that mean they just had to re-apply the same schema?
	}

	return nil
}

func connect() (*dgo.Dgraph, func(), error) {
	var clients []api.DgraphClient
	disconnect := func() {}

	tlsCfg, err := x.LoadClientTLSConfig(GraphQL.Conf)
	if err != nil {
		return nil, nil, err
	}

	ds := strings.Split(opt.alpha, ",")
	for _, d := range ds {
		conn, err := x.SetupConnection(d, tlsCfg, opt.useCompression)
		if err != nil {
			disconnect()
			return nil, nil, fmt.Errorf("couldn't connect to %s, %s", d, err)
		}
		disconnect = func(dis func()) func() {
			return func() { dis(); conn.Close() }
		}(disconnect)

		clients = append(clients, api.NewDgraphClient(conn))
	}

	return dgo.NewDgraphClient(clients...), disconnect, nil
}
