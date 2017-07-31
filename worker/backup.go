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
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/trace"
)

func handleBackupForGroup(ctx context.Context, gid uint32, bdir string) error {
	if err := os.MkdirAll(bdir, 0700); err != nil {
		return err
	}

	curTime := time.Now().Format("2006-01-02-15-04")
	fpath := path.Join(bdir, fmt.Sprintf("dgraph-backup-%d-%s.bin.gz", gid, curTime))
	x.Printf("Backing up to: %v\n", fpath)

	chb := make(chan []byte, 1000)
	che := make(chan error, 1)
	go func(fpath string, chb chan []byte) {
		che <- writeToFile(fpath, chb)
	}(fpath, chb)

	blen := make([]byte, 4)
	buf := new(bytes.Buffer)
	buf.Grow(50000)

	it := pstore.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	count := 0
	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		key := item.Key()
		pk := x.Parse(key)
		if g := group.BelongsTo(pk.Attr); g != gid {
			continue
		}
		count++
		k := item.Key()
		v := item.Value()
		binary.LittleEndian.PutUint32(blen, uint32(len(k)))
		buf.Write(blen)
		buf.Write(k)
		binary.LittleEndian.PutUint32(blen, uint32(len(v)))
		buf.Write(blen)
		buf.Write(v)
		if buf.Len() >= 40000 {
			tmp := make([]byte, buf.Len())
			copy(tmp, buf.Bytes())
			chb <- tmp
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		// No need to copy as we are not going to use the buffer anymore.
		chb <- buf.Bytes()
	}
	fmt.Println("Count", count)
	close(chb)
	return <-che
}

func Backup(ctx context.Context) error {
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return err
	}
	if err := syncAllMarks(ctx); err != nil {
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

	che := make(chan error, len(gids))
	for _, gid := range gids {
		go func(group uint32) {
			che <- handleBackupForGroup(ctx, group, Config.BackupPath)
		}(gid)
	}

	for i := 0; i < len(gids); i++ {
		err := <-che
		if err != nil {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Backup error: %v for group id: %d", err, gids[i])
			}
			return fmt.Errorf("Backup error: %v for group id: %d", err, gids[i])
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Backup successful for group: %v", gids[i])
		}
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("DONE backup")
	}
	return nil
}
