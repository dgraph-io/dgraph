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
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

// Restore is the sub-command used to restore a backup.
var Restore x.SubCommand

// LsBackup is the sub-command used to list the backups in a folder.
var LsBackup x.SubCommand

var opt struct {
	location, pdir, zero string
}

func init() {
	initRestore()
	initBackupLs()
}

func initRestore() {
	Restore.Cmd = &cobra.Command{
		Use:   "restore",
		Short: "Run Dgraph (EE) Restore backup",
		Long: `
Restore loads objects created with the backup feature in Dgraph Enterprise Edition (EE).

Backups are originated from HTTP at /admin/backup, then can be restored using CLI restore
command. Restore is intended to be used with new Dgraph clusters in offline state.

The --location flag indicates a source URI with Dgraph backup objects. This URI supports all
the schemes used for backup.

Source URI formats:
  [scheme]://[host]/[path]?[args]
  [scheme]:///[path]?[args]
  /[path]?[args] (only for local or NFS)

Source URI parts:
  scheme - service handler, one of: "s3", "minio", "file"
    host - remote address. ex: "dgraph.s3.amazonaws.com"
    path - directory, bucket or container at target. ex: "/dgraph/backups/"
    args - specific arguments that are ok to appear in logs.

The --posting flag sets the posting list parent dir to store the loaded backup files.

Using the --zero flag will use a Dgraph Zero address to update the start timestamp using
the restored version. Otherwise, the timestamp must be manually updated through Zero's HTTP
'assign' command.

Dgraph backup creates a unique backup object for each node group, and restore will create
a posting directory 'p' matching the backup group ID. Such that a backup file
named '.../r32-g2.backup' will be loaded to posting dir 'p2'.

Usage examples:

# Restore from local dir or NFS mount:
$ dgraph restore -p . -l /var/backups/dgraph

# Restore from S3:
$ dgraph restore -p /var/db/dgraph -l s3://s3.us-west-2.amazonaws.com/srfrog/dgraph

# Restore from dir and update Ts:
$ dgraph restore -p . -l /var/backups/dgraph -z localhost:5080

		`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Restore.Conf).Stop()
			if err := runRestoreCmd(); err != nil {
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
	flag.StringVarP(&opt.zero, "zero", "z", "", "gRPC address for Dgraph zero. ex: localhost:5080")
	_ = Restore.Cmd.MarkFlagRequired("postings")
	_ = Restore.Cmd.MarkFlagRequired("location")
}

func initBackupLs() {
	LsBackup.Cmd = &cobra.Command{
		Use:   "lsbackup",
		Short: "List info on backups in given location",
		Long: `
lsbackup looks at a location where backups are stored and prints information about them.

Backups are originated from HTTP at /admin/backup, then can be restored using CLI restore
command. Restore is intended to be used with new Dgraph clusters in offline state.

The --location flag indicates a source URI with Dgraph backup objects. This URI supports all
the schemes used for backup.

Source URI formats:
  [scheme]://[host]/[path]?[args]
  [scheme]:///[path]?[args]
  /[path]?[args] (only for local or NFS)

Source URI parts:
  scheme - service handler, one of: "s3", "minio", "file"
    host - remote address. ex: "dgraph.s3.amazonaws.com"
    path - directory, bucket or container at target. ex: "/dgraph/backups/"
    args - specific arguments that are ok to appear in logs.

Dgraph backup creates a unique backup object for each node group, and restore will create
a posting directory 'p' matching the backup group ID. Such that a backup file
named '.../r32-g2.backup' will be loaded to posting dir 'p2'.

Usage examples:

# Run using location in S3:
$ dgraph lsbackup -l s3://s3.us-west-2.amazonaws.com/srfrog/dgraph
		`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Restore.Conf).Stop()
			if err := runLsbackupCmd(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}

	flag := LsBackup.Cmd.Flags()
	flag.StringVarP(&opt.location, "location", "l", "",
		"Sets the source location URI (required).")
	_ = LsBackup.Cmd.MarkFlagRequired("location")
}

func runRestoreCmd() error {
	var (
		start time.Time
		zc    pb.ZeroClient
	)

	fmt.Println("Restoring backups from:", opt.location)
	fmt.Println("Writing postings to:", opt.pdir)

	// TODO: Remove this dependency on Zero. It complicates restore for the end
	// user.
	if opt.zero != "" {
		fmt.Println("Updating Zero timestamp at:", opt.zero)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		zero, err := grpc.DialContext(ctx, opt.zero,
			grpc.WithBlock(),
			grpc.WithInsecure())
		if err != nil {
			return errors.Wrapf(err, "Unable to connect to %s", opt.zero)
		}
		zc = pb.NewZeroClient(zero)
	}

	start = time.Now()
	version, err := RunRestore(opt.pdir, opt.location)
	if err != nil {
		return err
	}
	if version == 0 {
		return errors.Errorf("Failed to obtain a restore version")
	}
	if glog.V(2) {
		fmt.Printf("Restore version: %d\n", version)
	}

	if zc != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		_, err = zc.Timestamps(ctx, &pb.Num{Val: version})
		if err != nil {
			fmt.Printf("Failed to assign timestamp %d in Zero: %v", version, err)
		}
	}

	fmt.Printf("Restore: Time elapsed: %s\n", time.Since(start).Round(time.Second))
	return nil
}

// RunRestore calls badger.Load and tries to load data into a new DB.
func RunRestore(pdir, location string) (uint64, error) {
	bo := badger.DefaultOptions
	bo.SyncWrites = true
	bo.TableLoadingMode = options.MemoryMap
	bo.ValueThreshold = 1 << 10
	bo.NumVersionsToKeep = math.MaxInt32
	if !glog.V(2) {
		bo.Logger = nil
	}

	// Scan location for backup files and load them. Each file represents a node group,
	// and we create a new p dir for each.
	return Load(location, func(r io.Reader, groupId int) error {
		bo := bo
		bo.Dir = filepath.Join(pdir, fmt.Sprintf("p%d", groupId))
		bo.ValueDir = bo.Dir
		db, err := badger.OpenManaged(bo)
		if err != nil {
			return err
		}
		defer db.Close()
		if glog.V(2) {
			fmt.Printf("Restoring groupId: %d\n", groupId)
			if !pathExist(bo.Dir) {
				fmt.Println("Creating new db:", bo.Dir)
			}
		}
		return db.Load(r, 16)
	})
}

func runLsbackupCmd() error {
	fmt.Println("Listing backups from:", opt.location)
	manifests, err := ListManifests(opt.location)
	if err != nil {
		return errors.Wrapf(err, "while listing manifests")
	}

	fmt.Printf("Name\tSince\tReadTs\tGroups\n")
	for _, manifest := range manifests {
		fmt.Printf("%v\t%v\t%v\t%v\n",
			manifest.FileName,
			manifest.Since,
			manifest.ReadTs,
			manifest.Groups)
	}

	return nil
}
