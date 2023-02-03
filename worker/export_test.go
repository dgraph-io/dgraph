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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/dql"
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

const (
	gqlSchema = "type Example { name: String }"
)

var personType = &pb.TypeUpdate{
	TypeName: x.GalaxyAttr("Person"),
	Fields: []*pb.SchemaUpdate{
		{
			Predicate: x.GalaxyAttr("name"),
		},
		{
			Predicate: x.GalaxyAttr("friend"),
		},
		{
			Predicate: x.GalaxyAttr("~friend"),
		},
		{
			Predicate: x.GalaxyAttr("friend_not_served"),
		},
	},
}

func populateGraphExport(t *testing.T) {
	rdfEdges := []string{
		`<1> <friend> <5> .`,
		`<2> <friend> <5> .`,
		`<3> <friend> <5> .`,
		`<4> <friend> <5> (since=2005-05-02T15:04:05,close=true,` +
			`age=33,game="football",poem="roses are red\nviolets are blue") .`,
		`<1> <name> "pho\ton\u0000" .`,
		`<2> <name> "pho\ton"@en .`,
		`<3> <name> "First Line\nSecondLine" .`,
		"<1> <friend_not_served> <5> .",
		`<5> <name> "" .`,
		`<6> <name> "Ding!\u0007Ding!\u0007Ding!\u0007" .`,
		`<7> <name> "node_to_delete" .`,
		fmt.Sprintf("<8> <dgraph.graphql.schema> \"%s\" .", gqlSchema),
		`<8> <dgraph.graphql.xid> "dgraph.graphql.schema" .`,
		`<8> <dgraph.type> "dgraph.graphql" .`,
		`<9> <name> "ns2" <0x2> .`,
		`<10> <name> "ns2_node_to_delete" <0x2> .`,
	}
	// This triplet will be deleted to ensure deleted nodes do not affect the output of the export.
	edgesToDelete := []string{
		`<7> <name> "node_to_delete" .`,
		`<10> <name> "ns2_node_to_delete" <0x2> .`,
	}

	idMap := map[string]uint64{
		"1": 1,
		"2": 2,
		"3": 3,
		"4": 4,
		"5": 5,
		"6": 6,
		"7": 7,
	}

	l := &lex.Lexer{}
	processEdge := func(edge string, set bool) {
		nq, err := chunker.ParseRDF(edge, l)
		require.NoError(t, err)
		rnq := dql.NQuad{NQuad: &nq}
		err = facets.SortAndValidate(rnq.Facets)
		require.NoError(t, err)
		e, err := rnq.ToEdgeUsing(idMap)
		e.Attr = x.NamespaceAttr(nq.Namespace, e.Attr)
		require.NoError(t, err)
		if set {
			addEdge(t, e, getOrCreate(x.DataKey(e.Attr, e.Entity)))
		} else {
			delEdge(t, e, getOrCreate(x.DataKey(e.Attr, e.Entity)))
		}
	}

	for _, edge := range rdfEdges {
		processEdge(edge, true)
	}
	for _, edge := range edgesToDelete {
		processEdge(edge, false)
	}
}

func initTestExport(t *testing.T, schemaStr string) {
	require.NoError(t, schema.ParseBytes([]byte(schemaStr), 1))

	val, err := (&pb.SchemaUpdate{ValueType: pb.Posting_UID}).Marshal()
	require.NoError(t, err)

	txn := pstore.NewTransactionAt(math.MaxUint64, true)
	require.NoError(t, txn.Set(testutil.GalaxySchemaKey("friend"), val))
	// Schema is always written at timestamp 1
	require.NoError(t, txn.CommitAt(1, nil))

	require.NoError(t, err)
	val, err = (&pb.SchemaUpdate{ValueType: pb.Posting_UID}).Marshal()
	require.NoError(t, err)

	txn = pstore.NewTransactionAt(math.MaxUint64, true)
	err = txn.Set(testutil.GalaxySchemaKey("http://www.w3.org/2000/01/rdf-schema#range"), val)
	require.NoError(t, err)
	require.NoError(t, txn.Set(testutil.GalaxySchemaKey("friend_not_served"), val))
	require.NoError(t, txn.Set(testutil.GalaxySchemaKey("age"), val))
	require.NoError(t, txn.CommitAt(1, nil))

	val, err = personType.Marshal()
	require.NoError(t, err)

	txn = pstore.NewTransactionAt(math.MaxUint64, true)
	require.NoError(t, txn.Set(testutil.GalaxyTypeKey("Person"), val))
	require.NoError(t, txn.CommitAt(1, nil))

	populateGraphExport(t)

	// Drop age predicate after populating DB.
	// age should not exist in the exported schema.
	txn = pstore.NewTransactionAt(math.MaxUint64, true)
	require.NoError(t, txn.Delete(testutil.GalaxySchemaKey("age")))
	require.NoError(t, txn.CommitAt(1, nil))
}

func getExportFileList(t *testing.T, bdir string) (dataFiles, schemaFiles, gqlSchema []string) {
	searchDir := bdir
	err := filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			return nil
		}
		if path != bdir {
			switch {
			case strings.Contains(path, "gql_schema"):
				gqlSchema = append(gqlSchema, path)
			case strings.Contains(path, "schema"):
				schemaFiles = append(schemaFiles, path)
			default:
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
	var buf bytes.Buffer
	buf.ReadFrom(r)

	result, err := schema.Parse(buf.String())
	require.NoError(t, err)

	require.Equal(t, 2, len(result.Preds))
	require.Equal(t, "uid", types.TypeID(result.Preds[0].ValueType).Name())
	require.Equal(t, x.GalaxyAttr("http://www.w3.org/2000/01/rdf-schema#range"),
		result.Preds[1].Predicate)
	require.Equal(t, "uid", types.TypeID(result.Preds[1].ValueType).Name())

	require.Equal(t, 1, len(result.Types))
	require.True(t, proto.Equal(result.Types[0], personType))
}

func checkExportGqlSchema(t *testing.T, gqlSchemaFiles []string) {
	require.Equal(t, 1, len(gqlSchemaFiles))
	file := gqlSchemaFiles[0]
	f, err := os.Open(file)
	require.NoError(t, err)

	r, err := gzip.NewReader(f)
	require.NoError(t, err)
	var buf bytes.Buffer
	buf.ReadFrom(r)
	expected := []x.ExportedGQLSchema{{Namespace: x.GalaxyNamespace, Schema: gqlSchema}}
	b, err := json.Marshal(expected)
	require.NoError(t, err)
	require.JSONEq(t, string(b), buf.String())
}

func TestExportRdf(t *testing.T) {
	// Index the name predicate. We ensure it doesn't show up on export.
	initTestExport(t, `
		name: string @index(exact) .
		age: int .
		[0x2] name: string @index(exact) .
		`)

	bdir, err := ioutil.TempDir("", "export")
	require.NoError(t, err)
	defer os.RemoveAll(bdir)

	time.Sleep(1 * time.Second)

	// We have 4 friend type edges. FP("friends")%10 = 2.
	x.WorkerConfig.ExportPath = bdir
	readTs := timestamp()
	// Do the following so export won't block forever for readTs.
	posting.Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
	files, err := export(context.Background(), &pb.ExportRequest{ReadTs: readTs, GroupId: 1,
		Namespace: math.MaxUint64, Format: "rdf"})
	require.NoError(t, err)

	fileList, schemaFileList, gqlSchema := getExportFileList(t, bdir)
	require.Equal(t, len(files), len(fileList)+len(schemaFileList)+len(gqlSchema))

	file := fileList[0]
	f, err := os.Open(file)
	require.NoError(t, err)

	r, err := gzip.NewReader(f)
	require.NoError(t, err)

	scanner := bufio.NewScanner(r)
	count := 0

	l := &lex.Lexer{}
	for scanner.Scan() {
		nq, err := chunker.ParseRDF(scanner.Text(), l)
		require.NoError(t, err)
		require.Contains(t, []string{"0x1", "0x2", "0x3", "0x4", "0x5", "0x6", "0x9"}, nq.Subject)
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
				require.Equal(t, `<0x5> <name> "" <0x0> .`, scanner.Text())
			case "0x6":
				require.Equal(t, `<0x6> <name> "Ding!\u0007Ding!\u0007Ding!\u0007" <0x0> .`,
					scanner.Text())
			case "0x9":
				require.Equal(t, `<0x9> <name> "ns2" <0x2> .`, scanner.Text())
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
	require.Equal(t, 10, count)

	checkExportSchema(t, schemaFileList)
	checkExportGqlSchema(t, gqlSchema)
}

func TestExportJson(t *testing.T) {
	// Index the name predicate. We ensure it doesn't show up on export.
	initTestExport(t, `name: string @index(exact) .
				 [0x2] name: string @index(exact) .`)

	bdir, err := ioutil.TempDir("", "export")
	require.NoError(t, err)
	defer os.RemoveAll(bdir)

	time.Sleep(1 * time.Second)

	// We have 4 friend type edges. FP("friends")%10 = 2.
	x.WorkerConfig.ExportPath = bdir
	readTs := timestamp()
	// Do the following so export won't block forever for readTs.
	posting.Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
	req := pb.ExportRequest{ReadTs: readTs, GroupId: 1, Format: "json", Namespace: math.MaxUint64}
	files, err := export(context.Background(), &req)
	require.NoError(t, err)

	fileList, schemaFileList, gqlSchema := getExportFileList(t, bdir)
	require.Equal(t, len(files), len(fileList)+len(schemaFileList)+len(gqlSchema))

	file := fileList[0]
	f, err := os.Open(file)
	require.NoError(t, err)

	r, err := gzip.NewReader(f)
	require.NoError(t, err)

	wantJson := `
	[
		{"uid":"0x1","namespace":"0x0","name":"pho\ton"},
		{"uid":"0x2","namespace":"0x0","name@en":"pho\ton"},
		{"uid":"0x3","namespace":"0x0","name":"First Line\nSecondLine"},
		{"uid":"0x5","namespace":"0x0","name":""},
		{"uid":"0x6","namespace":"0x0","name":"Ding!\u0007Ding!\u0007Ding!\u0007"},
		{"uid":"0x1","namespace":"0x0","friend":[{"uid":"0x5"}]},
		{"uid":"0x2","namespace":"0x0","friend":[{"uid":"0x5"}]},
		{"uid":"0x3","namespace":"0x0","friend":[{"uid":"0x5"}]},
		{"uid":"0x4","namespace":"0x0","friend":[{"uid":"0x5","friend|age":33,
			"friend|close":"true","friend|game":"football",
			"friend|poem":"roses are red\nviolets are blue","friend|since":"2005-05-02T15:04:05Z"}]},
		{"uid":"0x9","namespace":"0x2","name":"ns2"}
	]
	`
	gotJson, err := ioutil.ReadAll(r)
	require.NoError(t, err)
	var expected interface{}
	err = json.Unmarshal([]byte(wantJson), &expected)
	require.NoError(t, err)

	var actual interface{}
	err = json.Unmarshal(gotJson, &actual)
	require.NoError(t, err)
	require.ElementsMatch(t, expected, actual)

	checkExportSchema(t, schemaFileList)
	checkExportGqlSchema(t, gqlSchema)
}

const exportRequest = `mutation export($format: String!) {
	export(input: {format: $format}) {
		response { code }
		taskId
	}
}`

func TestExportFormat(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "export")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	adminUrl := "http://" + testutil.SockAddrHttp + "/admin"
	err = testutil.CheckForGraphQLEndpointToReady(t)
	require.NoError(t, err)

	params := testutil.GraphQLParams{
		Query:     exportRequest,
		Variables: map[string]interface{}{"format": "json"},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	require.Equal(t, "Success", testutil.JsonGet(data, "data", "export", "response", "code").(string))
	taskId := testutil.JsonGet(data, "data", "export", "taskId").(string)
	testutil.WaitForTask(t, taskId, false, testutil.SockAddrHttp)

	params.Variables["format"] = "rdf"
	b, err = json.Marshal(params)
	require.NoError(t, err)

	resp, err = http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	testutil.RequireNoGraphQLErrors(t, resp)

	params.Variables["format"] = "xml"
	b, err = json.Marshal(params)
	require.NoError(t, err)
	resp, err = http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)

	defer resp.Body.Close()
	b, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	var result *testutil.GraphQLResponse
	err = json.Unmarshal(b, &result)
	require.NoError(t, err)
	require.NotNil(t, result.Errors)
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
				attr: x.GalaxyAttr("Alice"),
				schema: pb.SchemaUpdate{
					Predicate: x.GalaxyAttr("mother"),
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_REVERSE,
					List:      false,
					Count:     true,
					Upsert:    true,
					Lang:      true,
				},
			},
			expected: "[0x0] <Alice>:string @reverse @count @lang @upsert . \n",
		},
		{
			skv: &skv{
				attr: x.NamespaceAttr(0xf2, "Alice:best"),
				schema: pb.SchemaUpdate{
					Predicate: x.NamespaceAttr(0xf2, "mother"),
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_REVERSE,
					List:      false,
					Count:     false,
					Upsert:    false,
					Lang:      true,
				},
			},
			expected: "[0xf2] <Alice:best>:string @reverse @lang . \n",
		},
		{
			skv: &skv{
				attr: x.GalaxyAttr("username/password"),
				schema: pb.SchemaUpdate{
					Predicate: x.GalaxyAttr(""),
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_NONE,
					List:      false,
					Count:     false,
					Upsert:    false,
					Lang:      false,
				},
			},
			expected: "[0x0] <username/password>:string . \n",
		},
		{
			skv: &skv{
				attr: x.GalaxyAttr("B*-tree"),
				schema: pb.SchemaUpdate{
					Predicate: x.GalaxyAttr(""),
					ValueType: pb.Posting_UID,
					Directive: pb.SchemaUpdate_REVERSE,
					List:      true,
					Count:     false,
					Upsert:    false,
					Lang:      false,
				},
			},
			expected: "[0x0] <B*-tree>:[uid] @reverse . \n",
		},
		{
			skv: &skv{
				attr: x.GalaxyAttr("base_de_données"),
				schema: pb.SchemaUpdate{
					Predicate: x.GalaxyAttr(""),
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_NONE,
					List:      false,
					Count:     false,
					Upsert:    false,
					Lang:      true,
				},
			},
			expected: "[0x0] <base_de_données>:string @lang . \n",
		},
		{
			skv: &skv{
				attr: x.GalaxyAttr("data_base"),
				schema: pb.SchemaUpdate{
					Predicate: x.GalaxyAttr(""),
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_NONE,
					List:      false,
					Count:     false,
					Upsert:    false,
					Lang:      true,
				},
			},
			expected: "[0x0] <data_base>:string @lang . \n",
		},
		{
			skv: &skv{
				attr: x.GalaxyAttr("data.base"),
				schema: pb.SchemaUpdate{
					Predicate: x.GalaxyAttr(""),
					ValueType: pb.Posting_STRING,
					Directive: pb.SchemaUpdate_NONE,
					List:      false,
					Count:     false,
					Upsert:    false,
					Lang:      true,
				},
			},
			expected: "[0x0] <data.base>:string @lang . \n",
		},
	}
	for _, testCase := range testCases {
		kv := toSchema(testCase.skv.attr, &testCase.skv.schema)
		require.Equal(t, testCase.expected, string(kv.Value))
	}
}
