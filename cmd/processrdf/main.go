// This script is used to convert all uids to blank nodes so that uid's
// present in rdf file doesn't collide with the already generated
// uid's in dgraph
//
// You can run the script like
// go build . && ./processrdf -r path-to-gzipped-rdf.gz
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

// Converts all uids to xids.
// Ignore _lease_ edges
// Move _xid_ to end of file (In case backup is from previous version -
//  helps in restoring older xids)
var (
	files     = flag.String("r", "", "Location of rdf files to load")
	outputDir = flag.String("output", "output", "Folder in which to store output.")
)

// Reads a single line from a buffered reader. The line is read into the
// passed in buffer to minimize allocations. This is the preferred
// method for loading long lines which could be longer than the buffer
// size of bufio.Scanner.
func readLine(r *bufio.Reader, buf *bytes.Buffer) error {
	isPrefix := true
	var err error
	for isPrefix && err == nil {
		var line []byte
		// The returned line is an internal buffer in bufio and is only
		// valid until the next call to ReadLine. It needs to be copied
		// over to our own buffer.
		line, isPrefix, err = r.ReadLine()
		if err == nil {
			buf.Write(line)
		}
	}
	return err
}

func toRDF(nq *protos.NQuad, buf *bytes.Buffer) error {
	buf.WriteString("<")
	buf.WriteString(nq.Subject)
	buf.WriteString("> <")
	buf.WriteString(nq.Predicate)
	buf.WriteString("> ")

	if len(nq.ObjectId) == 0 {
		p := rdf.TypeValFrom(nq.ObjectValue)
		var err error
		var p1 types.Val
		if p.Tid == types.GeoID || p.Tid == types.DateID || p.Tid == types.DateTimeID {
			if p1, err = types.Convert(p, types.StringID); err != nil {
				return err
			}
		} else {
			p1 = types.ValueForType(types.StringID)
			if err = types.Marshal(p, &p1); err != nil {
				return err
			}
		}
		buf.WriteByte('"')
		buf.WriteString(p1.Value.(string))
		buf.WriteByte('"')
		vID := types.TypeID(nq.ObjectType)
		if vID == types.GeoID {
			buf.WriteString("^^<geo:geojson> ")
		} else if vID == types.PasswordID {
			buf.WriteString("^^<pwd:")
			buf.WriteString(vID.Name())
			buf.WriteByte('>')
		} else if len(nq.Lang) > 0 {
			buf.WriteByte('@')
			buf.WriteString(nq.Lang)
		} else if vID != types.BinaryID &&
			vID != types.DefaultID {
			buf.WriteString("^^<xs:")
			buf.WriteString(vID.Name())
			buf.WriteByte('>')
		}
	} else {
		buf.WriteString("<")
		buf.WriteString(nq.ObjectId)
		buf.WriteByte('>')
	}
	// Label
	if len(nq.Label) > 0 {
		buf.WriteString(" <")
		buf.WriteString(nq.Label)
		buf.WriteByte('>')
	}
	// Facets.
	fcs := nq.Facets
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
				buf.WriteByte('"')
				buf.WriteString(fVal.Value.(string))
				buf.WriteByte('"')
			} else {
				buf.WriteString(fVal.Value.(string))
			}
		}
		buf.WriteByte(')')
	}
	// End dot.
	buf.WriteString(" .\n")
	return nil
}

// processFile sends mutations for a given gz file.
func processFile(file string) {
	fmt.Printf("\nProcessing %s\n", file)
	f, err := os.Open(file)
	x.Check(err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	x.Check(err)
	fpath := path.Join(*outputDir, path.Base(file))
	of, err := os.Create(fpath)
	x.Check(err)
	defer of.Close()
	var line int

	var buf bytes.Buffer
	bufReader := bufio.NewReader(gr)
	w := bufio.NewWriterSize(of, 1000000)
	gw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	x.Check(err)

	for {
		err = readLine(bufReader, &buf)
		if err != nil {
			break
		}
		nq, err := rdf.Parse(buf.String())
		if err == rdf.ErrEmpty { // special case: comment/empty line
			buf.Reset()
			continue
		} else if err != nil {
			log.Fatalf("Error while parsing RDF: %v, on line:%v %v", err, line, buf.String())
		}
		if nq.Predicate == "_lease_" {
			buf.Reset()
			continue
		}
		if _, err := strconv.ParseUint(nq.Subject, 0, 64); err == nil {
			nq.Subject = "_:" + nq.Subject
		}
		if len(nq.ObjectId) > 0 {
			if _, err := strconv.ParseUint(nq.ObjectId, 0, 64); err == nil {
				nq.ObjectId = "_:" + nq.ObjectId
			}
		}
		buf.Reset()
		err = toRDF(&nq, &buf)
		x.Check(err)
		gw.Write(buf.Bytes())
		buf.Reset()
	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}

	err = gw.Flush()
	x.Check(err)
	err = gw.Close()
	x.Check(err)
	err = w.Flush()
	x.Check(err)
}

func main() {
	flag.Parse()
	if !flag.Parsed() {
		log.Fatal("Unable to parse flags")
	}

	err := os.MkdirAll(*outputDir, 0700)
	x.Checkf(err, "Error creating directory:%s\n", *outputDir)

	x.AssertTruef(len(*files) > 0, "Input files not specified")
	filesList := strings.Split(*files, ",")
	for _, file := range filesList {
		processFile(file)
	}
	fmt.Printf("Finished processing files\n")
}
