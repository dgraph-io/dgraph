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
	"fmt"
	"os"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
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
	fmt.Printf("Loading into Dgraph at %q\n", opt.alpha)

	fmt.Printf("...Go make a cup of tea while I implement this\n")

	return nil
}
