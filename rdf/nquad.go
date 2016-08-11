package rdf

// NQuad is the data structure used for storing rdf N-Quads.
type NQuad struct {
	Subject     string
	Predicate   string
	ObjectId    string
	ObjectValue []byte
	Label       string
}
