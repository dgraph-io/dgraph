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

// Package graphql is a http server for GraphQL on Dgraph
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
	"fmt"
	"net/http"
	"os"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/admin"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/resolve"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/web"

	"github.com/dgraph-io/dgraph/x"
	"go.opencensus.io/trace"
	"go.opencensus.io/zpages"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	_ "github.com/vektah/gqlparser/validator/rules" // make gql validator init() all rules

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
)

var GraphQL x.SubCommand

func init() {
	GraphQL.Cmd = &cobra.Command{
		Use:   "graphql",
		Short: "Run the Dgraph GraphQL API",
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
	flags.IntP("port", "p", 9000, "Port on which to run the HTTP service")
	flags.Bool("introspection", true, "Set to false for no GraphQL schema introspection")

	// OpenCensus flags.
	flags.Float64("trace", 1.0, "The ratio of queries to trace.")
	flags.String("jaeger.collector", "", "Send opencensus traces to Jaeger.")

	// TLS configuration
	x.RegisterClientTLSFlags(flags)
}

func run() error {
	x.PrintVersion()

	bindall := GraphQL.Conf.GetBool("bindall")
	port := GraphQL.Conf.GetInt("port")
	introspection := GraphQL.Conf.GetBool("introspection")
	tlsCfg, err := x.LoadClientTLSConfig(GraphQL.Conf)
	if err != nil {
		return err
	}

	settings := &admin.ConnectionSettings{
		Alphas:         GraphQL.Conf.GetString("alpha"),
		TlScfg:         tlsCfg,
		UseCompression: false,
	}

	glog.Infof("Starting GraphQL with Dgraph at: %s", settings.Alphas)

	gqlSchema, err := schema.FromString("")
	if err != nil {
		return err
	}
	resolvers := resolve.New(gqlSchema, resolve.NewResolverFactory())
	mainServer := web.NewServer(resolvers)

	fns := &resolve.ResolverFns{
		Qrw: resolve.NewQueryRewriter(),
		Mrw: resolve.NewMutationRewriter(),
		Drw: resolve.NewDeleteRewriter(),
	}
	adminResolvers := admin.NewAdminResolver(settings, mainServer, fns, introspection)
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

	bind := "localhost"
	if bindall {
		bind = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", bind, port)

	glog.Infof("Bringing up GraphQL HTTP API at %s/graphql", addr)
	glog.Infof("Bringing up GraphQL HTTP admin API at %s/admin", addr)
	return errors.Wrap(http.ListenAndServe(addr, nil), "GraphQL server failed")
}
