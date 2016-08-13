package rdf

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	p "github.com/dgraph-io/dgraph/parser"
	"github.com/dgraph-io/dgraph/uid"
)

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

func Parse(line string) (rnq NQuad, err error) {
	s := p.NewByteStream(bytes.NewBufferString(line))
	c := p.NewContext(s).Parse(pNQuadStatement)
	if c.Good() {
		rnq = c.Value().(NQuad)
	}
	err = c.Err()
	return
}
