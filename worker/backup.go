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
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/trace"
)

func writeBackupToFile(f *os.File, ch chan []byte) error {
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

func handleBackupForGroup(ctx context.Context, gid uint32, bdir string) *protos.ExportPayload {
	reply := &protos.ExportPayload{
		Status: protos.ExportPayload_FAILED,
	}

	if ctx.Err() != nil {
		return reply, ctx.Err()
	}

	// Use a goroutine to write to file.
	if err := os.MkdirAll(bdir, 0700); err != nil {
		return err
	}
	fmt.Println(bdir)

	curTime := time.Now().Format("2006-01-02-15-04")
	fpath := path.Join(bdir, fmt.Sprintf("dgraph-backup-%d-%s.rdf.gz", gid, curTime))
	var allPaths []string
	for _, gid := range gids {
		fmt.Printf("Backing up to: %v", fpath)
		chb := make(chan []byte, 1000)
		f, err := os.Create(fpath)
		if err != nil {
			return err
		}
		allPaths = append(allPaths, fpath)
		fileMap[gid] = chb
		go func(f *os.File, chb chan []byte) {
			errChan <- writeBackupToFile(f, chb)
		}(f, chb)
		defer f.Close()
	}

	b := make([]byte, 4)
	// Iterate over the store.
	it := pstore.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	prefix := new(bytes.Buffer)
	prefix.Grow(100)
	var debugCount int
	for it.Rewind(); it.Valid(); debugCount++; it.Next() {
		item := it.Item()
		key := item.Key()
		pk := x.Parse(key)

		if g := group.BelongsTo(pk.Attr); g != gid {
			continue
		}
			k := item.Key()
			v := item.Value()
			binary.LittleEndian.PutUint32(b, uint32(len(k)))
			prefix.Write(b)
			prefix.Write(k)
			binary.LittleEndian.PutUint32(b, uint32(len(v)))
			prefix.Write(b)
			prefix.Write(v)
			prefix.WriteRune('\n')
			chb <- prefix.Bytes()
			prefix.Reset()
	}

	for _, v := range fileMap {
		close(v) // We have stopped output to chb.
		if err := <-errChan; err != nil {
			return err
		}
	}
	return nil
}

func Backup(ctx context.Context) error {
	// If we haven't even had a single membership update, don't run export.
	if err := syncAllMarks(ctx); err != nil {
		return err
	}
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return err
	}
	// Let's first collect all groups.
	gids := groups().KnownGroups()

	for i := 0; i < len(gids); i++ {
		gid := gids[i]
		n := groups().Node(gid)
		if !n.AmLeader() || gid == 0 || !groups().ServesGroup(gid) {
			gids[i] = gids[len(gids)-1]
			gids = gids[:len(gids)-1]
			i--
		}
	}
	fmt.Println(gids)

	// TODO - Maybe later change to BackupPayload
	ch := make(chan *protos.ExportPayload, len(gids))
	for _, gid := range gids {
		go func(group uint32) {
			ch <- handleBackupForGroup(ctx, group)
		}(gid)
	}

	for i := 0; i < len(gids); i++ {
		bp := <-ch
		if bp.Status != protos.ExportPayload_SUCCESS {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Backup status: %v for group id: %d", bp.Status, bp.GroupId)
			}
			return fmt.Errorf("Backup status: %v for group id: %d", bp.Status, bp.GroupId)
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Backup successful for group: %v", bp.GroupId)
		}
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("DONE export")
	}
	return nil
}
