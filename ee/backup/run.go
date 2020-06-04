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
	"os"
	"time"

	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

// Restore is the sub-command used to restore a backup.
var Restore x.SubCommand

// LsBackup is the sub-command used to list the backups in a folder.
var LsBackup x.SubCommand

var opt struct {
	backupId, location, pdir, zero string
	key                            x.SensitiveByteSlice
	forceZero                      bool
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
	flag.StringVarP(&opt.backupId, "backup_id", "", "", "The ID of the backup series to "+
		"restore. If empty, it will restore the latest series.")
	flag.BoolVarP(&opt.forceZero, "force_zero", "", true, "If false, no connection to "+
		"a zero in the cluster will be required. Keep in mind this requires you to manually "+
		"update the timestamp and max uid when you start the cluster. The correct values are "+
		"printed near the end of this command's output.")
	enc.RegisterFlags(flag)
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
		err   error
	)
	if opt.key, err = enc.ReadKey(Restore.Conf); err != nil {
		return err
	}
	fmt.Println("Restoring backups from:", opt.location)
	fmt.Println("Writing postings to:", opt.pdir)

	if opt.zero == "" && opt.forceZero {
		return errors.Errorf("No Dgraph Zero address passed. Use the --force_zero option if you " +
			"meant to do this")
	}

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
	result := worker.RunRestore(opt.pdir, opt.location, opt.backupId, opt.key)
	if result.Err != nil {
		return result.Err
	}
	if result.Version == 0 {
		return errors.Errorf("Failed to obtain a restore version")
	}
	fmt.Printf("Restore version: %d\n", result.Version)
	fmt.Printf("Restore max uid: %d\n", result.MaxLeaseUid)

	if zc != nil {
		ctx, cancelTs := context.WithTimeout(context.Background(), time.Minute)
		defer cancelTs()

		_, err := zc.Timestamps(ctx, &pb.Num{Val: result.Version})
		if err != nil {
			fmt.Printf("Failed to assign timestamp %d in Zero: %v", result.Version, err)
			return err
		}

		ctx, cancelUid := context.WithTimeout(context.Background(), time.Minute)
		defer cancelUid()

		_, err = zc.AssignUids(ctx, &pb.Num{Val: result.MaxLeaseUid})
		if err != nil {
			fmt.Printf("Failed to assign maxLeaseId %d in Zero: %v\n", result.MaxLeaseUid, err)
			return err
		}
	}

	fmt.Printf("Restore: Time elapsed: %s\n", time.Since(start).Round(time.Second))
	return nil
}

func runLsbackupCmd() error {
	fmt.Println("Listing backups from:", opt.location)
	manifests, err := worker.ListBackupManifests(opt.location, nil)
	if err != nil {
		return errors.Wrapf(err, "while listing manifests")
	}

	fmt.Printf("Name\tSince\tGroups\tEncrypted\n")
	for path, manifest := range manifests {
		fmt.Printf("%v\t%v\t%v\t%v\n", path, manifest.Since, manifest.Groups, manifest.Encrypted)
	}

	return nil
}
