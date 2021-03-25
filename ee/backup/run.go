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
	"io/ioutil"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v3"
	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"golang.org/x/sync/errgroup"

	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/upgrade"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
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
	key         x.Sensitive
	forceZero   bool
	destination string
	format      string
	verbose     bool
	upgrade     bool // used by export backup command.
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
			panic("This is not implemented")
			// TODO: Remote this later.
			// if err := runRestoreCmd(); err != nil {
			// 	fmt.Fprintln(os.Stderr, err)
			// 	os.Exit(1)
			// }
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

	ExportBackup.Cmd.SetHelpTemplate(x.NonRootTemplate)
	flag := ExportBackup.Cmd.Flags()
	flag.StringVarP(&opt.location, "location", "l", "",
		`Sets the location of the backup. Both file URIs and s3 are supported.
		This command will take care of all the full + incremental backups present in the location.`)
	flag.StringVarP(&opt.destination, "destination", "d", "",
		"The folder to which export the backups.")
	flag.StringVarP(&opt.format, "format", "f", "rdf",
		"The format of the export output. Accepts a value of either rdf or json")
	flag.BoolVar(&opt.upgrade, "upgrade", false,
		`If true, retrieve the CORS from DB and append at the end of GraphQL schema.
		It also deletes the deprecated types and predicates.
		Use this option when exporting a backup of 20.11 for loading onto 21.03.`)
	enc.RegisterFlags(flag)
}

// TODO: This function is broken. Needs to be re-written.
func runExportBackup2() error {
	_, opt.key = ee.GetKeys(ExportBackup.Conf)
	if opt.format != "json" && opt.format != "rdf" {
		return errors.Errorf("invalid format %s", opt.format)
	}
	// Create exportDir and temporary folder to store the restored backup.
	exportDir, err := filepath.Abs(opt.destination)
	if err != nil {
		return errors.Wrapf(err, "cannot convert path %s to absolute path", exportDir)
	}
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return errors.Wrapf(err, "cannot create dir %s", exportDir)
	}
	tmpDir, err := ioutil.TempDir("", "export_backup")
	if err != nil {
		return errors.Wrapf(err, "cannot create temp dir")
	}

	// TODO: This needs to be re-written.
	// restore := worker.RunRestore(tmpDir, opt.location, "", opt.key, options.None, 0)
	// if restore.Err != nil {
	// 	return restore.Err
	// }

	files, err := ioutil.ReadDir(tmpDir)
	if err != nil {
		return err
	}
	// Export the data from the p directories produced by the last step.
	eg, _ := errgroup.WithContext(context.Background())
	for _, f := range files {
		if !f.IsDir() {
			continue
		}

		dir := filepath.Join(filepath.Join(tmpDir, f.Name()))
		gid, err := strconv.ParseUint(strings.TrimPrefix(f.Name(), "p"), 32, 10)
		if err != nil {
			fmt.Printf("WARNING WARNING WARNING: unable to get group id from directory "+
				"inside DB at %s: %v", dir, err)
			continue
		}
		if opt.upgrade && gid == 1 {
			// Query the cors in badger db and append it at the end of GraphQL schema.
			// This change was introduced in v21.03. Backups with 20.07 <= version < 21.03
			// should apply this.
			db, err := badger.OpenManaged(badger.DefaultOptions(dir).
				WithNumVersionsToKeep(math.MaxInt32).
				WithEncryptionKey(opt.key))
			if err != nil {
				return err
			}
			if err := upgrade.OfflineUpgradeFrom2011To2103(db); err != nil {
				return errors.Wrapf(err, "while fixing cors")
			}
			if err := db.Close(); err != nil {
				return err
			}
		}
		eg.Go(func() error {
			return worker.StoreExport(&pb.ExportRequest{
				GroupId: uint32(gid),
				// ReadTs:      restore.Version,
				UnixTs:      time.Now().Unix(),
				Format:      opt.format,
				Destination: exportDir,
			}, dir, opt.key)
		})
	}

	if err := eg.Wait(); err != nil {
		return errors.Wrapf(err, "error while exporting data")
	}

	// Clean up temporary directory.
	if err := os.RemoveAll(tmpDir); err != nil {
		return errors.Wrapf(err, "cannot remove temp directory at %s", tmpDir)
	}

	return nil
}

func runExportBackup() error {
	_, opt.key = ee.GetKeys(ExportBackup.Conf)
	if opt.format != "json" && opt.format != "rdf" {
		return errors.Errorf("invalid format %s", opt.format)
	}
	// Create exportDir and temporary folder to store the restored backup.
	exportDir, err := filepath.Abs(opt.destination)
	if err != nil {
		return errors.Wrapf(err, "cannot convert path %s to absolute path", exportDir)
	}
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return errors.Wrapf(err, "cannot create dir %s", exportDir)
	}

	uri, err := url.Parse(opt.location)
	if err != nil {
		return errors.Wrapf(err, "runExportBackup")
	}
	handler, err := worker.NewUriHandler(uri, nil)
	if err != nil {
		return errors.Wrapf(err, "runExportBackup")
	}
	latestManifest, err := worker.GetLatestManifest(handler, uri)
	if err != nil {
		return errors.Wrapf(err, "runExportBackup")
	}

	exportSchema := func(writers *worker.Writers, val []byte, pk x.ParsedKey) error {
		kv := &bpb.KV{}
		var err error
		if pk.IsSchema() {
			kv, err = worker.SchemaExportKv(pk.Attr, val, true)
			if err != nil {
				return err
			}
		} else {
			kv, err = worker.TypeExportKv(pk.Attr, val)
			if err != nil {
				return err
			}
		}
		return worker.WriteExport(writers, kv, "rdf")
	}

	processKvBuf := func(ch chan *z.Buffer, req *pb.ExportRequest, writers *worker.Writers) error {
		for buf := range ch {
			glog.Info("Received buf of size: ", humanize.IBytes(uint64(buf.LenNoPadding())))
			kv := &bpb.KV{}
			err := buf.SliceIterate(func(s []byte) error {
				kv.Reset()
				if err := kv.Unmarshal(s); err != nil {
					return errors.Wrap(err, "processKvBuf")
				}
				pk, err := x.Parse(kv.Key)
				if err != nil {
					return errors.Wrap(err, "processKvBuf")
				}

				if pk.Attr == "_predicate_" {
					return nil
				}
				if pk.IsSchema() || pk.IsType() {
					return exportSchema(writers, kv.Value, pk)
				}

				pl := &pb.PostingList{}
				if err := pl.Unmarshal(kv.Value); err != nil {
					return errors.Wrap(err, "ProcessKvBuf")
				}

				l := posting.NewList(kv.Key, pl, kv.Version)
				kvList, err := worker.ToExportKvList(pk, l, req)
				if err != nil {
					return errors.Wrap(err, "processKvBuf")
				}
				exportKv := kvList.Kv[0]
				return worker.WriteExport(writers, exportKv, req.Format)
			})
			if err != nil {
				return err
			}
			buf.Release()
		}
		return nil
	}

	// TODO: Make this procesing concurrent.
	for gid, _ := range latestManifest.Groups {
		glog.Infof("Exporting group: %d", gid)
		req := &pb.RestoreRequest{
			GroupId:           gid,
			Location:          opt.location,
			EncryptionKeyFile: ExportBackup.Conf.GetString("encryption_key_file"),
		}
		if err := worker.MapBackup(req); err != nil {
			return errors.Wrap(err, "Failed to map the backups")
		}
		in := &pb.ExportRequest{
			GroupId:     uint32(gid),
			ReadTs:      latestManifest.Since,
			UnixTs:      time.Now().Unix(),
			Format:      opt.format,
			Destination: exportDir,
		}
		uts := time.Unix(in.UnixTs, 0)
		destPath := fmt.Sprintf("dgraph.r%d.u%s", in.ReadTs, uts.UTC().Format("0102.1504"))
		exportStorage, err := worker.NewExportStorage(in, destPath)
		if err != nil {
			return err
		}

		writers, err := worker.InitWriters(exportStorage, in)
		if err != nil {
			return err
		}

		r := worker.NewBackupReducer(nil)

		errCh := make(chan error, 1)
		go func() {
			errCh <- processKvBuf(r.WriteCh(), in, writers)
			glog.Infof("Export DONE for group %d at timestamp %d.", in.GroupId, in.ReadTs)
		}()

		if err := r.Reduce(); err != nil {
			return errors.Wrap(err, "Failed to reduce the map")
		}
		if err := <-errCh; err != nil {
			errors.Wrap(err, "Failed to process reduced buffers")
		}
		if _, err := exportStorage.FinishWriting(writers); err != nil {
			return errors.Wrap(err, "Failed to finish write")
		}
	}
	return nil
}
