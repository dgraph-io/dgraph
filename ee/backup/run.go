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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

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
	initBackupLs()
	initExportBackup()
}

func initBackupLs() {
	LsBackup.Cmd = &cobra.Command{
		Use:   "lsbackup",
		Short: "List info on backups in a given location",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(LsBackup.Conf).Stop()
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
		ReadTs         uint64              `json:"read_ts"`
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
			Since:     manifest.SinceTsDeprecated,
			ReadTs:    manifest.ReadTs,
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
	ee.RegisterEncFlag(flag)
}

type bufWriter struct {
	writers *worker.Writers
	req     *pb.ExportRequest
}

func exportSchema(writers *worker.Writers, val []byte, pk x.ParsedKey) error {
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

func (bw *bufWriter) Write(buf *z.Buffer) error {
	kv := &bpb.KV{}
	err := buf.SliceIterate(func(s []byte) error {
		kv.Reset()
		if err := kv.Unmarshal(s); err != nil {
			return errors.Wrap(err, "processKvBuf failed to unmarshal kv")
		}
		pk, err := x.Parse(kv.Key)
		if err != nil {
			return errors.Wrap(err, "processKvBuf failed to parse key")
		}
		if pk.Attr == "_predicate_" {
			return nil
		}
		if pk.IsSchema() || pk.IsType() {
			return exportSchema(bw.writers, kv.Value, pk)
		}
		if pk.IsData() {
			pl := &pb.PostingList{}
			if err := pl.Unmarshal(kv.Value); err != nil {
				return errors.Wrap(err, "ProcessKvBuf failed to Unmarshal pl")
			}
			l := posting.NewList(kv.Key, pl, kv.Version)
			kvList, err := worker.ToExportKvList(pk, l, bw.req)
			if err != nil {
				return errors.Wrap(err, "processKvBuf failed to Export")
			}
			if len(kvList.Kv) == 0 {
				return nil
			}
			exportKv := kvList.Kv[0]
			return worker.WriteExport(bw.writers, exportKv, bw.req.Format)
		}
		return nil
	})
	return errors.Wrap(err, "bufWriter failed to write")
}

func runExportBackup() error {
	keys, err := ee.GetKeys(ExportBackup.Conf)
	if err != nil {
		return err
	}
	opt.key = keys.EncKey
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
	handler, err := x.NewUriHandler(uri, nil)
	if err != nil {
		return errors.Wrapf(err, "runExportBackup")
	}
	latestManifest, err := worker.GetLatestManifest(handler, uri)
	if err != nil {
		return errors.Wrapf(err, "runExportBackup")
	}

	mapDir, err := ioutil.TempDir(x.WorkerConfig.TmpDir, "restore-export")
	x.Check(err)
	defer os.RemoveAll(mapDir)
	glog.Infof("Created temporary map directory: %s\n", mapDir)

	encFlag := z.NewSuperFlag(ExportBackup.Conf.GetString("encryption")).
		MergeAndCheckDefault(ee.EncDefaults)
	// TODO: Can probably make this procesing concurrent.
	for gid := range latestManifest.Groups {
		glog.Infof("Exporting group: %d", gid)
		req := &pb.RestoreRequest{
			GroupId:           gid,
			Location:          opt.location,
			EncryptionKeyFile: encFlag.GetPath("key-file"),
			RestoreTs:         1,
		}
		if _, err := worker.RunMapper(req, mapDir); err != nil {
			return errors.Wrap(err, "Failed to map the backups")
		}

		in := &pb.ExportRequest{
			GroupId:     uint32(gid),
			ReadTs:      latestManifest.ValidReadTs(),
			UnixTs:      time.Now().Unix(),
			Format:      opt.format,
			Destination: exportDir,
		}
		writers, err := worker.NewWriters(in)
		defer writers.Close()
		if err != nil {
			return err
		}

		w := &bufWriter{req: in, writers: writers}
		if err := worker.RunReducer(w, mapDir); err != nil {
			return errors.Wrap(err, "Failed to reduce the map")
		}
		if err := writers.Close(); err != nil {
			return errors.Wrap(err, "Failed to finish write")
		}
	}
	return nil
}
