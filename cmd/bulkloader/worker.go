package main

// TODO: Review for phase 1

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

type worker struct {
	rdfCh       chan string
	um          *uidMap
	ss          *schemaStore
	prog        *progress
	postingsOut chan<- *protos.DenormalisedPosting
}

func newWorker(
	rdfCh chan string,
	um *uidMap,
	ss *schemaStore,
	prog *progress,
	postingsOut chan<- *protos.DenormalisedPosting,
) *worker {
	return &worker{
		rdfCh:       rdfCh,
		um:          um,
		ss:          ss,
		prog:        prog,
		postingsOut: postingsOut,
	}
}

func (w *worker) run() {
	for rdf := range w.rdfCh {
		w.parseRDF(rdf)
		atomic.AddInt64(&w.prog.rdfCount, 1)
	}
}

func (w *worker) addPosting(key []byte, posting *protos.Posting) {
	p := &protos.DenormalisedPosting{
		PostingListKey: key,
	}
	if posting.PostingType == protos.Posting_REF {
		p.Posting = &protos.DenormalisedPosting_UidPosting{UidPosting: posting.Uid}
	} else {
		p.Posting = &protos.DenormalisedPosting_FullPosting{FullPosting: posting}
	}
	w.postingsOut <- p
}

func (w *worker) parseRDF(rdfLine string) {
	nq, err := parseNQuad(rdfLine)
	if err != nil {
		if err == rdf.ErrEmpty {
			return
		}
		x.Checkf(err, "Could not parse RDF.")
	}

	sUID := w.um.assignUID(nq.GetSubject())
	uidM := map[string]uint64{nq.GetSubject(): sUID}
	var oUID uint64
	if nq.GetObjectValue() == nil {
		oUID = w.um.assignUID(nq.GetObjectId())
		uidM[nq.GetObjectId()] = oUID
	}

	fwdPosting, revPosting := w.createEdgePostings(nq, uidM)
	key := x.DataKey(nq.GetPredicate(), sUID)
	w.addPosting(key, fwdPosting)

	if revPosting != nil {
		key = x.ReverseKey(nq.GetPredicate(), oUID)
		w.addPosting(key, revPosting)
	}

	key = x.DataKey("_predicate_", sUID)
	pp := createPredicatePosting(nq.GetPredicate())

	w.addPosting(key, pp)

	w.addIndexPostings(nq, uidM)
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

func (w *worker) createEdgePostings(nq gql.NQuad, uidM map[string]uint64) (*protos.Posting, *protos.Posting) {

	de, err := nq.ToEdgeUsing(uidM)
	x.Check(err)

	w.ss.fixEdge(de, nq.ObjectValue == nil)

	p := posting.NewPosting(de)
	if nq.GetObjectValue() != nil {
		if lang := de.GetLang(); lang == "" {
			p.Uid = math.MaxUint64
		} else {
			p.Uid = farm.Fingerprint64([]byte(lang))
		}
	}

	// Early exit for no reverse edge.
	sch := w.ss.getSchema(nq.GetPredicate())
	if sch.GetDirective() != protos.SchemaUpdate_REVERSE {
		return p, nil
	}

	// Reverse predicate
	x.AssertTruef(nq.GetObjectValue() == nil, "only has reverse schema if object is UID")
	rde, err := nq.ToEdgeUsing(uidM)
	x.Check(err)
	rde.Entity, rde.ValueId = rde.ValueId, rde.Entity
	w.ss.fixEdge(rde, true)
	rp := posting.NewPosting(rde)

	return p, rp
}

func (w *worker) addIndexPostings(nq gql.NQuad, uidM map[string]uint64) {

	if nq.GetObjectValue() == nil {
		return // Cannot index UIDs
	}

	sch := w.ss.getSchema(nq.GetPredicate())

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
			w.addPosting(
				x.IndexKey(nq.Predicate, t),
				&protos.Posting{
					Uid:         de.GetEntity(),
					PostingType: protos.Posting_REF,
				},
			)
		}
	}
}
