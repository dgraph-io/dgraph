package worker

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkb"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

func populateGraphBackup(t *testing.T) {
	friendFacets := map[string]string{
		"since": "2005-05-02T15:04:05", "close": "true", "age": "33"}
	edge := &task.DirectedEdge{
		ValueId: 5,
		Label:   "author0",
		Attr:    "friend",
	}
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 1)))

	edge.Entity = 2
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 2)))

	edge.Entity = 3
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 3)))

	//Add an edge with facet.
	addEdgeToUID(t, "friend", 4, 5, friendFacets)

	edge.Entity = 1
	edge.ValueId = 0
	edge.Value = []byte("pho\\ton")
	edge.Attr = "name"
	addEdge(t, edge, getOrCreate(x.DataKey("name", 1)))

	edge.Entity = 2
	edge.Lang = "en"
	addEdge(t, edge, getOrCreate(x.DataKey("name", 2)))
}

func makeFacets(facetKVs map[string]string) (fs []*facets.Facet, err error) {
	if len(facetKVs) == 0 {
		return nil, nil
	}
	allKeys := make([]string, 0, len(facetKVs))
	for k := range facetKVs {
		allKeys = append(allKeys, k)
	}
	sort.Strings(allKeys)

	for _, k := range allKeys {
		v := facetKVs[k]
		typ, err := facets.ValType(v)
		if err != nil {
			return nil, err
		}
		fs = append(fs, &facets.Facet{
			k,
			[]byte(v),
			typ,
		})
	}
	return fs, nil
}

func addEdgeToUID(t *testing.T, attr string, src uint64,
	dst uint64, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &task.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      task.DirectedEdge_SET,
		Facets:  fs,
	}
	addEdge(t, edge, getOrCreate(x.DataKey(attr, src)))
}

func initTestBackup(t *testing.T, schemaStr string) (string, *store.Store) {
	schema.ParseBytes([]byte(schemaStr))
	group.ParseGroupConfig("groups.conf")

	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	posting.Init(ps)
	Init(ps)
	populateGraphBackup(t)
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	return dir, ps
}

func TestBackup(t *testing.T) {
	// Index the name predicate. We ensure it doesn't show up on backup.
	dir, ps := initTestBackup(t, "scalar name:string @index")
	defer os.RemoveAll(dir)
	defer ps.Close()
	// Remove already existing backup folders is any.
	bdir, err := ioutil.TempDir("", "backup")
	require.NoError(t, err)
	defer os.RemoveAll(bdir)

	posting.CommitLists(10)
	time.Sleep(time.Second)

	// We have 4 friend type edges. FP("friends")%10 = 2.
	err = backup(group.BelongsTo("friend"), bdir)
	require.NoError(t, err)

	// We have 2 name type edges(with index). FP("name")%10 =7.
	err = backup(group.BelongsTo("name"), bdir)
	require.NoError(t, err)

	searchDir := bdir
	fileList := []string{}
	err = filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
		if path != bdir {
			fileList = append(fileList, path)
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
				require.Equal(t, &graph.Value{&graph.Value_DefaultVal{"pho\\ton"}},
					nq.ObjectValue)
			}
			// The only objectId we set was uid 5.
			if nq.ObjectId != "" {
				require.Equal(t, "0x5", nq.ObjectId)
			}

			if nq.Subject == "0x4" {
				require.Equal(t, "age", nq.Facets[0].Key)
				require.Equal(t, "close", nq.Facets[1].Key)
				require.Equal(t, "since", nq.Facets[2].Key)
				require.Equal(t, "33", string(nq.Facets[0].Value))
				require.Equal(t, "true", string(nq.Facets[1].Value))
				require.Equal(t, "2005-05-02T15:04:05", string(nq.Facets[2].Value))
				require.Equal(t, 1, int(nq.Facets[0].ValType))
				require.Equal(t, 3, int(nq.Facets[1].ValType))
				require.Equal(t, 4, int(nq.Facets[2].ValType))
			}
			count++
		}
		counts = append(counts, count)
		require.NoError(t, scanner.Err())
	}
	// This order will bw presereved due to file naming.
	require.Equal(t, []int{4, 2}, counts)
}

func generateBenchValues() []kv {
	byteInt := make([]byte, 4)
	binary.LittleEndian.PutUint32(byteInt, 123)

	fac := []*facets.Facet{
		&facets.Facet{
			Key:   "facetTest",
			Value: []byte("testVal"),
		},
	}

	geoData, _ := wkb.Marshal(geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518}), binary.LittleEndian)

	// Posting_STRING   Posting_ValType = 0
	// Posting_BINARY   Posting_ValType = 1
	// Posting_INT32    Posting_ValType = 2
	// Posting_FLOAT    Posting_ValType = 3
	// Posting_BOOL     Posting_ValType = 4
	// Posting_DATE     Posting_ValType = 5
	// Posting_DATETIME Posting_ValType = 6
	// Posting_GEO      Posting_ValType = 7
	// Posting_UID      Posting_ValType = 8
	benchItems := []kv{
		kv{
			prefix: "testString",
			list: &types.PostingList{
				Postings: []*types.Posting{&types.Posting{
					ValType: types.Posting_STRING,
					Value:   []byte("手機裡的眼淚"),
					Uid:     uint64(65454),
					Facets:  fac,
				}},
			},
		},
		kv{prefix: "testGeo",
			list: &types.PostingList{
				Postings: []*types.Posting{&types.Posting{
					ValType: types.Posting_GEO,
					Value:   geoData,
					Uid:     uint64(65454),
					Facets:  fac,
				}},
			}},
		kv{prefix: "testPassword",
			list: &types.PostingList{
				Postings: []*types.Posting{&types.Posting{
					ValType: types.Posting_PASSWORD,
					Value:   []byte("test"),
					Uid:     uint64(65454),
					Facets:  fac,
				}},
			}},
		kv{prefix: "testInt",
			list: &types.PostingList{
				Postings: []*types.Posting{&types.Posting{
					ValType: types.Posting_INT32,
					Value:   byteInt,
					Uid:     uint64(65454),
					Facets:  fac,
				}},
			}},
		kv{prefix: "testUid",
			list: &types.PostingList{
				Postings: []*types.Posting{&types.Posting{
					ValType: types.Posting_INT32,
					Uid:     uint64(65454),
					Facets:  fac,
				}},
			}},
	}

	return benchItems
}

func BenchmarkToRDF_New(b *testing.B) {
	runBenchToRDF(b, toRDF)
}

func runBenchToRDF(b *testing.B, f func(buf *bytes.Buffer, item kv)) {
	buf := new(bytes.Buffer)
	buf.Grow(50000)

	items := generateBenchValues()

	var i int
	b.ReportAllocs()
	b.ResetTimer()
	for i = 0; i < b.N; i++ {
		f(buf, items[0])
		f(buf, items[1])
		f(buf, items[2])
		f(buf, items[3])
		f(buf, items[4])
		buf.Reset()
	}
}
