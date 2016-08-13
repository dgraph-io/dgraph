package parser

import "errors"

type MinTimes struct {
	Min     int
	Prod    Prod
	Initial func() Value
	Reduce  func(Value, Value) Value
}

func (me MinTimes) Match(_sm Stream) (sm Stream, v Value, ok bool) {
	sm = _sm
	if me.Initial != nil {
		v = me.Initial()
	}
	for i := 0; ; i++ {
		var (
			sm1 Stream
			v1  Value
		)
		sm1, v1, ok = me.Prod.Match(sm)
		if !ok {
			if i >= me.Min {
				ok = true
			}
			return
		}
		sm = sm1
		if me.Reduce != nil {
			v = me.Reduce(v, v1)
		}
	}
	return
}

type Value interface{}

type Prod interface {
	Match(Stream) (Stream, Value, bool)
}

type MatchFunc func(Stream) (Stream, Value, bool)

func (mf MatchFunc) Match(s Stream) (Stream, Value, bool) {
	return mf(s)
}

type OneOf []Prod

func (me OneOf) Match(_s Stream) (s Stream, v Value, ok bool) {
	for _, p := range me {
		s1, v1, ok1 := p.Match(_s)
		if !ok1 {
			continue
		}
		if ok {
			panic("oneof matched more than one")
		}
		s = s1
		v = v1
		ok = true
	}
	return
}

func Seq(prods ...Prod) seq {
	return seq(prods)
}

type seq []Prod

func (sq seq) Match(sm Stream) (Stream, Value, bool) {
	ok := true
	v := make([]Value, 0, len(sq))
	for _, p := range sq {
		var v1 Value
		sm, v1, ok = p.Match(sm)
		if !ok {
			break
		}
		v = append(v, v1)
	}
	return sm, v, ok
}

func Parse(s Stream, p Prod) (_s Stream, v Value, err error) {
	_s, v, ok := p.Match(s)
	if !ok {
		err = errors.New("couldn't parse")
	}
	return
}

func StringReducer(v, v1 Value) Value {
	return v.(string) + v1.(string)
}

func Maybe(p Prod) maybe {
	return maybe{p}
}

type maybe struct {
	Prod Prod
}

var NoMatch = &struct{}{}

func (m maybe) Match(_s Stream) (s Stream, v Value, ok bool) {
	s, v, ok = m.Prod.Match(_s)
	if ok {
		return
	}
	return _s, NoMatch, true
}

func ClobberReducer(v, v1 Value) Value {
	return v1
}
