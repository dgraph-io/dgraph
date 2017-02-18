package worker

import (
	"bufio"
	"compress/gzip"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

func populateGraphBackup(t *testing.T) {
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

	edge.Entity = 4
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 4)))

	edge.Entity = 1
	edge.ValueId = 0
	edge.Value = []byte("pho\\ton")
	edge.Attr = "name"
	addEdge(t, edge, getOrCreate(x.DataKey("name", 1)))

	edge.Entity = 2
	edge.Lang = "en"
	addEdge(t, edge, getOrCreate(x.DataKey("name", 2)))
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
			count++
		}
		counts = append(counts, count)
		require.NoError(t, scanner.Err())
	}
	// This order will bw presereved due to file naming.
	require.Equal(t, []int{4, 2}, counts)
}
