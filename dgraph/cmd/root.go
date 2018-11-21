/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package cmd

import (
	goflag "flag"
	"fmt"
	"os"

	"github.com/dgraph-io/dgraph/ee/acl/cmd"

	"github.com/dgraph-io/dgraph/dgraph/cmd/alpha"
	"github.com/dgraph-io/dgraph/dgraph/cmd/bulk"
	"github.com/dgraph-io/dgraph/dgraph/cmd/cert"
	"github.com/dgraph-io/dgraph/dgraph/cmd/conv"
	"github.com/dgraph-io/dgraph/dgraph/cmd/debug"
	"github.com/dgraph-io/dgraph/dgraph/cmd/live"
	"github.com/dgraph-io/dgraph/dgraph/cmd/version"
	"github.com/dgraph-io/dgraph/dgraph/cmd/zero"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "dgraph",
	Short: "Dgraph: Distributed Graph Database",
	Long: `
Dgraph is a horizontally scalable and distributed graph database,
providing ACID transactions, consistent replication and linearizable reads.
It's built from ground up to perform for a rich set of queries. Being a native
graph database, it tightly controls how the data is arranged on disk to optimize
for query performance and throughput, reducing disk seeks and network calls in a
cluster.
` + x.BuildDetails(),
	PersistentPreRunE: cobra.NoArgs,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	goflag.Parse()
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var rootConf = viper.New()

func init() {
	RootCmd.PersistentFlags().String("profile_mode", "",
		"Enable profiling mode, one of [cpu, mem, mutex, block]")
	RootCmd.PersistentFlags().Int("block_rate", 0,
		"Block profiling rate. Must be used along with block profile_mode")
	RootCmd.PersistentFlags().String("config", "",
		"Configuration file. Takes precedence over default values, but is "+
			"overridden to values set with environment variables and flags.")
	RootCmd.PersistentFlags().Bool("bindall", true,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	RootCmd.PersistentFlags().Bool("expose_trace", false,
		"Allow trace endpoint to be accessible from remote")
	rootConf.BindPFlags(RootCmd.PersistentFlags())

	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	// Always set stderrthreshold=0. Don't let users set it themselves.
	x.Check(flag.Set("stderrthreshold", "0"))
	x.Check(flag.CommandLine.MarkDeprecated("stderrthreshold",
		"Dgraph always sets this flag to 0. It can't be overwritten."))

	var subcommands = []*x.SubCommand{
		&bulk.Bulk, &cert.Cert, &conv.Conv, &live.Live, &alpha.Alpha, &zero.Zero,
		&version.Version, &debug.Debug, &acl.Acl,
	}
	for _, sc := range subcommands {
		RootCmd.AddCommand(sc.Cmd)
		sc.Conf = viper.New()
		sc.Conf.BindPFlags(sc.Cmd.Flags())
		sc.Conf.BindPFlags(RootCmd.PersistentFlags())
		sc.Conf.AutomaticEnv()
		sc.Conf.SetEnvPrefix(sc.EnvPrefix)
	}
	cobra.OnInitialize(func() {
		cfg := rootConf.GetString("config")
		if cfg == "" {
			return
		}
		for _, sc := range subcommands {
			sc.Conf.SetConfigFile(cfg)
			x.Check(x.Wrapf(sc.Conf.ReadInConfig(), "reading config"))
		}
	})
}
