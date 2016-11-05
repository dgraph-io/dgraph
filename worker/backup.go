package worker

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"math"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	stype "github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const numBackupRoutines = 10

type kv struct {
	key, value []byte
}

func toRDF(buf *bytes.Buffer, item kv, ch chan []byte) {
	buf.Reset()
	p := new(types.Posting)
	pre := item.key

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

			_, err = buf.WriteString(fmt.Sprintf("\"%s\"", str))
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
		if buf.Len() >= 50000 {
			ch <- buf.Bytes()
			buf.Reset()
		}
	}
	ch <- buf.Bytes()
}

func writeToFile(fpath string, ch chan []byte, errChan chan error) {
	err := os.MkdirAll(fpath, 0700)
	if err != nil {
		errChan <- err
	}
	f, err := os.Create(file)
	if err != nil {
		errChan <- err
	}
	defer f.Close()
	x.Check(err)
	w := bufio.NewWriterSize(f, 1000000)
	gw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		errChan <- err
		return
	}

	for item := range ch {
		gw.Write(item)
	}
	gw.Flush()
	gw.Close()
	w.Flush()
	errChan <- nil
}

// Backup creates a backup of data by exporting it as an RDF gzip.
func backup(gid uint32, bdir string) error {
	// Use a goroutine to write to file.
	fpath := path.Join(bdir, fmt.Sprintf("dgraph-%d-%s.gz", gid,
		time.Now().Format("2006-01-02-15-04")))
	chb := make(chan []byte, 1000)
	errChan := make(chan error, 1)
	go writeToFile(fpath, chb, errChan)

	// Use a bunch of goroutines to convert to RDF format.
	chkv := make(chan kv, 1000)
	var wg sync.WaitGroup
	wg.Add(numBackupRoutines)
	for i := 0; i < numBackupRoutines; i++ {
		go func() {
			buf := new(bytes.Buffer)
			for item := range chkv {
				toRDF(buf, item, chb)
			}
			wg.Done()
		}()
	}

	uidPrefix = []byte("_uid_")
	// Iterate over rocksdb.
	it := pstore.NewIterator()
	defer it.Close()
	var lastPred string
	for it.SeekToFirst(); it.Valid(); {
		key := it.Key().Data()
		cidx := bytes.IndexRune(key, ':')
		if cidx > -1 {
			// Seek to the end of index keys.
			pre := key[:cidx]
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

	close(chkv)
	// Wait for numBackupRoutines to finish.
	wg.Wait()
	close(ch)

	err := <-errChan
	return err
}

func handleRPCForGroup(ctx context.Context, reqId uint64, gid uint32) *BackupPayload {
	nid := reqId & uint64(0xffffffff00000000)
	nid += uint64(gid)
	reply := &BackupPayload{ReqId: nid}

	n := groups().Node(gid)
	if n.AmLeader() {
		x.Trace(ctx, "Leader of group: %d. Running backup.", gid)
		// TODO: Do this.
		// che <- backup(gid)
		x.Trace(ctx, "Backup done for group: %d.", gid)
		return reply
	}

	// I'm not the leader. Relay to someone who I think is.
	_, addr := groups().Leader(gid)
	var conn *grpc.ClientConn
	for i := 0; i < 3; i++ {
		pl := pools().get(addr)
		var err error
		conn, err = pl.Get()
		if err == nil {
			defer pl.Put(conn)
			break
		}
		x.TraceError(ctx, err)
		addr = groups().AnyServer(gid)
	}
	if conn == nil {
		x.Trace(ctx, fmt.Sprintf("Unable to find a server to backup group: %d", gid))
		reply.Status = BackupPayload_FAILED
		return reply
	}

	c := NewWorkerClient(conn)
	nr.Groups = append(nr.Groups, gid)
	nrep, err := c.Backup(ctx, nr)
	if err != nil {
		x.TraceError(ctx, err)
		reply.Status = BackupPayload_FAILED
		return reply
	}
	return nrep
}

// Backup request is used to trigger backups for the request list of groups.
// If a server receives request to backup a group that it doesn't handle, it would
// automatically relay that request to the server that it thinks should handle the request.
// The response would contain the list of groups that were backed up.
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

	chb := make(chan *BackupPayload, len(req.Groups))
	for _, gid := range req.Groups {
		go func() {
			chb <- backupGroup(gid)
		}()
	}
	for i := 0; i < len(req.Groups); i++ {
		select {
		case rep := <-chb:
			if rep.Status == BackupPayload_SUCCESS {
				reply.Groups = append(reply.Groups, rep.Groups...)
			}
		case ctx.Done():
			return reply, ctx.Err()
		}
	}

	if len(reply.Groups) > 0 {
		reply.Status = BackupPayload_SUCCESS
	}
	return reply, nil
}
