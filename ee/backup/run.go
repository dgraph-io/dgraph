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
	"encoding/json"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

// Restore is the sub-command used to restore a backup.
var Restore x.SubCommand

// LsBackup is the sub-command used to list the backups in a folder.
var LsBackup x.SubCommand

var ExportBackup x.SubCommand

var opt struct {
	backupId    string
	badger      string
	location    string
	pdir        string
	zero        string
	key         x.SensitiveByteSlice
	forceZero   bool
	destination string
	format      string
	verbose     bool
}

func init() {
	initRestore()
	initBackupLs()
	initExportBackup()
}

func initRestore() {
	Restore.Cmd = &cobra.Command{
		Use:   "restore",
		Short: "Restore backup from Dgraph Enterprise Edition",
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
		Annotations: map[string]string{"group": "data-load"},
	}
	Restore.Cmd.SetHelpTemplate(x.NonRootTemplate)
	flag := Restore.Cmd.Flags()

	flag.StringVarP(&opt.badger, "badger", "b", worker.BadgerDefaults,
		z.NewSuperFlagHelp(worker.BadgerDefaults).
			Head("Badger options").
			Flag("compression",
				"Specifies the compression algorithm and compression level (if applicable) for the "+
					`postings directory. "none" would disable compression, while "zstd:1" would set `+
					"zstd compression at level 1.").
			Flag("goroutines",
				"The number of goroutines to use in badger.Stream.").
			String())

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
	x.RegisterClientTLSFlags(flag)
	enc.RegisterFlags(flag)
	_ = Restore.Cmd.MarkFlagRequired("postings")
	_ = Restore.Cmd.MarkFlagRequired("location")
}

func initBackupLs() {
	LsBackup.Cmd = &cobra.Command{
		Use:   "lsbackup",
		Short: "List info on backups in a given location",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Restore.Conf).Stop()
			if err := runLsbackupCmd(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
		Annotations: map[string]string{"group": "tool"},
	}
	LsBackup.Cmd.SetHelpTemplate(x.NonRootTemplate)
	flag := LsBackup.Cmd.Flags()
	flag.StringVarP(&opt.location, "location", "l", "",
		"Sets the source location URI (required).")
	flag.BoolVar(&opt.verbose, "verbose", false,
		"Outputs additional info in backup list.")
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

		tlsConfig, err := x.LoadClientTLSConfigForInternalPort(Restore.Conf)
		x.Checkf(err, "Unable to generate helper TLS config")
		callOpts := []grpc.DialOption{grpc.WithBlock()}
		if tlsConfig != nil {
			callOpts = append(callOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		} else {
			callOpts = append(callOpts, grpc.WithInsecure())
		}

		zero, err := grpc.DialContext(ctx, opt.zero, callOpts...)
		if err != nil {
			return errors.Wrapf(err, "Unable to connect to %s", opt.zero)
		}
		zc = pb.NewZeroClient(zero)
	}

	badger := z.NewSuperFlag(opt.badger).MergeAndCheckDefault(worker.BadgerDefaults)
	ctype, clevel := x.ParseCompression(badger.GetString("compression"))

	start = time.Now()
	result := worker.RunRestore(opt.pdir, opt.location, opt.backupId, opt.key, ctype, clevel)
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

		if _, err := zc.Timestamps(ctx, &pb.Num{Val: result.Version}); err != nil {
			fmt.Printf("Failed to assign timestamp %d in Zero: %v", result.Version, err)
			return err
		}

		leaseID := func(val uint64, typ pb.NumLeaseType) error {
			// MaxLeaseUid can be zero if the backup was taken on an empty DB.
			if val == 0 {
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, err = zc.AssignIds(ctx, &pb.Num{Val: val, Type: typ}); err != nil {
				fmt.Printf("Failed to assign %s %d in Zero: %v\n",
					pb.NumLeaseType_name[int32(typ)], val, err)
				return err
			}
			return nil
		}

		if err := leaseID(result.MaxLeaseUid, pb.Num_UID); err != nil {
			return errors.Wrapf(err, "cannot update max uid lease after restore.")
		}
		if err := leaseID(result.MaxLeaseNsId, pb.Num_NS_ID); err != nil {
			return errors.Wrapf(err, "cannot update max namespace lease after restore.")
		}
	}

	fmt.Printf("Restore: Time elapsed: %s\n", time.Since(start).Round(time.Second))
	return nil
}

func runLsbackupCmd() error {
	manifests, err := worker.ListBackupManifests(opt.location, nil)
	if err != nil {
		return errors.Wrapf(err, "while listing manifests")
	}

	type backupEntry struct {
		Path           string              `json:"path"`
		Since          uint64              `json:"since"`
		BackupId       string              `json:"backup_id"`
		BackupNum      uint64              `json:"backup_num"`
		Encrypted      bool                `json:"encrypted"`
		Type           string              `json:"type"`
		Groups         map[uint32][]string `json:"groups,omitempty"`
		DropOperations []*pb.DropOperation `json:"drop_operations,omitempty"`
	}

	type backupOutput []backupEntry

	var output backupOutput
	for _, manifest := range manifests {

		be := backupEntry{
			Path:      manifest.Path,
			Since:     manifest.Since,
			BackupId:  manifest.BackupId,
			BackupNum: manifest.BackupNum,
			Encrypted: manifest.Encrypted,
			Type:      manifest.Type,
		}
		if opt.verbose {
			be.Groups = manifest.Groups
			be.DropOperations = manifest.DropOperations
		}
		output = append(output, be)
	}
	b, err := json.MarshalIndent(output, "", "\t")
	if err != nil {
		fmt.Println("error:", err)
	}
	os.Stdout.Write(b)
	fmt.Println()
	return nil
}

func initExportBackup() {
	ExportBackup.Cmd = &cobra.Command{
		Use:   "export_backup",
		Short: "Export data inside single full or incremental backup",
		Long:  ``,
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(ExportBackup.Conf).Stop()
			if err := runExportBackup(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
		Annotations: map[string]string{"group": "tool"},
	}

	flag := ExportBackup.Cmd.Flags()
	flag.StringVarP(&opt.location, "location", "l", "",
		"Sets the location of the backup. Only file URIs are supported for now.")
	flag.StringVarP(&opt.destination, "destination", "d", "",
		"The folder to which export the backups.")
	flag.StringVarP(&opt.format, "format", "f", "rdf",
		"The format of the export output. Accepts a value of either rdf or json")
	enc.RegisterFlags(flag)
}

func runExportBackup() error {
	var err error
	if opt.key, err = enc.ReadKey(ExportBackup.Conf); err != nil {
		return err
	}

	exporter := worker.BackupExporter{}
	return exporter.ExportBackup(opt.location, opt.destination, opt.format, opt.key)
}
