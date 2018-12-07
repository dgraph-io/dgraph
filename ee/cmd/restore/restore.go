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
	"context"
	"fmt"
	"math"
	"os"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var Restore x.SubCommand

var opt struct {
	verify       bool
	source, pdir string
	group        uint32
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
	flag.StringVarP(&opt.source, "source", "l", "", "Sets the source location URI (required).")
	flag.Uint32VarP(&opt.group, "group", "g", 0, "Group ID to restore (0 for all).")
	flag.StringVarP(&opt.pdir, "postings", "p", "", "Directory where posting lists are stored (required).")
	flag.BoolVar(&opt.verify, "verify", false, "Verify the posting contents.")
	Restore.Cmd.MarkFlagRequired("postings")
	Restore.Cmd.MarkFlagRequired("source")
}

func run() error {
	if opt.pdir == "" {
		fmt.Println("Must specify posting dir with -p")
		return Restore.Cmd.Usage()
	}
	if opt.source == "" {
		fmt.Println("Must specify a restore source with --loc")
		return Restore.Cmd.Usage()
	}

	bo := badger.DefaultOptions
	bo.SyncWrites = false
	bo.TableLoadingMode = options.MemoryMap
	bo.ValueThreshold = 1 << 10
	bo.NumVersionsToKeep = math.MaxInt32
	bo.Dir = opt.pdir
	bo.ValueDir = opt.pdir
	db, err := badger.OpenManaged(bo)
	if err != nil {
		return err
	}
	defer db.Close()

	req := &backup.Request{
		DB: db,
		Backup: &pb.BackupRequest{
			Source:  opt.source,
			GroupId: opt.group,
		},
	}
	return req.Restore(context.Background())
}
