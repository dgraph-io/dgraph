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
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"

	"github.com/dgraph-io/badger"
	bpb "github.com/dgraph-io/badger/pb"

	"github.com/dgraph-io/dgo/protos/api"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

// DefaultExportFormat stores the name of the default format for exports.
const DefaultExportFormat = "rdf"

type exportFormat struct {
	ext  string // file extension
	pre  string // string to write before exported records
	post string // string to write after exported records
}

var exportFormats = map[string]exportFormat{
	"json": {
		ext:  ".json",
		pre:  "[\n",
		post: "\n]\n",
	},
	"rdf": {
		ext:  ".rdf",
		pre:  "",
		post: "",
	},
}

type exporter struct {
	pl      *posting.List
	uid     uint64
	attr    string
	readTs  uint64
	counter int
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
	types.PasswordID: "xs:password",
}

// Having '<' and '>' around all predicates makes the exported schema harder
// for humans to look at, so only put them on predicates containing "exotic"
// characters (i.e. ones not in this list).
var predNonSpecialChars = unicode.RangeTable{
	R16: []unicode.Range16{
		// Ranges must be in order.
		{'.', '.', 1},
		{'0', '9', 1},
		{'A', 'Z', 1},
		{'_', '_', 1},
		{'a', 'z', 1},
	},
}

// UIDs like 0x1 look weird but 64-bit ones like 0x0000000000000001 are too long.
var uidFmtStrRdf = "<0x%x>"
var uidFmtStrJson = "\"0x%x\""

// valToStr converts a posting value to a string.
func valToStr(v types.Val) (string, error) {
	v2, err := types.Convert(v, types.StringID)
	if err != nil {
		return "", fmt.Errorf("converting %v to string: %v\n", v2.Value, err)
	}

	// Strip terminating null, if any.
	return strings.TrimRight(v2.Value.(string), "\x00"), nil
}

// facetToString convert a facet value to a string.
func facetToString(fct *api.Facet) (string, error) {
	v1, err := facets.ValFor(fct)
	if err != nil {
		return "", fmt.Errorf("getting value from facet %#v: %v", fct, err)
	}

	v2 := &types.Val{Tid: types.StringID}
	if err = types.Marshal(v1, v2); err != nil {
		return "", fmt.Errorf("marshaling facet value %v to string: %v", v1, err)
	}

	return v2.Value.(string), nil
}

// escapedString converts a string into an escaped string for exports.
func escapedString(str string) string {
	// We use the Marshal function in the JSON package for all export formats
	// because it properly escapes strings.
	byt, err := json.Marshal(str)
	if err != nil {
		// All valid stings should be able to be escaped to a JSON string so
		// it's safe to panic here. Marshal has to return an error because it
		// accepts an interface.
		panic("Could not marshal string to JSON string")
	}
	return string(byt)
}

func (e *exporter) toJSON() (*bpb.KVList, error) {
	bp := new(bytes.Buffer)

	if e.counter != 1 {
		fmt.Fprint(bp, ",\n")
	}

	// We could output more compact JSON at the cost of code complexity.
	// Leaving it simple for now.

	continuing := false
	mapStart := fmt.Sprintf("  {\"uid\":"+uidFmtStrJson, e.uid)
	err := e.pl.Iterate(e.readTs, 0, func(p *pb.Posting) error {
		if continuing {
			fmt.Fprint(bp, ",\n")
		} else {
			continuing = true
		}

		fmt.Fprint(bp, mapStart)
		if p.PostingType == pb.Posting_REF {
			fmt.Fprintf(bp, `,"%s":[`, e.attr)
			fmt.Fprintf(bp, "{\"uid\":"+uidFmtStrJson+"}", p.Uid)
			fmt.Fprint(bp, "]")
		} else {
			if p.PostingType == pb.Posting_VALUE_LANG {
				fmt.Fprintf(bp, `,"%s@%s":`, e.attr, string(p.LangTag))
			} else {
				fmt.Fprintf(bp, `,"%s":`, e.attr)
			}

			val := types.Val{Tid: types.TypeID(p.ValType), Value: p.Value}
			str, err := valToStr(val)
			if err != nil {
				// Copying this behavior from RDF exporter.
				// TODO Investigate why returning here before before completely
				//      exporting this posting is not considered data loss.
				glog.Errorf("Ignoring error: %+v\n", err)
				return nil
			}

			if !val.Tid.IsNumber() {
				str = escapedString(str)
			}

			fmt.Fprint(bp, str)
		}

		for _, fct := range p.Facets {
			fmt.Fprintf(bp, `,"%s|%s":`, e.attr, fct.Key)

			str, err := facetToString(fct)
			if err != nil {
				glog.Errorf("Ignoring error: %+v", err)
				return nil
			}

			tid, err := facets.TypeIDFor(fct)
			if err != nil {
				glog.Errorf("Error getting type id from facet %#v: %v", fct, err)
				continue
			}

			if !tid.IsNumber() {
				str = escapedString(str)
			}

			fmt.Fprint(bp, str)
		}

		fmt.Fprint(bp, "}")
		return nil
	})

	kv := &bpb.KV{
		Value:   bp.Bytes(),
		Version: 1,
	}
	return listWrap(kv), err
}

func (e *exporter) toRDF() (*bpb.KVList, error) {
	bp := new(bytes.Buffer)

	prefix := fmt.Sprintf(uidFmtStrRdf+" <%s> ", e.uid, e.attr)
	err := e.pl.Iterate(e.readTs, 0, func(p *pb.Posting) error {
		fmt.Fprint(bp, prefix)
		if p.PostingType == pb.Posting_REF {
			fmt.Fprint(bp, fmt.Sprintf(uidFmtStrRdf, p.Uid))
		} else {
			val := types.Val{Tid: types.TypeID(p.ValType), Value: p.Value}
			str, err := valToStr(val)
			if err != nil {
				glog.Errorf("Ignoring error: %+v\n", err)
				return nil
			}
			fmt.Fprintf(bp, "%s", escapedString(str))

			tid := types.TypeID(p.ValType)
			if p.PostingType == pb.Posting_VALUE_LANG {
				fmt.Fprint(bp, "@"+string(p.LangTag))
			} else if tid != types.DefaultID {
				rdfType, ok := rdfTypeMap[tid]
				x.AssertTruef(ok, "Didn't find RDF type for dgraph type: %+v", tid.Name())
				fmt.Fprint(bp, "^^<"+rdfType+">")
			}
		}
		// Let's skip labels. Dgraph doesn't support them for any functionality.

		// Facets.
		if len(p.Facets) != 0 {
			fmt.Fprint(bp, " (")
			for i, fct := range p.Facets {
				if i != 0 {
					fmt.Fprint(bp, ",")
				}
				fmt.Fprint(bp, fct.Key+"=")

				str, err := facetToString(fct)
				if err != nil {
					glog.Errorf("Ignoring error: %+v", err)
					return nil
				}

				tid, err := facets.TypeIDFor(fct)
				if err != nil {
					glog.Errorf("Error getting type id from facet %#v: %v", fct, err)
					continue
				}

				if tid == types.StringID {
					str = escapedString(str)
				}
				fmt.Fprint(bp, str)
			}
			fmt.Fprint(bp, ")")
		}
		// End dot.
		fmt.Fprint(bp, " .\n")
		return nil
	})

	kv := &bpb.KV{
		Value:   bp.Bytes(),
		Version: 1,
	}
	return listWrap(kv), err
}

func toSchema(attr string, update pb.SchemaUpdate) (*bpb.KVList, error) {
	// bytes.Buffer never returns error for any of the writes. So, we don't need to check them.
	var buf bytes.Buffer
	isSpecial := func(r rune) bool {
		return !(unicode.In(r, &predNonSpecialChars))
	}
	if strings.IndexFunc(attr, isSpecial) >= 0 {
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
	kv := &bpb.KV{
		Value:   buf.Bytes(),
		Version: 2, // Schema value
	}
	return listWrap(kv), nil
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

// export creates a export of data by exporting it as an RDF gzip.
func export(ctx context.Context, in *pb.ExportRequest) error {
	if in.GroupId != groups().groupId() {
		return errors.Errorf("Export request group mismatch. Mine: %d. Requested: %d\n",
			groups().groupId(), in.GroupId)
	}
	glog.Infof("Export requested at %d.", in.ReadTs)

	// Let's wait for this server to catch up to all the updates until this ts.
	if err := posting.Oracle().WaitForTs(ctx, in.ReadTs); err != nil {
		return err
	}
	glog.Infof("Running export for group %d at timestamp %d.", in.GroupId, in.ReadTs)

	uts := time.Unix(in.UnixTs, 0)
	bdir := path.Join(x.WorkerConfig.ExportPath, fmt.Sprintf(
		"dgraph.r%d.u%s", in.ReadTs, uts.UTC().Format("0102.1504")))

	if err := os.MkdirAll(bdir, 0700); err != nil {
		return err
	}

	xfmt := exportFormats[in.Format]
	path := func(suffix string) (string, error) {
		return filepath.Abs(path.Join(bdir, fmt.Sprintf("g%02d%s", in.GroupId, suffix)))
	}

	// Open data file now.
	dataPath, err := path(xfmt.ext + ".gz")
	if err != nil {
		return err
	}

	glog.Infof("Exporting data for group: %d at %s\n", in.GroupId, dataPath)
	dataWriter := &fileWriter{}
	if err := dataWriter.open(dataPath); err != nil {
		return err
	}

	// Open schema file now.
	schemaPath, err := path(".schema.gz")
	if err != nil {
		return err
	}
	glog.Infof("Exporting schema for group: %d at %s\n", in.GroupId, schemaPath)
	schemaWriter := &fileWriter{}
	if err := schemaWriter.open(schemaPath); err != nil {
		return err
	}

	e := &exporter{
		readTs:  in.ReadTs,
		counter: 0,
	}

	stream := pstore.NewStreamAt(in.ReadTs)
	stream.LogPrefix = "Export"
	stream.ChooseKey = func(item *badger.Item) bool {
		pk := x.Parse(item.Key())
		// _predicate_ is deprecated but leaving this here so that users with a
		// binary with version >= 1.1 can export data from a version < 1.1 without
		// this internal data showing up.
		if pk.Attr == "_predicate_" {
			return false
		}
		if servesTablet, err := groups().ServesTablet(pk.Attr); err != nil || !servesTablet {
			return false
		}
		// We need to ensure that schema keys are separately identifiable, so they can be
		// written to a different file.
		return pk.IsData() || pk.IsSchema()
	}
	stream.KeyToList = func(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
		item := itr.Item()
		pk := x.Parse(item.Key())

		e.counter += 1
		e.uid = pk.Uid
		e.attr = pk.Attr

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
				return nil, nil
			}
			return toSchema(pk.Attr, update)

		case pk.IsData():
			e.pl, err = posting.ReadPostingList(key, itr)
			if err != nil {
				return nil, err
			}
			switch in.Format {
			case "json":
				return e.toJSON()
			case "rdf":
				return e.toRDF()
			default:
				glog.Fatalf("Invalid export format found: %s", in.Format)
			}

		default:
			glog.Fatalf("Invalid key found: %+v\n", pk)
		}
		return nil, nil
	}

	stream.Send = func(list *bpb.KVList) error {
		for _, kv := range list.Kv {
			var writer *fileWriter
			switch kv.Version {
			case 1: // data
				writer = dataWriter
			case 2: // schema
				writer = schemaWriter
			default:
				glog.Fatalf("Invalid data type found: %x", kv.Key)
			}
			if _, err := writer.gw.Write(kv.Value); err != nil {
				return err
			}
		}
		// Once all the sends are done, writers must be flushed and closed in order.
		return nil
	}

	// All prepwork done. Time to roll.
	if _, err = dataWriter.gw.Write([]byte(xfmt.pre)); err != nil {
		return err
	}
	if err := stream.Orchestrate(ctx); err != nil {
		return err
	}
	if _, err = dataWriter.gw.Write([]byte(xfmt.post)); err != nil {
		return err
	}
	if err := dataWriter.Close(); err != nil {
		return err
	}
	if err := schemaWriter.Close(); err != nil {
		return err
	}
	glog.Infof("Export DONE for group %d at timestamp %d.", in.GroupId, in.ReadTs)
	return nil
}

// Export request is used to trigger exports for the request list of groups.
// If a server receives request to export a group that it doesn't handle, it would
// automatically relay that request to the server that it thinks should handle the request.
func (w *grpcWorker) Export(ctx context.Context, req *pb.ExportRequest) (*pb.Status, error) {
	glog.Infof("Received export request via Grpc: %+v\n", req)
	if ctx.Err() != nil {
		glog.Errorf("Context error during export: %v\n", ctx.Err())
		return nil, ctx.Err()
	}

	glog.Infof("Issuing export request...")
	if err := export(ctx, req); err != nil {
		glog.Errorf("While running export. Request: %+v. Error=%v\n", req, err)
		return nil, err
	}
	glog.Infof("Export request: %+v OK.\n", req)
	return &pb.Status{Msg: "SUCCESS"}, nil
}

func handleExportOverNetwork(ctx context.Context, in *pb.ExportRequest) error {
	if in.GroupId == groups().groupId() {
		return export(ctx, in)
	}

	pl := groups().Leader(in.GroupId)
	if pl == nil {
		return errors.Errorf("Unable to find leader of group: %d\n", in.GroupId)
	}

	glog.Infof("Sending export request to group: %d, addr: %s\n", in.GroupId, pl.Addr)
	c := pb.NewWorkerClient(pl.Get())
	_, err := c.Export(ctx, in)
	if err != nil {
		glog.Errorf("Export error received from group: %d. Error: %v\n", in.GroupId, err)
	}
	return err
}

func ExportOverNetwork(ctx context.Context, format string) error {
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
			req := &pb.ExportRequest{
				GroupId: group,
				ReadTs:  readTs,
				UnixTs:  time.Now().Unix(),
				Format:  format,
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

// NormalizeExportFormat returns the normalized string for the export format if it is valid, an
// empty string otherwise.
func NormalizeExportFormat(fmt string) string {
	fmt = strings.ToLower(fmt)
	if _, ok := exportFormats[fmt]; ok {
		return fmt
	}
	return ""
}
