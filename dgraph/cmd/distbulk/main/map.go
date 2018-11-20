package main

import (
	// "fmt"
	// "os"
	"log"

	"math"
	"strings"

	"github.com/chrislusf/gleam/gio"
	// "github.com/chrislusf/gleam/util"

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
	"github.com/pkg/errors"
)

// uid can be either a uint64 (normal) or a string (for indexing only)
func emitMapEntry(pred string, key []byte, posting *pb.Posting) error {
	pbytes, err := proto.Marshal(&pb.SivaPostingList{
		Postings: []*pb.Posting{posting},
	})
	if err != nil {
		return err
	}

    ubytes, err := proto.Marshal(&pb.List{
        Uids: []uint64{posting.Uid},
    })
    if err != nil {
        return err
    }

	gio.Emit(pred, key, pbytes, ubytes)
	return nil
}

func rdfToMapEntry(row []interface{}) error {
	rdfLine := gio.ToString(row[0])
	nq, err := parseNQuad(rdfLine)
	if err != nil {
		if err == rdf.ErrEmpty {
			return nil
		}
		return errors.Wrapf(err, "while parsing line %q", rdfLine)
	}

	if err := facets.SortAndValidate(nq.Facets); err != nil {
		return err
	}

	return processNQuad(nq)
}

func parseNQuad(line string) (gql.NQuad, error) {
	nq, err := rdf.Parse(line)
	if err != nil {
		return gql.NQuad{}, err
	}
	return gql.NQuad{NQuad: &nq}, nil
}

func processNQuad(nq gql.NQuad) error {
	sid, err := lookupUid(nq.GetSubject())
	var oid uint64
	var de *pb.DirectedEdge
	if nq.GetObjectValue() == nil {
		oid, err = lookupUid(nq.GetObjectId())
		if err != nil {
			return err
		}
		de = nq.CreateUidEdge(sid, oid)
	} else {
		de, err = nq.CreateValueEdge(sid)
		if err != nil {
			return err
		}
	}

	fwd, rev := createPostings(nq, de)
	key := x.DataKey(nq.Predicate, sid)
	err = emitMapEntry(nq.Predicate, key, fwd)
	if err != nil {
		return err
	}

	if rev != nil {
		key = x.ReverseKey(nq.Predicate, oid)
		err = emitMapEntry(nq.Predicate, key, rev)
		if err != nil {
			return err
		}
	}

	if err = addIndexMapEntries(nq, de); err != nil {
		return err
	}

	if Opt.ExpandEdges {
		key = x.DataKey("_predicate_", sid)
		pp := createPredicatePosting(nq.Predicate)
		err = emitMapEntry("_predicate_", key, pp)
		if err != nil {
			return err
		}
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
	if err != nil {
		return 0, err
	}

	return uid, nil
}

func createPostings(nq gql.NQuad, de *pb.DirectedEdge) (*pb.Posting, *pb.Posting) {
	Schema.validateType(de, nq.ObjectValue == nil)

	p := posting.NewPosting(de)
	sch := Schema.getSchema(nq.Predicate)
	if nq.GetObjectValue() != nil {
		if lang := de.GetLang(); len(lang) > 0 {
			p.Uid = farm.Fingerprint64([]byte(lang))
		} else if sch.List {
			p.Uid = farm.Fingerprint64(de.Value)
		} else {
			p.Uid = math.MaxUint64
		}
	}
	p.Facets = nq.Facets

	// Early exit for no reverse edge.
	if sch.GetDirective() != pb.SchemaUpdate_REVERSE {
		return p, nil
	}

	// Reverse predicate
	x.AssertTruef(nq.GetObjectValue() == nil, "only has reverse schema if object is UID")
	de.Entity, de.ValueId = de.ValueId, de.Entity
	Schema.validateType(de, true)
	rp := posting.NewPosting(de)

	de.Entity, de.ValueId = de.ValueId, de.Entity // de reused so swap back.

	return p, rp
}

func addIndexMapEntries(nq gql.NQuad, de *pb.DirectedEdge) error {
	// Cannot index UIDs
	if nq.GetObjectValue() == nil {
		return nil
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
		if err != nil {
			return err
		}

		// Extract tokens.
		toks, err := tok.BuildTokens(schemaVal.Value, toker)
		if err != nil {
			return err
		}

		// Store index posting.
		entity := de.GetEntity()
		for _, t := range toks {
			key := x.IndexKey(nq.Predicate, t)
			p := &pb.Posting{
				Uid:         entity,
				PostingType: pb.Posting_REF,
			}
			err = emitMapEntry(nq.Predicate, key, p)
			if err != nil {
				return err
			}
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
