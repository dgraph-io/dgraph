package rdf

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	p "github.com/dgraph-io/dgraph/parsing"
)

type subject string

func (me *subject) Parse(s p.Stream) p.Stream {
	var (
		iriRef  iriRef
		bnLabel bnLabel
	)
	oo := p.OneOf(&iriRef, &bnLabel)
	s = p.Parse(s, &oo)
	switch oo.Index {
	case 0:
		*me = subject(iriRef)
	case 1:
		*me = subject(bnLabel)
	}
	return s
}

type object struct {
	Id      string
	Literal literal
}

func (me *object) Parse(s p.Stream) p.Stream {
	var (
		iriRef  iriRef
		bnLabel bnLabel
	)
	oo := p.OneOf(&iriRef, &bnLabel, &me.Literal)
	s = p.Parse(s, &oo)
	switch oo.Index {
	case 0:
		me.Id = string(iriRef)
	case 1:
		me.Id = string(bnLabel)
	}
	return s
}

func pByte(s p.Stream, b byte) p.Stream {
	if !s.Good() {
		panic(p.NewSyntaxError(p.SyntaxErrorContext{Err: s.Err()}))
	}
	_b := s.Token().(byte)
	if _b != b {
		panic(p.NewSyntaxError(p.SyntaxErrorContext{
			Err:    fmt.Errorf("wanted %q", b),
			Stream: s,
		}))
	}
	return s.Next()
}

type eChar byte

func (me *eChar) Parse(s p.Stream) p.Stream {
	s = pByte(s, '\\')
	if !s.Good() {
		panic(p.NewSyntaxError(p.SyntaxErrorContext{
			Stream: s,
			Err:    s.Err(),
		}))
	}
	b := s.Token().(byte)
	// ECHAR ::= '\' [tbnrf"'\]
	switch b {
	case 't':
		*me = '\t'
	case 'b':
		*me = '\b'
	case 'n':
		*me = '\n'
	case 'r':
		*me = '\r'
	case 'f':
		*me = '\f'
	case '"':
		*me = '"'
	case '\'':
		*me = '\''
	default:
		panic(p.NewSyntaxError(p.SyntaxErrorContext{
			Stream: s,
			Err:    fmt.Errorf("can't escape %q", b),
		}))
	}
	return s.Next()
}

type quotedStringLiteral string

func (me *quotedStringLiteral) Parse(s p.Stream) p.Stream {
	s = pByte(s, '"')
	var bs []byte
	for s.Good() {
		b := s.Token().(byte)
		switch b {
		case '"':
			*me = quotedStringLiteral(string(bs))
			return s.Next()
		case '\\':
			var e eChar
			s = p.Parse(s, &e)
			bs = append(bs, byte(e))
		default:
			bs = append(bs, b)
			s = s.Next()
		}
	}
	panic(p.NewSyntaxError(p.SyntaxErrorContext{
		Stream: s,
		Err:    s.Err(),
	}))
}

type literal struct {
	Value   string
	LangTag string
}

func (l *literal) Parse(s p.Stream) p.Stream {
	var qsl quotedStringLiteral
	s = p.Parse(s, &qsl)
	l.Value = string(qsl)
	var (
		langTag langTag
		iriRef  iriRef
	)
	oo := p.OneOf(
		&langTag,
		p.ParseFunc(func(s p.Stream) p.Stream {
			s = p.Bytes(s, "^^")
			return p.Parse(s, &iriRef)
		}),
	)
	s, err := p.ParseErr(s, &oo)
	if err == nil {
		switch oo.Index {
		case 0:
			l.LangTag = string(langTag)
		case 1:
			l.Value += "@@" + string(iriRef)
		}
	}
	return s
}

type bytesWhile struct {
	b    []byte
	pred func(byte) bool
}

func (me *bytesWhile) Parse(s p.Stream) p.Stream {
	for s.Good() {
		b := s.Token().(byte)
		if !me.pred(b) {
			break
		}
		me.b = append(me.b, b)
		s = s.Next()
	}
	return s
}

type untilByte struct {
	b  byte
	bs []byte
}

func (me *untilByte) Parse(s p.Stream) p.Stream {
	for s.Good() {
		b := s.Token().(byte)
		s = s.Next()
		if b == me.b {
			return s
		}
		me.bs = append(me.bs, b)
	}
	panic(p.NewSyntaxError(p.SyntaxErrorContext{
		Stream: s,
		Err:    s.Err(),
	}))
}

type langTag string

func (me *langTag) Parse(s p.Stream) p.Stream {
	s = pByte(s, '@')
	bw := bytesWhile{
		pred: func(b byte) bool { return unicode.IsLetter(rune(b)) },
	}
	s = p.Parse(s, &bw)
	if len(bw.b) < 1 {
		panic(p.NewSyntaxError(p.SyntaxErrorContext{
			Stream: s,
			Err:    errors.New("require at least one letter"),
		}))
	}
	bw.pred = func(b byte) bool {
		return b == '-' || unicode.IsLetter(rune(b)) || unicode.IsNumber(rune(b))
	}
	s = p.Parse(s, &bw)
	*me = langTag(bw.b)
	return s
}

type iriRef string

func (me *iriRef) Parse(s p.Stream) p.Stream {
	s = pByte(s, '<')
	bw := bytesWhile{
		pred: func(b byte) bool {
			return b > 0x20 && !strings.ContainsRune("<>\"{}|^`\\", rune(b))
		},
	}
	s = p.Parse(s, &bw)
	s = pByte(s, '>')
	*me = iriRef(bw.b)
	return s
}

type bnLabel string

func (me *bnLabel) Parse(s p.Stream) p.Stream {
	s = pByte(s, '_')
	beforeColon := bytesWhile{
		pred: func(b byte) bool {
			return b != ':' && !unicode.IsSpace(rune(b))
		},
	}
	s = beforeColon.Parse(s)
	s = pByte(s, ':')
	rest := bytesWhile{
		pred: func(b byte) bool {
			return !unicode.IsSpace(rune(b))
		},
	}
	s = rest.Parse(s)
	*me = bnLabel(fmt.Sprintf("_%s:%s", beforeColon.b, rest.b))
	return s
}

type predicate struct {
	iriRef
}

type label struct {
	subject
}

type nQuadParser NQuad

func (me *nQuadParser) Parse(s p.Stream) p.Stream {
	var (
		sub   subject
		pred  predicate
		obj   object
		label label
	)
	s = p.Parse(s, &sub)
	me.Subject = string(sub)
	betweenNQuadFields(&s)
	s = p.Parse(s, &pred)
	me.Predicate = string(pred.iriRef)
	betweenNQuadFields(&s)
	s = p.Parse(s, &obj)
	me.ObjectId = obj.Id
	if obj.Literal.Value != "" {
		me.ObjectValue = []byte(obj.Literal.Value)
	}
	if obj.Literal.LangTag != "" {
		me.Predicate += "." + obj.Literal.LangTag
	}
	betweenNQuadFields(&s)
	s = p.Maybe(s, p.ParseFunc(func(s p.Stream) p.Stream {
		s = p.Parse(s, &label)
		me.Label = string(label.subject)
		betweenNQuadFields(&s)
		return s
	}))
	s = pByte(s, '.')
	return s
}

func discardWhitespace(s *p.Stream) {
	discardWhilePred(s, func(b byte) bool {
		return unicode.IsSpace(rune(b))
	})
}

func betweenNQuadFields(s *p.Stream) {
	discardWhilePred(s, func(b byte) bool {
		return unicode.IsSpace(rune(b)) && b != '\n'
	})
}

func discardWhilePred(s *p.Stream, pred func(byte) bool) {
	_s := *s
	for _s.Good() {
		b := _s.Token().(byte)
		if !pred(b) {
			break
		}
		_s = _s.Next()
	}
	*s = _s
}

type nQuadsDoc []NQuad

func (me *nQuadsDoc) Parse(s p.Stream) p.Stream {
	var err p.SyntaxError
	for {
		discardWhitespace(&s)
		var nqp nQuadParser
		var s1 p.Stream
		s1, err = p.ParseErr(s, &nqp)
		if err != nil {
			break
		}
		*me = append(*me, NQuad(nqp))
		s = s1
	}
	if s.Good() {
		panic(err)
	}
	return s
}
