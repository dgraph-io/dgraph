package parser

type Stream interface {
	Token() interface{}
	Next() Stream
	Err() error
	Good() bool
	Position() interface{}
}

type Parser interface {
	Parse(Stream) Stream
}
