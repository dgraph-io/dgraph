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
			run()
		},
	}

	GraphQL.EnvPrefix = "DGRAPH_GRAPHQL"

	flags := GraphQL.Cmd.Flags()
	flags.StringP("alpha", "a", "127.0.0.1:9080",
		"Comma-separated list of Dgraph alpha gRPC server addresses")
	flags.BoolP("use_compression", "C", false,
		"Enable compression on connection to alpha server")
	flags.StringP("schema", "s", "schema.graphql",
		"Location of GraphQL schema file")

	cmdInit := &cobra.Command{
		Use:   "init",
		Short: "Initializes Dgraph for a GraphQL schema",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(GraphQL.Conf).Stop()
			if err := initDgraph(); err != nil {
				fmt.Printf("Error : %s\n", err)
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

func run() {
	x.PrintVersion()
	opt = options{
		schemaFile:     GraphQL.Conf.GetString("schema"),
		alpha:          GraphQL.Conf.GetString("alpha"),
		useCompression: GraphQL.Conf.GetBool("use_compression"),
	}

	glog.Infof("Bringing up GraphQL API for Dgraph at %s\n", opt.alpha)

	dgraphClient, disconnect := connect()
	defer disconnect()

	q := `query {
		graphqlSchemas(func: type(dgraph.graphql.schema)) {
			dgraph.graphql.schema.schema
			dgraph.graphql.schema.date
		}
	}`

	ctx := context.Background()
	resp, err := dgraphClient.NewTxn().Query(ctx, q)
	x.Checkf(err, "Error querying GraphQL schema from Dgraph")

	var schemas graphqlSchemas
	err = json.Unmarshal(resp.Json, &schemas)
	x.Checkf(err, "Error reading GraphQL schema")

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: string(schemas.Schemas[0].Schema)})
	if gqlErr != nil {
		x.Checkf(gqlErr, "Error parsing GraphQL schema")
	}

	addScalars(doc)

	schema, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		x.Checkf(gqlErr, "Error validating GraphQL schema")
	}

	gschema.GenerateCompleteSchema(schema)
	fmt.Println(fullSchmea)

	handler := &graphqlHandler{
		dgraphClient: dgraphClient,
		schema:       schema,
	}

	http.Handle("/graphql", handler)

	// the ports and urls etc that the endpoint serves should be input options
	glog.Fatal(http.ListenAndServe(":8765", nil))
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
		return fmt.Errorf("schema is invalid %s", gqlErr)
	}

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
					// still need to write [] ! and reverse in here
					fmt.Fprintf(&ty, "  %s.%s: uid\n", def.Name, f.Name)
					fmt.Fprintf(&preds, "%s.%s: uid .\n", def.Name, f.Name)
				case ast.Scalar:
					// indexes needed here
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
	fmt.Printf("Built Dgraph schema:\n\n%s\n", dgSchema)

	fmt.Printf("Loading schema into Dgraph at %q\n", opt.alpha)
	dgraphClient, disconnect := connect()
	defer disconnect()

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
		return fmt.Errorf("failed to write Dgraph schema %s", err)
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
		return fmt.Errorf("couldn't generate mutation to save schema %s", err)
	}

	mu.SetJson = pb
	_, err = dgraphClient.NewTxn().Mutate(ctx, mu)
	if err != nil {
		return fmt.Errorf("failed to save GraphQL schema to Dgraph %s", err)
		// But this would mean we are in an inconsistent state,
		// so would that mean they just had to re-apply the same schema?
	}

	return nil
}

func connect() (*dgo.Dgraph, func()) {
	tlsCfg, err := x.LoadClientTLSConfig(GraphQL.Conf)
	x.Checkf(err, "While trying to load tls config")

	var clients []api.DgraphClient
	disconnect := func() {}

	ds := strings.Split(opt.alpha, ",")
	for _, d := range ds {
		conn, err := x.SetupConnection(d, tlsCfg, opt.useCompression)
		x.Checkf(err, "While trying to setup connection to Dgraph alpha %v", ds)
		disconnect = func(c func()) func() {
			return func() { c(); conn.Close() }
		}(disconnect)

		dc := api.NewDgraphClient(conn)
		clients = append(clients, dc)
	}

	return dgo.NewDgraphClient(clients...), disconnect
}
