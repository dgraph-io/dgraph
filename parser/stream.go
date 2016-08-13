package parser

type Stream interface {
	Token() interface{}
	Next() Stream
	Err() error
	Good() bool
	Position() interface{}
}

type Parser interface {
	Parse(Context) Context
}

type ParseFunc func(Context) Context

func (pf ParseFunc) Parse(c Context) Context {
	return pf(c)
}
