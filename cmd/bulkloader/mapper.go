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
	rdfCh      chan string
	um         *uidMap
	ss         *schemaStore
	prog       *progress
	postingsCh chan<- *protos.FlatPosting
}

func (m *mapper) run() {
	for rdf := range m.rdfCh {
		x.Checkf(m.parseRDF(rdf), "Could not parse RDF.")
		atomic.AddInt64(&m.prog.rdfCount, 1)
	}
}

func (m *mapper) addPosting(key []byte, posting *protos.Posting) {
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
	uidM := map[string]uint64{nq.GetSubject(): sid}
	var oid uint64
	if nq.GetObjectValue() == nil {
		oid = m.um.assignUID(nq.GetObjectId())
		uidM[nq.GetObjectId()] = oid
	}

	fwdPosting, revPosting := m.createFwdAndRevPostings(nq, uidM)
	key := x.DataKey(nq.GetPredicate(), sid)
	m.addPosting(key, fwdPosting)

	if revPosting != nil {
		key = x.ReverseKey(nq.GetPredicate(), oid)
		m.addPosting(key, revPosting)
	}

	key = x.DataKey("_predicate_", sid)
	pp := createPredicatePosting(nq.GetPredicate())
	m.addPosting(key, pp)
	m.addIndexPostings(nq, uidM)

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

func (m *mapper) createFwdAndRevPostings(nq gql.NQuad,
	uidM map[string]uint64) (*protos.Posting, *protos.Posting) {

	de, err := nq.ToEdgeUsing(uidM)
	x.Check(err)

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
	rde, err := nq.ToEdgeUsing(uidM)
	x.Check(err)
	rde.Entity, rde.ValueId = rde.ValueId, rde.Entity
	m.ss.validateType(rde, true)
	rp := posting.NewPosting(rde)

	return p, rp
}

func (m *mapper) addIndexPostings(nq gql.NQuad, uidM map[string]uint64) {

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
		de, err := nq.ToEdgeUsing(uidM)
		x.Check(err)
		storageVal := types.Val{
			Tid:   types.TypeID(de.GetValueType()),
			Value: de.GetValue(),
		}

		// Convert from storage type to schema type.
		var schemaVal types.Val
		schemaVal, err = types.Convert(storageVal, types.TypeID(sch.GetValueType()))
		x.Check(err) // Shouldn't error, since we've already checked for convertibility when doing edge postings.

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
