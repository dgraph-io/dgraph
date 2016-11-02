package worker

import (
	"bufio"
	"compress/gzip"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func populateGraphBackup(t *testing.T) {
	edge := x.DirectedEdge{
		ValueId:   23,
		Source:    "author0",
		Timestamp: time.Now(),
		Attribute: "friend",
	}
	edge.Entity = 10
	addEdge(t, edge, getOrCreate(posting.Key(10, "friend")))

	edge.Entity = 11
	addEdge(t, edge, getOrCreate(posting.Key(11, "friend")))

	edge.Entity = 12
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend")))

	edge.ValueId = 25
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend")))

	edge.ValueId = 26
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend")))

	edge.Entity = 10
	edge.ValueId = 31
	addEdge(t, edge, getOrCreate(posting.Key(10, "friend")))

	edge.Entity = 12
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend")))

	edge.Entity = 12
	edge.Value = []byte("photon")
	edge.Attribute = "name"
	addEdge(t, edge, getOrCreate(posting.Key(12, "name")))

	edge.Entity = 10
	addEdge(t, edge, getOrCreate(posting.Key(10, "name")))
}

func initTestBackup(t *testing.T, schemaStr string) (string, *store.Store) {
	schema.ParseBytes([]byte(schemaStr))
	ParseGroupConfig("groups.conf")

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
	os.RemoveAll("backup/")
	defer os.RemoveAll("backup/")

	posting.MergeLists(10)

	// We have 7 friend type edges.
	err := Backup(BelongsTo("friend"))
	require.NoError(t, err)

	// We have 2 name type edges(with index).
	err = Backup(BelongsTo("name"))
	require.NoError(t, err)

	searchDir := "backup/"
	fileList := []string{}
	err = filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
		if path != "backup/" {
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
			count++
		}
		counts = append(counts, count)
		require.NoError(t, scanner.Err())
	}

	require.Equal(t, counts, []int{7, 2})
}
