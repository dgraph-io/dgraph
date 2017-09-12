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
	mapEntries     []*protos.MapEntry
	sz             int64
	buf            bytes.Buffer
	freePostings   []*protos.Posting
	freeMapEntries []*protos.MapEntry
	mu             sync.Mutex
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

func (m *mapper) writeMapEntriesToFile() {
	sort.Slice(m.mapEntries, func(i, j int) bool {
		return bytes.Compare(m.mapEntries[i].Key, m.mapEntries[j].Key) < 0
	})

	m.mu.Lock()
	m.buf.Reset()
	var varintBuf [binary.MaxVarintLen64]byte
	for _, me := range m.mapEntries {
		n := binary.PutUvarint(varintBuf[:], uint64(me.Size()))
		m.buf.Write(varintBuf[:n])
		x.AssertTrue(len(me.Key) != 0)
		postBuf, err := me.Marshal()
		x.Check(err)
		m.buf.Write(postBuf)
		if me.Posting != nil {
			m.freePostings = append(m.freePostings, me.Posting)
		}
	}
	m.freeMapEntries = m.mapEntries
	m.mapEntries = make([]*protos.MapEntry, 0, len(m.freeMapEntries))
	m.sz = 0

	fileNum := atomic.AddUint32(&m.mapFileId, 1)
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
			m.writeMapEntriesToFile()
		}
	}
	if len(m.mapEntries) > 0 {
		m.writeMapEntriesToFile()
	}
	m.mu.Lock() // Ensure that the last file write finishes.
}

func (m *mapper) addMapEntry(key []byte, posting *protos.Posting) {
	atomic.AddInt64(&m.prog.mapEdgeCount, 1)

	var me *protos.MapEntry
	if len(m.freeMapEntries) > 0 {
		// Try to reuse memory.
		ln := len(m.freeMapEntries)
		me = m.freeMapEntries[ln-1]
		me.Reset()
		m.freeMapEntries = m.freeMapEntries[:ln-1]
		me.Key = key
	} else {
		me = &protos.MapEntry{
			Key: key,
		}
	}

	if posting.PostingType == protos.Posting_REF {
		me.Uid = posting.Uid
	} else {
		me.Posting = posting
	}
	m.sz += int64(me.Size())
	m.mapEntries = append(m.mapEntries, me)
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
	m.addMapEntry(key, fwd)

	if rev != nil {
		key = x.ReverseKey(nq.GetPredicate(), oid)
		m.addMapEntry(key, rev)
	}

	key = x.DataKey("_predicate_", sid)
	pp := m.createPredicatePosting(nq.GetPredicate())
	m.addMapEntry(key, pp)
	m.addIndexMapEntries(nq, de)

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
	posting.SetPosting(de, p)
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
	posting.SetPosting(de, p)

	de.Entity, de.ValueId = de.ValueId, de.Entity // de reused so swap back.

	return p, rp
}

func (m *mapper) addIndexMapEntries(nq gql.NQuad, de *protos.DirectedEdge) {
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
			m.addMapEntry(
				x.IndexKey(nq.Predicate, t),
				&protos.Posting{
					Uid:         de.GetEntity(),
					PostingType: protos.Posting_REF,
				},
			)
		}
	}
}
