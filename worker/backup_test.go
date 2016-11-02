package worker

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func populateGraphBackup(t *testing.T) {
	edge := x.DirectedEdge{
		ValueId:   5,
		Source:    "author0",
		Timestamp: time.Now(),
		Attribute: "friend",
	}
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend")))

	edge.Entity = 2
	addEdge(t, edge, getOrCreate(posting.Key(2, "friend")))

	edge.Entity = 3
	addEdge(t, edge, getOrCreate(posting.Key(3, "friend")))

	edge.Entity = 4
	addEdge(t, edge, getOrCreate(posting.Key(4, "friend")))

	edge.Entity = 1
	edge.ValueId = 0
	edge.Value = []byte("photon")
	edge.Attribute = "name"
	addEdge(t, edge, getOrCreate(posting.Key(1, "name")))

	edge.Entity = 2
	addEdge(t, edge, getOrCreate(posting.Key(2, "name")))
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
	bdir := "backup_test"
	os.RemoveAll(bdir)
	defer os.RemoveAll(bdir)

	posting.MergeLists(10)

	// We have 4 friend type edges. FP("friends")%10 = 2.
	err := Backup(BelongsTo("friend"), bdir)
	require.NoError(t, err)

	// We have 2 name type edges(with index). FP("name")%10 =7.
	err = Backup(BelongsTo("name"), bdir)
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
			require.Contains(t, []string{"_uid_:1", "_uid_:2", "_uid_:3", "_uid_:4"}, nq.Subject)
			// The only value we set was "photon".
			if !bytes.Equal(nq.ObjectValue, nil) {
				require.Equal(t, nq.ObjectValue, []byte("photon"))
			}
			// The only objectId we set was uid 5.
			if nq.ObjectId != "" {
				require.Equal(t, nq.ObjectId, "_uid_:5")
			}
			count++
		}
		counts = append(counts, count)
		require.NoError(t, scanner.Err())
	}
	// This order will bw presereved due to file naming.
	require.Equal(t, counts, []int{4, 2})
}
