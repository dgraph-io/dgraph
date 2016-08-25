package bidx

import (
	"github.com/blevesearch/bleve"
	"github.com/dgryski/go-farm"
)

type FieldType int

const (
	Text     = iota
	Number   = iota
	DateTime = iota
	Boolean  = iota
)

type Field struct {
	ft    FieldType
	name  string
	index bleve.Index	// One index per field.
}

type Index struct {
	dir   string
	field map[string]*Field
}

func NewSchema(js string) Schema {
farm.
}
