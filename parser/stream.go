package parser

type Stream interface {
	Token() Token
	Next() Stream
	Err() error
}

type Token interface {
	// Origin() interface{}
	Value() interface{}
}
