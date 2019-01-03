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
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

var Restore x.SubCommand

var opt struct {
	location, pdir string
	progress       bool
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
	_ = Restore.Cmd.MarkFlagRequired("postings")
	_ = Restore.Cmd.MarkFlagRequired("location")
}

func run() error {
	fmt.Println("Restoring backups from:", opt.location)
	fmt.Println("Writing postings to:", opt.pdir)

	// Scan location for backup files and load them. Each file represents a node group,
	// and we create a new p dir for each.
	start := time.Now()
	err := Load(opt.location, func(r io.Reader, object string) error {
		bo := badger.DefaultOptions
		bo.SyncWrites = false
		bo.TableLoadingMode = options.MemoryMap
		bo.ValueThreshold = 1 << 10
		bo.NumVersionsToKeep = math.MaxInt32
		bo.Dir = filepath.Join(opt.pdir, pN(object))
		bo.ValueDir = bo.Dir
		db, err := badger.OpenManaged(bo)
		if err != nil {
			return err
		}
		defer db.Close()
		fmt.Println("--- Creating new db:", bo.Dir)
		return db.Load(r)
	})
	if err != nil {
		return err
	}
	fmt.Printf("Restore: Time elapsed: %s", time.Since(start).Round(time.Second))
	return nil
}

func pN(object string) string {
	var readTs, groupId int
	_, err := fmt.Sscanf(path.Base(object), backupFmt, &readTs, &groupId)
	if err != nil {
		x.Fatalf("Could not scan backup info for %q: %v", object, err)
	}
	fmt.Println("--- Scanned group ID:", groupId)
	return fmt.Sprintf("p%d", groupId)
}
