/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package cmd

import (
	"fmt"
	"os"

	"github.com/dgraph-io/dgraph/dgraph/cmd/bulk"
	"github.com/dgraph-io/dgraph/dgraph/cmd/debug"
	"github.com/dgraph-io/dgraph/dgraph/cmd/live"
	"github.com/dgraph-io/dgraph/dgraph/cmd/server"
	"github.com/dgraph-io/dgraph/dgraph/cmd/version"
	"github.com/dgraph-io/dgraph/dgraph/cmd/zero"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "dgraph",
	Short: "Dgraph: Distributed Graph Database",
	Long: `
Dgraph is an open source, horizontally scalable and distributed graph database,
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

	var subcommands = []*x.SubCommand{
		&bulk.Bulk, &live.Live, &server.Server, &zero.Zero, &version.Version, &debug.Debug,
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
