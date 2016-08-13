package parser

import (
	"errors"
	"fmt"
)

type MinTimes struct {
	Count  int
	Parser Parser
}

func (mt MinTimes) Parse(_c Context) (c Context, vs []Value) {
	c = _c
	for i := 0; ; i++ {
		_c = mt.Parser.Parse(c)
		if !_c.Good() {
			if i < mt.Count {
				c = c.WithError(fmt.Errorf("%d repetitions, minimum is %d", i+1, mt.Count))
				return
			}
			break
		}
		c = _c
		vs = append(vs, c.Value())
	}
	c = c.WithValue(vs)
	return
}

type Value interface{}

type OneOf []Parser

func (me OneOf) ParseIndex(c Context) (int, Context) {
	for i, p := range me {
		_c := c.Parse(p)
		if _c.Good() {
			return i, _c
		}
	}
	return -1, c.WithError(errors.New("no match"))
}

func (me OneOf) Parse(c Context) Context {
	_, c = me.ParseIndex(c)
	return c
}
