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

package worker

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v3"
	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	ostats "go.opencensus.io/stats"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
)

// Backup handles a request coming from another node.
func (w *grpcWorker) Backup(ctx context.Context, req *pb.BackupRequest) (*pb.BackupResponse, error) {
	glog.V(2).Infof("Received backup request via Grpc: %+v", req)
	return backupCurrentGroup(ctx, req)
}

func backupCurrentGroup(ctx context.Context, req *pb.BackupRequest) (*pb.BackupResponse, error) {
	glog.Infof("Backup request: group %d at %d", req.GroupId, req.ReadTs)
	if err := ctx.Err(); err != nil {
		glog.Errorf("Context error during backup: %v\n", err)
		return nil, err
	}

	g := groups()
	if g.groupId() != req.GroupId {
		return nil, errors.Errorf("Backup request group mismatch. Mine: %d. Requested: %d\n",
			g.groupId(), req.GroupId)
	}

	if err := posting.Oracle().WaitForTs(ctx, req.ReadTs); err != nil {
		return nil, err
	}

	closer, err := g.Node.startTaskAtTs(opBackup, req.ReadTs)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot start backup operation")
	}
	defer closer.Done()

	bp := NewBackupProcessor(pstore, req)
	defer bp.Close()

	return bp.WriteBackup(closer.Ctx())
}

// BackupGroup backs up the group specified in the backup request.
func BackupGroup(ctx context.Context, in *pb.BackupRequest) (*pb.BackupResponse, error) {
	glog.V(2).Infof("Sending backup request: %+v\n", in)
	if groups().groupId() == in.GroupId {
		return backupCurrentGroup(ctx, in)
	}

	// This node is not part of the requested group, send the request over the network.
	pl := groups().AnyServer(in.GroupId)
	if pl == nil {
		return nil, errors.Errorf("Couldn't find a server in group %d", in.GroupId)
	}
	res, err := pb.NewWorkerClient(pl.Get()).Backup(ctx, in)
	if err != nil {
		glog.Errorf("Backup error group %d: %s", in.GroupId, err)
		return nil, err
	}

	return res, nil
}

// backupLock is used to synchronize backups to avoid more than one backup request
// to be processed at the same time. Multiple requests could lead to multiple
// backups with the same backupNum in their manifest.
var backupLock sync.Mutex

// BackupRes is used to represent the response and error of the Backup gRPC call together to be
// transported via a channel.
type BackupRes struct {
	res *pb.BackupResponse
	err error
}

func ProcessBackupRequest(ctx context.Context, req *pb.BackupRequest) error {
	if err := x.HealthCheck(); err != nil {
		glog.Errorf("Backup canceled, not ready to accept requests: %s", err)
		return err
	}

	// Grab the lock here to avoid more than one request to be processed at the same time.
	backupLock.Lock()
	defer backupLock.Unlock()

	backupSuccessful := false
	ostats.Record(ctx, x.NumBackups.M(1), x.PendingBackups.M(1))
	defer func() {
		if backupSuccessful {
			ostats.Record(ctx, x.NumBackupsSuccess.M(1), x.PendingBackups.M(-1))
		} else {
			ostats.Record(ctx, x.NumBackupsFailed.M(1), x.PendingBackups.M(-1))
		}
	}()

	ts, err := Timestamps(ctx, &pb.Num{ReadOnly: true})
	if err != nil {
		glog.Errorf("Unable to retrieve readonly timestamp for backup: %s", err)
		return err
	}

	req.ReadTs = ts.ReadOnly
	req.UnixTs = time.Now().UTC().Format("20060102.150405.000")

	// Read the manifests to get the right timestamp from which to start the backup.
	uri, err := url.Parse(req.Destination)
	if err != nil {
		return err
	}
	handler, err := x.NewUriHandler(uri, GetCredentialsFromRequest(req))
	if err != nil {
		return err
	}
	latestManifest, err := GetLatestManifest(handler, uri)
	if err != nil {
		return err
	}

	// Use the readTs as the sinceTs for the next backup. If not found, use the
	// SinceTsDeprecated value from the latest manifest.
	req.SinceTs = latestManifest.ValidReadTs()

	if req.ForceFull {
		// To force a full backup we'll set the sinceTs to zero.
		req.SinceTs = 0
	} else {
		if x.WorkerConfig.EncryptionKey != nil {
			// If encryption key given, latest backup should be encrypted.
			if latestManifest.Type != "" && !latestManifest.Encrypted {
				err = errors.Errorf("latest manifest indicates the last backup was not encrypted " +
					"but this instance has encryption turned on. Try \"forceFull\" flag.")
				return err
			}
		} else {
			// If encryption turned off, latest backup should be unencrypted.
			if latestManifest.Type != "" && latestManifest.Encrypted {
				err = errors.Errorf("latest manifest indicates the last backup was encrypted " +
					"but this instance has encryption turned off. Try \"forceFull\" flag.")
				return err
			}
		}
	}

	// Update the membership state to get the latest mapping of groups to predicates.
	if err := UpdateMembershipState(ctx); err != nil {
		return err
	}

	// Get the current membership state and parse it for easier processing.
	state := GetMembershipState()
	var groups []uint32
	predMap := make(map[uint32][]string)
	for gid, group := range state.Groups {
		groups = append(groups, gid)
		predMap[gid] = make([]string, 0)
		for pred := range group.Tablets {
			predMap[gid] = append(predMap[gid], pred)
		}
	}

	glog.Infof(
		"Created backup request: read_ts:%d since_ts:%d unix_ts:%q destination:%q. Groups=%v\n",
		req.ReadTs, req.SinceTs, req.UnixTs, req.Destination, groups)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var dropOperations []*pb.DropOperation
	{ // This is the code which sends out Backup requests and waits for them to finish.
		resCh := make(chan BackupRes, len(state.Groups))
		for _, gid := range groups {
			br := proto.Clone(req).(*pb.BackupRequest)
			br.GroupId = gid
			br.Predicates = predMap[gid]
			go func(req *pb.BackupRequest) {
				res, err := BackupGroup(ctx, req)
				resCh <- BackupRes{res: res, err: err}
			}(br)
		}

		for range groups {
			backupRes := <-resCh
			if backupRes.err != nil {
				glog.Errorf("Error received during backup: %v", backupRes.err)
				return backupRes.err
			}
			dropOperations = append(dropOperations, backupRes.res.GetDropOperations()...)
		}
	}

	dir := fmt.Sprintf(backupPathFmt, req.UnixTs)
	m := Manifest{
		ReadTs:         req.ReadTs,
		Groups:         predMap,
		Version:        x.ManifestVersion,
		DropOperations: dropOperations,
		Path:           dir,
		Compression:    "snappy",
	}
	if req.SinceTs == 0 {
		m.Type = "full"
		m.BackupId = x.GetRandomName(1)
		m.BackupNum = 1
	} else {
		m.Type = "incremental"
		m.BackupId = latestManifest.BackupId
		m.BackupNum = latestManifest.BackupNum + 1
	}
	m.Encrypted = (x.WorkerConfig.EncryptionKey != nil)

	bp := NewBackupProcessor(nil, req)
	defer bp.Close()
	err = bp.CompleteBackup(ctx, &m)

	if err != nil {
		return err
	}

	backupSuccessful = true
	return nil
}

func ProcessListBackups(ctx context.Context, location string, creds *x.MinioCredentials) (
	[]*Manifest, error) {

	manifests, err := ListBackupManifests(location, creds)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read manifests at location %s", location)
	}

	res := make([]*Manifest, 0)
	for _, m := range manifests {
		res = append(res, m)
	}
	return res, nil
}

// BackupProcessor handles the different stages of the backup process.
type BackupProcessor struct {
	// DB is the Badger pstore managed by this node.
	DB *badger.DB
	// Request stores the backup request containing the parameters for this backup.
	Request *pb.BackupRequest

	// txn is used for the iterators in the threadLocal
	txn     *badger.Txn
	threads []*threadLocal
}

type threadLocal struct {
	Request *pb.BackupRequest
	// pre-allocated pb.PostingList object.
	pl pb.PostingList
	// pre-allocated pb.BackupPostingList object.
	bpl   pb.BackupPostingList
	alloc *z.Allocator
	itr   *badger.Iterator
	buf   *z.Buffer
}

func NewBackupProcessor(db *badger.DB, req *pb.BackupRequest) *BackupProcessor {
	bp := &BackupProcessor{
		DB:      db,
		Request: req,
		threads: make([]*threadLocal, x.WorkerConfig.Badger.NumGoroutines),
	}
	if req.SinceTs > 0 && db != nil {
		bp.txn = db.NewTransactionAt(req.ReadTs, false)
	}
	for i := range bp.threads {
		buf := z.NewBuffer(32<<20, "Worker.BackupProcessor").
			WithAutoMmap(1<<30, "").
			WithMaxSize(32 << 30)

		bp.threads[i] = &threadLocal{
			Request: bp.Request,
			buf:     buf,
		}
		if bp.txn != nil {
			iopt := badger.DefaultIteratorOptions
			iopt.AllVersions = true
			bp.threads[i].itr = bp.txn.NewIterator(iopt)
		}
	}
	return bp
}

func (pr *BackupProcessor) Close() {
	for _, th := range pr.threads {
		if pr.txn != nil {
			th.itr.Close()
		}
		th.buf.Release()
	}
	if pr.txn != nil {
		pr.txn.Discard()
	}
}

// LoadResult holds the output of a Load operation.
type LoadResult struct {
	// Version is the timestamp at which the database is after loading a backup.
	Version uint64
	// MaxLeaseUid is the max UID seen by the load operation. Needed to request zero
	// for the proper number of UIDs.
	MaxLeaseUid uint64
	// MaxLeaseNsId is the max namespace ID seen by the load operation.
	MaxLeaseNsId uint64
	// The error, if any, of the load operation.
	Err error
}

func createBackupFile(h x.UriHandler, uri *url.URL, req *pb.BackupRequest) (io.WriteCloser, error) {
	if !h.DirExists("./") {
		if err := h.CreateDir("./"); err != nil {
			return nil, errors.Wrap(err, "while creating backup file")
		}
	}
	fileName := backupName(req.ReadTs, req.GroupId)
	dir := fmt.Sprintf(backupPathFmt, req.UnixTs)
	if err := h.CreateDir(dir); err != nil {
		return nil, errors.Wrap(err, "while creating backup file")
	}
	backupFile := filepath.Join(dir, fileName)
	w, err := h.CreateFile(backupFile)
	return w, errors.Wrap(err, "while creating backup file")
}

// WriteBackup uses the request values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func (pr *BackupProcessor) WriteBackup(ctx context.Context) (*pb.BackupResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	uri, err := url.Parse(pr.Request.Destination)
	if err != nil {
		return nil, err
	}
	handler, err := x.NewUriHandler(uri, GetCredentialsFromRequest(pr.Request))
	if err != nil {
		return nil, err
	}
	w, err := createBackupFile(handler, uri, pr.Request)
	if err != nil {
		return nil, err
	}
	glog.V(3).Infof("Backup manifest version: %d", pr.Request.SinceTs)

	eWriter, err := enc.GetWriter(x.WorkerConfig.EncryptionKey, w)
	if err != nil {
		return nil, err
	}

	// Snappy is much faster than gzip compression, even with the BestSpeed
	// gzip option. In fact, in my experiments, gzip compression caused the
	// output speed to be ~30 MBps. Snappy can write at ~90 MBps, and overall
	// the speed is similar to writing uncompressed data on disk.
	//
	// These are the times I saw:
	// Without compression: 7m2s 33GB output.
	// With snappy: 7m11s 9.5GB output.
	// With snappy + S3: 7m54s 9.5GB output.
	cWriter := snappy.NewBufferedWriter(eWriter)

	stream := pr.DB.NewStreamAt(pr.Request.ReadTs)
	stream.LogPrefix = "Dgraph.Backup"
	// Ignore versions less than given sinceTs timestamp, or skip older versions of
	// the given key by returning an empty list.
	// Do not do this for schema and type keys. Those keys always have a
	// version of one. They're handled separately.
	stream.SinceTs = pr.Request.SinceTs
	stream.Prefix = []byte{x.ByteData}

	var response pb.BackupResponse
	stream.KeyToList = func(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
		tl := pr.threads[itr.ThreadId]
		tl.alloc = itr.Alloc

		bitr := itr
		// Use the threadlocal iterator because "itr" has the sinceTs set and
		// it will not be able to read all the data.
		if tl.itr != nil {
			bitr = tl.itr
			bitr.Seek(key)
		}

		kvList, dropOp, err := tl.toBackupList(key, bitr)
		if err != nil {
			return nil, err
		}
		// we don't want to append a nil value to the slice, so need to check.
		if dropOp != nil {
			response.DropOperations = append(response.DropOperations, dropOp)
		}
		return kvList, nil
	}

	predMap := make(map[string]struct{})
	for _, pred := range pr.Request.Predicates {
		predMap[pred] = struct{}{}
	}
	stream.ChooseKey = func(item *badger.Item) bool {
		parsedKey, err := x.Parse(item.Key())
		if err != nil {
			glog.Errorf("error %v while parsing key %v during backup. Skipping...",
				err, hex.EncodeToString(item.Key()))
			return false
		}

		// Do not choose keys that contain parts of a multi-part list. These keys
		// will be accessed from the main list.
		if parsedKey.HasStartUid {
			return false
		}

		// Skip backing up the schema and type keys. They will be backed up separately.
		if parsedKey.IsSchema() || parsedKey.IsType() {
			return false
		}
		_, ok := predMap[parsedKey.Attr]
		return ok
	}

	var maxVersion uint64
	stream.Send = func(buf *z.Buffer) error {
		list, err := badger.BufferToKVList(buf)
		if err != nil {
			return err
		}
		for _, kv := range list.Kv {
			if maxVersion < kv.Version {
				maxVersion = kv.Version
			}
		}
		return writeKVList(list, cWriter)
	}

	// This is where the execution happens.
	if err := stream.Orchestrate(ctx); err != nil {
		glog.Errorf("While taking backup: %v", err)
		return &response, err
	}

	// This is used to backup the schema and types.
	writePrefix := func(prefix byte) error {
		tl := threadLocal{
			alloc: z.NewAllocator(1<<10, "BackupProcessor.WritePrefix"),
		}
		defer tl.alloc.Release()

		txn := pr.DB.NewTransactionAt(pr.Request.ReadTs, false)
		defer txn.Discard()
		// We don't need to iterate over all versions.
		iopts := badger.DefaultIteratorOptions
		iopts.Prefix = []byte{prefix}

		itr := txn.NewIterator(iopts)
		defer itr.Close()

		list := &bpb.KVList{}
		for itr.Rewind(); itr.Valid(); itr.Next() {
			item := itr.Item()
			// Don't export deleted items.
			if item.IsDeletedOrExpired() {
				continue
			}
			parsedKey, err := x.Parse(item.Key())
			if err != nil {
				glog.Errorf("error %v while parsing key %v during backup. Skipping...",
					err, hex.EncodeToString(item.Key()))
				continue
			}
			// This check makes sense only for the schema keys. The types are not stored in it.
			if _, ok := predMap[parsedKey.Attr]; !parsedKey.IsType() && !ok {
				continue
			}
			kv := y.NewKV(tl.alloc)
			if err := item.Value(func(val []byte) error {
				kv.Value = append(kv.Value, val...)
				return nil
			}); err != nil {
				return errors.Wrapf(err, "while copying value")
			}

			backupKey, err := tl.toBackupKey(item.Key())
			if err != nil {
				return err
			}
			kv.Key = backupKey
			kv.UserMeta = tl.alloc.Copy([]byte{item.UserMeta()})
			kv.Version = item.Version()
			kv.ExpiresAt = item.ExpiresAt()
			list.Kv = append(list.Kv, kv)
		}
		return writeKVList(list, cWriter)
	}

	for _, prefix := range []byte{x.ByteSchema, x.ByteType} {
		if err := writePrefix(prefix); err != nil {
			glog.Errorf("While writing prefix %d to backup: %v", prefix, err)
			return &response, err
		}
	}

	if maxVersion > pr.Request.ReadTs {
		glog.Errorf("Max timestamp seen during backup (%d) is greater than readTs (%d)",
			maxVersion, pr.Request.ReadTs)
	}

	glog.V(2).Infof("Backup group %d version: %d", pr.Request.GroupId, pr.Request.ReadTs)
	if err = cWriter.Close(); err != nil {
		glog.Errorf("While closing gzipped writer: %v", err)
		return &response, err
	}

	if err = w.Close(); err != nil {
		glog.Errorf("While closing handler: %v", err)
		return &response, err
	}
	glog.Infof("Backup complete: group %d at %d", pr.Request.GroupId, pr.Request.ReadTs)
	return &response, nil
}

// CompleteBackup will finalize a backup by writing the manifest at the backup destination.
func (pr *BackupProcessor) CompleteBackup(ctx context.Context, m *Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	uri, err := url.Parse(pr.Request.Destination)
	if err != nil {
		return err
	}
	handler, err := x.NewUriHandler(uri, GetCredentialsFromRequest(pr.Request))
	if err != nil {
		return err
	}

	manifest, err := GetManifestNoUpgrade(handler, uri)
	if err != nil {
		return err
	}
	manifest.Manifests = append(manifest.Manifests, m)

	if err := CreateManifest(handler, uri, manifest); err != nil {
		return errors.Wrap(err, "Complete backup failed")
	}
	glog.Infof("Backup completed OK.")
	return nil
}

// GoString implements the GoStringer interface for Manifest.
func (m *Manifest) GoString() string {
	return fmt.Sprintf(`Manifest{Since: %d, ReadTs: %d, Groups: %v, Encrypted: %v}`,
		m.SinceTsDeprecated, m.ReadTs, m.Groups, m.Encrypted)
}

func (tl *threadLocal) toBackupList(key []byte, itr *badger.Iterator) (
	*bpb.KVList, *pb.DropOperation, error) {
	list := &bpb.KVList{}
	var dropOp *pb.DropOperation

	item := itr.Item()
	if item.Version() < tl.Request.SinceTs {
		return list, nil,
			errors.Errorf("toBackupList: Item.Version(): %d should be less than sinceTs: %d",
				item.Version(), tl.Request.SinceTs)
	}
	if item.IsDeletedOrExpired() {
		return list, nil, nil
	}

	switch item.UserMeta() {
	case posting.BitEmptyPosting, posting.BitCompletePosting, posting.BitDeltaPosting:
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "while reading posting list")
		}

		// Don't allocate kv on tl.alloc, because we don't need it by the end of this func.
		kv, err := l.ToBackupPostingList(&tl.bpl, tl.alloc, tl.buf)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "while rolling up list")
		}

		backupKey, err := tl.toBackupKey(kv.Key)
		if err != nil {
			return nil, nil, err
		}

		// check if this key was storing a DROP operation record. If yes, get the drop operation.
		dropOp, err = checkAndGetDropOp(key, l, tl.Request.ReadTs)
		if err != nil {
			return nil, nil, err
		}

		kv.Key = backupKey
		list.Kv = append(list.Kv, kv)
	default:
		return nil, nil, errors.Errorf(
			"Unexpected meta: %d for key: %s", item.UserMeta(), hex.Dump(key))
	}
	return list, dropOp, nil
}

func (tl *threadLocal) toBackupKey(key []byte) ([]byte, error) {
	parsedKey, err := x.Parse(key)
	if err != nil {
		return nil, errors.Wrapf(err, "could not parse key %s", hex.Dump(key))
	}
	bk := parsedKey.ToBackupKey()

	out := tl.alloc.Allocate(bk.Size())
	n, err := bk.MarshalToSizedBuffer(out)
	return out[:n], err
}

func writeKVList(list *bpb.KVList, w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, uint64(list.Size())); err != nil {
		return err
	}
	buf, err := list.Marshal()
	if err != nil {
		return err
	}
	_, err = w.Write(buf)
	return err
}

func checkAndGetDropOp(key []byte, l *posting.List, readTs uint64) (*pb.DropOperation, error) {
	isDropOpKey, err := x.IsDropOpKey(key)
	if err != nil || !isDropOpKey {
		return nil, err
	}

	vals, err := l.AllValues(readTs)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read value of dgraph.drop.op")
	}
	switch len(vals) {
	case 0:
		// do nothing, it means this one was deleted with S * * deletion.
		// So, no need to consider it.
		return nil, nil
	case 1:
		val, ok := vals[0].Value.([]byte)
		if !ok {
			return nil, errors.Errorf("cannot convert value of dgraph.drop.op to byte array, "+
				"got type: %s, value: %v, tid: %v", reflect.TypeOf(vals[0].Value), vals[0].Value,
				vals[0].Tid)
		}
		// A dgraph.drop.op record can have values in only one of the following formats:
		// * DROP_ALL;
		// * DROP_DATA;ns
		// * DROP_ATTR;attrName
		// * DROP_NS;ns
		// So, accordingly construct the *pb.DropOperation.
		dropOp := &pb.DropOperation{}
		dropInfo := strings.SplitN(string(val), ";", 2)
		if len(dropInfo) != 2 {
			return nil, errors.Errorf("Unexpected value: %s for dgraph.drop.op", val)
		}
		switch dropInfo[0] {
		case "DROP_ALL":
			dropOp.DropOp = pb.DropOperation_ALL
		case "DROP_DATA":
			dropOp.DropOp = pb.DropOperation_DATA
			dropOp.DropValue = dropInfo[1] // contains namespace.
		case "DROP_ATTR":
			dropOp.DropOp = pb.DropOperation_ATTR
			dropOp.DropValue = dropInfo[1]
		case "DROP_NS":
			dropOp.DropOp = pb.DropOperation_NS
			dropOp.DropValue = dropInfo[1] // contains namespace.
		}
		return dropOp, nil
	default:
		// getting more than one values for a non-list predicate is an error
		return nil, errors.Errorf("found multiple values for dgraph.drop.op: %v", vals)
	}
}
