package worker

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	stype "github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const numBackupRoutines = 10

type kv struct {
	key, value []byte
}

func toRDF(buf *bytes.Buffer, item kv) {
	pre := item.key
	p := new(types.Posting)
	pl := types.GetRootAsPostingList(item.value, 0)

	for pidx := 0; pidx < pl.PostingsLength(); pidx++ {
		x.AssertTruef(pl.Postings(p, pidx), "Unable to parse Posting at index: %v", pidx)
		buf.Write(pre)
		if p.Uid() == math.MaxUint64 && !bytes.Equal(p.ValueBytes(), nil) {
			// Value posting
			// Convert to appropriate type
			val := p.ValueBytes()
			typ := stype.ValueForType(stype.TypeID(p.ValType()))
			x.Check(typ.UnmarshalBinary(val))
			str, err := typ.MarshalText()
			x.Check(err)

			_, err = buf.WriteString(fmt.Sprintf("%q", str))
			x.Check(err)
			if p.ValType() != 0 {
				_, err = buf.WriteString(fmt.Sprintf("^^<xs:%s> ", typ.Type().Name))
				x.Check(err)
			}
			_, err = buf.WriteString(" .\n")
			x.Check(err)
			break
		}
		// Uid list
		destUID := p.Uid()
		strUID := strconv.FormatUint(destUID, 16)

		_, err := buf.WriteString(fmt.Sprintf("<_uid_:%s> .\n", strUID))
		x.Check(err)
	}
}

func writeToFile(fpath string, ch chan []byte) error {
	f, err := os.Create(fpath)
	if err != nil {
		return err
	}

	defer f.Close()
	x.Check(err)
	w := bufio.NewWriterSize(f, 1000000)
	gw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}

	for buf := range ch {
		if _, err := gw.Write(buf); err != nil {
			return err
		}
	}
	if err := gw.Flush(); err != nil {
		return err
	}
	if err := gw.Close(); err != nil {
		return err
	}
	return w.Flush()
}

// Backup creates a backup of data by exporting it as an RDF gzip.
func backup(gid uint32, bdir string) error {
	// Use a goroutine to write to file.
	err := os.MkdirAll(bdir, 0700)
	if err != nil {
		return err
	}
	fpath := path.Join(bdir, fmt.Sprintf("dgraph-%d-%s.rdf.gz", gid,
		time.Now().Format("2006-01-02-15-04")))
	fmt.Printf("Backing up at: %v\n", fpath)
	chb := make(chan []byte, 1000)
	errChan := make(chan error, 1)
	go func() {
		errChan <- writeToFile(fpath, chb)
	}()

	// Use a bunch of goroutines to convert to RDF format.
	chkv := make(chan kv, 1000)
	var wg sync.WaitGroup
	wg.Add(numBackupRoutines)
	for i := 0; i < numBackupRoutines; i++ {
		go func() {
			buf := new(bytes.Buffer)
			buf.Grow(50000)
			for item := range chkv {
				toRDF(buf, item)
				if buf.Len() >= 40000 {
					chb <- buf.Bytes()
					buf.Reset()
				}
			}
			if buf.Len() > 0 {
				chb <- buf.Bytes()
			}
			wg.Done()
		}()
	}

	uidPrefix := []byte("_uid_")
	// Iterate over rocksdb.
	it := pstore.NewIterator()
	defer it.Close()
	var lastPred string
	for it.SeekToFirst(); it.Valid(); {
		key := it.Key().Data()
		cidx := bytes.IndexRune(key, ':')
		if cidx > -1 {
			// Seek to the end of index keys.
			pre := key[:cidx+1]
			pre = append(pre, '~')
			it.Seek(pre)
			continue
		}
		if bytes.HasPrefix(key, uidPrefix) {
			// Skip the UID mappings.
			it.Seek([]byte("_uid_~"))
			continue
		}
		pred, uid := posting.SplitKey(key)
		if pred != lastPred && BelongsTo(pred) != gid {
			it.Seek([]byte(fmt.Sprintf("%s~", pred)))
			continue
		}

		k := []byte(fmt.Sprintf("<_uid_:%x> <%s> ", uid, pred))
		v := make([]byte, len(it.Value().Data()))
		copy(v, it.Value().Data())
		chkv <- kv{
			key:   k,
			value: v,
		}
		lastPred = pred
		it.Next()
	}

	close(chkv) // We have stopped output to chkv.
	wg.Wait()   // Wait for numBackupRoutines to finish.
	close(chb)  // We have stopped output to chb.

	err = <-errChan
	return err
}

func handleBackupForGroup(ctx context.Context, reqId uint64, gid uint32) *BackupPayload {
	n := groups().Node(gid)
	if n.AmLeader() {
		x.Trace(ctx, "Leader of group: %d. Running backup.", gid)
		if err := backup(gid, *backupPath); err != nil {
			x.TraceError(ctx, err)
			return &BackupPayload{
				ReqId:  reqId,
				Status: BackupPayload_FAILED,
			}
		}
		x.Trace(ctx, "Backup done for group: %d.", gid)
		return &BackupPayload{
			ReqId:   reqId,
			Status:  BackupPayload_SUCCESS,
			GroupId: gid,
		}
	}

	// I'm not the leader. Relay to someone who I think is.
	var addrs []string
	{
		// Try in order: leader of given group, any server from given group, leader of group zero.
		_, addr := groups().Leader(gid)
		addrs = append(addrs, addr)
		addrs = append(addrs, groups().AnyServer(gid))
		_, addr = groups().Leader(0)
		addrs = append(addrs, addr)
	}

	var conn *grpc.ClientConn
	for _, addr := range addrs {
		pl := pools().get(addr)
		var err error
		conn, err = pl.Get()
		if err == nil {
			x.Trace(ctx, "Relaying backup request for group %d to %q", gid, pl.Addr)
			defer pl.Put(conn)
			break
		}
		x.TraceError(ctx, err)
	}

	// Unable to find any connection to any of these servers. This should be exceedingly rare.
	// But probably not worthy of crashing the server. We can just skip the backup.
	if conn == nil {
		x.Trace(ctx, fmt.Sprintf("Unable to find a server to backup group: %d", gid))
		return &BackupPayload{
			ReqId:   reqId,
			Status:  BackupPayload_FAILED,
			GroupId: gid,
		}
	}

	c := NewWorkerClient(conn)
	nr := &BackupPayload{
		ReqId:   reqId,
		GroupId: gid,
	}
	nrep, err := c.Backup(ctx, nr)
	if err != nil {
		x.TraceError(ctx, err)
		return &BackupPayload{
			ReqId:   reqId,
			Status:  BackupPayload_FAILED,
			GroupId: gid,
		}
	}
	return nrep
}

// Backup request is used to trigger backups for the request list of groups.
// If a server receives request to backup a group that it doesn't handle, it would
// automatically relay that request to the server that it thinks should handle the request.
func (w *grpcWorker) Backup(ctx context.Context, req *BackupPayload) (*BackupPayload, error) {
	reply := &BackupPayload{ReqId: req.ReqId}
	reply.Status = BackupPayload_FAILED // Set by default.

	if ctx.Err() != nil {
		return reply, ctx.Err()
	}
	if !w.addIfNotPresent(req.ReqId) {
		reply.Status = BackupPayload_DUPLICATE
		return reply, nil
	}

	chb := make(chan *BackupPayload, 1)
	go func() {
		chb <- handleBackupForGroup(ctx, req.ReqId, req.GroupId)
	}()

	select {
	case rep := <-chb:
		return rep, nil
	case <-ctx.Done():
		return reply, ctx.Err()
	}
}

func BackupOverNetwork(ctx context.Context) {
	// If we haven't even had a single membership update, don't run backup.
	if groups().LastUpdate() == 0 {
		x.Trace(ctx, "This server hasn't yet been fully initiated. Please retry later.")
		return
	}
	// Let's first collect all groups.
	gids := groups().KnownGroups()

	ch := make(chan *BackupPayload, len(gids))
	for _, gid := range gids {
		go func(group uint32) {
			reqId := uint64(rand.Int63())
			ch <- handleBackupForGroup(ctx, reqId, group)
		}(gid)
	}

	for i := 0; i < len(gids); i++ {
		bp := <-ch
		if bp.Status != BackupPayload_SUCCESS {
			x.Trace(ctx, "Backup status: %v for group id: %d", bp.Status, bp.GroupId)
		} else {
			x.Trace(ctx, "Backup successful for group: %v", bp.GroupId)
		}
	}
	x.Trace(ctx, "DONE backup")
}
