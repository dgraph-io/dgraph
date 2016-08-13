package parser

import "fmt"

type Context struct {
	s   Stream
	err error
	v   Value
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
	return fmt.Errorf("%s: %s", c.Stream().Position(), c.err)
}

func (c Context) Stream() Stream {
	return c.s
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
