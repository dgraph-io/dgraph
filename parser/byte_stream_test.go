package parser

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestByteStream(t *testing.T) {
	bs := NewByteStream(bytes.NewBufferString("<a> \n\x00"))
	assert.NoError(t, bs.Err())
	assert.Equal(t, byte('<'), bs.Token().Value())
	bs1 := bs.Next()
	assert.NoError(t, bs1.Err())
	assert.Equal(t, byte('a'), bs1.Token().Value())
	assert.NoError(t, bs.Err())
	assert.Equal(t, byte('<'), bs.Token().Value())
	bsEOF := bs1.Next().Next().Next().Next().Next()
	assert.Equal(t, io.EOF, bsEOF.Err())
	assert.Panics(t, func() { bsEOF.Next() })
	// assert.Panics(t, func() { bsEOF.Token() })
}
