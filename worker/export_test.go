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
	"net/http"
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
	"github.com/dgraph-io/dgraph/z"

	"github.com/dgraph-io/dgraph/chunker/rdf"
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
		`<6> <name> "Ding!\u0007Ding!\u0007Ding!\u0007" .`,
	}
	idMap := map[string]uint64{
		"1": 1,
		"2": 2,
		"3": 3,
		"4": 4,
		"5": 5,
		"6": 6,
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

func getExportFileList(t *testing.T, bdir string) (dataFiles, schemaFiles []string) {
	searchDir := bdir
	err := filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}
		if path != bdir {
			if strings.Contains(path, "schema") {
				schemaFiles = append(schemaFiles, path)
			} else {
				dataFiles = append(dataFiles, path)
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(dataFiles), "filelist=%v", dataFiles)

	return
}

func checkExportSchema(t *testing.T, schemaFileList []string) {
	require.Equal(t, 1, len(schemaFileList))
	file := schemaFileList[0]
	f, err := os.Open(file)
	require.NoError(t, err)

	r, err := gzip.NewReader(f)
	require.NoError(t, err)

	scanner := bufio.NewScanner(r)
	count := 0
	for scanner.Scan() {
		result, err := schema.Parse(scanner.Text())
		require.NoError(t, err)
		require.Equal(t, 1, len(result.Schemas))
		// We wrote schema for only two predicates
		if result.Schemas[0].Predicate == "friend" {
			require.Equal(t, "uid", types.TypeID(result.Schemas[0].ValueType).Name())
		} else {
			require.Equal(t, "http://www.w3.org/2000/01/rdf-schema#range",
				result.Schemas[0].Predicate)
			require.Equal(t, "uid", types.TypeID(result.Schemas[0].ValueType).Name())
		}
		count = len(result.Schemas)
	}
	require.NoError(t, scanner.Err())
	// This order will be preserved due to file naming
	require.Equal(t, 1, count)
}

func TestExportRdf(t *testing.T) {
	// Index the name predicate. We ensure it doesn't show up on export.
	initTestExport(t, "name:string @index .")
	// Remove already existing export folders is any.
	bdir, err := ioutil.TempDir("", "export")
	require.NoError(t, err)
	defer os.RemoveAll(bdir)

	time.Sleep(1 * time.Second)

	// We have 4 friend type edges. FP("friends")%10 = 2.
	x.WorkerConfig.ExportPath = bdir
	readTs := timestamp()
	// Do the following so export won't block forever for readTs.
	posting.Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
	err = export(context.Background(), &pb.ExportRequest{ReadTs: readTs, GroupId: 1, Format: "rdf"})
	require.NoError(t, err)

	fileList, schemaFileList := getExportFileList(t, bdir)

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
		require.Contains(t, []string{"0x1", "0x2", "0x3", "0x4", "0x5", "0x6"}, nq.Subject)
		if nq.ObjectValue != nil {
			switch nq.Subject {
			case "0x1", "0x2":
				require.Equal(t, &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "pho\ton"}},
					nq.ObjectValue)
			case "0x3":
				require.Equal(t, &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "First Line\nSecondLine"}},
					nq.ObjectValue)
			case "0x4":
			case "0x5":
				require.Equal(t, `<0x5> <name> "" .`, scanner.Text())
			case "0x6":
				require.Equal(t, `<0x6> <name> "Ding!\u0007Ding!\u0007Ding!\u0007" .`, scanner.Text())
			default:
				t.Errorf("Unexpected subject: %v", nq.Subject)
			}
			if nq.Subject == "_:uid1" || nq.Subject == "0x2" {
				require.Equal(t, &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "pho\ton"}},
					nq.ObjectValue)
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
	require.Equal(t, 9, count)

	checkExportSchema(t, schemaFileList)
}

func TestExportJson(t *testing.T) {
	bdir, err := ioutil.TempDir("", "export")
	require.NoError(t, err)
	defer os.RemoveAll(bdir)

	time.Sleep(1 * time.Second)

	// We have 4 friend type edges. FP("friends")%10 = 2.
	x.WorkerConfig.ExportPath = bdir
	readTs := timestamp()
	// Do the following so export won't block forever for readTs.
	posting.Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
	req := pb.ExportRequest{ReadTs: readTs, GroupId: 1, Format: "json"}
	err = export(context.Background(), &req)
	require.NoError(t, err)

	fileList, schemaFileList := getExportFileList(t, bdir)

	file := fileList[0]
	f, err := os.Open(file)
	require.NoError(t, err)

	r, err := gzip.NewReader(f)
	require.NoError(t, err)

	wantJson := `
[
  {"uid":"0x1","name":"pho\ton"},
  {"uid":"0x2","name@en":"pho\ton"},
  {"uid":"0x3","name":"First Line\nSecondLine"},
  {"uid":"0x5","name":""},
  {"uid":"0x6","name":"Ding!\u0007Ding!\u0007Ding!\u0007"},
  {"uid":"0x1","friend":[{"uid":"0x5"}]},
  {"uid":"0x2","friend":[{"uid":"0x5"}]},
  {"uid":"0x3","friend":[{"uid":"0x5"}]},
  {"uid":"0x4","friend":[{"uid":"0x5"}],"friend|age":33,"friend|close":"true","friend|game":"football","friend|poem":"roses are red\nviolets are blue","friend|since":"2005-05-02T15:04:05Z"}
]
`
	gotJson, err := ioutil.ReadAll(r)
	require.NoError(t, err)
	require.JSONEq(t, wantJson, string(gotJson))

	checkExportSchema(t, schemaFileList)
}

func TestExportFormat(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "export")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	resp, err := http.Get("http://" + z.SockAddrHttp + "/admin/export?format=json")
	require.NoError(t, err)

	resp, err = http.Get("http://" + z.SockAddrHttp + "/admin/export?format=rdf")
	require.NoError(t, err)

	resp, err = http.Get("http://" + z.SockAddrHttp + "/admin/export?format=xml")
	require.NoError(t, err)
	require.NotEqual(t, resp.StatusCode, http.StatusOK)

	resp, err = http.Get("http://" + z.SockAddrHttp + "/admin/export?output=rdf")
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)
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
