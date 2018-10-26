/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

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

func toRDF(pl *posting.List, prefix string, readTs uint64) (*pb.KV, error) {
	var buf bytes.Buffer

	err := pl.Iterate(readTs, 0, func(p *pb.Posting) error {
		buf.WriteString(prefix)
		if p.PostingType == pb.Posting_REF {
			// TODO: Weird that it doesn't need brackets?
			buf.WriteString("_:uid")
			buf.WriteString(strconv.FormatUint(p.Uid, 16))

		} else {
			// Value posting
			// Convert to appropriate type
			vID := types.TypeID(p.ValType)
			src := types.ValueForType(vID)
			src.Value = p.Value
			str, err := types.Convert(src, types.StringID)
			if err != nil {
				glog.Errorf("While converting %v to string. Err=%v. Ignoring.\n", src, err)
				return nil
			}

			// trim null character at end
			trimmed := strings.TrimRight(str.Value.(string), "\x00")
			buf.WriteString(strconv.Quote(trimmed))
			if p.PostingType == pb.Posting_VALUE_LANG {
				buf.WriteByte('@')
				buf.WriteString(string(p.LangTag))

			} else if vID != types.DefaultID {
				rdfType, ok := rdfTypeMap[vID]
				x.AssertTruef(ok, "Didn't find RDF type for dgraph type: %+v", vID.Name())
				buf.WriteString("^^<")
				buf.WriteString(rdfType)
				buf.WriteByte('>')
			}
		}
		// Let's skip labels. Dgraph doesn't support them for any functionality.

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
		return nil
	})
	kv := &pb.KV{
		Val:     buf.Bytes(), // Don't think we need to copy these, because buf is not being reused.
		Version: 1,           // Data value.
	}
	return kv, err
}

func toSchema(attr string, update pb.SchemaUpdate) (*pb.KV, error) {
	// bytes.Buffer never returns error for any of the writes. So, we don't need to check them.
	var buf bytes.Buffer
	if strings.ContainsRune(attr, ':') {
		buf.WriteRune('<')
		buf.WriteString(attr)
		buf.WriteRune('>')
	} else {
		buf.WriteString(attr)
	}
	buf.WriteByte(':')
	if update.List {
		buf.WriteRune('[')
	}
	buf.WriteString(types.TypeID(update.ValueType).Name())
	if update.List {
		buf.WriteRune(']')
	}
	if update.Directive == pb.SchemaUpdate_REVERSE {
		buf.WriteString(" @reverse")
	} else if update.Directive == pb.SchemaUpdate_INDEX && len(update.Tokenizer) > 0 {
		buf.WriteString(" @index(")
		buf.WriteString(strings.Join(update.Tokenizer, ","))
		buf.WriteByte(')')
	}
	if update.Count {
		buf.WriteString(" @count")
	}
	if update.Lang {
		buf.WriteString(" @lang")
	}
	if update.Upsert {
		buf.WriteString(" @upsert")
	}
	buf.WriteString(" . \n")
	kv := &pb.KV{
		Val:     buf.Bytes(),
		Version: 2, // Schema value
	}
	return kv, nil
}

type fileWriter struct {
	fd *os.File
	bw *bufio.Writer
	gw *gzip.Writer
}

func (writer *fileWriter) open(fpath string) error {
	var err error
	writer.fd, err = os.Create(fpath)
	if err != nil {
		return err
	}

	writer.bw = bufio.NewWriterSize(writer.fd, 1e6)
	writer.gw, err = gzip.NewWriterLevel(writer.bw, gzip.BestCompression)
	return err
}

func (writer *fileWriter) Close() error {
	if err := writer.gw.Flush(); err != nil {
		return err
	}
	if err := writer.gw.Close(); err != nil {
		return err
	}
	if err := writer.bw.Flush(); err != nil {
		return err
	}
	if err := writer.fd.Sync(); err != nil {
		return err
	}
	return writer.fd.Close()
}

type writerMux struct {
	data   *fileWriter
	schema *fileWriter
}

func (mux *writerMux) Send(kvs *pb.KVS) error {
	for _, kv := range kvs.Kv {
		var writer *fileWriter
		switch kv.Version {
		case 1: // data
			writer = mux.data
		case 2: // schema
			writer = mux.schema
		default:
			glog.Fatalf("Invalid data type found: %x", kv.Key)
		}
		if _, err := writer.gw.Write(kv.Val); err != nil {
			return err
		}
	}
	// Once all the sends are done, writers must be flushed and closed in order.
	return nil
}

// export creates a export of data by exporting it as an RDF gzip.
func export(ctx context.Context, in *pb.ExportPayload) error {
	if in.GroupId != groups().groupId() {
		return x.Errorf("Export request group mismatch. Mine: %d. Requested: %d\n",
			groups().groupId(), in.GroupId)
	}
	glog.Infof("Export requested at %d.", in.ReadTs)

	// Let's wait for this server to catch up to all the updates until this ts.
	if err := posting.Oracle().WaitForTs(ctx, in.ReadTs); err != nil {
		return err
	}
	glog.Infof("Running export for group %d at timestamp %d.", in.GroupId, in.ReadTs)

	uts := time.Unix(in.UnixTs, 0)
	bdir := path.Join(Config.ExportPath, fmt.Sprintf(
		"dgraph.r%06d.u%s", in.ReadTs, uts.UTC().Format("0102.1504")))

	if err := os.MkdirAll(bdir, 0700); err != nil {
		return err
	}
	path := func(suffix string) (string, error) {
		return filepath.Abs(path.Join(bdir, fmt.Sprintf("g%02d.%s", in.GroupId, suffix)))
	}

	// Open data file now.
	dataPath, err := path("rdf.gz")
	if err != nil {
		return err
	}
	glog.Infof("Exporting data for group: %d at %s\n", in.GroupId, dataPath)
	dataWriter := &fileWriter{}
	if err := dataWriter.open(dataPath); err != nil {
		return err
	}

	// Open schema file now.
	schemaPath, err := path("schema.gz")
	if err != nil {
		return err
	}
	glog.Infof("Exporting schema for group: %d at %s\n", in.GroupId, schemaPath)
	schemaWriter := &fileWriter{}
	if err := schemaWriter.open(schemaPath); err != nil {
		return err
	}

	mux := writerMux{data: dataWriter, schema: schemaWriter}
	sl := streamLists{stream: &mux, db: pstore}
	sl.chooseKey = func(item *badger.Item) bool {
		// Pick all keys.
		pk := x.Parse(item.Key())
		if pk.Attr == "_predicate_" {
			return false
		}
		if !groups().ServesTablet(pk.Attr) {
			return false
		}
		// We need to ensure that schema keys are separately identifiable, so they can be
		// written to a different file.
		return pk.IsData() || pk.IsSchema()
	}
	sl.itemToKv = func(key []byte, itr *badger.Iterator) (*pb.KV, error) {
		item := itr.Item()
		pk := x.Parse(item.Key())

		switch {
		case pk.IsSchema():
			// Schema should be handled first. Because schema keys are also considered data keys.
			var update pb.SchemaUpdate
			err := item.Value(func(val []byte) error {
				return update.Unmarshal(val)
			})
			if err != nil {
				// Let's not propagate this error. We just log this and continue onwards.
				glog.Errorf("Unable to unmarshal schema: %+v. Err=%v\n", pk, err)
				// TODO: Ensure that itemToKV can return a nil pb.KV.
				return nil, nil
			}
			return toSchema(pk.Attr, update)

		case pk.IsData():
			prefix := fmt.Sprintf("<_:uid%d> <%s> ", pk.Uid, pk.Attr)
			pl, err := posting.ReadPostingList(key, itr)
			if err != nil {
				return nil, err
			}
			return toRDF(pl, prefix, in.ReadTs)

		default:
			glog.Fatalf("Invalid key found: %+v\n", pk)
		}
		return nil, nil
	}

	// All prepwork done. Time to roll.
	if err := sl.orchestrate(ctx, "Export", in.ReadTs); err != nil {
		return err
	}
	if err := mux.data.Close(); err != nil {
		return err
	}
	if err := mux.schema.Close(); err != nil {
		return err
	}
	glog.Infof("Export DONE for group %d at timestamp %d.", in.GroupId, in.ReadTs)
	return nil
}

// Export request is used to trigger exports for the request list of groups.
// If a server receives request to export a group that it doesn't handle, it would
// automatically relay that request to the server that it thinks should handle the request.
func (w *grpcWorker) Export(ctx context.Context, req *pb.ExportPayload) (*pb.ExportPayload, error) {
	glog.Infof("Received export request via Grpc: %+v\n", req)
	reply := proto.Clone(req).(*pb.ExportPayload)
	reply.Status = pb.ExportPayload_FAILED // Set by default.

	if ctx.Err() != nil {
		glog.Errorf("Context error during export: %v\n", ctx.Err())
		return reply, ctx.Err()
	}

	glog.Infof("Issuing export request...")
	if err := export(ctx, req); err != nil {
		glog.Errorf("While running export. Request: %+v. Error=%v\n", req, err)
		return nil, err
	}
	glog.Infof("Export request: %+v OK.\n", req)
	return &pb.ExportPayload{Status: pb.ExportPayload_SUCCESS}, nil
}

func handleExportOverNetwork(ctx context.Context, in *pb.ExportPayload) error {
	if in.GroupId == groups().groupId() {
		return export(ctx, in)
	}

	pl := groups().Leader(in.GroupId)
	if pl == nil {
		return x.Errorf("Unable to find leader of group: %d\n", in.GroupId)
	}

	glog.Infof("Sending export request to group: %d, addr: %s\n", in.GroupId, pl.Addr)
	c := pb.NewWorkerClient(pl.Get())
	_, err := c.Export(ctx, in)
	if err != nil {
		glog.Errorf("Export error received from group: %d. Error: %v\n", in.GroupId, err)
	}
	return err
}

func ExportOverNetwork(ctx context.Context) error {
	// If we haven't even had a single membership update, don't run export.
	if err := x.HealthCheck(); err != nil {
		glog.Errorf("Rejecting export request due to health check error: %v\n", err)
		return err
	}
	// Get ReadTs from zero and wait for stream to catch up.
	ts, err := Timestamps(ctx, &pb.Num{ReadOnly: true})
	if err != nil {
		glog.Errorf("Unable to retrieve readonly ts for export: %v\n", err)
		return err
	}
	readTs := ts.ReadOnly
	glog.Infof("Got readonly ts from Zero: %d\n", readTs)

	// Let's first collect all groups.
	gids := groups().KnownGroups()
	glog.Infof("Requesting export for groups: %v\n", gids)

	ch := make(chan error, len(gids))
	for _, gid := range gids {
		go func(group uint32) {
			req := &pb.ExportPayload{
				GroupId: group,
				ReadTs:  readTs,
				UnixTs:  time.Now().Unix(),
			}
			ch <- handleExportOverNetwork(ctx, req)
		}(gid)
	}

	for i := 0; i < len(gids); i++ {
		err := <-ch
		if err != nil {
			rerr := fmt.Errorf("Export failed at readTs %d. Err=%v", readTs, err)
			glog.Errorln(rerr)
			return rerr
		}
	}
	glog.Infof("Export at readTs %d DONE", readTs)
	return nil
}
