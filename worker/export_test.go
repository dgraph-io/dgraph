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
	"compress/gzip"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"

	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

func populateGraphExport(t *testing.T) {
	rdfEdges := []string{
		`<1> <friend> <5> <author0> .`,
		`<2> <friend> <5> <author0> .`,
		`<3> <friend> <5> .`,
		`<4> <friend> <5> <author0> (since=2005-05-02T15:04:05,close=true,` +
			`age=33,game="football",poem="roses are red\nviolets are blue") .`,
		`<1> <name> "pho\ton" <author0> .`,
		`<2> <name> "pho\ton"@en <author0> .`,
		`<3> <name> "First Line\nSecondLine" .`,
		"<1> <friend_not_served> <5> <author0> .",
		`<5> <name> "" .`,
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
		rnq := gql.NQuad{&nq}
		err = facets.SortAndValidate(rnq.Facets)
		require.NoError(t, err)
		e, err := rnq.ToEdgeUsing(idMap)
		require.NoError(t, err)
		addEdge(t, e, getOrCreate(x.DataKey(e.Attr, e.Entity)))
	}
}

func initTestExport(t *testing.T, schemaStr string) (string, *badger.ManagedDB) {
	schema.ParseBytes([]byte(schemaStr), 1)

	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)

	opt := badger.DefaultOptions
	opt.Dir = dir
	opt.ValueDir = dir
	db, err := badger.OpenManaged(opt)
	x.Check(err)

	posting.Init(db)
	Init(db)
	val, err := (&protos.SchemaUpdate{ValueType: uint32(protos.Posting_UID)}).Marshal()
	require.NoError(t, err)

	txn := db.NewTransactionAt(math.MaxUint64, true)
	txn.Set(x.SchemaKey("friend"), val, 0x00)
	txn.CommitAt(timestamp(), func(err error) {
		require.NoError(t, err)
	})
	txn.Discard()

	require.NoError(t, err)
	val, err = (&protos.SchemaUpdate{ValueType: uint32(protos.Posting_UID)}).Marshal()
	require.NoError(t, err)

	txn = db.NewTransactionAt(math.MaxUint64, true)
	txn.Set(x.SchemaKey("http://www.w3.org/2000/01/rdf-schema#range"), val, 0x00)
	require.NoError(t, err)
	txn.Set(x.SchemaKey("friend_not_served"), val, 0x00)
	txn.CommitAt(timestamp(), func(err error) {
		require.NoError(t, err)
	})
	txn.Discard()

	require.NoError(t, err)
	populateGraphExport(t)

	return dir, db
}

func TestExport(t *testing.T) {
	// Index the name predicate. We ensure it doesn't show up on export.
	dir, ps := initTestExport(t, "name:string @index .")
	defer os.RemoveAll(dir)
	defer ps.Close()
	// Remove already existing export folders is any.
	bdir, err := ioutil.TempDir("", "export")
	require.NoError(t, err)
	defer os.RemoveAll(bdir)

	time.Sleep(1 * time.Second)

	// We have 4 friend type edges. FP("friends")%10 = 2.
	err = export(bdir, timestamp())
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
	require.Equal(t, 1, len(fileList))

	file := fileList[0]
	f, err := os.Open(file)
	require.NoError(t, err)

	r, err := gzip.NewReader(f)
	require.NoError(t, err)

	scanner := bufio.NewScanner(r)
	count := 0
	for scanner.Scan() {
		nq, err := rdf.Parse(scanner.Text())
		require.NoError(t, err)
		require.Contains(t, []string{"_:uid1", "_:uid2", "_:uid3", "_:uid4", "_:uid5"}, nq.Subject)
		if nq.ObjectValue != nil {
			switch nq.Subject {
			case "_:uid1", "_:uid2":
				require.Equal(t, &protos.Value{&protos.Value_DefaultVal{"pho\ton"}},
					nq.ObjectValue)
			case "_:uid3":
				require.Equal(t, &protos.Value{&protos.Value_DefaultVal{"First Line\nSecondLine"}},
					nq.ObjectValue)
			case "_:uid4":
			case "_:uid5":
				// Compare directly with the raw string to avoid the object
				// being convert from "" to "_nil_" when it is parsed as an RDF.
				require.Equal(t, `<_:uid5> <name> "" .`, scanner.Text())
			default:
				t.Errorf("Unexpected subject: %v", nq.Subject)
			}
			if nq.Subject == "_:uid1" || nq.Subject == "_:uid2" {
				require.Equal(t, &protos.Value{&protos.Value_DefaultVal{"pho\ton"}},
					nq.ObjectValue)
			}
		}

		// The only objectId we set was uid 5.
		if nq.ObjectId != "" {
			require.Equal(t, "_:uid5", nq.ObjectId)
		}
		// Test lang.
		if nq.Subject == "_:uid2" && nq.Predicate == "name" {
			require.Equal(t, "en", nq.Lang)
		}
		// Test facets.
		if nq.Subject == "_:uid4" {
			require.Equal(t, "age", nq.Facets[0].Key)
			require.Equal(t, "close", nq.Facets[1].Key)
			require.Equal(t, "game", nq.Facets[2].Key)
			require.Equal(t, "poem", nq.Facets[3].Key)
			require.Equal(t, "since", nq.Facets[4].Key)
			// byte representation for facets.
			require.Equal(t, []byte{0x21, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, nq.Facets[0].Value)
			require.Equal(t, []byte{0x1}, nq.Facets[1].Value)
			require.Equal(t, []byte("football"), nq.Facets[2].Value)
			require.Equal(t, []byte("roses are red\nviolets are blue"), nq.Facets[3].Value)
			require.Equal(t, "\x01\x00\x00\x00\x0e\xba\b8e\x00\x00\x00\x00\xff\xff",
				string(nq.Facets[4].Value))
			// valtype for facets.
			require.Equal(t, 1, int(nq.Facets[0].ValType))
			require.Equal(t, 3, int(nq.Facets[1].ValType))
			require.Equal(t, 0, int(nq.Facets[2].ValType))
			require.Equal(t, 4, int(nq.Facets[4].ValType))
		}
		// Test label
		if nq.Subject != "_:uid3" && nq.Subject != "_:uid5" {
			require.Equal(t, "author0", nq.Label)
		} else {
			require.Equal(t, "", nq.Label)
		}
		count++
	}
	require.NoError(t, scanner.Err())
	// This order will be preserved due to file naming.
	require.Equal(t, 8, count)

	require.Equal(t, 1, len(schemaFileList))
	file = schemaFileList[0]
	f, err = os.Open(file)
	require.NoError(t, err)

	r, err = gzip.NewReader(f)
	require.NoError(t, err)

	scanner = bufio.NewScanner(r)
	count = 0
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
	require.NoError(t, scanner.Err())
	// This order will be preserved due to file naming
	require.Equal(t, 1, count)
}

// func generateBenchValues() []kv {
// 	byteInt := make([]byte, 4)
// 	binary.LittleEndian.PutUint32(byteInt, 123)
//
// 	fac := []*protos.Facet{
// 		{
// 			Key:   "facetTest",
// 			Value: []byte("testVal"),
// 		},
// 	}
//
// 	geoData, _ := wkb.Marshal(geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518}), binary.LittleEndian)
//
// 	// Posting_STRING   Posting_ValType = 0
// 	// Posting_BINARY   Posting_ValType = 1
// 	// Posting_INT    Posting_ValType = 2
// 	// Posting_FLOAT    Posting_ValType = 3
// 	// Posting_BOOL     Posting_ValType = 4
// 	// Posting_DATE     Posting_ValType = 5
// 	// Posting_DATETIME Posting_ValType = 6
// 	// Posting_GEO      Posting_ValType = 7
// 	// Posting_UID      Posting_ValType = 8
// 	benchItems := []kv{
// 		{
// 			prefix: "testString",
// 			list: &protos.PostingList{
// 				Postings: []*protos.Posting{{
// 					ValType: protos.Posting_STRING,
// 					Value:   []byte("手機裡的眼淚"),
// 					Uid:     uint64(65454),
// 					Facets:  fac,
// 				}},
// 			},
// 		},
// 		{prefix: "testGeo",
// 			list: &protos.PostingList{
// 				Postings: []*protos.Posting{{
// 					ValType: protos.Posting_GEO,
// 					Value:   geoData,
// 					Uid:     uint64(65454),
// 					Facets:  fac,
// 				}},
// 			}},
// 		{prefix: "testPassword",
// 			list: &protos.PostingList{
// 				Postings: []*protos.Posting{{
// 					ValType: protos.Posting_PASSWORD,
// 					Value:   []byte("test"),
// 					Uid:     uint64(65454),
// 					Facets:  fac,
// 				}},
// 			}},
// 		{prefix: "testInt",
// 			list: &protos.PostingList{
// 				Postings: []*protos.Posting{{
// 					ValType: protos.Posting_INT,
// 					Value:   byteInt,
// 					Uid:     uint64(65454),
// 					Facets:  fac,
// 				}},
// 			}},
// 		{prefix: "testUid",
// 			list: &protos.PostingList{
// 				Postings: []*protos.Posting{{
// 					ValType: protos.Posting_INT,
// 					Uid:     uint64(65454),
// 					Facets:  fac,
// 				}},
// 			}},
// 	}
//
// 	return benchItems
// }
//
// func BenchmarkToRDF(b *testing.B) {
// 	buf := new(bytes.Buffer)
// 	buf.Grow(50000)
//
// 	items := generateBenchValues()
//
// 	b.ReportAllocs()
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		toRDF(buf, items[0])
// 		toRDF(buf, items[1])
// 		toRDF(buf, items[2])
// 		toRDF(buf, items[3])
// 		toRDF(buf, items[4])
// 		buf.Reset()
// 	}
// }
