package worker

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"math/rand"
	"os"
	"path"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const numBackupRoutines = 10

type kv struct {
	prefix string
	list   *types.PostingList
}

func toRDF(buf *bytes.Buffer, item kv) {
	pl := item.list
	for _, p := range pl.Postings {
		x.Check2(buf.WriteString(item.prefix))

		if !bytes.Equal(p.Value, nil) {
			// Value posting
			// Convert to appropriate type
			vID := types.TypeID(p.ValType)
			src := types.ValueForType(vID)
			src.Value = p.Value
			str, err := types.Convert(src, types.StringID)
			x.Check(err)
			x.Check2(buf.WriteString(fmt.Sprintf("\"%s\"", str.Value)))
			if types.TypeID(p.ValType) == types.GeoID {
				x.Check2(buf.WriteString(fmt.Sprintf("^^<geo:geojson> ")))
				// TODO(tzdybal) - uncomment
				/* } else if len(p.Lang) > 0 {
				x.Check2(buf.WriteString(fmt.Sprintf("@%s", p.Lang))) */
			} else if types.TypeID(p.ValType) != types.BinaryID &&
				types.TypeID(p.ValType) != types.DefaultID {
				x.Check2(buf.WriteString(fmt.Sprintf("^^<xs:%s>", vID.Name())))
			}
			x.Check2(buf.WriteString(" .\n"))
			return
		}
		x.Check2(buf.WriteString(fmt.Sprintf("<%#x> .\n", p.Uid)))
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
					tmp := make([]byte, buf.Len())
					copy(tmp, buf.Bytes())
					chb <- tmp
					buf.Reset()
				}
			}
			if buf.Len() > 0 {
				tmp := make([]byte, buf.Len())
				copy(tmp, buf.Bytes())
				chb <- tmp
			}
			wg.Done()
		}()
	}

	// Iterate over rocksdb.
	it := pstore.NewIterator()
	defer it.Close()
	var lastPred string
	for it.SeekToFirst(); it.Valid(); {
		key := it.Key().Data()
		pk := x.Parse(key)

		if pk.IsIndex() {
			// Seek to the end of index keys.
			it.Seek(pk.SkipRangeOfSameType())
			continue
		}
		if pk.IsReverse() {
			// Seek to the end of reverse keys.
			it.Seek(pk.SkipRangeOfSameType())
			continue
		}
		if pk.Attr == "_uid_" {
			// Skip the UID mappings.
			it.Seek(pk.SkipPredicate())
			continue
		}
		if pk.IsSchema() {
			// index values are the last keys currently
			it.Seek(pk.SkipSchema())
			continue
		}

		x.AssertTrue(pk.IsData())
		pred, uid := pk.Attr, pk.Uid
		if pred != lastPred && group.BelongsTo(pred) != gid {
			it.Seek(pk.SkipPredicate())
			continue
		}

		prefix := fmt.Sprintf("<%#x> <%s> ", uid, pred)
		pl := &types.PostingList{}
		x.Check(pl.Unmarshal(it.Value().Data()))
		chkv <- kv{
			prefix: prefix,
			list:   pl,
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
	n := Groups().Node(gid)
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
		_, addr := Groups().Leader(gid)
		addrs = append(addrs, addr)
		addrs = append(addrs, Groups().AnyServer(gid))
		_, addr = Groups().Leader(0)
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

func BackupOverNetwork(ctx context.Context) error {
	// If we haven't even had a single membership update, don't run backup.
	if len(*peerAddr) > 0 && Groups().LastUpdate() == 0 {
		x.Trace(ctx, "This server hasn't yet been fully initiated. Please retry later.")
		return x.Errorf("Uninitiated server. Please retry later")
	}
	// Let's first collect all groups.
	gids := Groups().KnownGroups()

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
			return fmt.Errorf("Backup status: %v for group id: %d", bp.Status, bp.GroupId)
		} else {
			x.Trace(ctx, "Backup successful for group: %v", bp.GroupId)
		}
	}
	x.Trace(ctx, "DONE backup")
	return nil
}
