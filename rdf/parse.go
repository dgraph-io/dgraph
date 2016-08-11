/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package rdf

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	p "github.com/dgraph-io/dgraph/parser"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
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

func toUid(xid string, xidToUID map[string]uint64) (uid uint64, rerr error) {
	if id, present := xidToUID[xid]; present {
		return id, nil
	}

	if !strings.HasPrefix(xid, "_uid_:") {
		return 0, fmt.Errorf("Unable to find xid: %v", xid)
	}
	return strconv.ParseUint(xid[6:], 0, 64)
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

// This function is used to extract an IRI from an IRIREF.
func stripBracketsIfPresent(val string) string {
	if val[0] != '<' && val[len(val)-1] != '>' {
		return val
	}
	return val[1 : len(val)-1]
}

type bnLabel struct{}

var subject = p.SeqStep{
	Prod: p.OneOf{pIriRef, pBNLabel},
	Reduce: func(v, v1 p.Value) p.Value {
		r := v.(NQuad)
		switch _v1 := v1.(type) {
		case string:
			r.Subject = _v1
		case iriRef:
			r.Subject = string(_v1)
		default:
			panic(v1)
		}
		return r
	},
}

var predicate = p.SeqStep{
	Prod: pIriRef,
	Reduce: func(v, v1 p.Value) p.Value {
		r := v.(NQuad)
		r.Predicate = string(v1.(iriRef))
		return r
	},
}

var object = p.SeqStep{
	Prod: p.OneOf{pIriRef, pBNLabel, pLiteral},
	Reduce: func(v, v1 p.Value) p.Value {
		r := v.(NQuad)
		switch _v1 := v1.(type) {
		case string:
			r.ObjectId = _v1
		case literal:
			r.ObjectValue = []byte(_v1.Value)
			if _v1.LangTag != "" {
				r.Predicate += "." + _v1.LangTag
			}
		case iriRef:
			r.ObjectId = string(_v1)
		default:
			panic(v1)
		}
		return r
	},
}

type literal struct {
	Value   string
	LangTag string
}

var pLiteral = p.Seq{
	Initial: func() p.Value { return literal{} },
	Steps: []p.SeqStep{
		{Prod: pByte('"')},
		{Prod: pStringUntilByte('"'), Reduce: func(v, v1 p.Value) p.Value {
			_v := v.(literal)
			_v.Value = v1.(string)
			return _v
		}},
		{Prod: pByte('"')},
		{
			Prod: p.Maybe{
				Prod: p.OneOf([]p.Prod{
					pLangTag,
					p.Seq{
						Steps: []p.SeqStep{
							pBytes("^^"),
							{Prod: pIriRef, Reduce: p.ClobberReducer},
						},
					},
				}),
				IfNot: "",
			},
			Reduce: func(v, v1 p.Value) p.Value {
				_v := v.(literal)
				switch _v1 := v1.(type) {
				case string:
					_v.LangTag = _v1
				case iriRef:
					_v.Value += "@@" + string(_v1)
				default:
					panic(_v1)
				}
				return _v
			},
		},
	},
}

var pLangTag = p.Seq{
	Initial: func() p.Value { return "" },
	Steps: []p.SeqStep{
		ssByte('@'),
		{Prod: p.MinTimes{
			Initial: func() p.Value { return "" },
			Min:     1,
			Prod: pPred(func(b byte) bool {
				return unicode.IsLetter(rune(b))
			}),
			Reduce: appendStringByteReducer,
		}, Reduce: p.StringReducer},
		{Prod: p.MinTimes{
			Initial: func() p.Value { return "" },
			Prod: pPred(func(b byte) bool {
				return b == '-' || unicode.IsLetter(rune(b)) || unicode.IsNumber(rune(b))
			}),
			Reduce: appendStringByteReducer,
		}, Reduce: p.StringReducer},
	},
}

func pStringUntilByte(b byte) p.Prod {
	return p.MinTimes{
		Prod:    pNotByte(b),
		Initial: func() p.Value { return "" },
		Reduce: func(v, v1 p.Value) p.Value {
			return v.(string) + string(v1.(byte))
		},
	}
}

func pPred(pred func(byte) bool) p.Prod {
	return p.MatchFunc(func(s p.Stream) (p.Stream, p.Value, bool) {
		_b := s.Token().Value().(byte)
		return s.Next(), _b, pred(_b)
	})
}

func pWhilePred(pred func(b byte) bool) p.Prod {
	return p.MinTimes{
		Prod:    pPred(pred),
		Initial: func() p.Value { return "" },
		Reduce:  appendStringByteReducer,
	}
}

func appendStringByteReducer(v, v1 p.Value) p.Value {
	return v.(string) + string(v1.(byte))
}

func ssStringUntilByte(b byte) p.SeqStep {
	return p.SeqStep{pStringUntilByte(b), p.StringReducer}
}

func ssStringWhilePred(pred func(b byte) bool) p.SeqStep {
	return p.SeqStep{
		Prod: p.MinTimes{
			Prod:    pPred(pred),
			Initial: func() p.Value { return "" },
			Reduce:  appendStringByteReducer,
		},
		Reduce: p.StringReducer,
	}
}

func ssByte(b byte) p.SeqStep {
	return p.SeqStep{pByte(b), nil}
}

var pIriRef = p.Seq{
	Steps: []p.SeqStep{
		{pByte('<'), nil},
		ssStringUntilByte('>'),
		{pByte('>'), nil}},
	Initial: func() p.Value { return "" },
	Finally: func(v p.Value) p.Value { return iriRef(v.(string)) },
}

func pByte(b byte) p.Prod {
	return p.MatchFunc(func(s p.Stream) (p.Stream, p.Value, bool) {
		if s.Err() != nil {
			return nil, nil, false
		}
		_b := s.Token().Value().(byte)
		return s.Next(), _b, _b == b
	})
}

func pNotByte(b byte) p.Prod {
	return p.MatchFunc(func(s p.Stream) (p.Stream, p.Value, bool) {
		if s.Err() != nil {
			return nil, nil, false
		}
		_b := s.Token().Value().(byte)
		return s.Next(), _b, _b != b
	})
}

type iriRef string

var pBNLabel = p.Seq{
	Steps:   []p.SeqStep{pBytes("_:"), {notWS, p.StringReducer}},
	Initial: func() p.Value { return "_:" },
}

var notWS = predStar(func(b byte) bool {
	return !unicode.IsSpace(rune(b))
})

func pBytes(bs string) p.SeqStep {
	return p.SeqStep{
		Prod: p.MatchFunc(func(_s p.Stream) (s p.Stream, v p.Value, ok bool) {
			s = _s
			for _, b := range []byte(bs) {
				if s.Err() != nil || s.Token().Value().(byte) != b {
					return
				}
				s = s.Next()
			}
			ok = true
			return
		}),
	}
}

func predStar(pred func(b byte) bool) p.Prod {
	return p.MatchFunc(func(s p.Stream) (p.Stream, p.Value, bool) {
		v := ""
		for {
			if s.Err() != nil {
				break
			}
			b1 := s.Token().Value().(byte)
			if !pred(b1) {
				break
			}
			v += string(b1)
			s = s.Next()
		}
		return s, v, true
	})
}

var optWS = p.SeqStep{
	Prod: predStar(func(b byte) bool {
		return unicode.IsSpace(rune(b))
	}),
}

var period = p.SeqStep{Prod: p.MatchFunc(func(s p.Stream) (p.Stream, p.Value, bool) {
	return s.Next(), '.', s.Err() == nil && s.Token().Value().(byte) == '.'
})}

var nquadStatement = p.Seq{
	Steps: []p.SeqStep{
		subject,
		optWS,
		predicate,
		optWS,
		object,
		optWS,
		period,
	},
	Initial: func() p.Value { return NQuad{} },
	// predicate,
	// object,
	// p.Maybe(graphLabel),
	// period,
}

func Parse(line string) (rnq NQuad, err error) {
	s := p.NewByteStream(bytes.NewBufferString(line))
	_, v, err := p.Parse(s, nquadStatement)
	rnq = v.(NQuad)
	return
}

// // Parse parses a mutation string and returns the NQuad representation for it.
// func Parse(line string) (rnq NQuad, rerr error) {
// 	l := &lex.Lexer{}
// 	l.Init(line)

// 	go run(l)
// 	var oval string
// 	var vend bool
// 	// We read items from the l.Items channel to which the lexer sends items.
// 	for item := range l.Items {
// 		if item.Typ == itemSubject {
// 			rnq.Subject = stripBracketsIfPresent(item.Val)
// 		}
// 		if item.Typ == itemPredicate {
// 			rnq.Predicate = stripBracketsIfPresent(item.Val)
// 		}
// 		if item.Typ == itemObject {
// 			rnq.ObjectId = stripBracketsIfPresent(item.Val)
// 		}
// 		if item.Typ == itemLiteral {
// 			oval = item.Val
// 		}
// 		if item.Typ == itemLanguage {
// 			rnq.Predicate += "." + item.Val
// 		}
// 		if item.Typ == itemObjectType {
// 			// TODO: Strictly parse common types like integers, floats etc.
// 			if len(oval) == 0 {
// 				log.Fatalf(
// 					"itemObject should be emitted before itemObjectType. Input: [%s]",
// 					line)
// 			}
// 			oval += "@@" + stripBracketsIfPresent(item.Val)
// 		}
// 		if item.Typ == lex.ItemError {
// 			return rnq, fmt.Errorf(item.Val)
// 		}
// 		if item.Typ == itemValidEnd {
// 			vend = true
// 		}
// 		if item.Typ == itemLabel {
// 			rnq.Label = stripBracketsIfPresent(item.Val)
// 		}
// 	}
// 	if !vend {
// 		return rnq, fmt.Errorf("Invalid end of input. Input: [%s]", line)
// 	}
// 	if len(oval) > 0 {
// 		rnq.ObjectValue = []byte(oval)
// 	}
// 	if len(rnq.Subject) == 0 || len(rnq.Predicate) == 0 {
// 		return rnq, fmt.Errorf("Empty required fields in NQuad. Input: [%s]", line)
// 	}
// 	if len(rnq.ObjectId) == 0 && rnq.ObjectValue == nil {
// 		return rnq, fmt.Errorf("No Object in NQuad. Input: [%s]", line)
// 	}

// 	return rnq, nil
// }

func isNewline(r rune) bool {
	return r == '\n' || r == '\r'
}
