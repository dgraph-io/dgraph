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

package backup

import (
	"fmt"
	"os"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var Backup x.SubCommand

var opt struct {
	restore, version bool
	http, loc, zero  string
	pdir             string
}

func init() {
	Backup.Cmd = &cobra.Command{
		Use:   "backup",
		Short: "Dgraph Enterprise Backup and Restore",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Backup.Conf).Stop()
			if err := run(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}

	flag := Backup.Cmd.Flags()
	flag.BoolVarP(&opt.restore, "restore", "r", true, "perform a restore of a previous backup.")
	flag.StringVar(&opt.http, "http", "localhost:8080", "Address to serve http (pprof).")
	flag.StringVarP(&opt.loc, "loc", "l", "", "sets the location URI to a source or target.")
	flag.StringVarP(&opt.pdir, "postings", "p", "", "Directory where posting lists are stored.")
	flag.BoolVar(&opt.version, "version", false, "prints the version of Dgraph Backup.")
	flag.StringVarP(&opt.zero, "zero", "z", "localhost:5080", "gRPC address for Dgraph zero")

	cmdList := &cobra.Command{
		Use:   "ls",
		Short: "lists existing backups at a location.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listBackup()
		},
	}
	cmdList.Flags().AddFlag(Backup.Cmd.Flag("loc"))
	Backup.Cmd.AddCommand(cmdList)
}

func run() error {
	x.PrintVersion()
	switch {
	case opt.version:
		return nil
	case opt.pdir == "":
		return x.Errorf("Must specify posting dir with -p")
	}

	return runRestore()
}

func listBackup() error {
	return nil
}
