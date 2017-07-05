/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package worker

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/trace"
)

// Export creates a export of data by exporting it as an RDF gzip.
func backup(gid uint32, bdir string) error {
	// Use a goroutine to write to file.
	err := os.MkdirAll(bdir, 0700)
	if err != nil {
		return err
	}
	fpath := path.Join(bdir, fmt.Sprintf("dgraph-%d-%s.rdf.gz", gid,
		time.Now().Format("2006-01-02-15-04")))
	fspath := path.Join(bdir, fmt.Sprintf("dgraph-schema-%d-%s.rdf.gz", gid,
		time.Now().Format("2006-01-02-15-04")))
	fmt.Printf("Exporting to: %v, schema at %v\n", fpath, fspath)
	chb := make(chan []byte, 1000)
	errChan := make(chan error, 2)
	go func() {
		errChan <- writeToFile(fpath, chb)
	}()
	chsb := make(chan []byte, 1000)
	go func() {
		errChan <- writeToFile(fspath, chsb)
	}()

	// Use a bunch of goroutines to convert to RDF format.
	chkv := make(chan kv, 1000)
	var wg sync.WaitGroup
	wg.Add(numExportRoutines)
	for i := 0; i < numExportRoutines; i++ {
		go func(i int) {
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
		}(i)
	}

	// Use a goroutine to convert protos.Schema to string
	chs := make(chan *skv, 1000)
	wg.Add(1)
	go func() {
		buf := new(bytes.Buffer)
		buf.Grow(50000)
		for item := range chs {
			toSchema(buf, item)
			if buf.Len() >= 40000 {
				tmp := make([]byte, buf.Len())
				copy(tmp, buf.Bytes())
				chsb <- tmp
				buf.Reset()
			}
		}
		if buf.Len() > 0 {
			tmp := make([]byte, buf.Len())
			copy(tmp, buf.Bytes())
			chsb <- tmp
		}
		wg.Done()
	}()

	// Iterate over rocksdb.
	it := pstore.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	var lastPred string
	prefix := new(bytes.Buffer)
	prefix.Grow(100)
	var debugCount int
	for it.Rewind(); it.Valid(); debugCount++ {
		item := it.Item()
		key := item.Key()
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
		if pk.Attr == "_uid_" || pk.Attr == "_predicate_" ||
			pk.Attr == "_lease_" {
			// Skip the UID mappings.
			it.Seek(pk.SkipPredicate())
			continue
		}
		if pk.IsSchema() {
			if group.BelongsTo(pk.Attr) == gid {
				s := &protos.SchemaUpdate{}
				x.Check(s.Unmarshal(item.Value()))
				chs <- &skv{
					attr:   pk.Attr,
					schema: s,
				}
			}
			// skip predicate
			it.Next()
			continue
		}
		x.AssertTrue(pk.IsData())
		pred, uid := pk.Attr, pk.Uid
		if pred != lastPred && group.BelongsTo(pred) != gid {
			it.Seek(pk.SkipPredicate())
			continue
		}

		prefix.WriteString("<_:uid")
		prefix.WriteString(strconv.FormatUint(uid, 16))
		prefix.WriteString("> <")
		prefix.WriteString(pred)
		prefix.WriteString("> ")
		pl := &protos.PostingList{}
		x.Check(pl.Unmarshal(item.Value()))
		chkv <- kv{
			prefix: prefix.String(),
			list:   pl,
		}
		prefix.Reset()
		lastPred = pred
		it.Next()
	}

	close(chkv) // We have stopped output to chkv.
	close(chs)  // we have stopped output to chs (schema)
	wg.Wait()   // Wait for numExportRoutines to finish.
	close(chb)  // We have stopped output to chb.
	close(chsb) // we have stopped output to chs (schema)

	err = <-errChan
	err = <-errChan
	return err
}

func handleExportForGroup(ctx context.Context, reqId uint64, gid uint32) *protos.ExportPayload {
	n := groups().Node(gid)
	if n.AmLeader() {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Leader of group: %d. Running export.", gid)
		}
		if err := export(gid, *exportPath); err != nil {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf(err.Error())
			}
			return &protos.ExportPayload{
				ReqId:  reqId,
				Status: protos.ExportPayload_FAILED,
			}
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Export done for group: %d.", gid)
		}
		return &protos.ExportPayload{
			ReqId:   reqId,
			Status:  protos.ExportPayload_SUCCESS,
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
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Relaying export request for group %d to %q", gid, pl.Addr)
			}
			defer pl.Put(conn)
			break
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
	}

	// Unable to find any connection to any of these servers. This should be exceedingly rare.
	// But probably not worthy of crashing the server. We can just skip the export.
	if conn == nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Unable to find a server to export group: %d", gid)
		}
		return &protos.ExportPayload{
			ReqId:   reqId,
			Status:  protos.ExportPayload_FAILED,
			GroupId: gid,
		}
	}

	c := protos.NewWorkerClient(conn)
	nr := &protos.ExportPayload{
		ReqId:   reqId,
		GroupId: gid,
	}
	nrep, err := c.Export(ctx, nr)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return &protos.ExportPayload{
			ReqId:   reqId,
			Status:  protos.ExportPayload_FAILED,
			GroupId: gid,
		}
	}
	return nrep
}

func Backup(ctx context.Context) error {
	// If we haven't even had a single membership update, don't run export.
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return err
	}
	// Let's first collect all groups.
	gids := groups().KnownGroups()

	for i, gid := range gids {
		if gid == 0 {
			gids[i] = gids[len(gids)-1]
			gids = gids[:len(gids)-1]
		}
	}

	ch := make(chan *protos.ExportPayload, len(gids))
	for _, gid := range gids {
		go func(group uint32) {
			reqId := uint64(rand.Int63())
			ch <- handleExportForGroup(ctx, reqId, group)
		}(gid)
	}

	for i := 0; i < len(gids); i++ {
		bp := <-ch
		if bp.Status != protos.ExportPayload_SUCCESS {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Export status: %v for group id: %d", bp.Status, bp.GroupId)
			}
			return fmt.Errorf("Export status: %v for group id: %d", bp.Status, bp.GroupId)
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Export successful for group: %v", bp.GroupId)
		}
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("DONE export")
	}
	return nil
}
