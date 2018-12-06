// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package restore

import (
	"fmt"
	"os"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var Restore x.SubCommand

var opt struct {
	version   bool
	loc, pdir string
}

func init() {
	Restore.Cmd = &cobra.Command{
		Use:   "restore",
		Short: "Dgraph Enterprise Restore",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Restore.Conf).Stop()
			if err := run(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}

	flag := Restore.Cmd.Flags()
	flag.StringVarP(&opt.loc, "loc", "l", "", "sets the location URI to a source or target.")
	flag.StringVarP(&opt.pdir, "postings", "p", "", "Directory where posting lists are stored.")
	flag.BoolVar(&opt.version, "version", false, "prints the version of Dgraph Restore.")
	flag.Bool("debugmode", false, "Enable debug mode for more debug information.")
}

func run() error {
	x.PrintVersion()
	if opt.version {
		return nil
	}
	x.Config.DebugMode = Restore.Conf.GetBool("debugmode")

	return runRestore()
}
