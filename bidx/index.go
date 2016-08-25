package bidx

import (
	"github.com/blevesearch/bleve"
	//	"github.com/dgryski/go-farm"
)

type IndexType int

const (
	Text     = iota
	Number   = iota
	DateTime = iota
	Boolean  = iota
)

type Index struct {
	itype    IndexType
	name     string
	index    bleve.Index
	filename string // Fingerprint of name
}

type Indices struct {
	basedir string
	index   map[string]*Index
}

// CreateEmptyIndices reads schema at basedir, creates empty indices.
func CreateEmptyIndices(basedir string) (*Indices, error) {
	// Read in the schema.
	_, err := NewSchema(basedir)
	if err != nil {
		return nil, err
	}
	for _, e := range schema {
		index := bleve.NewIndexMapping()
		//	docMapping := bleve.NewDocumentMapping()
		//	nameFieldMapping := bleve.NewTextFieldMapping()
		//	docMapping.AddFieldMappingsAt("name", nameFieldMapping)
		//	index.AddDocumentMapping("mydoctype", docMapping)
	}
	return nil, nil
}
