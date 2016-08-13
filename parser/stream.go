package parser

type Stream interface {
	Token() Token
	Next() Stream
	Err() error
	Good() bool
}

type Token interface {
	// Origin() interface{}
	Value() interface{}
}

type Context interface {
	Stream() Stream
	Err() error
	Parse(Parser) Context
	Good() bool
	Value() Value
	WithValue(Value) Context
	NextToken() Context
	WithError(error) Context
}

type context struct {
	s   Stream
	err error
	v   Value
}

func (c context) NextToken() Context {
	c.s = c.s.Next()
	return c
}

func (c context) WithValue(v Value) Context {
	c.v = v
	return c
}

func (c context) Value() Value {
	return c.v
}

func (c context) Good() bool {
	return c.err == nil
}

func (c context) Err() error {
	return c.err
}

func (c context) Stream() Stream {
	return c.s
}

func (c context) Parse(p Parser) Context {
	if c.err != nil {
		return c
	}
	return p.Parse(c)
}

func NewContext(s Stream) Context {
	return &context{
		s: s,
	}
}

type Parser interface {
	Parse(Context) Context
}

type ParseFunc func(Context) Context

func (pf ParseFunc) Parse(c Context) Context {
	return pf(c)
}

func (c context) WithError(err error) Context {
	if c.err == nil {
		c.err = err
	}
	return c
}
