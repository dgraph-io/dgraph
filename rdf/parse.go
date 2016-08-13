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

var pSubject = p.ParseFunc(func(c p.Context) p.Context {
	var i int
	i, c = p.OneOf{pIriRef, pBNLabel}.ParseIndex(c)
	var v string
	switch i {
	case 0:
		v = c.Value().(string)
	case 1:
		v = "_:" + c.Value().(string)
	}
	return c.WithValue(v)
})

var pObject = p.ParseFunc(func(c p.Context) p.Context {
	var i int
	i, c = p.OneOf{pIriRef, pBNLabel, pLiteral}.ParseIndex(c)
	switch i {
	case 1:
		c = c.WithValue("_:" + c.Value().(string))
	}
	return c
})

type literal struct {
	Value   string
	LangTag string
}

func pStringUntilByte(b byte) p.Parser {
	return p.ParseFunc(func(c p.Context) p.Context {
		v := ""
		for c.Stream().Good() {
			_b := c.Stream().Token().Value().(byte)
			if _b == b {
				break
			}
			v += string(_b)
			c = c.NextToken()
		}
		return c.WithValue(v)
	})
}

func pNotByteIn(s string) p.Parser {
	return pPred(func(b byte) bool {
		for _, _b := range []byte(s) {
			if b == _b {
				return false
			}
		}
		return true
	})
}

var pEChar = p.ParseFunc(func(c p.Context) p.Context {
	c = c.Parse(pByte('\\'))
	if c.Stream().Err() != nil {
		return c.WithError(fmt.Errorf("incomplete echar: %s", c.Stream().Err()))
	}
	s, err := strconv.Unquote(`"\` + string(c.Stream().Token().Value().(byte)) + `"`)
	if err == nil && len(s) != 1 {
		panic(s)
	}
	return c.NextToken().WithValue(s[0]).WithError(err)
})

var pQuotedStringLiteral = p.ParseFunc(func(c p.Context) p.Context {
	c = c.Parse(pByte('"'))
	c, vs := p.MinTimes{0, p.OneOf{pNotByteIn(`\"`), pEChar}}.Parse(c)
	return c.Parse(pByte('"')).WithValue(catBytes(vs))
})

func catBytes(vs []p.Value) (ret string) {
	for _, v := range vs {
		ret += string(v.(byte))
	}
	return
}

var pLiteral = p.ParseFunc(func(c p.Context) p.Context {
	c = pQuotedStringLiteral.Parse(c)
	l := literal{
		Value: c.Value().(string),
	}
	i, _c := p.OneOf{pLangTag, p.ParseFunc(func(c p.Context) p.Context {
		c = c.Parse(pBytes("^^"))
		return c.Parse(pIriRef)
	})}.ParseIndex(c)
	if _c.Good() {
		switch i {
		case 0:
			l.LangTag = _c.Value().(string)
		case 1:
			l.Value += "@@" + _c.Value().(string)
		}
		c = _c
	}
	return c.WithValue(l)
})

var pLangTag = p.ParseFunc(func(c p.Context) p.Context {
	c = c.Parse(pByte('@'))
	s := ""
	c, vs := p.MinTimes{1, pPred(func(b byte) bool {
		return unicode.IsLetter(rune(b))
	})}.Parse(c)
	for _, v := range vs {
		s += string(v.(byte))
	}
	c, vs = p.MinTimes{0, pPred(func(b byte) bool {
		return b == '-' || unicode.IsLetter(rune(b)) || unicode.IsNumber(rune(b))
	})}.Parse(c)
	for _, v := range vs {
		s += string(v.(byte))
	}
	return c.WithValue(s)
})

func pPred(pred func(byte) bool) p.Parser {
	return p.ParseFunc(func(c p.Context) p.Context {
		if c.Stream().Err() != nil {
			return c.WithError(c.Stream().Err())
		}
		_b := c.Stream().Token().Value().(byte)
		if pred(_b) {
			return c.WithValue(_b).NextToken()
		}
		return c.WithError(fmt.Errorf("%q does not satisfy %s", _b, pred))
	})
}

func pByte(b byte) p.Parser {
	return p.ParseFunc(func(c p.Context) p.Context {
		if c.Stream().Err() != nil {
			return c.WithError(c.Stream().Err())
		}
		_b := c.Stream().Token().Value().(byte)
		if _b != b {
			return c.WithError(fmt.Errorf("expected %q but got %q", b, _b))
		}
		return c.WithValue(b).NextToken()
	})
}

var pIriRef = p.ParseFunc(func(c p.Context) p.Context {
	c = c.Parse(pByte('<'))
	c = c.Parse(pStringUntilByte('>'))
	v := c.Value()
	c = c.Parse(pByte('>'))
	return c.WithValue(v)
})

var pBNLabel = p.ParseFunc(func(c p.Context) p.Context {
	c = c.Parse(pBytes("_:"))
	c = c.Parse(notWS)
	return c
})

func predStar(pred func(b byte) bool) p.Parser {
	return p.ParseFunc(func(c p.Context) p.Context {
		v := ""
		for c.Stream().Err() == nil {
			_b := c.Stream().Token().Value().(byte)
			if !pred(_b) {
				break
			}
			v += string(_b)
			c = c.NextToken()
		}
		return c.WithValue(v)
	})
}

var pPredicate = pIriRef

var notWS = predStar(func(b byte) bool {
	return !unicode.IsSpace(rune(b))
})

func pBytes(bs string) p.Parser {
	return p.ParseFunc(func(c p.Context) p.Context {
		for _, b := range []byte(bs) {
			if err := c.Stream().Err(); err != nil {
				return c.WithError(fmt.Errorf("expected %q but got %s", b, err))
			}
			_b := c.Stream().Token().Value().(byte)
			if _b != b {
				return c.WithError(fmt.Errorf("expected %q but saw %q", b, _b))
			}
			c = c.NextToken()
		}
		return c
	})
}

var pWS = predStar(func(b byte) bool {
	return unicode.IsSpace(rune(b))
})

var pLabel = pSubject

var pNQuadStatement = p.ParseFunc(func(c p.Context) p.Context {
	var ret NQuad
	c = c.Parse(pSubject)
	ret.Subject = c.Value().(string)
	c = c.Parse(pWS)
	c = c.Parse(pPredicate)
	ret.Predicate = c.Value().(string)
	c = c.Parse(pWS)
	c = c.Parse(pObject)
	switch v := c.Value().(type) {
	case string:
		ret.ObjectId = v
	case literal:
		ret.ObjectValue = []byte(v.Value)
		if v.LangTag != "" {
			ret.Predicate += "." + v.LangTag
		}
	}
	c = c.Parse(pWS)
	_c := c.Parse(pLabel)
	if _c.Good() {
		c = _c
		ret.Label = c.Value().(string)
	}
	c = c.Parse(pWS)
	c = c.Parse(pByte('.'))
	return c.WithValue(ret)
})

func Parse(line string) (rnq NQuad, err error) {
	s := p.NewByteStream(bytes.NewBufferString(line))
	c := p.NewContext(s).Parse(pNQuadStatement)
	rnq = c.Value().(NQuad)
	err = c.Err()
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
