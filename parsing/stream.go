package parsing

import "github.com/joeshaw/gengen/generic"

type Stream interface {
	Token() generic.T
	Next() Stream
	Err() error
	Good() bool
	Position() interface{}
}

type Parser interface {
	Parse(Stream) Stream
}
