package main

import (
	"log"
	"math"
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
}

func (m *mapper) run() {
	for rdf := range m.rdfCh {
		x.Checkf(m.parseRDF(rdf), "Could not parse RDF.")
		atomic.AddInt64(&m.prog.rdfCount, 1)
	}
}

func (m *mapper) addPosting(key []byte, posting *protos.Posting) {
	atomic.AddInt64(&m.prog.mapEdgeCount, 1)

	p := &protos.FlatPosting{
		Key: key,
	}
	if posting.PostingType == protos.Posting_REF {
		p.Posting = &protos.FlatPosting_UidPosting{UidPosting: posting.Uid}
	} else {
		p.Posting = &protos.FlatPosting_FullPosting{FullPosting: posting}
	}
	m.postingsCh <- p
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
	pp := createPredicatePosting(nq.GetPredicate())
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

func createPredicatePosting(predicate string) *protos.Posting {
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
