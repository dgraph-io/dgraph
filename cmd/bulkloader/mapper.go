package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

type mapper struct {
	*state
	fpostings        []*protos.FlatPosting
	sz               int64
	buf              bytes.Buffer
	freePostings     []*protos.Posting
	freeFlatPostings []*protos.FlatPosting
	mu               sync.Mutex
}

func (m *mapper) getPosting() *protos.Posting {
	ln := len(m.freePostings)
	if ln > 0 {
		p := m.freePostings[ln-1]
		m.freePostings = m.freePostings[:ln-1]
		p.Reset()
		return p
	}
	return new(protos.Posting)
}

func (m *mapper) writePostings() {
	sort.Slice(m.fpostings, func(i, j int) bool {
		return bytes.Compare(m.fpostings[i].Key, m.fpostings[j].Key) < 0
	})

	m.mu.Lock()
	m.buf.Reset()
	var varintBuf [binary.MaxVarintLen64]byte
	for _, fposting := range m.fpostings {
		n := binary.PutUvarint(varintBuf[:], uint64(fposting.Size()))
		m.buf.Write(varintBuf[:n])
		postBuf, err := fposting.Marshal()
		x.Check(err)
		m.buf.Write(postBuf)
		if fposting.Full != nil {
			m.freePostings = append(m.freePostings, fposting.Full)
		}
	}
	m.freeFlatPostings = m.fpostings
	m.fpostings = make([]*protos.FlatPosting, 0, len(m.freeFlatPostings))
	m.sz = 0

	fileNum := atomic.AddUint32(&m.mapId, 1)
	filename := filepath.Join(m.opt.tmpDir, fmt.Sprintf("%06d.map", fileNum))
	go func() {
		x.Check(x.WriteFileSync(filename, m.buf.Bytes(), 0644))
		m.mu.Unlock()
	}()
}

func (m *mapper) run() {
	for rdf := range m.rdfCh {
		x.Check(m.parseRDF(rdf))
		atomic.AddInt64(&m.prog.rdfCount, 1)
		if m.sz >= m.opt.mapBufSize {
			m.writePostings()
		}
	}
	if len(m.fpostings) > 0 {
		m.writePostings()
	}
	m.mu.Lock() // Ensure that the last file write finishes.
}

func (m *mapper) addPosting(key []byte, posting *protos.Posting) {
	atomic.AddInt64(&m.prog.mapEdgeCount, 1)

	var p *protos.FlatPosting
	if len(m.freeFlatPostings) > 0 {
		// Try to reuse memory.
		ln := len(m.freeFlatPostings)
		p = m.freeFlatPostings[ln-1]
		p.Reset()
		m.freeFlatPostings = m.freeFlatPostings[:ln-1]
		p.Key = key
	} else {
		p = &protos.FlatPosting{
			Key: key,
		}
	}

	if posting.PostingType == protos.Posting_REF {
		p.UidOnly = posting.Uid
	} else {
		p.Full = posting
	}
	m.sz += int64(p.Size())
	m.fpostings = append(m.fpostings, p)
}

func (m *mapper) parseRDF(rdfLine string) error {
	nq, err := parseNQuad(rdfLine)
	if err != nil {
		if err == rdf.ErrEmpty {
			return nil
		}
		return err
	}

	sid := m.um.assignUID(nq.GetSubject())
	var oid uint64
	var de *protos.DirectedEdge
	if nq.GetObjectValue() == nil {
		oid = m.um.assignUID(nq.GetObjectId())
		de = nq.CreateUidEdge(sid, oid)
	} else {
		de, err = nq.CreateValueEdge(sid)
		x.Check(err)
	}

	fwd, rev := m.createPostings(nq, de)
	key := x.DataKey(nq.GetPredicate(), sid)
	m.addPosting(key, fwd)

	if rev != nil {
		key = x.ReverseKey(nq.GetPredicate(), oid)
		m.addPosting(key, rev)
	}

	key = x.DataKey("_predicate_", sid)
	pp := m.createPredicatePosting(nq.GetPredicate())
	m.addPosting(key, pp)
	m.addIndexPostings(nq, de)

	return nil
}

func parseNQuad(line string) (gql.NQuad, error) {
	nq, err := rdf.Parse(line)
	if err != nil {
		return gql.NQuad{}, err
	}
	return gql.NQuad{NQuad: &nq}, nil
}

func (m *mapper) createPredicatePosting(predicate string) *protos.Posting {
	fp := farm.Fingerprint64([]byte(predicate))
	p := m.getPosting()
	p.Uid = fp
	p.Value = []byte(predicate)
	p.ValType = protos.Posting_DEFAULT
	p.PostingType = protos.Posting_VALUE
	return p
}

func (m *mapper) createPostings(nq gql.NQuad,
	de *protos.DirectedEdge) (*protos.Posting, *protos.Posting) {

	m.ss.validateType(de, nq.ObjectValue == nil)

	p := m.getPosting()
	posting.NewPostingNoAlloc(de, p)
	if nq.GetObjectValue() != nil {
		if lang := de.GetLang(); lang == "" {
			p.Uid = math.MaxUint64
		} else {
			p.Uid = farm.Fingerprint64([]byte(lang))
		}
	}

	// Early exit for no reverse edge.
	sch := m.ss.getSchema(nq.GetPredicate())
	if sch.GetDirective() != protos.SchemaUpdate_REVERSE {
		return p, nil
	}

	// Reverse predicate
	x.AssertTruef(nq.GetObjectValue() == nil, "only has reverse schema if object is UID")
	de.Entity, de.ValueId = de.ValueId, de.Entity
	m.ss.validateType(de, true)
	rp := m.getPosting()
	posting.NewPostingNoAlloc(de, p)

	de.Entity, de.ValueId = de.ValueId, de.Entity // de reused so swap back.

	return p, rp
}

func (m *mapper) addIndexPostings(nq gql.NQuad, de *protos.DirectedEdge) {
	if nq.GetObjectValue() == nil {
		return // Cannot index UIDs
	}

	sch := m.ss.getSchema(nq.GetPredicate())

	for _, tokerName := range sch.GetTokenizer() {

		// Find tokeniser.
		toker, ok := tok.GetTokenizer(tokerName)
		if !ok {
			log.Fatalf("unknown tokenizer %q", tokerName)
		}

		// Create storage value.
		storageVal := types.Val{
			Tid:   types.TypeID(de.GetValueType()),
			Value: de.GetValue(),
		}

		// Convert from storage type to schema type.
		schemaVal, err := types.Convert(storageVal, types.TypeID(sch.GetValueType()))
		// Shouldn't error, since we've already checked for convertibility when
		// doing edge postings. So okay to be fatal.
		x.Check(err)

		// Extract tokens.
		toks, err := toker.Tokens(schemaVal)

		// Store index posting.
		for _, t := range toks {
			m.addPosting(
				x.IndexKey(nq.Predicate, t),
				&protos.Posting{
					Uid:         de.GetEntity(),
					PostingType: protos.Posting_REF,
				},
			)
		}
	}
}
