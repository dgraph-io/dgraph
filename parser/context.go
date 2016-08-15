package parser

import (
	"errors"
	"fmt"
)

type Context struct {
	s    Stream
	err  error
	v    Value
	name string
}

func (c Context) NextToken() Context {
	c.s = c.s.Next()
	return c
}

func (c Context) WithValue(v Value) Context {
	c.v = v
	return c
}

func (c Context) Value() Value {
	return c.v
}

func (c Context) Good() bool {
	return c.err == nil
}

func (c Context) Err() error {
	if c.err == nil {
		return nil
	}
	s := ""
	if c.Stream().Err() == nil {
		s = fmt.Sprintf("%q at ", c.Stream().Token())
	}
	s += fmt.Sprintf("%s: ", c.Stream().Position())
	if c.name != "" {
		s += fmt.Sprintf("error parsing production %q: ", c.name)
	}
	s += fmt.Sprintf("%s", c.err)
	return errors.New(s)
}

func (c Context) Stream() Stream {
	return c.s
}

func (c Context) ParseName(name string, p Parser) Context {
	hadErr := c.err != nil
	c = p.Parse(c)
	if hadErr {
		return c
	}
	if c.err != nil {
		c.name = name
	}
	return c
}

func (c Context) Parse(p Parser) Context {
	// if c.err != nil {
	// 	return c
	// }
	return p.Parse(c)
}

func NewContext(s Stream) Context {
	return Context{
		s: s,
	}
}

func (c Context) WithError(err error) Context {
	if c.err == nil {
		c.err = err
	}
	return c
}
