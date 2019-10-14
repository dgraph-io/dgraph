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

// Pacakge graphql is a http server for GraphQL on Dgraph
//
// GraphQL spec:
// https://graphql.github.io/graphql-spec/June2018
//
//
// GraphQL servers should serve both GET and POST
// https://graphql.org/learn/serving-over-http/
//
// GET should be like
// http://myapi/graphql?query={me{name}}
//
// POST should have a json content body like
// {
//   "query": "...",
//   "operationName": "...",
//   "variables": { "myVariable": "someValue", ... }
// }
//
// GraphQL servers should return 200 (even on errors),
// and result body should be json:
// {
//   "data": { "query_name" : { ... } },
//   "errors": [ { "message" : ..., ...} ... ]
// }
//
// Key points about the response
// (https://graphql.github.io/graphql-spec/June2018/#sec-Response)
//
// - If an error was encountered before execution begins,
//   the data entry should not be present in the result.
//
// - If an error was encountered during the execution that
//   prevented a valid response, the data entry in the response should be null.
//
// - If there's errors and data, both are returned
//
// - If no errors were encountered during the requested operation,
//   the errors entry should not be present in the result.
//
// - There's rules around how errors work when there's ! fields in the schema
//   https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability
//
// - The "message" in an error is required, the rest is up to the implementation
//
// - The "data" works just like a Dgraph query
//
// - "extensions" is allowed and can be anything
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

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/resolve"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/web"

	"github.com/dgraph-io/dgo"
	dgoapi "github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"go.opencensus.io/trace"
	"go.opencensus.io/zpages"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
	_ "github.com/vektah/gqlparser/validator/rules" // make gql validator init() all rules

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
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

	// OpenCensus flags.
	flags.Float64("trace", 1.0, "The ratio of queries to trace.")
	flags.String("jaeger.collector", "", "Send opencensus traces to Jaeger.")

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
	cmdInit.Cmd.Flags().AddFlag(GraphQL.Cmd.Flag("jaeger.collector"))
}

func run() error {
	x.PrintVersion()
	opt = options{
		schemaFile: GraphQL.Conf.GetString("schema"),
		alpha:      GraphQL.Conf.GetString("alpha"),
	}

	glog.Infof("Connecting to Dgraph at: %s", opt.alpha)

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

	// validator.Prelude includes a bunch of predefined types which help with schema introspection
	// queries, hence we include it as part of the schema.
	doc, gqlErr := parser.ParseSchemas(validator.Prelude,
		&ast.Source{Input: string(schemas.Schemas[0].Schema)})
	if gqlErr != nil {
		return errors.Wrap(gqlErr, "while parsing GraphQL schema")
	}

	gqlSchema, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return errors.Wrap(gqlErr, "while validating GraphQL schema")
	}

	queryRewriter := resolve.NewQueryRewriter()
	mutationRewriter := resolve.NewMutationRewriter()
	queryExecutor := resolve.DgoAsQueryExecutor(dgraphClient)
	mutationExecutor := resolve.DgoAsMutationExecutor(dgraphClient)

	resolverFactory :=
		resolve.NewResolverFactory(
			queryRewriter,
			resolve.NewMutationRewriter(),
			queryExecutor,
			mutationExecutor)

	// We don't have to add schema introspection here.  Often GraphQL servers turn
	// off schema introspection in production.  So this can easily be optional.
	resolverFactory.WithSchemaIntrospection()

	resolvers := resolve.New(schema.AsSchema(gqlSchema), resolverFactory)
	mainServer := web.NewServer(resolvers)

	adminResolvers :=
		NewAdminResolver(
			dgraphClient,
			mainServer,
			queryRewriter,
			mutationRewriter,
			queryExecutor,
			mutationExecutor)
	adminServer := web.NewServer(adminResolvers)

	http.Handle("/graphql", mainServer.HTTPHandler())
	http.Handle("/admin", adminServer.HTTPHandler())

	trace.ApplyConfig(trace.Config{
		DefaultSampler:             trace.ProbabilitySampler(GraphQL.Conf.GetFloat64("trace")),
		MaxAnnotationEventsPerSpan: 256,
	})
	x.RegisterExporters(GraphQL.Conf, "dgraph.graphql")

	// Add OpenCensus z-pages.
	zpages.Handle(http.DefaultServeMux, "/z")

	// TODO:
	// the ports and urls etc that the endpoint serves should be input options
	glog.Infof("Bringing up GraphQL HTTP API at 127.0.0.1:9000/graphql")
	glog.Infof("Bringing up GraphQL HTTP admin API at 127.0.0.1:9000/admin")
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

	schHandler, err := schema.NewHandler(inputSchema)
	if err != nil {
		return err
	}

	completeSchema := schHandler.GQLSchema()
	glog.V(2).Infof("Built GraphQL schema:\n\n%s\n", completeSchema)

	dgSchema := schHandler.DGSchema()

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

	op := &dgoapi.Operation{}
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

	mu := &dgoapi.Mutation{
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
	var clients []dgoapi.DgraphClient
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

		clients = append(clients, dgoapi.NewDgraphClient(conn))
	}

	return dgo.NewDgraphClient(clients...), disconnect, nil
}
