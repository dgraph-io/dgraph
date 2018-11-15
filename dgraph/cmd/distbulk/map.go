package distbulk


func parseNQuad(line string) (gql.NQuad, error) {
    nq, err := rdf.Parse(line)
    if err != nil {
        return gql.NQuad{}, err
    }
    return gql.NQuad{NQuad: &nq}, nil
}

func addIndexMapEntries(nq gql.NQuad, de *pb.DirectedEdge) {
    if nq.GetObjectValue() == nil {
        return // Cannot index UIDs
    }

    sch := m.schema.getSchema(nq.GetPredicate())
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
        toks, err := tok.BuildTokens(schemaVal.Value, toker)
        x.Check(err)

        // Store index posting.
        for _, t := range toks {
            m.addMapEntry(
                x.IndexKey(nq.Predicate, t),
                &pb.Posting{
                    Uid:         de.GetEntity(),
                    PostingType: pb.Posting_REF,
                },
                m.state.shards.shardFor(nq.Predicate),
            )
        }
    }
}

func lookupUid(xid string) uint64 {
    uid, isNew := m.xids.AssignUid(xid)
    if !isNew || !m.opt.StoreXids {
        return uid
    }
    if strings.HasPrefix(xid, "_:") {
        // Don't store xids for blank nodes.
        return uid
    }
    nq := gql.NQuad{NQuad: &api.NQuad{
        Subject:   xid,
        Predicate: "xid",
        ObjectValue: &api.Value{
            Val: &api.Value_StrVal{StrVal: xid},
        },
    }}
    m.processNQuad(nq)
    return uid
}

func createPredicatePosting(predicate string) *pb.Posting {
    fp := farm.Fingerprint64([]byte(predicate))
    return &pb.Posting{
        Uid:         fp,
        Value:       []byte(predicate),
        ValType:     pb.Posting_DEFAULT,
        PostingType: pb.Posting_VALUE,
    }
}

func createPostings(nq gql.NQuad,
de *pb.DirectedEdge) (*pb.Posting, *pb.Posting) {

    m.schema.validateType(de, nq.ObjectValue == nil)

    p := posting.NewPosting(de)
    sch := m.schema.getSchema(nq.GetPredicate())
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
    m.schema.validateType(de, true)
    rp := posting.NewPosting(de)

    de.Entity, de.ValueId = de.ValueId, de.Entity // de reused so swap back.

    return p, rp
}

func addMapEntry(key []byte, p *pb.Posting, shard int) {
    atomic.AddInt64(&m.prog.mapEdgeCount, 1)

    me := &pb.MapEntry{
        Key: key,
    }
    if p.PostingType != pb.Posting_REF || len(p.Facets) > 0 {
        me.Posting = p
    } else {
        me.Uid = p.Uid
    }
    sh := &m.shards[shard]

    var err error
    sh.entriesBuf = x.AppendUvarint(sh.entriesBuf, uint64(me.Size()))
    sh.entriesBuf, err = x.AppendProtoMsg(sh.entriesBuf, me)
    x.Check(err)
}

func processNQuad(nq gql.NQuad) RETVAL {
    sid := lookupUid(nq.GetSubject())
    var oid uint64
    var de *pb.DirectedEdge
    if nq.GetObjectValue() == nil {
        oid = lookupUid(nq.GetObjectId())
        de = nq.CreateUidEdge(sid, oid)
    } else {
        de, err := nq.CreateValueEdge(sid)
        x.Check(err)
    }

    fwd, rev := createPostings(nq, de)
    shard := m.state.shards.shardFor(nq.Predicate)
    key := x.DataKey(nq.Predicate, sid)
    addMapEntry(key, fwd, shard)

    if rev != nil {
        key = x.ReverseKey(nq.Predicate, oid)
        addMapEntry(key, rev, shard)
    }
    m.addIndexMapEntries(nq, de)

    if m.opt.ExpandEdges {
        shard := m.state.shards.shardFor("_predicate_")
        key = x.DataKey("_predicate_", sid)
        pp := m.createPredicatePosting(nq.Predicate)
        addMapEntry(key, pp, shard)
    }
}

func rdfToMapEntry(row []interface{}) error {
    nq, err := rdf.Parse(gio.ToString(row[0]))
    if err != nil {
        if err == rdf.ErrEmpty {
            return nil
        }
        return errors.Wrapf(err, "while parsing line %q", rdfLine)
    }
    if err := facets.SortAndValidate(nq.Facets); err != nil {
        return err
    }
    m.processNQuad(nq)
    return nil
}
