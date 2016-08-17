package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type someParser struct{}

func (*someParser) Parse(Stream) Stream {
	return nil
}

func TestParserName(t *testing.T) {
	p := ParseFunc(func(Stream) Stream { return nil })
	assert.Equal(t, "ParseFunc", ParserName(p))
	assert.Equal(t, "someParser", ParserName(&someParser{}))
	// var i Parser
	// assert.Equal(t, "", ParserName(i))
}
