package bidx

import (
	"bufio"
	"log"
	"os"
	"path"
	"strconv"

	"github.com/blevesearch/bleve"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
)

type jobOp int

const (
	jobOpAdd     = iota
	jobOpDelete  = iota
	jobOpReplace = iota

	// We execute a batch when it exceeds this size.
	batchSize = 10000

	// For backfilling, we can store up to this many jobs / additions in channel.
	jobBufferSize = 20000
)

type indexJob struct {
	op    jobOp
	uid   uint64
	value string
}

// Indices is the core object for working with Bleve indices.
type Indices struct {
	basedir string
	index   map[string]*Index
	config  *IndicesConfig
	done    chan error
}

// Index is a subobject of Indices. It should probably be internal, but that might
// cause a lot of name conflicts.
type Index struct {
	filename string // Fingerprint of attribute.
	config   *IndexConfig
	shard    []*IndexShard
	done     chan error
}

// IndexShard is a shard of Index. We run these shards in parallel.
type IndexShard struct {
	shard    int // Which shard is this.
	bindex   bleve.Index
	batch    *bleve.Batch
	jobQueue chan indexJob
	parser   valueParser
	config   *IndexConfig
}

func indexFilename(basedir, name string) string {
	lo, hi := farm.Fingerprint128([]byte(name))
	filename := strconv.FormatUint(lo, 36) + "_" + strconv.FormatUint(hi, 36)
	return path.Join(basedir, filename)
}

// CreateIndices creates new empty dirs given config file and basedir.
func CreateIndices(config *IndicesConfig, basedir string) error {
	x.Check(os.MkdirAll(basedir, 0700))
	config.write(basedir) // Copy config to basedir.
	for _, c := range config.Config {
		if err := createIndex(c, basedir); err != nil {
			return err
		}
	}
	return nil
}

func createIndex(c *IndexConfig, basedir string) error {
	filename := indexFilename(basedir, c.Attribute)
	for i := 0; i < c.NumShards; i++ {
		if err := createIndexShard(c, filename, i); err != nil {
			return err
		}
	}
	return nil
}

func createIndexShard(c *IndexConfig, filename string, shard int) error {
	indexMapping := bleve.NewIndexMapping()
	filename = filename + "_" + strconv.Itoa(shard)
	index, err := bleve.New(filename, indexMapping)
	if err != nil {
		return x.Wrap(err)
	}
	index.Close()
	return nil
}

// NewIndices constructs Indices from basedir which contains Bleve indices. We
// expect a config file in basedir
func NewIndices(basedir string) (*Indices, error) {
	// Read default config at basedir.
	configFilename := getDefaultConfig(basedir)
	fin, err := os.Open(configFilename)
	x.Check(err)
	defer fin.Close()
	config, err := NewIndicesConfig(bufio.NewReader(fin))
	if err != nil {
		return nil, err
	}
	indices := &Indices{
		basedir: basedir,
		index:   make(map[string]*Index),
		config:  config,
		done:    make(chan error),
	}
	for _, c := range config.Config {
		index, err := newIndex(c, basedir)
		if err != nil {
			return nil, err
		}
		indices.index[c.Attribute] = index
	}
	log.Printf("Successfully loaded indices at [%s]\n", basedir)
	return indices, nil
}

func newIndex(c *IndexConfig, basedir string) (*Index, error) {
	filename := indexFilename(basedir, c.Attribute)
	index := &Index{
		filename: filename,
		config:   c,
		done:     make(chan error),
	}
	for i := 0; i < c.NumShards; i++ {
		shard, err := newIndexShard(c, filename, i)
		if err != nil {
			return nil, err
		}
		index.shard = append(index.shard, shard)
	}
	return index, nil
}

func newIndexShard(c *IndexConfig, filename string, shard int) (*IndexShard, error) {
	filename = filename + "_" + strconv.Itoa(shard)
	bi, err := bleve.Open(filename)
	if err != nil {
		return nil, x.Wrap(err)
	}
	is := &IndexShard{
		shard:    shard,
		bindex:   bi,
		batch:    bi.NewBatch(),
		jobQueue: make(chan indexJob, jobBufferSize),
		parser:   getParser(c.Type),
		config:   c,
	}
	return is, nil
}
