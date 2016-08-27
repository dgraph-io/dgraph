package main

import (
	"fmt"
	"log"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/dgraph-io/dgraph/bidx"
	"github.com/dgraph-io/dgraph/store"
)

const (
	basedir = "/home/jchiu/dgraph/bidx"
)

func createConfig() *bidx.IndicesConfig {
	/*return &bidx.IndicesSchema{
	Entry: []*bidx.IndexSchema{
		&bidx.IndexSchema{
			Type: "text",
			Name: "type.object.name.en",
		},
		&bidx.IndexSchema{
			Type: "datetime",
			Name: "film.film.initial_release_date",
		}}}*/

	return &bidx.IndicesConfig{
		Config: []*bidx.IndexConfig{
			&bidx.IndexConfig{
				Type:      "text",
				Attribute: "type.object.name.en",
				NumShards: 8,
			}}}
	//	return &bidx.IndicesSchema{
	//		Entry: []*bidx.IndexSchema{
	//			&bidx.IndexSchema{
	//				Type: "datetime",
	//				Name: "film.film.initial_release_date",
	//			}}}
}

func run1() {
	{
		s := createConfig()
		if err := s.Write(basedir); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Config created")
	}

	config, err := bidx.NewIndicesConfig(basedir)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Config read")
	fmt.Printf("Number of entries: %d\n", len(config.Config))

	ps := new(store.Store)
	postingDir := "/home/jchiu/dgraph/p"
	if err := ps.InitReadOnly(postingDir); err != nil {
		log.Fatalf("error initializing postings store: %s", err)
	}
	defer ps.Close()
	fmt.Println("store opened")

	err = bidx.CreateIndices(config, basedir)
	if err != nil {
		log.Fatal(err)
	}

	indices, err := bidx.NewIndices(basedir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(indices.Index["type.object.name.en"].Shard)

	start := time.Now()
	indices.Backfill(ps)
	fmt.Printf("Elapsed %s\n", time.Since(start))
}

func run2() {
	indexMapping := bleve.NewIndexMapping()
	index, err := bleve.New("/tmp/a2.bleve", indexMapping)
	if err != nil {
		index, err = bleve.Open("/tmp/a2.bleve")
	}
	if err != nil {
		log.Fatal(err)
	}

	index.Index("id3", float64(256))
	index.Index("id1", int64(123))
	index.Index("id2", float64(0.0))
	index.Index("id4", float64(500.0))
	index.Index("id5", float64(400.0))
	js, _ := index.Stats().MarshalJSON()
	fmt.Println(string(js))

	start := 3.0
	end := 300.0
	query := bleve.NewNumericRangeQuery(&start, &end)
	//	search := bleve.NewSearchRequest(query)
	search := bleve.NewSearchRequestOptions(query, 100000, 0, false)
	searchResults, err := index.Search(search)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(searchResults.Total)
	for _, h := range searchResults.Hits {
		fmt.Println(h.ID)
	}
}

func main() {
	//	run1()
	run2()
}
