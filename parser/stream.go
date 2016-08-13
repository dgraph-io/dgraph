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

type Parser interface {
	Parse(Context) Context
}

type ParseFunc func(Context) Context

func (pf ParseFunc) Parse(c Context) Context {
	return pf(c)
}
