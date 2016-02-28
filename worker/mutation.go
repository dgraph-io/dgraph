package worker

import (
	"bytes"
	"encoding/gob"

	"github.com/dgraph-io/dgraph/x"
)

type Mutations struct {
	Set []x.DirectedEdge
	Del []x.DirectedEdge
}

func (m *Mutations) Encode() (data []byte, rerr error) {
	var b bytes.Buffer
	enc := gob.NewEncoder(&b)
	rerr = enc.Encode(*m)
	return b.Bytes(), rerr
}

func (m *Mutations) Decode(data []byte) error {
	r := bytes.NewReader(data)
	dec := gob.NewDecoder(r)
	return dec.Decode(m)
}
