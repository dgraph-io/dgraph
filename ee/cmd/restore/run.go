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
	verify         bool
	location, pdir string
}

func init() {
	Restore.Cmd = &cobra.Command{
		Use:   "restore",
		Short: "Run Dgraph (EE) Restore backup",
		Long: `
		Dgraph Restore is used to load backup files offline.
		`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Restore.Conf).Stop()
			if err := run(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}

	flag := Restore.Cmd.Flags()
	flag.StringVarP(&opt.location, "location", "l", "",
		"Sets the source location URI (required).")
	flag.StringVarP(&opt.pdir, "postings", "p", "",
		"Directory where posting lists are stored (required).")
	flag.BoolVar(&opt.verify, "verify", false, "Verify the posting contents.")
	flag.BoolVar(&x.Config.DebugMode, "debugmode", false,
		"Enable debug mode for more debug information.")
	_ = Restore.Cmd.MarkFlagRequired("postings")
	_ = Restore.Cmd.MarkFlagRequired("location")
}

func run() error {
	fmt.Println("Restoring backups from:", opt.location)
	fmt.Println("Writing postings to:", opt.pdir)

	return runRestore()
}
