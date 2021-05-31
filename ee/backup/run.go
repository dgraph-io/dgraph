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
<<<<<<< HEAD
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/options"
	"golang.org/x/sync/errgroup"

	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/ee/enc"
=======
	"net/url"
	"os"
	"path/filepath"
	"time"

	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/posting"
>>>>>>> master
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/upgrade"
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

<<<<<<< HEAD
func runRestoreCmd() error {
	var (
		start time.Time
		zc    pb.ZeroClient
		err   error
	)
	_, opt.key = ee.GetKeys(Restore.Conf)
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

=======
>>>>>>> master
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
<<<<<<< HEAD
			Since:     manifest.SinceTsDeprecated,
			ReadTs:    manifest.ReadTs,
=======
			Since:     manifest.Since,
>>>>>>> master
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
<<<<<<< HEAD
	enc.RegisterFlags(flag)
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
	tmpDir, err := ioutil.TempDir("", "export_backup")
	if err != nil {
		return errors.Wrapf(err, "cannot create temp dir")
	}

	restore := worker.RunRestore(tmpDir, opt.location, "", opt.key, options.None, 0)
	if restore.Err != nil {
		return restore.Err
	}

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
				GroupId:     uint32(gid),
				ReadTs:      restore.Version,
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

=======
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
			ReadTs:      latestManifest.Since,
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
>>>>>>> master
	return nil
}
