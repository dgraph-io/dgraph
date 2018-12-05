package main

import (
	"log"
    "encoding/binary"
	"math"
	"strings"

	"github.com/chrislusf/gleam/gio"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"

	farm "github.com/dgryski/go-farm"
	"github.com/gogo/protobuf/proto"

    // "fmt"
    // "os"
)

func emitMapEntry(pred string, key []byte, p *pb.Posting) error {
    var err error
    pbytes := []byte{}
    if p.PostingType != pb.Posting_REF || len(p.Facets) > 0 {
        pbytes, err = proto.Marshal(&pb.SivaPostingList{
            Postings: []*pb.Posting{p},
        })
        x.Check(err)
    }
    ubytes, err := proto.Marshal(&pb.List{
        Uids: []uint64{p.Uid},
    })
    x.Check(err)

    uidBuf := make([]byte, 8)
    binary.BigEndian.PutUint64(uidBuf, p.Uid)

    gio.Emit(pred, key, uidBuf, ubytes, pbytes)
	return nil
}

func rdfToMapEntry(row []interface{}) error {
	rdfLine := gio.ToString(row[0])
	nq, err := parseNQuad(rdfLine)
    if err == rdf.ErrEmpty {
        return nil
    }
    x.Check(err)
    x.Check(facets.SortAndValidate(nq.Facets))
    x.Check(processNQuad(nq))
    return nil
}

func parseNQuad(line string) (gql.NQuad, error) {
	nq, err := rdf.Parse(line)
    if err == rdf.ErrEmpty {
        return gql.NQuad{}, err
    }
    x.Check(err)
	return gql.NQuad{NQuad: &nq}, nil
}

func processNQuad(nq gql.NQuad) error {
	sid, err := lookupUid(nq.GetSubject())
	var oid uint64
	var de *pb.DirectedEdge
	if nq.GetObjectValue() == nil {
		oid, err = lookupUid(nq.GetObjectId())
        x.Check(err)
		de = nq.CreateUidEdge(sid, oid)
	} else {
		de, err = nq.CreateValueEdge(sid)
        x.Check(err)
	}

    // emit forward and (if exists) reverse postings
    x.Check(emitPostings(nq, de))
    // emit index postings for the given pred if any
    x.Check(emitIndexPostings(nq, de))

	if Opt.ExpandEdges {
		return emitMapEntry(
            "_predicate_",
            x.DataKey("_predicate_", sid),
            createPredicatePosting(nq.Predicate),
        )
	}

	return nil
}

func lookupUid(xid string) (uid uint64, err error) {
	var ok bool
	uid, ok = Xdb.LookupUid(xid)
	if !ok {
		log.Fatalf("Couldn't find UID for XID=%s", xid)
	}

	// Don't store xids for blank nodes.
	if !Opt.StoreXids || strings.HasPrefix(xid, "_:") {
		return uid, nil
	}

	err = processNQuad(gql.NQuad{NQuad: &api.NQuad{
		Subject:   xid,
		Predicate: "xid",
		ObjectValue: &api.Value{
			Val: &api.Value_StrVal{StrVal: xid},
		},
	}})
    x.Check(err)

	return uid, nil
}

func emitPostings(nq gql.NQuad, de *pb.DirectedEdge) error {
    Schema.convertToSchemaType(de, nq.ObjectValue == nil)
	fwd := posting.NewPosting(de)
	sch := Schema.getSchema(nq.Predicate)
	if nq.GetObjectValue() != nil {
		if lang := de.GetLang(); len(lang) > 0 {
			fwd.Uid = farm.Fingerprint64([]byte(lang))
		} else if sch.List {
			fwd.Uid = farm.Fingerprint64(de.Value)
		} else {
			fwd.Uid = math.MaxUint64
		}
	}
	fwd.Facets = nq.Facets

    err := emitMapEntry(
        nq.Predicate,
        x.DataKey(nq.Predicate, de.Entity),
        fwd,
    )
    x.Check(err)

	// Early exit for no reverse edge.
	if sch.GetDirective() != pb.SchemaUpdate_REVERSE {
        return nil
	}

	x.AssertTruef(nq.GetObjectValue() == nil, "only has reverse schema if object is UID")

    // reuse DE by swapping subject and object
	de.Entity, de.ValueId = de.ValueId, de.Entity
    Schema.convertToSchemaType(de, true)
	rev := posting.NewPosting(de)

    err = emitMapEntry(
        nq.Predicate,
        x.ReverseKey(nq.Predicate, de.Entity),
        rev,
    )
    x.Check(err)

    // swap back after reuse
	de.Entity, de.ValueId = de.ValueId, de.Entity
	return nil
}

func emitIndexPostings(nq gql.NQuad, de *pb.DirectedEdge) error {
	if nq.GetObjectValue() == nil {
		return nil // Cannot index UIDs
	}

	sch := Schema.getSchema(nq.GetPredicate())
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
		toks, err := tok.BuildTokens(schemaVal.Value, tok.GetLangTokenizer(toker, nq.Lang))
		x.Check(err)

		// Store index posting.
		for _, t := range toks {
            key := x.IndexKey(nq.Predicate, t)
            p := &pb.Posting{
                Uid:         de.GetEntity(),
                PostingType: pb.Posting_REF,
            }
			x.Check(emitMapEntry(nq.Predicate, key, p))
		}
	}

	return nil
}

func createPredicatePosting(predicate string) *pb.Posting {
	predbytes := []byte(predicate)
	return &pb.Posting{
		Uid:         farm.Fingerprint64(predbytes),
		Value:       predbytes,
		ValType:     pb.Posting_DEFAULT,
		PostingType: pb.Posting_VALUE,
	}
}
