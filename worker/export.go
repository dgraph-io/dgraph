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
	"fmt"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/trace"
)

const numExportRoutines = 100

type kv struct {
	prefix string
	key    []byte
}

type skv struct {
	attr   string
	schema *intern.SchemaUpdate
}

// Map from our types to RDF type. Useful when writing storage types
// for RDF's in export. This is the dgraph type name and rdf storage type
// might not be the same always (e.g. - datetime and bool).
var rdfTypeMap = map[types.TypeID]string{
	types.StringID:   "xs:string",
	types.DateTimeID: "xs:dateTime",
	types.IntID:      "xs:int",
	types.FloatID:    "xs:float",
	types.BoolID:     "xs:boolean",
	types.GeoID:      "geo:geojson",
	types.BinaryID:   "xs:base64Binary",
	types.PasswordID: "xs:string",
}

func toRDF(buf *bytes.Buffer, item kv, readTs uint64) {
	l := posting.GetNoStore(item.key)
	err := l.Iterate(readTs, 0, func(p *intern.Posting) bool {
		buf.WriteString(item.prefix)
		if p.PostingType != intern.Posting_REF {
			// Value posting
			// Convert to appropriate type
			vID := types.TypeID(p.ValType)
			src := types.ValueForType(vID)
			src.Value = p.Value
			str, err := types.Convert(src, types.StringID)
			x.Check(err)
			buf.WriteString(strconv.Quote(str.Value.(string)))
			if p.PostingType == intern.Posting_VALUE_LANG {
				buf.WriteByte('@')
				buf.WriteString(string(p.LangTag))
			} else if vID != types.DefaultID {
				rdfType, ok := rdfTypeMap[vID]
				x.AssertTruef(ok, "Didn't find RDF type for dgraph type: %+v", vID.Name())
				buf.WriteString("^^<")
				buf.WriteString(rdfType)
				buf.WriteByte('>')
			}
		} else {
			buf.WriteString("_:uid")
			buf.WriteString(strconv.FormatUint(p.Uid, 16))
		}
		// Label
		if len(p.Label) > 0 {
			buf.WriteString(" <")
			buf.WriteString(p.Label)
			buf.WriteByte('>')
		}
		// Facets.
		fcs := p.Facets
		if len(fcs) != 0 {
			buf.WriteString(" (")
			for i, f := range fcs {
				if i != 0 {
					buf.WriteByte(',')
				}
				buf.WriteString(f.Key)
				buf.WriteByte('=')
				fVal := &types.Val{Tid: types.StringID}
				x.Check(types.Marshal(facets.ValFor(f), fVal))
				if facets.TypeIDFor(f) == types.StringID {
					buf.WriteString(strconv.Quote(fVal.Value.(string)))
				} else {
					buf.WriteString(fVal.Value.(string))
				}
			}
			buf.WriteByte(')')
		}
		// End dot.
		buf.WriteString(" .\n")
		return true
	})
	if err != nil {
		// TODO: Throw error back to the user.
		// Ensure that we are not missing errCheck at other places.
		x.Printf("Error while exporting :%v\n", err)
	}
}

func toSchema(buf *bytes.Buffer, s *skv) {
	if strings.ContainsRune(s.attr, ':') {
		buf.WriteRune('<')
		buf.WriteString(s.attr)
		buf.WriteRune('>')
	} else {
		buf.WriteString(s.attr)
	}
	buf.WriteByte(':')
	isList := schema.State().IsList(s.attr)
	if isList {
		buf.WriteRune('[')
	}
	buf.WriteString(types.TypeID(s.schema.ValueType).Name())
	if isList {
		buf.WriteRune(']')
	}
	if s.schema.Directive == intern.SchemaUpdate_REVERSE {
		buf.WriteString(" @reverse")
	} else if s.schema.Directive == intern.SchemaUpdate_INDEX && len(s.schema.Tokenizer) > 0 {
		buf.WriteString(" @index(")
		buf.WriteString(strings.Join(s.schema.Tokenizer, ","))
		buf.WriteByte(')')
	}
	if s.schema.Count {
		buf.WriteString(" @count")
	}
	buf.WriteString(" . \n")
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

// Export creates a export of data by exporting it as an RDF gzip.
func export(bdir string, readTs uint64) error {
	// Use a goroutine to write to file.
	err := os.MkdirAll(bdir, 0700)
	if err != nil {
		return err
	}
	gid := groups().groupId()
	fpath := path.Join(bdir, fmt.Sprintf("dgraph-%d-%s.rdf.gz", gid,
		time.Now().Format("2006-01-02-15-04")))
	fspath := path.Join(bdir, fmt.Sprintf("dgraph-%d-%s.schema.gz", gid,
		time.Now().Format("2006-01-02-15-04")))
	x.Printf("Exporting to: %v, schema at %v\n", fpath, fspath)
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
				toRDF(buf, item, readTs)
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

	// Iterate over key-value store
	txn := pstore.NewTransactionAt(readTs, false)
	defer txn.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.PrefetchValues = false
	it := txn.NewIterator(iterOpts)
	defer it.Close()
	prefix := new(bytes.Buffer)
	prefix.Grow(100)
	var debugCount int
	for it.Rewind(); it.Valid(); debugCount++ {
		item := it.Item()
		key := item.Key()
		pk := x.Parse(key)
		if pk == nil {
			it.Next()
			continue
		}

		if pk.IsIndex() || pk.IsReverse() || pk.IsCount() {
			// Seek to the end of index, reverse and count keys.
			it.Seek(pk.SkipRangeOfSameType())
			continue
		}

		// Skip if we don't serve the tablet.
		if !groups().ServesTablet(pk.Attr) {
			if pk.IsData() {
				it.Seek(pk.SkipPredicate())
			} else if pk.IsSchema() {
				it.Seek(pk.SkipSchema())
			}
			continue
		}

		if pk.Attr == "_predicate_" || pk.Attr == "_dummy_" {
			// Skip the UID mappings.
			it.Seek(pk.SkipPredicate())
			continue
		}

		if pk.IsSchema() {
			s := &intern.SchemaUpdate{}
			val, err := item.Value()
			if err != nil {
				return err
			}
			x.Check(s.Unmarshal(val))
			chs <- &skv{
				attr:   pk.Attr,
				schema: s,
			}
			// skip predicate
			it.Next()
			continue
		}
		x.AssertTrue(pk.IsData())
		pred, uid := pk.Attr, pk.Uid
		prefix.WriteString("<_:uid")
		prefix.WriteString(strconv.FormatUint(uid, 16))
		prefix.WriteString("> <")
		prefix.WriteString(pred)
		prefix.WriteString("> ")
		nkey := make([]byte, len(key))
		copy(nkey, key)
		chkv <- kv{
			prefix: prefix.String(),
			key:    nkey,
		}
		prefix.Reset()
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

// TODO: How do we want to handle export for group, do we pause mutations, sync all and then export ?
// TODO: Should we move export logic to dgraphzero?
func handleExportForGroupOverNetwork(ctx context.Context, in *intern.ExportPayload) *intern.ExportPayload {
	n := groups().Node
	if in.GroupId == groups().groupId() && n != nil && n.AmLeader() {
		return handleExportForGroup(ctx, in)
	}

	pl := groups().Leader(in.GroupId)
	if pl == nil {
		// Unable to find any connection to any of these servers. This should be exceedingly rare.
		// But probably not worthy of crashing the server. We can just skip the export.
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Unable to find a server to export group: %d", in.GroupId)
		}
		in.Status = intern.ExportPayload_FAILED
		return in
	}

	c := intern.NewWorkerClient(pl.Get())
	nrep, err := c.Export(ctx, in)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		in.Status = intern.ExportPayload_FAILED
		return in
	}
	return nrep
}

func handleExportForGroup(ctx context.Context, in *intern.ExportPayload) *intern.ExportPayload {
	n := groups().Node
	if in.GroupId != groups().groupId() || !n.AmLeader() {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("I am not leader of group %d.", in.GroupId)
		}
		in.Status = intern.ExportPayload_FAILED
		return in
	}
	n.applyAllMarks(n.ctx)
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Leader of group: %d. Running export.", in.GroupId)
	}
	if err := export(Config.ExportPath, in.ReadTs); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		in.Status = intern.ExportPayload_FAILED
		return in
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Export done for group: %d.", in.GroupId)
	}
	in.Status = intern.ExportPayload_SUCCESS
	return in
}

// Export request is used to trigger exports for the request list of groups.
// If a server receives request to export a group that it doesn't handle, it would
// automatically relay that request to the server that it thinks should handle the request.
func (w *grpcWorker) Export(ctx context.Context, req *intern.ExportPayload) (*intern.ExportPayload, error) {
	reply := &intern.ExportPayload{ReqId: req.ReqId, GroupId: req.GroupId}
	reply.Status = intern.ExportPayload_FAILED // Set by default.

	if ctx.Err() != nil {
		return reply, ctx.Err()
	}
	if !w.addIfNotPresent(req.ReqId) {
		reply.Status = intern.ExportPayload_DUPLICATE
		return reply, nil
	}

	chb := make(chan *intern.ExportPayload, 1)
	go func() {
		chb <- handleExportForGroup(ctx, req)
	}()

	select {
	case rep := <-chb:
		return rep, nil
	case <-ctx.Done():
		return reply, ctx.Err()
	}
}

func ExportOverNetwork(ctx context.Context) error {
	// If we haven't even had a single membership update, don't run export.
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return err
	}
	// Get ReadTs from zero and wait for stream to catch up.
	ts, err := Timestamps(ctx, &intern.Num{Val: 1})
	if err != nil {
		return err
	}
	readTs := ts.StartId
	posting.Oracle().WaitForTs(ctx, readTs)

	// Let's first collect all groups.
	gids := groups().KnownGroups()

	ch := make(chan *intern.ExportPayload, len(gids))
	for _, gid := range gids {
		go func(group uint32) {
			req := &intern.ExportPayload{
				ReqId:   uint64(rand.Int63()),
				GroupId: group,
				ReadTs:  readTs,
			}
			ch <- handleExportForGroupOverNetwork(ctx, req)
		}(gid)
	}

	for i := 0; i < len(gids); i++ {
		bp := <-ch
		if bp.Status != intern.ExportPayload_SUCCESS {
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
