package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"path/filepath"
	"sort"
	"strings"
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
	"github.com/pkg/errors"
)

type mapper struct {
	*state
	mapEntries []*protos.MapEntry
	sz         int64

	mu  sync.Mutex
	buf bytes.Buffer
}

func (m *mapper) writeMapEntriesToFile(mapEntries []*protos.MapEntry) {
	sort.Slice(mapEntries, func(i, j int) bool {
		return bytes.Compare(mapEntries[i].Key, mapEntries[j].Key) < 0
	})

	var varintBuf [binary.MaxVarintLen64]byte
	for _, me := range mapEntries {
		n := binary.PutUvarint(varintBuf[:], uint64(me.Size()))
		m.buf.Write(varintBuf[:n])
		postBuf, err := me.Marshal()
		x.Check(err)
		m.buf.Write(postBuf)
	}

	fileNum := atomic.AddUint32(&m.mapFileId, 1)
	filename := filepath.Join(m.opt.tmpDir, fmt.Sprintf("%06d.map", fileNum))
	x.Check(x.WriteFileSync(filename, m.buf.Bytes(), 0644))
	m.buf.Reset()
	m.mu.Unlock() // Locked by caller.
}

func (m *mapper) run() {
	for batchBuf := range m.batchCh {
		atomic.AddInt64(&m.prog.mappersRunning, 1)
		done := false
		for !done {
			rdf, err := batchBuf.ReadString('\n')
			if err == io.EOF {
				// Process the last RDF rather than breaking immediately.
				done = true
			} else {
				x.Check(err)
			}

			// TODO: Might not have to do this. Or if we do, there might be a
			// more efficient way.
			rdf = strings.TrimSpace(rdf)

			x.Check(m.parseRDF(rdf))
			atomic.AddInt64(&m.prog.rdfCount, 1)
			if m.sz >= m.opt.mapBufSize {
				m.mu.Lock() // One write at a time.
				go m.writeMapEntriesToFile(m.mapEntries)
				m.mapEntries = nil
				m.sz = 0
			}
		}
		atomic.AddInt64(&m.prog.mappersRunning, -1)
	}
	if len(m.mapEntries) > 0 {
		m.mu.Lock() // One write at a time.
		m.writeMapEntriesToFile(m.mapEntries)
	}
	m.mu.Lock() // Ensure that the last file write finishes.
}

func (m *mapper) addMapEntry(key []byte, posting *protos.Posting) {
	atomic.AddInt64(&m.prog.mapEdgeCount, 1)

	me := &protos.MapEntry{
		Key: key,
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
		// TODO: XXX: Handle error properly. For now, just logging.
		//return errors.Wrapf(err, "while parsing line %q:", rdfLine)
		log.Print(errors.Wrapf(err, "while parsing line %q:", rdfLine))
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

	if !m.opt.skipExpandEdges {
		key = x.DataKey("_predicate_", sid)
		pp := m.createPredicatePosting(nq.GetPredicate())
		m.addMapEntry(key, pp)
	}

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
	return &protos.Posting{
		Uid:         fp,
		Value:       []byte(predicate),
		ValType:     protos.Posting_DEFAULT,
		PostingType: protos.Posting_VALUE,
	}
}

func (m *mapper) createPostings(nq gql.NQuad,
	de *protos.DirectedEdge) (*protos.Posting, *protos.Posting) {

	m.ss.validateType(de, nq.ObjectValue == nil)

	p := posting.NewPosting(de)
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
	rp := posting.NewPosting(de)

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
