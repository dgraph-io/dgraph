package parser

import (
	"fmt"
	"reflect"
	"strings"
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
				err.AddContext(SyntaxErrorContext{
					Err:    fmt.Errorf("repetition %d failed", i),
					Stream: s,
				})
				panic(err)
			}
			break
		}
		s = s1
		vs = append(vs, p)
	}
	return s, vs
}

type SyntaxError interface {
	error
	AddContext(SyntaxErrorContext)
}

type syntaxError struct {
	Err   error
	Stack []SyntaxErrorContext
}

func (se *syntaxError) AddContext(c SyntaxErrorContext) {
	se.Stack = append(se.Stack, c)
}

func NewSyntaxError(c SyntaxErrorContext) SyntaxError {
	se := new(syntaxError)
	se.Stack = append(se.Stack, c)
	return se
}

type SyntaxErrorContext struct {
	Stream Stream
	Parser Parser
	Err    error
}

func (se *syntaxError) Error() string {
	var ss []string
	for _, c := range se.Stack {
		if c.Err != nil {
			ss = append(ss, c.Err.Error())
		}
		if c.Parser != nil {
			ss = append(ss, fmt.Sprintf("while parsing %s", ParserName(c.Parser)))
		}
		if c.Stream != nil {
			ss = append(ss, fmt.Sprintf("starting with %q at %s", c.Stream.Token(), c.Stream.Position()))
		}
	}
	s := strings.Join(ss, " ")
	ret := "syntax error"
	if s != "" {
		ret += ": " + s
	}
	return ret
}

func ParseErr(s Stream, p Parser) (_s Stream, err SyntaxError) {
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
		se.AddContext(SyntaxErrorContext{
			Parser: p,
			Stream: s,
		})
		panic(se)
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
	panic(NewSyntaxError(SyntaxErrorContext{Err: fmt.Errorf("couldn't match one of %s", ps)}))
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
