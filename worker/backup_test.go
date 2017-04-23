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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkb"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/facetsp"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/types"

	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func populateGraphBackup(t *testing.T) {
	rdfEdges := []string{
		"<1> <friend> <5> <author0> .",
		"<2> <friend> <5> <author0> .",
		"<3> <friend> <5> .",
		"<4> <friend> <5> <author0> (since=2005-05-02T15:04:05,close=true,age=33,game=\"football\") .",
		"<1> <name> \"pho\\ton\" <author0> .",
		"<2> <name> \"pho\\ton\"@en <author0> .",
		"<2> <zdata> \"-179769313486231570814527423731704356798070567525844996598917476803157260780028538760589558632766878171540458953514382464234321326889464182768467546703537516986049910576551282076245490090389328944075868508455133942304583236903222948165808559332123348274797826204144723168738177180919299881250404026184124858368.000000\"^^<xs:float> .",
	}
	idMap := map[string]uint64{
		"1": 1,
		"2": 2,
		"3": 3,
		"4": 4,
		"5": 5,
	}

	for _, edge := range rdfEdges {
		nq, err := rdf.Parse(edge)
		require.NoError(t, err)
		rnq := rdf.NQuad{&nq}
		e, err := rnq.ToEdgeUsing(idMap)
		require.NoError(t, err)
		addEdge(t, e, getOrCreate(x.DataKey(e.Attr, e.Entity)))
	}
}

func initTestBackup(t *testing.T, schemaStr string) (string, *store.Store) {
	group.ParseGroupConfig("groups.conf")
	schema.ParseBytes([]byte(schemaStr), 1)

	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	posting.Init(ps)
	Init(ps)
	val, err := (&typesp.Schema{ValueType: uint32(typesp.Posting_UID)}).Marshal()
	require.NoError(t, err)
	ps.SetOne(x.SchemaKey("friend"), val)
	val, err = (&typesp.Schema{ValueType: uint32(typesp.Posting_UID)}).Marshal()
	require.NoError(t, err)
	ps.SetOne(x.SchemaKey("http://www.w3.org/2000/01/rdf-schema#range"), val)
	populateGraphBackup(t)
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	return dir, ps
}

func TestBackup(t *testing.T) {
	// Index the name predicate. We ensure it doesn't show up on backup.
	dir, ps := initTestBackup(t, "name:string @index .")
	defer os.RemoveAll(dir)
	defer ps.Close()
	// Remove already existing backup folders is any.
	bdir, err := ioutil.TempDir("", "backup")
	require.NoError(t, err)
	//defer os.RemoveAll(bdir)

	posting.CommitLists(10, 1)
	time.Sleep(time.Second)

	// We have 4 friend type edges. FP("friends")%10 = 2.
	err = backup(group.BelongsTo("friend"), bdir)
	require.NoError(t, err)

	// We have 2 name type edges(with index). FP("name")%10 =7.
	err = backup(group.BelongsTo("name"), bdir)
	require.NoError(t, err)

	searchDir := bdir
	fileList := []string{}
	schemaFileList := []string{}
	err = filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
		if path != bdir {
			if strings.Contains(path, "schema") {
				schemaFileList = append(schemaFileList, path)
			} else {
				fileList = append(fileList, path)
			}
		}
		return nil
	})
	require.NoError(t, err)

	var counts []int
	for _, file := range fileList {
		f, err := os.Open(file)
		require.NoError(t, err)

		r, err := gzip.NewReader(f)
		require.NoError(t, err)

		scanner := bufio.NewScanner(r)
		count := 0
		for scanner.Scan() {
			nq, err := rdf.Parse(scanner.Text())
			require.NoError(t, err)
			// Subject should have uid 1/2/3/4.
			require.Contains(t, []string{"0x1", "0x2", "0x3", "0x4"}, nq.Subject)
			// The only value we set was "photon".
			if nq.ObjectValue != nil {
				require.Equal(t, &graphp.Value{&graphp.Value_DefaultVal{"pho\\ton"}},
					nq.ObjectValue)
				// Test objecttype
				if nq.Subject == "0x1" {
					require.Equal(t, int32(0), nq.ObjectType)
				} else if nq.Subject == "0x2" {
					// string type because of lang @en
					require.Equal(t, int32(10), nq.ObjectType)
				}
			}

			// The only objectId we set was uid 5.
			if nq.ObjectId != "" {
				require.Equal(t, "0x5", nq.ObjectId)
			}
			// Test lang.
			if nq.Subject == "0x2" && nq.Predicate == "name" {
				require.Equal(t, "en", nq.Lang)
			}
			// Test facets.
			if nq.Subject == "0x4" {
				require.Equal(t, "age", nq.Facets[0].Key)
				require.Equal(t, "close", nq.Facets[1].Key)
				require.Equal(t, "game", nq.Facets[2].Key)
				require.Equal(t, "since", nq.Facets[3].Key)
				// byte representation for facets.
				require.Equal(t, []byte{0x21, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, nq.Facets[0].Value)
				require.Equal(t, []byte{0x1}, nq.Facets[1].Value)
				require.Equal(t, []byte("football"), nq.Facets[2].Value)
				require.Equal(t, "\x01\x00\x00\x00\x0e\xba\b8e\x00\x00\x00\x00\xff\xff",
					string(nq.Facets[3].Value))
				// valtype for facets.
				require.Equal(t, 1, int(nq.Facets[0].ValType))
				require.Equal(t, 3, int(nq.Facets[1].ValType))
				require.Equal(t, 0, int(nq.Facets[2].ValType))
				require.Equal(t, 4, int(nq.Facets[3].ValType))
			}
			// Test label
			if nq.Subject != "0x3" {
				require.Equal(t, "author0", nq.Label)
			} else {
				require.Equal(t, "", nq.Label)
			}
			count++
		}
		counts = append(counts, count)
		require.NoError(t, scanner.Err())
	}
	// This order will bw presereved due to file naming.
	require.Equal(t, []int{4, 2}, counts)

	var schemaCounts []int
	for _, file := range schemaFileList {
		f, err := os.Open(file)
		require.NoError(t, err)

		r, err := gzip.NewReader(f)
		require.NoError(t, err)

		scanner := bufio.NewScanner(r)
		count := 0
		for scanner.Scan() {
			schemas, err := schema.Parse(scanner.Text())
			require.NoError(t, err)
			require.Equal(t, 1, len(schemas))
			// We wrote schema for only two predicates
			if schemas[0].Predicate == "friend" {
				require.Equal(t, "uid", types.TypeID(schemas[0].ValueType).Name())
			} else {
				require.Equal(t, "http://www.w3.org/2000/01/rdf-schema#range", schemas[0].Predicate)
				require.Equal(t, "uid", types.TypeID(schemas[0].ValueType).Name())
			}
			count = len(schemas)
		}
		schemaCounts = append(schemaCounts, count)
		require.NoError(t, scanner.Err())
	}
	// This order will be preserved due to file naming
	require.Equal(t, []int{1, 0}, schemaCounts)
}

func generateBenchValues() []kv {
	byteInt := make([]byte, 4)
	binary.LittleEndian.PutUint32(byteInt, 123)

	fac := []*facetsp.Facet{
		{
			Key:   "facetTest",
			Value: []byte("testVal"),
		},
	}

	geoData, _ := wkb.Marshal(geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518}), binary.LittleEndian)

	// Posting_STRING   Posting_ValType = 0
	// Posting_BINARY   Posting_ValType = 1
	// Posting_INT    Posting_ValType = 2
	// Posting_FLOAT    Posting_ValType = 3
	// Posting_BOOL     Posting_ValType = 4
	// Posting_DATE     Posting_ValType = 5
	// Posting_DATETIME Posting_ValType = 6
	// Posting_GEO      Posting_ValType = 7
	// Posting_UID      Posting_ValType = 8
	benchItems := []kv{
		{
			prefix: "testString",
			list: &typesp.PostingList{
				Postings: []*typesp.Posting{{
					ValType: typesp.Posting_STRING,
					Value:   []byte("手機裡的眼淚"),
					Uid:     uint64(65454),
					Facets:  fac,
				}},
			},
		},
		{prefix: "testGeo",
			list: &typesp.PostingList{
				Postings: []*typesp.Posting{{
					ValType: typesp.Posting_GEO,
					Value:   geoData,
					Uid:     uint64(65454),
					Facets:  fac,
				}},
			}},
		{prefix: "testPassword",
			list: &typesp.PostingList{
				Postings: []*typesp.Posting{{
					ValType: typesp.Posting_PASSWORD,
					Value:   []byte("test"),
					Uid:     uint64(65454),
					Facets:  fac,
				}},
			}},
		{prefix: "testInt",
			list: &typesp.PostingList{
				Postings: []*typesp.Posting{{
					ValType: typesp.Posting_INT,
					Value:   byteInt,
					Uid:     uint64(65454),
					Facets:  fac,
				}},
			}},
		{prefix: "testUid",
			list: &typesp.PostingList{
				Postings: []*typesp.Posting{{
					ValType: typesp.Posting_INT,
					Uid:     uint64(65454),
					Facets:  fac,
				}},
			}},
	}

	return benchItems
}

func BenchmarkToRDF(b *testing.B) {
	buf := new(bytes.Buffer)
	buf.Grow(50000)

	items := generateBenchValues()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toRDF(buf, items[0])
		toRDF(buf, items[1])
		toRDF(buf, items[2])
		toRDF(buf, items[3])
		toRDF(buf, items[4])
		buf.Reset()
	}
}
