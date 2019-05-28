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
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
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

	fmt.Printf("Bringing up GraphQL API\n")
	fmt.Printf("...Go make a cup of tea while I implement this\n")
}

func initDgraph() error {
	x.PrintVersion()
	opt = options{
		schemaFile: GraphQL.Conf.GetString("schema"),
		alpha:      GraphQL.Conf.GetString("alpha"),
	}

	fmt.Printf("\nProcessing schema file %q\n", opt.schemaFile)

	f, err := os.Open(opt.schemaFile)
	x.Checkf(err, "Unable to open schema file")
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	x.Checkf(err, "Error while reading schema file")

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: string(b)})
	if gqlErr != nil {
		x.Checkf(gqlErr, "Error parsing schema file")
	}

	schema, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		x.Checkf(gqlErr, "Error validating schema")
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

	// check the current schema, is it compatible with these additions?

	op := &api.Operation{}
	op.Schema = dgSchema

	ctx := context.Background()
	// plus auth token like live?
	err = dgraphClient.Alter(ctx, op)
	x.Checkf(err, "Error while writing schema to Dgraph")

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
