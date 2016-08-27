package bidx

import (
	"fmt"
	"log"
	"os"
	"path"
	"strconv"

	"github.com/blevesearch/bleve"
	"github.com/dgryski/go-farm"
)

type jobOp int

const (
	jobOpAdd     = iota
	jobOpDelete  = iota
	jobOpReplace = iota

	batchSize     = 10000
	jobBufferSize = 20000
)

type indexJob struct {
	op    jobOp
	uid   uint64
	value string
}

type Indices struct {
	Basedir string
	Index   map[string]*Index
	Config  *IndicesConfig
	Done    chan error
}

type Index struct {
	Filename string // Fingerprint of attribute.
	Config   *IndexConfig
	Shard    []*IndexShard
	Done     chan error
}

type IndexShard struct {
	Shard    int // Which shard is this.
	Bindex   bleve.Index
	Batch    *bleve.Batch
	JobQueue chan indexJob
	Parser   valueParser
	Config   *IndexConfig
}

func indexFilename(basedir, name string) string {
	lo, hi := farm.Fingerprint128([]byte(name))
	filename := strconv.FormatUint(lo, 36) + "_" + strconv.FormatUint(hi, 36)
	return path.Join(basedir, filename)
}

// CreateIndices creates new empty dirs given config file and basedir.
func CreateIndices(config *IndicesConfig, basedir string) error {
	if err := os.MkdirAll(basedir, 0700); err != nil {
		log.Fatalf("Error while creating the filepath for indices: %s", err)
	}

	config.Write(basedir) // Copy config to basedir.
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
		return err
	}
	index.Close()
	return nil
}

func NewIndices(basedir string) (*Indices, error) {
	// Read default config at basedir.
	config, err := NewIndicesConfig(getDefaultConfig(basedir))
	if err != nil {
		return nil, fmt.Errorf("Error reading indices config json: %s", err)
	}
	indices := &Indices{
		Basedir: basedir,
		Index:   make(map[string]*Index),
		Config:  config,
		Done:    make(chan error),
	}
	for _, c := range config.Config {
		index, err := newIndex(c, basedir)
		if err != nil {
			return nil, fmt.Errorf("Index error %s: %s", c.Attribute, err)
		}
		indices.Index[c.Attribute] = index
	}
	log.Printf("Successfully loaded indices at [%s]\n", basedir)
	return indices, nil
}

func newIndex(c *IndexConfig, basedir string) (*Index, error) {
	filename := indexFilename(basedir, c.Attribute)
	index := &Index{
		Filename: filename,
		Config:   c,
		Done:     make(chan error),
	}
	for i := 0; i < c.NumShards; i++ {
		shard, err := newIndexShard(c, filename, i)
		if err != nil {
			return nil, err
		}
		index.Shard = append(index.Shard, shard)
	}
	return index, nil
}

func newIndexShard(c *IndexConfig, filename string, shard int) (*IndexShard, error) {
	filename = filename + "_" + strconv.Itoa(shard)
	bi, err := bleve.Open(filename)
	if err != nil {
		return nil, err
	}

	is := &IndexShard{
		Shard:    shard,
		Bindex:   bi,
		Batch:    bi.NewBatch(),
		JobQueue: make(chan indexJob, jobBufferSize),
		Parser:   getParser(c.Type),
		Config:   c,
	}
	return is, nil
}
