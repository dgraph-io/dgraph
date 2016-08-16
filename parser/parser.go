package parser

import (
	"fmt"
	"reflect"
)

func Plus(s Stream, p Parser) (Stream, []Value) {
	return Repeat(s, 1, 0, p)
}

func Star(s Stream, p Parser) (Stream, []Value) {
	return Repeat(s, 0, 0, p)
}

func Repeat(s Stream, min, max int, p Parser) (Stream, []Value) {
	var vs []Value
	for i := 0; max != 0 && i < max; i++ {
		s1, err := ParseErr(s, p)
		if err != nil {
			if i < min {
				panic(SyntaxError{s, fmt.Errorf("repetition %d failed: %s", err)})
			}
			break
		}
		s = s1
		vs = append(vs, p)
	}
	return s, vs
}

type SyntaxError struct {
	Stream Stream
	Err    error
}

func (se SyntaxError) Error() string {
	return fmt.Sprintf("syntax error at %s: %s", se.Stream.Position(), se.Err)
}

func ParseErr(s Stream, p Parser) (_s Stream, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if se, ok := r.(SyntaxError); ok {
			err = se
			return
		}
		panic(r)
	}()
	_s = Parse(s, p)
	return
}

func ParserName(p Parser) string {
	t := reflect.ValueOf(p).Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Name() == "" {
		panic(p)
	}
	return t.Name()
}

func recoverSyntaxError(f func(SyntaxError)) {
	r := recover()
	if r == nil {
		return
	}
	se, ok := r.(SyntaxError)
	if !ok {
		panic(r)
	}
	f(se)
}

func Parse(s Stream, p Parser) Stream {
	defer recoverSyntaxError(func(se SyntaxError) {
		panic(SyntaxError{s, fmt.Errorf("while parsing %q at %s: %s", ParserName(p), s.Position(), se.Err)})
	})
	return p.Parse(s)
}

type Value interface{}

func OneOf(s Stream, ps ...Parser) (Stream, int) {
	for i, p := range ps {
		s1, err := ParseErr(s, p)
		if err == nil {
			return s1, i
		}
	}
	panic(SyntaxError{s, fmt.Errorf("couldn't match one of %s", ps)})
}

type ParseFunc func(Stream) Stream

func (pf ParseFunc) Parse(s Stream) Stream {
	return pf(s)
}

func Maybe(s Stream, p Parser) Stream {
	_s, err := ParseErr(s, p)
	if err != nil {
		return s
	}
	return _s
}
