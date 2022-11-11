/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/minio/minio-go/v6"
	"github.com/pkg/errors"

	"github.com/dgraph-io/badger/v3"
	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/ristretto/z"

	"github.com/dgraph-io/dgo/v210/protos/api"

	"github.com/dgraph-io/dgraph/ee/enc"
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
	pl        *posting.List
	uid       uint64
	attr      string
	namespace uint64
	readTs    uint64
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

// UIDs like 0x1 look weird but 64-bit ones like 0x0000000000000001 are too long.
var uidFmtStrRdf = "<%#x>"
var uidFmtStrJson = "\"%#x\""

// valToStr converts a posting value to a string.
func valToStr(v types.Val) (string, error) {
	v2, err := types.Convert(v, types.StringID)
	if err != nil {
		return "", errors.Wrapf(err, "while converting %v to string", v2.Value)
	}

	// Strip terminating null, if any.
	return strings.TrimRight(v2.Value.(string), "\x00"), nil
}

// facetToString convert a facet value to a string.
func facetToString(fct *api.Facet) (string, error) {
	v1, err := facets.ValFor(fct)
	if err != nil {
		return "", errors.Wrapf(err, "getting value from facet %#v", fct)
	}

	v2 := &types.Val{Tid: types.StringID}
	if err = types.Marshal(v1, v2); err != nil {
		return "", errors.Wrapf(err, "marshaling facet value %v to string", v1)
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
		x.Panic(errors.New("Could not marshal string to JSON string"))
	}
	return string(byt)
}

func (e *exporter) toJSON() (*bpb.KVList, error) {
	bp := new(bytes.Buffer)
	// We could output more compact JSON at the cost of code complexity.
	// Leaving it simple for now.

	writeFacets := func(pfacets []*api.Facet) error {
		for _, fct := range pfacets {
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
		return nil
	}

	continuing := false
	mapStart := fmt.Sprintf("  {\"uid\":"+uidFmtStrJson+`,"namespace":"0x%x"`, e.uid, e.namespace)
	err := e.pl.Iterate(e.readTs, 0, func(p *pb.Posting) error {
		if continuing {
			fmt.Fprint(bp, ",\n")
		} else {
			continuing = true
		}

		fmt.Fprint(bp, mapStart)
		if p.PostingType == pb.Posting_REF {
			fmt.Fprintf(bp, `,"%s":[`, e.attr)
			fmt.Fprintf(bp, "{\"uid\":"+uidFmtStrJson, p.Uid)
			if err := writeFacets(p.Facets); err != nil {
				return errors.Wrap(err, "While writing facets for posting_REF")
			}
			fmt.Fprint(bp, "}]")
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
			if err := writeFacets(p.Facets); err != nil {
				return errors.Wrap(err, "While writing facets for value postings")
			}
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
		// Use label for storing namespace.
		fmt.Fprintf(bp, " <%#x>", e.namespace)

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

func toSchema(attr string, update *pb.SchemaUpdate) *bpb.KV {
	// bytes.Buffer never returns error for any of the writes. So, we don't need to check them.
	ns, attr := x.ParseNamespaceAttr(attr)
	var buf bytes.Buffer
	x.Check2(buf.WriteString(fmt.Sprintf("[%#x]", ns)))
	x.Check2(buf.WriteRune(' '))
	x.Check2(buf.WriteRune('<'))
	x.Check2(buf.WriteString(attr))
	x.Check2(buf.WriteRune('>'))
	x.Check2(buf.WriteRune(':'))
	if update.GetList() {
		x.Check2(buf.WriteRune('['))
	}
	x.Check2(buf.WriteString(types.TypeID(update.GetValueType()).Name()))
	if update.GetList() {
		x.Check2(buf.WriteRune(']'))
	}
	switch {
	case update.GetDirective() == pb.SchemaUpdate_REVERSE:
		x.Check2(buf.WriteString(" @reverse"))
	case update.GetDirective() == pb.SchemaUpdate_INDEX && len(update.GetTokenizer()) > 0:
		x.Check2(buf.WriteString(" @index("))
		x.Check2(buf.WriteString(strings.Join(update.GetTokenizer(), ",")))
		x.Check2(buf.WriteRune(')'))
	}
	if update.GetCount() {
		x.Check2(buf.WriteString(" @count"))
	}
	if update.GetLang() {
		x.Check2(buf.WriteString(" @lang"))
	}
	if update.GetUpsert() {
		x.Check2(buf.WriteString(" @upsert"))
	}
	x.Check2(buf.WriteString(" . \n"))
	//TODO(Naman): We don't need the version anymore.
	return &bpb.KV{
		Value:   buf.Bytes(),
		Version: 3, // Schema value
	}
}

func toType(attr string, update pb.TypeUpdate) *bpb.KV {
	var buf bytes.Buffer
	ns, attr := x.ParseNamespaceAttr(attr)
	x.Check2(buf.WriteString(fmt.Sprintf("[%#x] type <%s> {\n", ns, attr)))
	for _, field := range update.Fields {
		x.Check2(buf.WriteString(fieldToString(field)))
	}

	x.Check2(buf.WriteString("}\n"))

	return &bpb.KV{
		Value:   buf.Bytes(),
		Version: 3, // Type value
	}
}

func fieldToString(update *pb.SchemaUpdate) string {
	var builder strings.Builder
	predicate := x.ParseAttr(update.Predicate)
	x.Check2(builder.WriteString("\t"))
	// We don't need the namespace information with the fields. We already have that with type.
	if strings.HasPrefix(predicate, "~") {
		// While exporting type definitions, "<" and ">" brackets must be written around
		// the name of reverse predicates or Dgraph won't be able to parse the exported schema.
		x.Check2(builder.WriteString("<"))
		x.Check2(builder.WriteString(predicate))
		x.Check2(builder.WriteString(">"))
	} else {
		x.Check2(builder.WriteString(predicate))
	}
	x.Check2(builder.WriteString("\n"))
	return builder.String()
}

type fileWriter struct {
	fd            *os.File
	bw            *bufio.Writer
	gw            *gzip.Writer
	relativePath  string
	hasDataBefore bool
}

func (writer *fileWriter) open(fpath string) error {
	var err error
	writer.fd, err = os.Create(fpath)
	if err != nil {
		return err
	}
	writer.bw = bufio.NewWriterSize(writer.fd, 1e6)
	w, err := enc.GetWriter(x.WorkerConfig.EncryptionKey, writer.bw)
	if err != nil {
		return err
	}
	writer.gw, err = gzip.NewWriterLevel(w, gzip.BestSpeed)
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

// ExportedFiles has the relative path of files that were written during export
type ExportedFiles []string

type exportStorage interface {
	openFile(relativePath string) (*fileWriter, error)
	finishWriting(fs ...*fileWriter) (ExportedFiles, error)
}

type localExportStorage struct {
	destination  string
	relativePath string
}

// remoteExportStorage uses localExportStorage to write files, then uploads to minio
type remoteExportStorage struct {
	mc     *x.MinioClient
	bucket string
	prefix string // stores the path within the bucket.
	les    *localExportStorage
}

func newLocalExportStorage(destination, backupName string) (*localExportStorage, error) {
	bdir, err := filepath.Abs(filepath.Join(destination, backupName))
	if err != nil {
		return nil, err
	}

	if err = os.MkdirAll(bdir, 0700); err != nil {
		return nil, err
	}

	return &localExportStorage{destination, backupName}, nil
}

func (l *localExportStorage) openFile(fileName string) (*fileWriter, error) {
	fw := &fileWriter{relativePath: filepath.Join(l.relativePath, fileName)}

	filePath, err := filepath.Abs(filepath.Join(l.destination, fw.relativePath))
	if err != nil {
		return nil, err
	}

	glog.Infof("Exporting to file at %s\n", filePath)

	if err := fw.open(filePath); err != nil {
		return nil, err
	}

	return fw, nil
}

func (l *localExportStorage) finishWriting(fs ...*fileWriter) (ExportedFiles, error) {
	var files ExportedFiles

	for _, file := range fs {
		err := file.Close()
		if err != nil {
			return nil, err
		}
		files = append(files, file.relativePath)
	}

	return files, nil
}

func newRemoteExportStorage(in *pb.ExportRequest, backupName string) (*remoteExportStorage, error) {
	tmpDir, err := ioutil.TempDir(x.WorkerConfig.TmpDir, "export")
	if err != nil {
		return nil, err
	}
	localStorage, err := newLocalExportStorage(tmpDir, backupName)
	if err != nil {
		return nil, err
	}

	uri, err := url.Parse(in.Destination)
	if err != nil {
		return nil, err
	}

	mc, err := x.NewMinioClient(uri, &x.MinioCredentials{
		AccessKey:    in.AccessKey,
		SecretKey:    in.SecretKey,
		SessionToken: in.SessionToken,
		Anonymous:    in.Anonymous,
	})
	if err != nil {
		return nil, err
	}

	bucket, prefix, err := mc.ValidateBucket(uri)
	if err != nil {
		return nil, err
	}

	return &remoteExportStorage{mc, bucket, prefix, localStorage}, nil
}

func (r *remoteExportStorage) openFile(fileName string) (*fileWriter, error) {
	return r.les.openFile(fileName)
}

func (r *remoteExportStorage) finishWriting(fs ...*fileWriter) (ExportedFiles, error) {
	files, err := r.les.finishWriting(fs...)
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		var d string
		if r.prefix == "" {
			d = f
		} else {
			d = r.prefix + "/" + f
		}
		filePath := filepath.Join(r.les.destination, f)
		// FIXME: tejas [06/2020] - We could probably stream these results, but it's easier to copy for now
		glog.Infof("Uploading from %s to %s\n", filePath, d)
		_, err := r.mc.FPutObject(r.bucket, d, filePath, minio.PutObjectOptions{
			ContentType: "application/gzip",
		})
		if err != nil {
			return nil, err
		}
	}

	return files, nil
}

func newExportStorage(in *pb.ExportRequest, backupName string) (exportStorage, error) {
	switch {
	case strings.HasPrefix(in.Destination, "/"):
		return newLocalExportStorage(in.Destination, backupName)
	case strings.HasPrefix(in.Destination, "minio://") || strings.HasPrefix(in.Destination, "s3://"):
		return newRemoteExportStorage(in, backupName)
	default:
		return newLocalExportStorage(x.WorkerConfig.ExportPath, backupName)
	}
}

// export creates a export of data by exporting it as an RDF gzip.
func export(ctx context.Context, in *pb.ExportRequest) (ExportedFiles, error) {

	if in.GroupId != groups().groupId() {
		return nil, errors.Errorf("Export request group mismatch. Mine: %d. Requested: %d",
			groups().groupId(), in.GroupId)
	}
	glog.Infof("Export requested at %d for namespace %d.", in.ReadTs, in.Namespace)

	// Let's wait for this server to catch up to all the updates until this ts.
	if err := posting.Oracle().WaitForTs(ctx, in.ReadTs); err != nil {
		return nil, err
	}
	glog.Infof("Running export for group %d at timestamp %d.", in.GroupId, in.ReadTs)

	return exportInternal(ctx, in, pstore, false)
}

// exportInternal contains the core logic to export a Dgraph database. If skipZero is set to
// false, the parts of this method that require to talk to zero will be skipped. This is useful
// when exporting a p directory directly from disk without a running cluster.
// It uses stream framework to export the data. While it uses an iterator for exporting the schema
// and types.
func exportInternal(ctx context.Context, in *pb.ExportRequest, db *badger.DB,
	skipZero bool) (ExportedFiles, error) {
	uts := time.Unix(in.UnixTs, 0)
	exportStorage, err := newExportStorage(in,
		fmt.Sprintf("dgraph.r%d.u%s", in.ReadTs, uts.UTC().Format("0102.1504")))
	if err != nil {
		return nil, err
	}

	xfmt := exportFormats[in.Format]

	dataWriter, err := exportStorage.openFile(fmt.Sprintf("g%02d%s", in.GroupId, xfmt.ext+".gz"))
	if err != nil {
		return nil, err
	}

	schemaWriter, err := exportStorage.openFile(fmt.Sprintf("g%02d%s", in.GroupId, ".schema.gz"))
	if err != nil {
		return nil, err
	}

	gqlSchemaWriter, err := exportStorage.openFile(
		fmt.Sprintf("g%02d%s", in.GroupId, ".gql_schema.gz"))
	if err != nil {
		return nil, err
	}

	// This stream exports only the data and the graphQL schema.
	stream := db.NewStreamAt(in.ReadTs)
	stream.Prefix = []byte{x.DefaultPrefix}
	if in.Namespace != math.MaxUint64 {
		// Export a specific namespace.
		stream.Prefix = append(stream.Prefix, x.NamespaceToBytes(in.Namespace)...)
	}
	stream.LogPrefix = "Export"
	stream.ChooseKey = func(item *badger.Item) bool {
		// Skip exporting delete data including Schema and Types.
		if item.IsDeletedOrExpired() {
			return false
		}
		pk, err := x.Parse(item.Key())
		if err != nil {
			glog.Errorf("error %v while parsing key %v during export. Skip.", err,
				hex.EncodeToString(item.Key()))
			return false
		}

		// Do not pick keys storing parts of a multi-part list. They will be read
		// from the main key.
		if pk.HasStartUid {
			return false
		}
		// _predicate_ is deprecated but leaving this here so that users with a
		// binary with version >= 1.1 can export data from a version < 1.1 without
		// this internal data showing up.
		if pk.Attr == "_predicate_" {
			return false
		}

		if !skipZero {
			if servesTablet, err := groups().ServesTablet(pk.Attr); err != nil || !servesTablet {
				return false
			}
		}
		return pk.IsData()
	}
	stream.KeyToList = func(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
		item := itr.Item()
		pk, err := x.Parse(item.Key())
		if err != nil {
			glog.Errorf("error %v while parsing key %v during export. Skip.", err,
				hex.EncodeToString(item.Key()))
			return nil, err
		}
		e := &exporter{
			readTs: in.ReadTs,
		}
		e.uid = pk.Uid
		e.namespace, e.attr = x.ParseNamespaceAttr(pk.Attr)

		switch {
		case e.attr == "dgraph.graphql.xid":
			// Ignore this predicate.
		case e.attr == "dgraph.drop.op":
			// Ignore this predicate.
		case e.attr == "dgraph.graphql.p_query":
			// Ignore this predicate.
		case pk.IsData() && e.attr == "dgraph.graphql.schema":
			// Export the graphql schema.
			pl, err := posting.ReadPostingList(key, itr)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot read posting list for GraphQL schema")
			}
			vals, err := pl.AllValues(in.ReadTs)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot read value of GraphQL schema")
			}
			// if the GraphQL schema node was deleted with S * * delete mutation,
			// then the data key will be overwritten with nil value.
			// So, just skip exporting it as there will be no value for this data key.
			if len(vals) == 0 {
				return nil, nil
			}
			// Give an error only if we find more than one value for the schema.
			if len(vals) > 1 {
				return nil, errors.Errorf("found multiple values for the GraphQL schema")
			}
			val, ok := vals[0].Value.([]byte)
			if !ok {
				return nil, errors.Errorf("cannot convert value of GraphQL schema to byte array")
			}

			exported := x.ExportedGQLSchema{
				Namespace: e.namespace,
				Schema:    string(val),
			}
			if val, err = json.Marshal(exported); err != nil {
				return nil, errors.Wrapf(err, "Error marshalling GraphQL schema to json")
			}

			kv := &bpb.KV{
				Value:   val,
				Version: 2, // GraphQL schema value
			}
			return listWrap(kv), nil

		// below predicates no longer exist internally starting v21.03 but leaving them here
		// so that users with a binary with version >= 21.03 can export data from a version < 21.03
		// without this internal data showing up.
		case e.attr == "dgraph.cors":
		case e.attr == "dgraph.graphql.schema_created_at":
		case e.attr == "dgraph.graphql.schema_history":
		case e.attr == "dgraph.graphql.p_sha256hash":
			// Ignore these predicates.

		case pk.IsData():
			e.pl, err = posting.ReadPostingList(key, itr)
			if err != nil {
				return nil, err
			}

			// The GraphQL layer will create a node of type "dgraph.graphql". That entry
			// should not be exported.
			if e.attr == "dgraph.type" {
				vals, err := e.pl.AllValues(in.ReadTs)
				if err != nil {
					return nil, errors.Wrapf(err, "cannot read value of dgraph.type entry")
				}
				if len(vals) == 1 {
					val, ok := vals[0].Value.([]byte)
					if !ok {
						return nil, errors.Errorf("cannot read value of dgraph.type entry")
					}
					if string(val) == "dgraph.graphql" {
						return nil, nil
					}
				}
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

	var dataSeparator []byte
	switch in.Format {
	case "json":
		dataSeparator = []byte(",\n")
	case "rdf":
		// The separator for RDF should be empty since the toRDF function already
		// adds newline to each RDF entry.
	default:
		glog.Fatalf("Invalid export format found: %s", in.Format)
	}

	stream.Send = func(buf *z.Buffer) error {
		kv := &bpb.KV{}
		return buf.SliceIterate(func(s []byte) error {
			kv.Reset()
			if err := kv.Unmarshal(s); err != nil {
				return err
			}
			// Skip nodes that have no data. Otherwise, the exported data could have
			// formatting and/or syntax errors.
			if len(kv.Value) == 0 {
				return nil
			}

			var writer *fileWriter
			var separator []byte
			switch kv.Version {
			case 1: // data
				writer = dataWriter
				separator = dataSeparator
			case 2: // graphQL schema
				writer = gqlSchemaWriter
				separator = []byte(",\n") // use json separator.
			default:
				glog.Fatalf("Invalid data type found: %x", kv.Key)
			}

			if writer.hasDataBefore {
				if _, err := writer.gw.Write(separator); err != nil {
					return err
				}
			}
			// change the hasDataBefore flag so that the next data entry will have a separator
			// prepended
			writer.hasDataBefore = true

			_, err = writer.gw.Write(kv.Value)
			return err
		})
	}

	// This is used to export the schema and types.
	writePrefix := func(prefix byte) error {
		txn := db.NewTransactionAt(in.ReadTs, false)
		defer txn.Discard()
		// We don't need to iterate over all versions.
		iopts := badger.DefaultIteratorOptions
		iopts.Prefix = []byte{prefix}
		if in.Namespace != math.MaxUint64 {
			iopts.Prefix = append(iopts.Prefix, x.NamespaceToBytes(in.Namespace)...)
		}

		itr := txn.NewIterator(iopts)
		defer itr.Close()
		for itr.Rewind(); itr.Valid(); itr.Next() {
			item := itr.Item()
			// Don't export deleted items.
			if item.IsDeletedOrExpired() {
				continue
			}
			pk, err := x.Parse(item.Key())
			if err != nil {
				glog.Errorf("error %v while parsing key %v during export. Skip.", err,
					hex.EncodeToString(item.Key()))
				return err
			}

			var kv *bpb.KV
			switch prefix {
			case x.ByteSchema:
				if !skipZero {
					servesTablet, err := groups().ServesTablet(pk.Attr)
					if err != nil || !servesTablet {
						continue
					}
				}

				var update pb.SchemaUpdate
				err = item.Value(func(val []byte) error {
					return update.Unmarshal(val)
				})
				if err != nil {
					// Let's not propagate this error. We just log this and continue onwards.
					glog.Errorf("Unable to unmarshal schema: %+v. Err=%v\n", pk, err)
					continue
				}
				kv = toSchema(pk.Attr, &update)

			case x.ByteType:
				var update pb.TypeUpdate
				err := item.Value(func(val []byte) error {
					return update.Unmarshal(val)
				})
				if err != nil {
					// Let's not propagate this error. We just log this and continue onwards.
					glog.Errorf("Unable to unmarshal type: %+v. Err=%v\n", pk, err)
					return nil
				}
				kv = toType(pk.Attr, update)

			default:
				glog.Fatalf("Unhandled byte prefix: %v", prefix)
			}

			// Write to the appropriate writer.
			if _, err := schemaWriter.gw.Write(kv.Value); err != nil {
				return err
			}
		}
		return nil
	}

	// All prepwork done. Time to roll.
	if _, err = gqlSchemaWriter.gw.Write([]byte(exportFormats["json"].pre)); err != nil {
		return nil, err
	}
	if _, err = dataWriter.gw.Write([]byte(xfmt.pre)); err != nil {
		return nil, err
	}
	if err := stream.Orchestrate(ctx); err != nil {
		return nil, err
	}
	if _, err = dataWriter.gw.Write([]byte(xfmt.post)); err != nil {
		return nil, err
	}
	if _, err = gqlSchemaWriter.gw.Write([]byte(exportFormats["json"].post)); err != nil {
		return nil, err
	}

	// Write the schema and types.
	if err := writePrefix(x.ByteSchema); err != nil {
		return nil, err
	}
	if err := writePrefix(x.ByteType); err != nil {
		return nil, err
	}

	glog.Infof("Export DONE for group %d at timestamp %d.", in.GroupId, in.ReadTs)
	return exportStorage.finishWriting(dataWriter, schemaWriter, gqlSchemaWriter)
}

// Export request is used to trigger exports for the request list of groups.
// If a server receives request to export a group that it doesn't handle, it would
// automatically relay that request to the server that it thinks should handle the request.
func (w *grpcWorker) Export(ctx context.Context, req *pb.ExportRequest) (*pb.ExportResponse, error) {
	glog.Infof("Received export request via Grpc: %+v\n", req)
	if ctx.Err() != nil {
		glog.Errorf("Context error during export: %v\n", ctx.Err())
		return nil, ctx.Err()
	}

	glog.Infof("Issuing export request...")
	files, err := export(ctx, req)
	if err != nil {
		glog.Errorf("While running export. Request: %+v. Error=%v\n", req, err)
		return nil, err
	}
	glog.Infof("Export request: %+v OK.\n", req)
	return &pb.ExportResponse{Msg: "SUCCESS", Files: files}, nil
}

func handleExportOverNetwork(ctx context.Context, in *pb.ExportRequest) (ExportedFiles, error) {
	if in.GroupId == groups().groupId() {
		return export(ctx, in)
	}

	pl := groups().Leader(in.GroupId)
	if pl == nil {
		return nil, errors.Errorf("Unable to find leader of group: %d\n", in.GroupId)
	}

	glog.Infof("Sending export request to group: %d, addr: %s\n", in.GroupId, pl.Addr)
	c := pb.NewWorkerClient(pl.Get())
	_, err := c.Export(ctx, in)
	if err != nil {
		glog.Errorf("Export error received from group: %d. Error: %v\n", in.GroupId, err)
	}
	return nil, err
}

// ExportOverNetwork sends export requests to all the known groups.
func ExportOverNetwork(ctx context.Context, input *pb.ExportRequest) (ExportedFiles, error) {
	// If we haven't even had a single membership update, don't run export.
	if err := x.HealthCheck(); err != nil {
		glog.Errorf("Rejecting export request due to health check error: %v\n", err)
		return nil, err
	}
	// Get ReadTs from zero and wait for stream to catch up.
	ts, err := Timestamps(ctx, &pb.Num{ReadOnly: true})
	if err != nil {
		glog.Errorf("Unable to retrieve readonly ts for export: %v\n", err)
		return nil, err
	}
	readTs := ts.ReadOnly
	glog.Infof("Got readonly ts from Zero: %d\n", readTs)

	// Let's first collect all groups.
	gids := groups().KnownGroups()
	glog.Infof("Requesting export for groups: %v\n", gids)

	type filesAndError struct {
		ExportedFiles
		error
	}
	ch := make(chan filesAndError, len(gids))
	for _, gid := range gids {
		go func(group uint32) {
			req := &pb.ExportRequest{
				GroupId:   group,
				ReadTs:    readTs,
				UnixTs:    time.Now().Unix(),
				Format:    input.Format,
				Namespace: input.Namespace,

				Destination:  input.Destination,
				AccessKey:    input.AccessKey,
				SecretKey:    input.SecretKey,
				SessionToken: input.SessionToken,
				Anonymous:    input.Anonymous,
			}
			files, err := handleExportOverNetwork(ctx, req)
			ch <- filesAndError{files, err}
		}(gid)
	}

	var allFiles ExportedFiles
	for i := 0; i < len(gids); i++ {
		pair := <-ch
		if pair.error != nil {
			rerr := errors.Wrapf(pair.error, "Export failed at readTs %d", readTs)
			glog.Errorln(rerr)
			return nil, rerr
		}
		allFiles = append(allFiles, pair.ExportedFiles...)
	}

	glog.Infof("Export at readTs %d DONE", readTs)
	return allFiles, nil
}

// NormalizeExportFormat returns the normalized string for the export format if it is valid, an
// empty string otherwise.
func NormalizeExportFormat(format string) string {
	format = strings.ToLower(format)
	if _, ok := exportFormats[format]; ok {
		return format
	}
	return ""
}
