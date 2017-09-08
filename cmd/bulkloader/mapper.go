package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sort"
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
	postings     []*protos.FlatPosting
	freeList     []*protos.FlatPosting
	sz           int64
	buf          bytes.Buffer
	freePostings []*protos.Posting
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

func (m *mapper) putPosting(p *protos.Posting) {
	m.freePostings = append(m.freePostings, p)
}

func (m *mapper) writePostings() {
	sort.Slice(m.postings, func(i, j int) bool {
		return bytes.Compare(m.postings[i].Key, m.postings[j].Key) < 0
	})

	m.buf.Reset()
	var varintBuf [binary.MaxVarintLen64]byte
	for _, posting := range m.postings {
		n := binary.PutUvarint(varintBuf[:], uint64(posting.Size()))
		m.buf.Write(varintBuf[:n])
		postBuf, err := posting.Marshal()
		x.Check(err)
		m.buf.Write(postBuf)
		if p, ok := posting.Posting.(*protos.FlatPosting_FullPosting); ok {
			m.putPosting(p.FullPosting)
		}
	}
	m.freeList = m.postings
	m.postings = make([]*protos.FlatPosting, 0, len(m.freeList))
	m.sz = 0

	fileNum := atomic.AddUint32(&m.mapId, 1)
	filename := filepath.Join(m.opt.tmpDir, fmt.Sprintf("%06d.map", fileNum))
	x.Check(x.WriteFileSync(filename, m.buf.Bytes(), 0644))
}

func (m *mapper) run() {
	for rdf := range m.rdfCh {
		x.Check(m.parseRDF(rdf))
		atomic.AddInt64(&m.prog.rdfCount, 1)
		if m.sz >= m.opt.mapBufSize {
			m.writePostings()
		}
	}
	if len(m.postings) > 0 {
		m.writePostings()
	}
}

func (m *mapper) addPosting(key []byte, posting *protos.Posting) {
	atomic.AddInt64(&m.prog.mapEdgeCount, 1)

	var p *protos.FlatPosting
	if len(m.freeList) > 0 {
		// Try to reuse memory.
		ln := len(m.freeList)
		p = m.freeList[ln-1]
		p.Reset()
		m.freeList = m.freeList[:ln-1]
	} else {
		p = &protos.FlatPosting{
			Key: key,
		}
	}

	if posting.PostingType == protos.Posting_REF {
		p.Posting = &protos.FlatPosting_UidPosting{UidPosting: posting.Uid}
	} else {
		p.Posting = &protos.FlatPosting_FullPosting{FullPosting: posting}
	}
	m.sz += int64(p.Size())
	m.postings = append(m.postings, p)
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
