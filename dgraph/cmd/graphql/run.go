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
	"log"
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

	gschema "github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
)

type options struct {
	schemaFile     string
	alpha          string
	useCompression bool
}

type gqlSchema struct {
	Type   string    `json:"dgraph.type,omitempty"`
	Schema string    `json:"dgraph.graphql.schema,omitempty"`
	Date   time.Time `json:"dgraph.graphql.date,omitempty"`
}

type gqlSchemas struct {
	Schemas []gqlSchema `json:"gqlSchemas,omitempty"`
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

	// TODO: ones passed to sub commands should be persistent flags
	flags := GraphQL.Cmd.Flags()
	flags.StringP("alpha", "a", "127.0.0.1:9080",
		"Comma-separated list of Dgraph alpha gRPC server addresses")
	flags.StringP("schema", "s", "schema.graphql",
		"Location of GraphQL schema file")

	// TLS configuration
	x.RegisterClientTLSFlags(flags)

	var cmdInit x.SubCommand
	cmdInit.Cmd = &cobra.Command{
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
	GraphQL.Cmd.AddCommand(cmdInit.Cmd)

	cmdInit.Cmd.Flags().AddFlag(GraphQL.Cmd.Flag("alpha"))
	cmdInit.Cmd.Flags().AddFlag(GraphQL.Cmd.Flag("schema"))
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
		gqlSchemas(func: type(dgraph.graphql)) {
			dgraph.graphql.schema
			dgraph.graphql.date
		}
	}`

	ctx := context.Background()
	resp, err := dgraphClient.NewTxn().Query(ctx, q)
	if err != nil {
		return errors.Wrap(err, "while querying GraphQL schema from Dgraph")
	}

	var schemas gqlSchemas
	err = json.Unmarshal(resp.Json, &schemas)
	if err != nil {
		return errors.Wrap(err, "while reading GraphQL schema")
	}

	if len(schemas.Schemas) < 1 {
		return fmt.Errorf("No GraphQL schema was found")
	}

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: string(schemas.Schemas[0].Schema)})
	if gqlErr != nil {
		return errors.Wrap(gqlErr, "while parsing GraphQL schema")
	}

	if gqlErrList := gschema.ValidateSchema(doc); gqlErrList != nil {
		var errStr strings.Builder
		for _, err := range gqlErrList {
			errStr.WriteString(err.Message + "\n")
		}

		log.Fatalf(errStr.String())
	}

	gschema.AddScalars(doc)

	schema, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return errors.Wrap(gqlErr, "while validating GraphQL schema")
	}

	gschema.GenerateCompleteSchema(schema)

	fmt.Println(gschema.Stringify(schema))

	handler := &graphqlHandler{
		dgraphClient: dgraphClient,
		schema:       schema,
	}

	http.Handle("/graphql", handler)

	// TODO:
	// the ports and urls etc that the endpoint serves should be input options
	return errors.Wrap(http.ListenAndServe(":9000", nil), "GraphQL server failed")
}

func initDgraph() error {
	x.PrintVersion()
	opt = options{
		schemaFile: GraphQL.Conf.GetString("schema"),
		alpha:      GraphQL.Conf.GetString("alpha"),
	}

	fmt.Printf("Processing schema file %q\n", opt.schemaFile)

	b, err := ioutil.ReadFile(opt.schemaFile)
	if err != nil {
		return err
	}
	inputSchema := string(b)

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: inputSchema})
	if gqlErr != nil {
		return fmt.Errorf("cannot parse schema %s", gqlErr)
	}

	if gqlErrList := gschema.ValidateSchema(doc); gqlErrList != nil {
		return gqlErrList
	}

	gschema.AddScalars(doc)

	schema, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return fmt.Errorf("GraphQL schema is invalid %s", gqlErr)
	}

	gschema.GenerateCompleteSchema(schema)
	completeSchema := gschema.Stringify(schema)
	if glog.V(2) {
		fmt.Printf("Built GraphQL schema:\n\n%s\n", completeSchema)
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

			var typeDef, preds strings.Builder
			fmt.Fprintf(&typeDef, "type %s {\n", def.Name)
			for _, f := range def.Fields {
				if f.Type.Name() == "ID" {
					continue
				}

				switch schema.Types[f.Type.Name()].Kind {
				case ast.Object:
					// TODO: still need to write [] ! and reverse in here
					fmt.Fprintf(&typeDef, "  %s.%s: uid\n", def.Name, f.Name)
					fmt.Fprintf(&preds, "%s.%s: uid .\n", def.Name, f.Name)
				case ast.Scalar:
					// TODO: indexes needed here
					fmt.Fprintf(&typeDef, "  %s.%s: %s\n",
						def.Name, f.Name, strings.ToLower(f.Type.Name()))
					fmt.Fprintf(&preds, "%s.%s: %s .\n",
						def.Name, f.Name, strings.ToLower(f.Type.Name()))
				case ast.Enum:
					fmt.Fprintf(&typeDef, "  %s.%s: string\n", def.Name, f.Name)
					fmt.Fprintf(&preds, "%s.%s: string @index(exact) .\n", def.Name, f.Name)
				}
			}
			fmt.Fprintf(&typeDef, "}\n")

			fmt.Fprintf(&schemaB, "%s%s\n", typeDef.String(), preds.String())

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

	if glog.V(2) {
		fmt.Printf("Built Dgraph schema:\n\n%s\n", dgSchema)
	}

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

	s := gqlSchema{
		Type:   "dgraph.graphql",
		Schema: completeSchema,
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
	if _, err = dgraphClient.NewTxn().Mutate(ctx, mu); err != nil {
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
