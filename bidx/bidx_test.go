package bidx

import (
	"fmt"
	"testing"

	"github.com/blevesearch/bleve"
	//	"github.com/blevesearch/bleve/document"
	//	"github.com/blevesearch/bleve/index/store"
)

const (
	kPath = "/tmp/example2.bleve"
)

func createIndex() (bleve.Index, error) {
	index := bleve.NewIndexMapping()
	//	docMapping := bleve.NewDocumentMapping()
	//	nameFieldMapping := bleve.NewTextFieldMapping()
	//	docMapping.AddFieldMappingsAt("name", nameFieldMapping)
	//	index.AddDocumentMapping("mydoctype", docMapping)
	return bleve.New(kPath, index)
}

func openIndex() (bleve.Index, error) {
	index, err := bleve.Open(kPath)
	fmt.Println(index.Mapping().TypeMapping)
	return index, err
}

type Data struct {
	V string
}

func printStats(index bleve.Index) {
	js, _ := index.Stats().MarshalJSON()
	fmt.Println(string(js))
}

func search(index bleve.Index, text string) {
	//	query := bleve.NewMatchQuery(text)
	query := bleve.NewTermQuery(text)
	search := bleve.NewSearchRequest(query)
	searchResults, err := index.Search(search)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(searchResults)
}

func Test1(t *testing.T) {
	//index, err := createIndex()
	index, err := openIndex()
	if err != nil {
		t.Error(err)
		return
	}

	printStats(index)
	index.Index("id1", &Data{
		V: "nothing",
	})
	index.Index("id2", &Data{
		V: "nothing",
	})

	search(index, "nothing")
	printStats(index)

	index.Delete("id2")
	search(index, "nothing")
	printStats(index)
}
