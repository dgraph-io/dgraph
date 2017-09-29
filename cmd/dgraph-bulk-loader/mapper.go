package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"
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
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
)

type mapper struct {
	*state
	shards []shardState // shard is based on predicate
}

type shardState struct {
	// Buffer up map entries until we have a sufficient amount, then sort and
	// write them to file.
	entriesBuf []byte
	mu         sync.Mutex // Allow only 1 write per shard at a time.
}

func newMapper(st *state) *mapper {
	return &mapper{
		state:  st,
		shards: make([]shardState, st.opt.MapShards),
	}
}

func less(lhs, rhs *protos.MapEntry) bool {
	if keyCmp := bytes.Compare(lhs.Key, rhs.Key); keyCmp != 0 {
		return keyCmp < 0
	}
	lhsUID := lhs.Uid
	rhsUID := rhs.Uid
	if lhs.Posting != nil {
		lhsUID = lhs.Posting.Uid
	}
	if rhs.Posting != nil {
		rhsUID = rhs.Posting.Uid
	}
	return lhsUID < rhsUID
}

func (m *mapper) writeMapEntriesToFile(entriesBuf []byte, shardIdx int) {
	var buf []byte = entriesBuf
	var entries []*protos.MapEntry
	for len(buf) > 0 {
		sz, n := binary.Uvarint(buf)
		x.AssertTrue(n > 0)
		buf = buf[n:]
		me := new(protos.MapEntry)
		x.Check(proto.Unmarshal(buf[:sz], me))
		entries = append(entries, me)
	}

	sort.Slice(entries, func(i, j int) bool {
		return less(entries[i], entries[j])
	})

	buf = entriesBuf
	for _, me := range entries {
		n := binary.PutUvarint(buf, uint64(me.Size()))
		buf = buf[n:]
		n, err := me.MarshalTo(buf)
		x.Check(err)
		buf = buf[n:]
	}
	x.AssertTrue(len(buf) == 0)

	fileNum := atomic.AddUint32(&m.mapFileId, 1)
	filename := filepath.Join(
		m.opt.TmpDir,
		"shards",
		fmt.Sprintf("%03d", shardIdx),
		fmt.Sprintf("%06d.map", fileNum),
	)
	x.Check(os.MkdirAll(filepath.Dir(filename), 0755))
	x.Check(x.WriteFileSync(filename, entriesBuf, 0644))
	m.shards[shardIdx].mu.Unlock() // Locked by caller.
}

func (m *mapper) run() {
	for chunkBuf := range m.rdfChunkCh {
		done := false
		for !done {
			rdf, err := chunkBuf.ReadString('\n')
			if err == io.EOF {
				// Process the last RDF rather than breaking immediately.
				done = true
			} else {
				x.Check(err)
			}
			rdf = strings.TrimSpace(rdf)

			x.Check(m.parseRDF(rdf))
			atomic.AddInt64(&m.prog.rdfCount, 1)
			for i := range m.shards {
				sh := &m.shards[i]
				if len(sh.entriesBuf) >= int(m.opt.MapBufSize) {
					sh.mu.Lock() // One write at a time.
					go m.writeMapEntriesToFile(sh.entriesBuf, i)
					sh.entriesBuf = make([]byte, 0, m.opt.MapBufSize*11/10)
				}
			}
		}
	}
	for i := range m.shards {
		sh := &m.shards[i]
		if len(sh.entriesBuf) > 0 {
			sh.mu.Lock() // One write at a time.
			m.writeMapEntriesToFile(sh.entriesBuf, i)
		}
		m.shards[i].mu.Lock() // Ensure that the last file write finishes.
	}
}

func (m *mapper) addMapEntry(key []byte, posting *protos.Posting, shard int) {
	atomic.AddInt64(&m.prog.mapEdgeCount, 1)

	me := &protos.MapEntry{
		Key: key,
	}
	if posting.PostingType == protos.Posting_REF {
		me.Uid = posting.Uid
	} else {
		me.Posting = posting
	}
	sh := &m.shards[shard]

	data, err := me.Marshal()
	x.Check(err)
	var szBuf [binary.MaxVarintLen64]byte
	sz := szBuf[:binary.PutUvarint(szBuf[:], uint64(len(data)))]
	sh.entriesBuf = append(sh.entriesBuf, data...)
	sh.entriesBuf = append(sh.entriesBuf, sz...)
}

func (m *mapper) parseRDF(rdfLine string) error {
	nq, err := parseNQuad(rdfLine)
	if err != nil {
		if err == rdf.ErrEmpty {
			return nil
		}
		return errors.Wrapf(err, "while parsing line %q", rdfLine)
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
	shard := m.state.sm.shardFor(nq.Predicate)
	key := x.DataKey(nq.Predicate, sid)
	m.addMapEntry(key, fwd, shard)

	if rev != nil {
		key = x.ReverseKey(nq.Predicate, oid)
		m.addMapEntry(key, rev, shard)
	}

	if m.opt.ExpandEdges {
		key = x.DataKey("_predicate_", sid)
		pp := m.createPredicatePosting(nq.Predicate)
		m.addMapEntry(key, pp, shard)
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
	sch := m.ss.getSchema(nq.GetPredicate())
	if nq.GetObjectValue() != nil {
		if lang := de.GetLang(); len(lang) > 0 {
			p.Uid = farm.Fingerprint64([]byte(lang))
		} else if sch.List {
			p.Uid = farm.Fingerprint64(de.Value)
		} else {
			p.Uid = math.MaxUint64
		}
	}

	// Early exit for no reverse edge.
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
				m.state.sm.shardFor(nq.Predicate),
			)
		}
	}
}
