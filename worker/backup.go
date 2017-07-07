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

// Export creates a export of data by exporting it as an RDF gzip.
func backup(gid uint32, bdir string) error {
	// Use a goroutine to write to file.
	err := os.MkdirAll(bdir, 0700)
	fmt.Println(bdir)
	if err != nil {
		return err
	}
	fpath := path.Join(bdir, fmt.Sprintf("dgraph-%d-%s.rdf.gz", gid,
		time.Now().Format("2006-01-02-15-04")))
	fmt.Printf("Backing up to: %v", fpath)
	chb := make(chan []byte, 1000)
	errChan := make(chan error, 2)
	go func() {
		errChan <- writeToFile(fpath, chb)
	}()

	b := make([]byte, 4)
	// Iterate over the store.
	it := pstore.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	prefix := new(bytes.Buffer)
	prefix.Grow(100)
	var debugCount int
	for it.Rewind(); it.Valid(); debugCount++ {
		item := it.Item()
		key := item.Key()
		pk := x.Parse(key)

		if group.BelongsTo(pk.Attr) == gid {
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

		it.Next()
	}

	close(chb) // We have stopped output to chb.

	err = <-errChan
	return err
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

	for i := 0; i < len(gids); i++ {
		if err := backup(gids[i], *backupPath); err != nil {
			return err
		}
	}
	return nil
}
