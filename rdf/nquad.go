package rdf

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

// NQuad is the data structure used for storing rdf N-Quads.
type NQuad struct {
	Subject     string
	Predicate   string
	ObjectId    string
	ObjectValue []byte
	Label       string
}

// ToEdge is useful when you want to find the UID corresponding to XID for
// just one edge. The method doesn't automatically generate a UID for an XID.
func (nq NQuad) ToEdge() (result x.DirectedEdge, rerr error) {
	sid, err := getUid(nq.Subject)
	if err != nil {
		return result, err
	}

	result.Entity = sid
	// An edge can have an id or a value.
	if len(nq.ObjectId) > 0 {
		oid, err := getUid(nq.ObjectId)
		if err != nil {
			return result, err
		}
		result.ValueId = oid
	} else {
		result.Value = nq.ObjectValue
	}
	result.Attribute = nq.Predicate
	result.Source = nq.Label
	result.Timestamp = time.Now()
	return result, nil
}

// ToEdgeUsing determines the UIDs for the provided XIDs and populates the
// xidToUid map.
func (nq NQuad) ToEdgeUsing(
	xidToUID map[string]uint64) (result x.DirectedEdge, rerr error) {
	uid, err := toUid(nq.Subject, xidToUID)
	if err != nil {
		return result, err
	}
	result.Entity = uid

	if len(nq.ObjectId) == 0 {
		result.Value = nq.ObjectValue
	} else {
		uid, err = toUid(nq.ObjectId, xidToUID)
		if err != nil {
			return result, err
		}
		result.ValueId = uid
	}
	result.Attribute = nq.Predicate
	result.Source = nq.Label
	result.Timestamp = time.Now()
	return result, nil
}

// Gets the uid corresponding to an xid from the posting list which stores the
// mapping.
func getUid(xid string) (uint64, error) {
	// If string represents a UID, convert to uint64 and return.
	if strings.HasPrefix(xid, "_uid_:") {
		return strconv.ParseUint(xid[6:], 0, 64)
	}
	// Get uid from posting list in UidStore.
	return uid.Get(xid)
}

func toUid(xid string, xidToUID map[string]uint64) (uid uint64, rerr error) {
	if id, present := xidToUID[xid]; present {
		return id, nil
	}

	if !strings.HasPrefix(xid, "_uid_:") {
		return 0, fmt.Errorf("Unable to find xid: %v", xid)
	}
	return strconv.ParseUint(xid[6:], 0, 64)
}
