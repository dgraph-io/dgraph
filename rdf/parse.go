package rdf

import (
	"bytes"

	p "github.com/dgraph-io/dgraph/parsing"
)

func Parse(line string) (rnq NQuad, err error) {
	s := p.NewByteStream(bytes.NewBufferString(line))
	var nqp nQuadParser
	s, err = p.ParseErr(s, &nqp)
	rnq = NQuad(nqp)
	return
}

func ParseDoc(doc string) (ret []NQuad, err error) {
	s := p.NewByteStream(bytes.NewBufferString(doc))
	var nqd nQuadsDoc
	s, err = p.ParseErr(s, &nqd)
	ret = nqd
	return
}
