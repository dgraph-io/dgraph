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
	"compress/gzip"
	"context"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
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
		`<1> <name> "pho\ton\u0000" <author0> .`,
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
		rnq := gql.NQuad{NQuad: &nq}
		err = facets.SortAndValidate(rnq.Facets)
		require.NoError(t, err)
		e, err := rnq.ToEdgeUsing(idMap)
		require.NoError(t, err)
		addEdge(t, e, getOrCreate(x.DataKey(e.Attr, e.Entity)))
	}
}

func initTestExport(t *testing.T, schemaStr string) {
	schema.ParseBytes([]byte(schemaStr), 1)

	val, err := (&pb.SchemaUpdate{ValueType: pb.Posting_UID}).Marshal()
	require.NoError(t, err)

	txn := pstore.NewTransactionAt(math.MaxUint64, true)
	require.NoError(t, txn.Set(x.SchemaKey("friend"), val))
	// Schema is always written at timestamp 1
	require.NoError(t, txn.CommitAt(1, nil))
	txn.Discard()

	require.NoError(t, err)
	val, err = (&pb.SchemaUpdate{ValueType: pb.Posting_UID}).Marshal()
	require.NoError(t, err)

	txn = pstore.NewTransactionAt(math.MaxUint64, true)
	txn.Set(x.SchemaKey("http://www.w3.org/2000/01/rdf-schema#range"), val)
	require.NoError(t, err)
	txn.Set(x.SchemaKey("friend_not_served"), val)
	require.NoError(t, txn.CommitAt(1, nil))
	txn.Discard()
	populateGraphExport(t)
}

func TestExport(t *testing.T) {
	// Index the name predicate. We ensure it doesn't show up on export.
	initTestExport(t, "name:string @index .")
	// Remove already existing export folders is any.
	bdir, err := ioutil.TempDir("", "export")
	require.NoError(t, err)
	defer os.RemoveAll(bdir)

	time.Sleep(1 * time.Second)

	// We have 4 friend type edges. FP("friends")%10 = 2.
	Config.ExportPath = bdir
	readTs := timestamp()
	// Do the following so export won't block forever for readTs.
	posting.Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
	err = export(context.Background(), &pb.ExportRequest{ReadTs: readTs, GroupId: 1})
	require.NoError(t, err)

	searchDir := bdir
	fileList := []string{}
	schemaFileList := []string{}
	err = filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}
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
	require.Equal(t, 1, len(fileList), "filelist=%v", fileList)

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
				require.Equal(t, &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "pho\ton"}},
					nq.ObjectValue)
			case "_:uid3":
				require.Equal(t, &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "First Line\nSecondLine"}},
					nq.ObjectValue)
			case "_:uid4":
			case "_:uid5":
				require.Equal(t, `<_:uid5> <name> "" .`, scanner.Text())
			default:
				t.Errorf("Unexpected subject: %v", nq.Subject)
			}
			if nq.Subject == "_:uid1" || nq.Subject == "_:uid2" {
				require.Equal(t, &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "pho\ton"}},
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
		// Labels have been removed.
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

type skv struct {
	attr   string
	schema pb.SchemaUpdate
}

func TestToSchema(t *testing.T) {
	testCases := []struct {
		skv      *skv
		expected string
	}{
		{
			skv: &skv{
				attr: "Alice",
				schema: pb.SchemaUpdate{
					Predicate: "mother",
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_REVERSE,
					List:      false,
					Count:     true,
					Upsert:    true,
					Lang:      true,
				},
			},
			expected: "Alice:string @reverse @count @lang @upsert . \n",
		},
		{
			skv: &skv{
				attr: "Alice:best",
				schema: pb.SchemaUpdate{
					Predicate: "mother",
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_REVERSE,
					List:      false,
					Count:     false,
					Upsert:    false,
					Lang:      true,
				},
			},
			expected: "<Alice:best>:string @reverse @lang . \n",
		},
		{
			skv: &skv{
				attr: "username/password",
				schema: pb.SchemaUpdate{
					Predicate: "",
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_NONE,
					List:      false,
					Count:     false,
					Upsert:    false,
					Lang:      false,
				},
			},
			expected: "<username/password>:string . \n",
		},
		{
			skv: &skv{
				attr: "B*-tree",
				schema: pb.SchemaUpdate{
					Predicate: "",
					ValueType: pb.Posting_UID,
					Directive: pb.SchemaUpdate_REVERSE,
					List:      true,
					Count:     false,
					Upsert:    false,
					Lang:      false,
				},
			},
			expected: "<B*-tree>:[uid] @reverse . \n",
		},
		{
			skv: &skv{
				attr: "base_de_données",
				schema: pb.SchemaUpdate{
					Predicate: "",
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_NONE,
					List:      false,
					Count:     false,
					Upsert:    false,
					Lang:      true,
				},
			},
			expected: "<base_de_données>:string @lang . \n",
		},
		{
			skv: &skv{
				attr: "data_base",
				schema: pb.SchemaUpdate{
					Predicate: "",
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_NONE,
					List:      false,
					Count:     false,
					Upsert:    false,
					Lang:      true,
				},
			},
			expected: "data_base:string @lang . \n",
		},
		{
			skv: &skv{
				attr: "data.base",
				schema: pb.SchemaUpdate{
					Predicate: "",
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_NONE,
					List:      false,
					Count:     false,
					Upsert:    false,
					Lang:      true,
				},
			},
			expected: "data.base:string @lang . \n",
		},
	}
	for _, testCase := range testCases {
		list, err := toSchema(testCase.skv.attr, testCase.skv.schema)
		require.NoError(t, err)
		require.Equal(t, testCase.expected, string(list.Kv[0].Value))
	}
}

// func generateBenchValues() []kv {
// 	byteInt := make([]byte, 4)
// 	binary.LittleEndian.PutUint32(byteInt, 123)
//
// 	fac := []*api.Facet{
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
// 			list: &pb.PostingList{
// 				Postings: []*pb.Posting{{
// 					ValType: pb.Posting_STRING,
// 					Value:   []byte("手機裡的眼淚"),
// 					Uid:     uint64(65454),
// 					Facets:  fac,
// 				}},
// 			},
// 		},
// 		{prefix: "testGeo",
// 			list: &pb.PostingList{
// 				Postings: []*pb.Posting{{
// 					ValType: pb.Posting_GEO,
// 					Value:   geoData,
// 					Uid:     uint64(65454),
// 					Facets:  fac,
// 				}},
// 			}},
// 		{prefix: "testPassword",
// 			list: &pb.PostingList{
// 				Postings: []*pb.Posting{{
// 					ValType: pb.Posting_PASSWORD,
// 					Value:   []byte("test"),
// 					Uid:     uint64(65454),
// 					Facets:  fac,
// 				}},
// 			}},
// 		{prefix: "testInt",
// 			list: &pb.PostingList{
// 				Postings: []*pb.Posting{{
// 					ValType: pb.Posting_INT,
// 					Value:   byteInt,
// 					Uid:     uint64(65454),
// 					Facets:  fac,
// 				}},
// 			}},
// 		{prefix: "testUid",
// 			list: &pb.PostingList{
// 				Postings: []*pb.Posting{{
// 					ValType: pb.Posting_INT,
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
